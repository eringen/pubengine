package pubengine

import (
	"context"
	"database/sql"
	"slices"
	"sort"
	"strings"
	"sync"
	"time"
)

var ErrNotFound = sql.ErrNoRows

type postSnapshot struct {
	posts   []BlogPost
	tags    []string
	slugs   map[string]int
	byTag   map[string][]int
	fetched time.Time
}

// PostCache stores summaries and a bounded set of article bodies.
type PostCache struct {
	mu         sync.RWMutex
	snapshot   *postSnapshot
	bodies     map[string]BlogPost
	generation uint64
	loading    chan struct{}
	lastErr    error
	retryAt    time.Time
	ttl        time.Duration
	store      *Store
}

func NewPostCache(s *Store, ttl time.Duration) *PostCache { return &PostCache{store: s, ttl: ttl} }
func (c *PostCache) valid() bool                          { return c.snapshot != nil && time.Since(c.snapshot.fetched) < c.ttl }
func (c *PostCache) Invalidate() {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.generation++
	c.snapshot = nil
	c.bodies = nil
	c.lastErr = nil
	c.retryAt = time.Time{}
}

func (c *PostCache) snapshotContext(ctx context.Context) (*postSnapshot, error) {
	for {
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		c.mu.Lock()
		if c.valid() {
			snapshot := c.snapshot
			c.mu.Unlock()
			return snapshot, nil
		}
		if c.loading != nil {
			done := c.loading
			c.mu.Unlock()
			select {
			case <-done:
				continue
			case <-ctx.Done():
				return nil, ctx.Err()
			}
		}
		if c.lastErr != nil && time.Now().Before(c.retryAt) {
			err := c.lastErr
			c.mu.Unlock()
			return nil, err
		}
		generation := c.generation
		done := make(chan struct{})
		c.loading = done
		c.mu.Unlock()
		posts, err := c.store.ListSummariesContext(ctx)
		snapshot := &postSnapshot{posts: posts, slugs: map[string]int{}, byTag: map[string][]int{}, fetched: time.Now()}
		if err == nil {
			for i, p := range posts {
				snapshot.slugs[p.Slug] = i
				for _, tag := range p.Tags {
					snapshot.byTag[tag] = append(snapshot.byTag[tag], i)
				}
			}
			for tag := range snapshot.byTag {
				snapshot.tags = append(snapshot.tags, tag)
			}
			sort.Strings(snapshot.tags)
		}
		c.mu.Lock()
		c.loading = nil
		current := generation == c.generation
		if current {
			if err == nil {
				c.snapshot = snapshot
				c.bodies = map[string]BlogPost{}
				c.lastErr = nil
			} else if ctx.Err() == nil {
				c.lastErr = err
				c.retryAt = time.Now().Add(100 * time.Millisecond)
			}
		}
		close(done)
		c.mu.Unlock()
		if !current {
			continue
		}
		return snapshot, err
	}
}

// ListPosts preserves the full-content API. Prefer ListPageContext for lists.
func (c *PostCache) ListPosts(tag string) ([]BlogPost, error) {
	return c.ListPostsContext(context.Background(), tag)
}
func (c *PostCache) ListPostsContext(ctx context.Context, tag string) ([]BlogPost, error) {
	posts, _, err := c.ListPageContext(ctx, tag, 0, 0)
	if err != nil {
		return nil, err
	}
	for i, p := range posts {
		posts[i], err = c.GetPostContext(ctx, p.Slug)
		if err != nil {
			return nil, err
		}
	}
	return posts, nil
}

// ListPageContext returns summaries and whether another page exists; zero limit means all.
func (c *PostCache) ListPageContext(ctx context.Context, tag string, offset, limit int) ([]BlogPost, bool, error) {
	snapshot, err := c.snapshotContext(ctx)
	if err != nil {
		return nil, false, err
	}
	if offset < 0 {
		offset = 0
	}
	var indices []int
	if tag != "" {
		indices = snapshot.byTag[normalizeTag(tag)]
	}
	count := len(snapshot.posts)
	if tag != "" {
		count = len(indices)
	}
	if offset > count {
		offset = count
	}
	end := count
	if limit > 0 && limit < count-offset {
		end = offset + limit
	}
	posts := make([]BlogPost, 0, end-offset)
	for i := offset; i < end; i++ {
		index := i
		if tag != "" {
			index = indices[i]
		}
		posts = append(posts, clonePost(snapshot.posts[index]))
	}
	return posts, end < count, nil
}

func (c *PostCache) ListTags() ([]string, error) { return c.ListTagsContext(context.Background()) }
func (c *PostCache) ListTagsContext(ctx context.Context) ([]string, error) {
	snapshot, err := c.snapshotContext(ctx)
	if err != nil {
		return nil, err
	}
	return slices.Clone(snapshot.tags), nil
}
func (c *PostCache) GetPost(slug string) (BlogPost, error) {
	return c.GetPostContext(context.Background(), slug)
}
func (c *PostCache) GetPostContext(ctx context.Context, slug string) (BlogPost, error) {
	for {
		snapshot, err := c.snapshotContext(ctx)
		if err != nil {
			return BlogPost{}, err
		}
		if _, ok := snapshot.slugs[slug]; !ok {
			return BlogPost{}, ErrNotFound
		}
		c.mu.RLock()
		p, ok := c.bodies[slug]
		current := snapshot == c.snapshot
		c.mu.RUnlock()
		if !current {
			continue
		}
		if ok {
			return clonePost(p), nil
		}
		p, err = c.store.GetPostContext(ctx, slug)
		if err != nil {
			return BlogPost{}, err
		}
		c.mu.Lock()
		if snapshot != c.snapshot {
			c.mu.Unlock()
			continue
		}
		if len(c.bodies) >= 128 {
			c.bodies = map[string]BlogPost{}
		}
		c.bodies[slug] = p
		c.mu.Unlock()
		return clonePost(p), nil
	}
}

func (c *PostCache) RelatedContext(ctx context.Context, p BlogPost, limit int) ([]BlogPost, error) {
	snapshot, err := c.snapshotContext(ctx)
	if err != nil {
		return nil, err
	}
	lists := make([][]int, 0, len(p.Tags))
	for _, tag := range p.Tags {
		lists = append(lists, snapshot.byTag[normalizeTag(tag)])
	}
	var result []BlogPost
	for limit <= 0 || len(result) < limit {
		next := len(snapshot.posts)
		for _, list := range lists {
			if len(list) > 0 && list[0] < next {
				next = list[0]
			}
		}
		if next == len(snapshot.posts) {
			break
		}
		for i, list := range lists {
			if len(list) > 0 && list[0] == next {
				lists[i] = list[1:]
			}
		}
		if snapshot.posts[next].Slug != p.Slug {
			result = append(result, clonePost(snapshot.posts[next]))
		}
	}

	return result, nil
}

func normalizeTag(t string) string  { return strings.ToLower(strings.TrimSpace(t)) }
func clonePost(p BlogPost) BlogPost { p.Tags = slices.Clone(p.Tags); return p }
func clonePosts(posts []BlogPost) []BlogPost {
	result := make([]BlogPost, len(posts))
	for i, p := range posts {
		result[i] = clonePost(p)
	}
	return result
}
