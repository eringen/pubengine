package pubengine

import (
	"path/filepath"
	"sync"
	"testing"
	"time"
)

func TestEmptyCacheAndInvalidation(t *testing.T) {
	s, err := NewStore(filepath.Join(t.TempDir(), "blog.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	c := NewPostCache(s, time.Hour)
	if _, err := c.ListPosts(""); err != nil {
		t.Fatal(err)
	}
	if !c.valid() {
		t.Fatal("empty cache is invalid")
	}
	if err := s.SavePost(BlogPost{Slug: "one", Title: "One", Published: true}); err != nil {
		t.Fatal(err)
	}
	posts, _ := c.ListPosts("")
	if len(posts) != 0 {
		t.Fatal("empty cache was reloaded before invalidation")
	}
	c.Invalidate()
	posts, err = c.ListPosts("")
	if err != nil || len(posts) != 1 {
		t.Fatalf("posts=%v error=%v", posts, err)
	}
}

func TestCacheOwnsAllReturnedSlices(t *testing.T) {
	s, err := NewStore(filepath.Join(t.TempDir(), "blog.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	if err := s.SavePost(BlogPost{Slug: "one", Title: "One", Tags: []string{"go"}, Published: true}); err != nil {
		t.Fatal(err)
	}
	c := NewPostCache(s, time.Hour)
	var wg sync.WaitGroup
	for i := 0; i < 20; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for _, tag := range []string{"", "go"} {
				posts, err := c.ListPosts(tag)
				if err != nil {
					t.Error(err)
					return
				}
				posts[0].Title = "changed"
				posts[0].Tags[0] = "changed"
			}
			p, err := c.GetPost("one")
			if err != nil {
				t.Error(err)
				return
			}
			p.Tags[0] = "changed"
			tags, err := c.ListTags()
			if err != nil {
				t.Error(err)
				return
			}
			tags[0] = "changed"
		}()
	}
	wg.Wait()
	p, _ := c.GetPost("one")
	tags, _ := c.ListTags()
	if p.Title != "One" || p.Tags[0] != "go" || tags[0] != "go" {
		t.Fatalf("cache changed: %+v %v", p, tags)
	}
}
