package main

import (
	"database/sql"
	"os"
	"testing"

	_ "modernc.org/sqlite"

	"pubengine/views"
)

func setupTestStore(t *testing.T) (*store, func()) {
	t.Helper()
	path := "data/test_blog.db"
	os.Remove(path) // clean up any existing test db

	s, err := newStore(path)
	if err != nil {
		t.Fatalf("failed to create store: %v", err)
	}

	cleanup := func() {
		s.Close()
		os.Remove(path)
	}

	return s, cleanup
}

func TestNewStore(t *testing.T) {
	s, cleanup := setupTestStore(t)
	defer cleanup()

	if s == nil {
		t.Fatal("store should not be nil")
	}
	if s.db == nil {
		t.Fatal("db should not be nil")
	}
}

func TestSaveAndGetPost(t *testing.T) {
	s, cleanup := setupTestStore(t)
	defer cleanup()

	post := views.BlogPost{
		Slug:      "test-post",
		Title:     "Test Post",
		Date:      "2024-01-15",
		Tags:      []string{"go", "testing"},
		Summary:   "A test post summary",
		Content:   "# Test Content\n\nThis is test content.",
		Published: true,
	}

	// Save post
	if err := s.SavePost(post); err != nil {
		t.Fatalf("SavePost failed: %v", err)
	}

	// Get post
	got, err := s.GetPost("test-post")
	if err != nil {
		t.Fatalf("GetPost failed: %v", err)
	}

	if got.Slug != post.Slug {
		t.Errorf("Slug = %q, want %q", got.Slug, post.Slug)
	}
	if got.Title != post.Title {
		t.Errorf("Title = %q, want %q", got.Title, post.Title)
	}
	if got.Date != post.Date {
		t.Errorf("Date = %q, want %q", got.Date, post.Date)
	}
	if got.Summary != post.Summary {
		t.Errorf("Summary = %q, want %q", got.Summary, post.Summary)
	}
	if got.Content != post.Content {
		t.Errorf("Content = %q, want %q", got.Content, post.Content)
	}
	if got.Link != "/blog/test-post" {
		t.Errorf("Link = %q, want %q", got.Link, "/blog/test-post")
	}
	if !got.Published {
		t.Error("Published should be true")
	}
	if len(got.Tags) != 2 || got.Tags[0] != "go" || got.Tags[1] != "testing" {
		t.Errorf("Tags = %v, want [go testing]", got.Tags)
	}
}

func TestSavePostUpdate(t *testing.T) {
	s, cleanup := setupTestStore(t)
	defer cleanup()

	post := views.BlogPost{
		Slug:      "update-test",
		Title:     "Original Title",
		Date:      "2024-01-01",
		Tags:      []string{"original"},
		Summary:   "Original summary",
		Content:   "Original content",
		Published: true,
	}

	if err := s.SavePost(post); err != nil {
		t.Fatalf("SavePost failed: %v", err)
	}

	// Update post
	post.Title = "Updated Title"
	post.Tags = []string{"updated", "modified"}
	if err := s.SavePost(post); err != nil {
		t.Fatalf("SavePost update failed: %v", err)
	}

	got, err := s.GetPost("update-test")
	if err != nil {
		t.Fatalf("GetPost failed: %v", err)
	}

	if got.Title != "Updated Title" {
		t.Errorf("Title = %q, want %q", got.Title, "Updated Title")
	}
	if len(got.Tags) != 2 {
		t.Errorf("Tags count = %d, want 2", len(got.Tags))
	}
}

func TestGetPostNotFound(t *testing.T) {
	s, cleanup := setupTestStore(t)
	defer cleanup()

	_, err := s.GetPost("nonexistent")
	if err != sql.ErrNoRows {
		t.Errorf("expected sql.ErrNoRows, got %v", err)
	}
}

func TestGetPostUnpublished(t *testing.T) {
	s, cleanup := setupTestStore(t)
	defer cleanup()

	post := views.BlogPost{
		Slug:      "unpublished-post",
		Title:     "Unpublished Post",
		Date:      "2024-01-01",
		Tags:      []string{"draft"},
		Summary:   "Draft summary",
		Content:   "Draft content",
		Published: false,
	}

	if err := s.SavePost(post); err != nil {
		t.Fatalf("SavePost failed: %v", err)
	}

	// GetPost should not find unpublished posts
	_, err := s.GetPost("unpublished-post")
	if err != sql.ErrNoRows {
		t.Errorf("GetPost should return ErrNoRows for unpublished, got %v", err)
	}

	// GetPostAny should find unpublished posts
	got, err := s.GetPostAny("unpublished-post")
	if err != nil {
		t.Fatalf("GetPostAny failed: %v", err)
	}
	if got.Published {
		t.Error("Published should be false")
	}
}

