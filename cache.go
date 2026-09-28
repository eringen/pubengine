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

const maxCachedBodies = 128
const maxBodyBytes = 16 << 20

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
	bodyOrder  []string
	bodyBytes  int
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
	c.bodyOrder = nil
	c.bodyBytes = 0
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
				c.bodyOrder = nil
				c.bodyBytes = 0
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
	for {
		snapshot, err := c.snapshotContext(ctx)
		if err != nil {
			return nil, err
		}
		indices := snapshot.byTag[normalizeTag(tag)]
		count := len(indices)
		if tag == "" {
			count = len(snapshot.posts)
		}
		posts := make([]BlogPost, count)
		complete := true
		c.mu.RLock()
		for i := range posts {
			index := i
			if tag != "" {
				index = indices[i]
			}
			summary := snapshot.posts[index]
			p, ok := c.bodies[summary.Slug]
			if ok {
				posts[i] = clonePost(p)
			} else {
				posts[i] = summary
				complete = false
			}
		}
		current := snapshot == c.snapshot
		c.mu.RUnlock()
		if !current {
			continue
		}
		if complete {
			return posts, nil
		}
		loaded, err := c.store.ListPostsContext(ctx, tag)
		if err != nil {
			return nil, err
		}
		bySlug := make(map[string]BlogPost, len(loaded))
		for _, p := range loaded {
			bySlug[p.Slug] = p
		}
		c.mu.Lock()
		if snapshot != c.snapshot {
			c.mu.Unlock()
			continue
		}
		for i, p := range posts {
			full, ok := bySlug[p.Slug]
			if !ok {
				c.mu.Unlock()
				return nil, ErrNotFound
			}
			posts[i] = full
			if i < maxCachedBodies {
				c.rememberBody(full)
			}
		}
		c.mu.Unlock()
		return posts, nil
	}
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
		c.rememberBody(p)
		c.mu.Unlock()
		return clonePost(p), nil
	}
}

// rememberBody requires the cache lock.
func (c *PostCache) rememberBody(p BlogPost) {
	if _, ok := c.bodies[p.Slug]; ok || len(p.Content) > maxBodyBytes {
		return
	}
	for len(c.bodies) >= maxCachedBodies || c.bodyBytes+len(p.Content) > maxBodyBytes {
		slug := c.bodyOrder[0]
		c.bodyOrder[0] = ""
		c.bodyOrder = c.bodyOrder[1:]
		c.bodyBytes -= len(c.bodies[slug].Content)
		delete(c.bodies, slug)
	}
	c.bodies[p.Slug] = clonePost(p)
	c.bodyOrder = append(c.bodyOrder, p.Slug)
	c.bodyBytes += len(p.Content)
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
