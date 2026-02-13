package pubengine

import (
	"database/sql"
	"strings"
	"sync"
	"time"
)

// ErrNotFound is returned when a requested post does not exist.
var ErrNotFound = sql.ErrNoRows

// PostCache is an in-memory cache of published blog posts and tags with TTL.
type PostCache struct {
	mu      sync.Mutex
	posts   []BlogPost
	tags    []string
	fetched time.Time
	ttl     time.Duration
	store   *Store
}

// NewPostCache creates a PostCache backed by the given Store.
func NewPostCache(s *Store, ttl time.Duration) *PostCache {
	return &PostCache{store: s, ttl: ttl}
}

func (c *PostCache) valid() bool {
	return c.posts != nil && time.Since(c.fetched) < c.ttl
}

// Invalidate clears the cache so the next read triggers a fresh load.
func (c *PostCache) Invalidate() {
	c.mu.Lock()
	c.posts = nil
	c.tags = nil
	c.mu.Unlock()
}

func (c *PostCache) load() error {
	if c.valid() {
		return nil
	}
	posts, err := c.store.ListPosts("")
	if err != nil {
		return err
	}
	tags, err := c.store.ListTags()
	if err != nil {
		return err
	}
	c.posts = posts
	c.tags = tags
	c.fetched = time.Now()
	return nil
}

// ListPosts returns published posts, optionally filtered by tag.
func (c *PostCache) ListPosts(tag string) ([]BlogPost, error) {
	c.mu.Lock()
	if err := c.load(); err != nil {
		c.mu.Unlock()
		return nil, err
	}
	posts := c.posts
	c.mu.Unlock()

	if tag == "" {
		return posts, nil
	}
	normalized := normalizeTag(tag)
	var filtered []BlogPost
	for _, p := range posts {
		for _, t := range p.Tags {
			if normalizeTag(t) == normalized {
				filtered = append(filtered, p)
				break
			}
		}
	}
	return filtered, nil
}

// ListTags returns all unique tags from published posts.
func (c *PostCache) ListTags() ([]string, error) {
	c.mu.Lock()
	if err := c.load(); err != nil {
		c.mu.Unlock()
		return nil, err
	}
	tags := c.tags
	c.mu.Unlock()
	return tags, nil
}

// GetPost returns a single published post by slug from the cache.
func (c *PostCache) GetPost(slug string) (BlogPost, error) {
	c.mu.Lock()
	if err := c.load(); err != nil {
		c.mu.Unlock()
		return BlogPost{}, err
	}
	posts := c.posts
	c.mu.Unlock()

	for _, p := range posts {
		if p.Slug == slug {
			return p, nil
		}
	}
	return BlogPost{}, ErrNotFound
}

func normalizeTag(t string) string {
	return strings.ToLower(strings.TrimSpace(t))
}
