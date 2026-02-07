package main

import (
	"database/sql"
	"os"
	"sort"
	"strings"

	"github.com/eringen/pubengine/views"
)

type store struct {
	db *sql.DB
}

func newStore(path string) (*store, error) {
	if err := os.MkdirAll("data", 0o755); err != nil {
		return nil, err
	}
	db, err := sql.Open("sqlite", path)
	if err != nil {
		return nil, err
	}
	s := &store{db: db}
	if err := s.ensureSchema(); err != nil {
		return nil, err
	}
	return s, nil
}

func (s *store) Close() error {
	return s.db.Close()
}

func (s *store) ensureSchema() error {
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
		// ignore if column already exists
		if strings.Contains(strings.ToLower(err.Error()), "duplicate column") {
			return nil
		}
		return err
	}
	return nil
}


func (s *store) ListPosts(tag string) ([]views.BlogPost, error) {
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

	var posts []views.BlogPost
	for rows.Next() {
		var slug, title, date, tags, summary, content string
		var published int
		if err := rows.Scan(&slug, &title, &date, &tags, &summary, &content, &published); err != nil {
			return nil, err
		}
		post := views.BlogPost{
			Slug:      slug,
			Title:     title,
			Date:      date,
			Tags:      parseTags(tags),
			Summary:   summary,
			Content:   content,
			Link:      "/blog/" + slug,
			Published: published == 1,
		}
		posts = append(posts, post)
	}
	return posts, nil
}

func (s *store) ListTags() ([]string, error) {
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
		for _, t := range parseTags(tags) {
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

func (s *store) GetPost(slug string) (views.BlogPost, error) {
	var title, date, tags, summary, content string
	var published int
	err := s.db.QueryRow(`SELECT title, date, tags, summary, content, published FROM posts WHERE slug = ? AND published = 1`, slug).
		Scan(&title, &date, &tags, &summary, &content, &published)
	if err != nil {
		return views.BlogPost{}, err
	}
	return views.BlogPost{
		Slug:      slug,
		Title:     title,
		Date:      date,
		Tags:      parseTags(tags),
		Summary:   summary,
		Content:   content,
		Link:      "/blog/" + slug,
		Published: published == 1,
	}, nil
}

func (s *store) GetPostAny(slug string) (views.BlogPost, error) {
	var title, date, tags, summary, content string
	var published int
	err := s.db.QueryRow(`SELECT title, date, tags, summary, content, published FROM posts WHERE slug = ?`, slug).
		Scan(&title, &date, &tags, &summary, &content, &published)
	if err != nil {
		return views.BlogPost{}, err
	}
	return views.BlogPost{
		Slug:      slug,
		Title:     title,
		Date:      date,
		Tags:      parseTags(tags),
		Summary:   summary,
		Content:   content,
		Link:      "/blog/" + slug,
		Published: published == 1,
	}, nil
}

func (s *store) ListAllPosts() ([]views.BlogPost, error) {
	rows, err := s.db.Query(`SELECT slug, title, date, tags, summary, content, published FROM posts ORDER BY date DESC`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var posts []views.BlogPost
	for rows.Next() {
		var slug, title, date, tags, summary, content string
		var published int
		if err := rows.Scan(&slug, &title, &date, &tags, &summary, &content, &published); err != nil {
			return nil, err
		}
		posts = append(posts, views.BlogPost{
			Slug:      slug,
			Title:     title,
			Date:      date,
			Tags:      parseTags(tags),
			Summary:   summary,
			Content:   content,
			Link:      "/blog/" + slug,
			Published: published == 1,
		})
	}
	return posts, nil
}

func (s *store) SavePost(p views.BlogPost) error {
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

func (s *store) DeletePost(slug string) error {
	_, err := s.db.Exec(`DELETE FROM posts WHERE slug = ?`, slug)
	return err
}

func parseTags(tagString string) []string {
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