func TestListPosts(t *testing.T) {
	s, cleanup := setupTestStore(t)
	defer cleanup()

	posts := []views.BlogPost{
		{Slug: "post-1", Title: "Post 1", Date: "2024-01-01", Tags: []string{"go"}, Summary: "s1", Content: "c1", Published: true},
		{Slug: "post-2", Title: "Post 2", Date: "2024-01-02", Tags: []string{"go", "web"}, Summary: "s2", Content: "c2", Published: true},
		{Slug: "post-3", Title: "Post 3", Date: "2024-01-03", Tags: []string{"rust"}, Summary: "s3", Content: "c3", Published: true},
		{Slug: "post-4", Title: "Post 4", Date: "2024-01-04", Tags: []string{"go"}, Summary: "s4", Content: "c4", Published: false}, // unpublished
	}

	for _, p := range posts {
		if err := s.SavePost(p); err != nil {
			t.Fatalf("SavePost failed: %v", err)
		}
	}

	// List all published posts
	got, err := s.ListPosts("")
	if err != nil {
		t.Fatalf("ListPosts failed: %v", err)
	}

	if len(got) != 3 {
		t.Errorf("ListPosts count = %d, want 3 (excluding unpublished)", len(got))
	}

	// Should be ordered by date DESC
	if got[0].Slug != "post-3" {
		t.Errorf("First post should be post-3 (latest), got %s", got[0].Slug)
	}
}

func TestListPostsByTag(t *testing.T) {
	s, cleanup := setupTestStore(t)
	defer cleanup()

	posts := []views.BlogPost{
		{Slug: "go-post-1", Title: "Go Post 1", Date: "2024-01-01", Tags: []string{"go", "tutorial"}, Summary: "s1", Content: "c1", Published: true},
		{Slug: "go-post-2", Title: "Go Post 2", Date: "2024-01-02", Tags: []string{"go", "web"}, Summary: "s2", Content: "c2", Published: true},
		{Slug: "rust-post", Title: "Rust Post", Date: "2024-01-03", Tags: []string{"rust"}, Summary: "s3", Content: "c3", Published: true},
	}

	for _, p := range posts {
		if err := s.SavePost(p); err != nil {
			t.Fatalf("SavePost failed: %v", err)
		}
	}

	// Filter by "go" tag
	got, err := s.ListPosts("go")
	if err != nil {
		t.Fatalf("ListPosts with tag failed: %v", err)
	}

	if len(got) != 2 {
		t.Errorf("ListPosts(go) count = %d, want 2", len(got))
	}

	// Filter by "rust" tag
	got, err = s.ListPosts("rust")
	if err != nil {
		t.Fatalf("ListPosts with tag failed: %v", err)
	}

	if len(got) != 1 {
		t.Errorf("ListPosts(rust) count = %d, want 1", len(got))
	}

	// Filter by nonexistent tag
	got, err = s.ListPosts("nonexistent")
	if err != nil {
		t.Fatalf("ListPosts with tag failed: %v", err)
	}

	if len(got) != 0 {
		t.Errorf("ListPosts(nonexistent) count = %d, want 0", len(got))
	}
}

func TestListPostsTagCaseInsensitive(t *testing.T) {
	s, cleanup := setupTestStore(t)
	defer cleanup()

	post := views.BlogPost{
		Slug:      "case-test",
		Title:     "Case Test",
		Date:      "2024-01-01",
		Tags:      []string{"GoLang", "WEB"},
		Summary:   "s",
		Content:   "c",
		Published: true,
	}

	if err := s.SavePost(post); err != nil {
		t.Fatalf("SavePost failed: %v", err)
	}

	// Should find with lowercase
	got, err := s.ListPosts("golang")
	if err != nil {
		t.Fatalf("ListPosts failed: %v", err)
	}

	if len(got) != 1 {
		t.Errorf("ListPosts(golang) should find post with GoLang tag, got %d", len(got))
	}

	// Should find with uppercase
	got, err = s.ListPosts("WEB")
	if err != nil {
		t.Fatalf("ListPosts failed: %v", err)
	}

	if len(got) != 1 {
		t.Errorf("ListPosts(WEB) should find post with web tag, got %d", len(got))
	}
}

