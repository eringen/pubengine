package pubengine

import (
	"database/sql"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

// Store wraps a SQLite database and provides CRUD operations for blog posts.
type Store struct {
	db *sql.DB
}

// NewStore opens (or creates) the SQLite database at path, ensures the data
// directory exists, and runs schema migrations.
func NewStore(path string) (*Store, error) {
	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return nil, err
	}
	db, err := sql.Open("sqlite", path)
	if err != nil {
		return nil, err
	}
	// Enable WAL mode for concurrent read/write access, set a busy timeout
	// so writers wait instead of returning SQLITE_BUSY immediately, and tune
	// performance: synchronous=NORMAL is safe with WAL and avoids an fsync
	// per transaction; larger cache and mmap reduce disk I/O.
	if _, err := db.Exec(`
		PRAGMA journal_mode=WAL;
		PRAGMA busy_timeout=5000;
		PRAGMA synchronous=NORMAL;
		PRAGMA cache_size=-8000;
		PRAGMA mmap_size=268435456;
	`); err != nil {
		return nil, err
	}
	db.SetMaxOpenConns(4)
	db.SetMaxIdleConns(4)
	s := &Store{db: db}
	if err := s.ensureSchema(); err != nil {
		return nil, err
	}
	return s, nil
}

// Close closes the underlying database connection.
func (s *Store) Close() error {
	return s.db.Close()
}

func (s *Store) ensureSchema() error {
	_, err := s.db.Exec(`
CREATE TABLE IF NOT EXISTS posts (
    slug TEXT PRIMARY KEY,
    title TEXT NOT NULL,
    date TEXT NOT NULL,
    tags TEXT NOT NULL,
    summary TEXT NOT NULL,
    content TEXT NOT NULL,
    published INTEGER NOT NULL DEFAULT 1
);
`)
	if err != nil {
		return err
	}
	if _, err := s.db.Exec(`ALTER TABLE posts ADD COLUMN published INTEGER NOT NULL DEFAULT 1;`); err != nil {
		if strings.Contains(strings.ToLower(err.Error()), "duplicate column") {
			return nil
		}
		return err
	}
	return nil
}

// ListPosts returns all published posts ordered by date descending.
// If tag is non-empty, results are filtered to posts containing that tag.
func (s *Store) ListPosts(tag string) ([]BlogPost, error) {
	var rows *sql.Rows
	var err error
	if tag == "" {
		rows, err = s.db.Query(`SELECT slug, title, date, tags, summary, content, published FROM posts WHERE published = 1 ORDER BY date DESC`)
	} else {
		normalizedTag := strings.ToLower(strings.TrimSpace(tag))
		rows, err = s.db.Query(`SELECT slug, title, date, tags, summary, content, published FROM posts WHERE published = 1 AND instr(lower(tags), ',' || ? || ',') > 0 ORDER BY date DESC`, normalizedTag)
	}
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var posts []BlogPost
	for rows.Next() {
		var slug, title, date, tags, summary, content string
		var published int
		if err := rows.Scan(&slug, &title, &date, &tags, &summary, &content, &published); err != nil {
			return nil, err
		}
		post := BlogPost{
			Slug:      slug,
			Title:     title,
			Date:      date,
			Tags:      ParseTags(tags),
			Summary:   summary,
			Content:   content,
			Link:      "/blog/" + slug,
			Published: published == 1,
		}
		posts = append(posts, post)
	}
	return posts, nil
}

// ListTags returns a sorted, deduplicated slice of all tags from published posts.
func (s *Store) ListTags() ([]string, error) {
	rows, err := s.db.Query(`SELECT tags FROM posts WHERE published = 1`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	set := make(map[string]struct{})
	for rows.Next() {
		var tags string
		if err := rows.Scan(&tags); err != nil {
			return nil, err
		}
		for _, t := range ParseTags(tags) {
			set[strings.ToLower(t)] = struct{}{}
		}
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	var result []string
	for t := range set {
		result = append(result, t)
	}
	sort.Strings(result)
	return result, nil
}

// GetPost returns a single published post by slug.
func (s *Store) GetPost(slug string) (BlogPost, error) {
	var title, date, tags, summary, content string
	var published int
	err := s.db.QueryRow(`SELECT title, date, tags, summary, content, published FROM posts WHERE slug = ? AND published = 1`, slug).
		Scan(&title, &date, &tags, &summary, &content, &published)
	if err != nil {
		return BlogPost{}, err
	}
	return BlogPost{
		Slug:      slug,
		Title:     title,
		Date:      date,
		Tags:      ParseTags(tags),
		Summary:   summary,
		Content:   content,
		Link:      "/blog/" + slug,
		Published: published == 1,
	}, nil
}

// GetPostAny returns a post by slug regardless of published status (for admin).
func (s *Store) GetPostAny(slug string) (BlogPost, error) {
	var title, date, tags, summary, content string
	var published int
	err := s.db.QueryRow(`SELECT title, date, tags, summary, content, published FROM posts WHERE slug = ?`, slug).
		Scan(&title, &date, &tags, &summary, &content, &published)
	if err != nil {
		return BlogPost{}, err
	}
	return BlogPost{
		Slug:      slug,
		Title:     title,
		Date:      date,
		Tags:      ParseTags(tags),
		Summary:   summary,
		Content:   content,
		Link:      "/blog/" + slug,
		Published: published == 1,
	}, nil
}

// ListAllPosts returns every post (published and drafts) ordered by date descending.
func (s *Store) ListAllPosts() ([]BlogPost, error) {
	rows, err := s.db.Query(`SELECT slug, title, date, tags, summary, content, published FROM posts ORDER BY date DESC`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var posts []BlogPost
	for rows.Next() {
		var slug, title, date, tags, summary, content string
		var published int
		if err := rows.Scan(&slug, &title, &date, &tags, &summary, &content, &published); err != nil {
			return nil, err
		}
		posts = append(posts, BlogPost{
			Slug:      slug,
			Title:     title,
			Date:      date,
			Tags:      ParseTags(tags),
			Summary:   summary,
			Content:   content,
			Link:      "/blog/" + slug,
			Published: published == 1,
		})
	}
	return posts, nil
}

// SavePost upserts a blog post. Tags are normalized to lowercase.
func (s *Store) SavePost(p BlogPost) error {
	normalizedTags := make([]string, len(p.Tags))
	for i, t := range p.Tags {
		normalizedTags[i] = strings.ToLower(strings.TrimSpace(t))
	}
	tagString := "," + strings.Join(normalizedTags, ",") + ","
	published := 0
	if p.Published {
		published = 1
	}
	_, err := s.db.Exec(`INSERT OR REPLACE INTO posts (slug, title, date, tags, summary, content, published) VALUES (?, ?, ?, ?, ?, ?, ?)`,
		p.Slug, p.Title, p.Date, tagString, p.Summary, p.Content, published)
	return err
}

// DeletePost removes a post by slug.
func (s *Store) DeletePost(slug string) error {
	_, err := s.db.Exec(`DELETE FROM posts WHERE slug = ?`, slug)
	return err
}

// ParseTags splits a comma-delimited tag string (e.g. ",go,web,") into a slice.
func ParseTags(tagString string) []string {
	tagString = strings.Trim(tagString, ",")
	if tagString == "" {
		return nil
	}
	parts := strings.Split(tagString, ",")
	for i := range parts {
		parts[i] = strings.TrimSpace(parts[i])
	}
	return parts
}
