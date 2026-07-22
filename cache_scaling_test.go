package pubengine

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"testing"
	"time"
)

func TestSummaryPagesAndUnicodeTags(t *testing.T) {
	s, err := NewStore(":memory:")
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	for _, slug := range []string{"third", "first", "second"} {
		if err := s.SavePost(BlogPost{Slug: slug, Title: slug, Date: "2026-01-01", Tags: []string{"ÉTÉ", "go"}, Content: "body", Published: true}); err != nil {
			t.Fatal(err)
		}
	}
	c := NewPostCache(s, time.Hour)
	posts, more, err := c.ListPageContext(context.Background(), "ÉTÉ", 0, 2)
	if err != nil || !more || len(posts) != 2 || posts[0].Slug != "first" || posts[0].Content != "" {
		t.Fatal(posts, more, err)
	}
	storePosts, err := s.ListPosts("ÉTÉ")
	if err != nil || len(storePosts) != 3 {
		t.Fatal(storePosts, err)
	}
	p, err := c.GetPost("first")
	if err != nil || p.Content != "body" {
		t.Fatal(p, err)
	}
	related, err := c.RelatedContext(context.Background(), p, 1)
	if err != nil || len(related) != 1 || related[0].Slug != "second" {
		t.Fatal(related, err)
	}
	p.Published = false
	if err := s.SavePost(p); err != nil {
		t.Fatal(err)
	}
	c.Invalidate()
	if _, err := c.GetPost("first"); !errors.Is(err, ErrNotFound) {
		t.Fatal(err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := c.GetPostContext(ctx, "second"); !errors.Is(err, context.Canceled) {
		t.Fatal(err)
	}
}

func TestInvalidationDoesNotWaitForDatabase(t *testing.T) {
	s, err := NewStore(":memory:")
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	c := NewPostCache(s, time.Hour)
	conn, err := s.db.Conn(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	defer conn.Close()
	done := make(chan error, 1)
	go func() { _, _, err := c.ListPageContext(context.Background(), "", 0, 10); done <- err }()
	deadline := time.After(time.Second)
	for {
		c.mu.RLock()
		loading := c.loading != nil
		c.mu.RUnlock()
		if loading {
			break
		}
		select {
		case <-deadline:
			t.Fatal("load did not start")
		default:
			time.Sleep(time.Millisecond)
		}
	}
	invalidated := make(chan struct{})
	go func() { c.Invalidate(); close(invalidated) }()
	select {
	case <-invalidated:
	case <-time.After(time.Second):
		t.Fatal("invalidation blocked on I/O")
	}
	conn.Close()
	if err := <-done; err != nil {
		t.Fatal(err)
	}
}

func BenchmarkPostCache(b *testing.B) {
	for _, count := range []int{100, 1000, 10000} {
		b.Run(fmt.Sprint(count), func(b *testing.B) {
			s, err := NewStore(":memory:")
			if err != nil {
				b.Fatal(err)
			}
			defer s.Close()
			tx, err := s.db.Begin()
			if err != nil {
				b.Fatal(err)
			}
			for i := 0; i < count; i++ {
				if _, err := tx.Exec(`INSERT INTO posts(slug,title,date,tags,summary,content,published) VALUES(?,?,'2026-01-01',',go,web,','summary',?,1)`, fmt.Sprintf("post-%05d", i), "Post", strings.Repeat("text ", 800)); err != nil {
					b.Fatal(err)
				}
			}
			if err := tx.Commit(); err != nil {
				b.Fatal(err)
			}
			c := NewPostCache(s, time.Hour)
			slug := fmt.Sprintf("post-%05d", count-1)
			post, err := c.GetPost(slug)
			if err != nil {
				b.Fatal(err)
			}
			b.Run("lookup", func(b *testing.B) {
				b.ReportAllocs()
				for i := 0; i < b.N; i++ {
					if _, err := c.GetPost(slug); err != nil {
						b.Fatal(err)
					}
				}
			})
			b.Run("page", func(b *testing.B) {
				b.ReportAllocs()
				for i := 0; i < b.N; i++ {
					if _, _, err := c.ListPageContext(context.Background(), "", 0, 20); err != nil {
						b.Fatal(err)
					}
				}
			})
			b.Run("related", func(b *testing.B) {
				b.ReportAllocs()
				for i := 0; i < b.N; i++ {
					if _, err := c.RelatedContext(context.Background(), post, 6); err != nil {
						b.Fatal(err)
					}
				}
			})
		})
	}
}