func TestListAllPosts(t *testing.T) {
	s, cleanup := setupTestStore(t)
	defer cleanup()

	posts := []views.BlogPost{
		{Slug: "published", Title: "Published", Date: "2024-01-01", Tags: []string{"a"}, Summary: "s1", Content: "c1", Published: true},
		{Slug: "unpublished", Title: "Unpublished", Date: "2024-01-02", Tags: []string{"b"}, Summary: "s2", Content: "c2", Published: false},
	}

	for _, p := range posts {
		if err := s.SavePost(p); err != nil {
			t.Fatalf("SavePost failed: %v", err)
		}
	}

	got, err := s.ListAllPosts()
	if err != nil {
		t.Fatalf("ListAllPosts failed: %v", err)
	}

	if len(got) != 2 {
		t.Errorf("ListAllPosts count = %d, want 2 (including unpublished)", len(got))
	}
}

func TestListTags(t *testing.T) {
	s, cleanup := setupTestStore(t)
	defer cleanup()

	posts := []views.BlogPost{
		{Slug: "p1", Title: "P1", Date: "2024-01-01", Tags: []string{"Go", "Web"}, Summary: "s1", Content: "c1", Published: true},
		{Slug: "p2", Title: "P2", Date: "2024-01-02", Tags: []string{"go", "api"}, Summary: "s2", Content: "c2", Published: true},
		{Slug: "p3", Title: "P3", Date: "2024-01-03", Tags: []string{"rust"}, Summary: "s3", Content: "c3", Published: false}, // unpublished
	}

	for _, p := range posts {
		if err := s.SavePost(p); err != nil {
			t.Fatalf("SavePost failed: %v", err)
		}
	}

	got, err := s.ListTags()
	if err != nil {
		t.Fatalf("ListTags failed: %v", err)
	}

	// Should only include tags from published posts, deduplicated and lowercase
	// Expected: [api, go, web] (sorted)
	if len(got) != 3 {
		t.Errorf("ListTags count = %d, want 3, got %v", len(got), got)
	}

	expected := []string{"api", "go", "web"}
	for i, tag := range expected {
		if i >= len(got) || got[i] != tag {
			t.Errorf("ListTags[%d] = %q, want %q", i, got[i], tag)
		}
	}
}

func TestDeletePost(t *testing.T) {
	s, cleanup := setupTestStore(t)
	defer cleanup()

	post := views.BlogPost{
		Slug:      "to-delete",
		Title:     "To Delete",
		Date:      "2024-01-01",
		Tags:      []string{"delete"},
		Summary:   "s",
		Content:   "c",
		Published: true,
	}

	if err := s.SavePost(post); err != nil {
		t.Fatalf("SavePost failed: %v", err)
	}

	// Verify post exists
	_, err := s.GetPost("to-delete")
	if err != nil {
		t.Fatalf("Post should exist before delete: %v", err)
	}

	// Delete post
	if err := s.DeletePost("to-delete"); err != nil {
		t.Fatalf("DeletePost failed: %v", err)
	}

	// Verify post is gone
	_, err = s.GetPost("to-delete")
	if err != sql.ErrNoRows {
		t.Errorf("Post should not exist after delete, got err: %v", err)
	}
}

func TestDeleteNonexistentPost(t *testing.T) {
	s, cleanup := setupTestStore(t)
	defer cleanup()

	// Should not error when deleting nonexistent post
	err := s.DeletePost("nonexistent")
	if err != nil {
		t.Errorf("DeletePost on nonexistent should not error, got: %v", err)
	}
}

func TestParseTags(t *testing.T) {
	tests := []struct {
		input string
		want  []string
	}{
		{"", nil},
		{",", nil},
		{",go,", []string{"go"}},
		{",go,web,", []string{"go", "web"}},
		{",go, web ,rust,", []string{"go", "web", "rust"}},
	}

	for _, tt := range tests {
		got := parseTags(tt.input)
		if len(got) != len(tt.want) {
			t.Errorf("parseTags(%q) = %v, want %v", tt.input, got, tt.want)
			continue
		}
		for i := range got {
			if got[i] != tt.want[i] {
				t.Errorf("parseTags(%q)[%d] = %q, want %q", tt.input, i, got[i], tt.want[i])
			}
		}
	}
}

func TestEmptyTags(t *testing.T) {
	s, cleanup := setupTestStore(t)
	defer cleanup()

	post := views.BlogPost{
		Slug:      "no-tags",
		Title:     "No Tags",
		Date:      "2024-01-01",
		Tags:      []string{},
		Summary:   "s",
		Content:   "c",
		Published: true,
	}

	if err := s.SavePost(post); err != nil {
		t.Fatalf("SavePost failed: %v", err)
	}

	got, err := s.GetPost("no-tags")
	if err != nil {
		t.Fatalf("GetPost failed: %v", err)
	}

	if len(got.Tags) != 0 {
		t.Errorf("Tags should be empty, got %v", got.Tags)
	}
}
