package main

import (
	"database/sql"
	"strings"
	"sync"
	"time"

	"github.com/eringen/pubengine/views"
)

var errNotFound = sql.ErrNoRows

type postCache struct {
	mu       sync.RWMutex
	posts    []views.BlogPost // all published posts (no tag filter)
	tags     []string
	fetched  time.Time
	ttl      time.Duration
	store    *store
}

func newPostCache(s *store, ttl time.Duration) *postCache {
	return &postCache{store: s, ttl: ttl}
}

func (c *postCache) valid() bool {
	return c.posts != nil && time.Since(c.fetched) < c.ttl
}

func (c *postCache) Invalidate() {
	c.mu.Lock()
	c.posts = nil
	c.tags = nil
	c.mu.Unlock()
}

// load fetches posts and tags into cache if stale. Must be called under write lock.
func (c *postCache) load() error {
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
func (c *postCache) ListPosts(tag string) ([]views.BlogPost, error) {
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
	var filtered []views.BlogPost
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

// ListTags returns all tags from published posts.
func (c *postCache) ListTags() ([]string, error) {
	c.mu.Lock()
	if err := c.load(); err != nil {
		c.mu.Unlock()
		return nil, err
	}
	tags := c.tags
	c.mu.Unlock()
	return tags, nil
}

// GetPost returns a single published post by slug.
func (c *postCache) GetPost(slug string) (views.BlogPost, error) {
	c.mu.Lock()
	if err := c.load(); err != nil {
		c.mu.Unlock()
		return views.BlogPost{}, err
	}
	posts := c.posts
	c.mu.Unlock()

	for _, p := range posts {
		if p.Slug == slug {
			return p, nil
		}
	}
	return views.BlogPost{}, errNotFound
}

func normalizeTag(t string) string {
	return strings.ToLower(strings.TrimSpace(t))
}
