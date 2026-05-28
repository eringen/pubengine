package pubengine

import (
	"database/sql"
	"errors"
	"path/filepath"
	"sync"
	"testing"
)

func TestPostEditsRejectCollisionsAndStaleRevisions(t *testing.T) {
	s, err := NewStore(filepath.Join(t.TempDir(), "posts.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	for _, slug := range []string{"first", "second"} {
		if err := s.SavePost(BlogPost{Slug: slug, Title: slug, Published: true}); err != nil {
			t.Fatal(err)
		}
	}
	if err := s.SavePost(BlogPost{Slug: "first", Title: "overwrite"}); !errors.Is(err, ErrPostConflict) {
		t.Fatal(err)
	}
	p, err := s.GetPostAny("first")
	if err != nil {
		t.Fatal(err)
	}
	stale := p
	p.Slug = "second"
	if err := s.SavePost(p); !errors.Is(err, ErrPostConflict) {
		t.Fatal(err)
	}
	p.Slug = "renamed"
	if err := s.SavePost(p); err != nil {
		t.Fatal(err)
	}
	if _, err := s.GetPostAny("first"); err != sql.ErrNoRows {
		t.Fatal("old row survives rename", err)
	}
	if target, err := s.ResolvePostRedirect("first"); err != nil || target != "renamed" {
		t.Fatalf("redirect=%q %v", target, err)
	}
	if err := s.SavePost(stale); !errors.Is(err, ErrPostConflict) {
		t.Fatal("stale edit accepted", err)
	}
	p, _ = s.GetPostAny("renamed")
	p.Published = false
	if err := s.SavePost(p); err != nil {
		t.Fatal(err)
	}
	if _, err := s.ResolvePostRedirect("first"); err != sql.ErrNoRows {
		t.Fatal("draft redirect exposed", err)
	}
	posts, _ := s.ListAllPosts()
	if len(posts) != 2 {
		t.Fatal(posts)
	}
}

func TestConcurrentPostEditsHaveOneWinner(t *testing.T) {
	s, err := NewStore(filepath.Join(t.TempDir(), "posts.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	if err := s.SavePost(BlogPost{Slug: "post", Title: "Post"}); err != nil {
		t.Fatal(err)
	}
	p, _ := s.GetPostAny("post")
	results := make(chan error, 2)
	var wg sync.WaitGroup
	for i := 0; i < 2; i++ {
		wg.Add(1)
		go func() { defer wg.Done(); results <- s.SavePost(p) }()
	}
	wg.Wait()
	close(results)
	wins := 0
	for err := range results {
		if err == nil {
			wins++
		} else if !errors.Is(err, ErrPostConflict) {
			t.Fatal(err)
		}
	}
	if wins != 1 {
		t.Fatal(wins)
	}
}

func TestPostWriteValidation(t *testing.T) {
	s, err := NewStore(":memory:")
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	for _, slug := range []string{"a/b", "..", "a?b", "a#b", "UPPER", "a--b", "-a", "a-", "a b", "new"} {
		if err := s.SavePost(BlogPost{Slug: slug, Title: "Title"}); err == nil {
			t.Fatalf("accepted %q", slug)
		}
	}
	if err := s.SavePost(BlogPost{Slug: "valid", Title: "  "}); err == nil {
		t.Fatal("accepted blank title")
	}
}

func TestOldPostSchemaUpgrade(t *testing.T) {
	path := filepath.Join(t.TempDir(), "old.db")
	db, err := sql.Open("sqlite", path)
	if err != nil {
		t.Fatal(err)
	}
	_, err = db.Exec(`CREATE TABLE posts (slug TEXT PRIMARY KEY,title TEXT,date TEXT,tags TEXT,summary TEXT,content TEXT); INSERT INTO posts VALUES ('old','Old','2026-01-01',',go,','','body')`)
	db.Close()
	if err != nil {
		t.Fatal(err)
	}
	s, err := NewStore(path)
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	p, err := s.GetPostAny("old")
	if err != nil || p.Revision != 1 || !p.Published {
		t.Fatalf("%+v %v", p, err)
	}
	if err := s.SavePost(p); err != nil {
		t.Fatal(err)
	}
}
