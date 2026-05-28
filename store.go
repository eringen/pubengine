package pubengine

import (
	"database/sql"
	"errors"
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/eringen/pubengine/internal/sqliteutil"
)

// Store wraps a SQLite database and provides CRUD operations for blog posts.
type Store struct {
	db *sql.DB
}

// NewStore opens (or creates) the SQLite database at path, ensures the data
// directory exists, and runs schema migrations.
func NewStore(path string) (*Store, error) {
	db, err := sqliteutil.Open(path)
	if err != nil {
		return nil, err
	}
	s := &Store{db: db}
	if err := s.ensureSchema(); err != nil {
		db.Close()
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
    published INTEGER NOT NULL DEFAULT 1,
    revision INTEGER NOT NULL DEFAULT 1
);
`)
	if err != nil {
		return err
	}
	rows, err := s.db.Query("PRAGMA table_info(posts)")
	if err != nil {
		return err
	}
	columns := map[string]bool{}
	for rows.Next() {
		var cid, notnull, pk int
		var name, kind string
		var def any
		if err := rows.Scan(&cid, &name, &kind, &notnull, &def, &pk); err != nil {
			rows.Close()
			return err
		}
		columns[name] = true
	}
	err = rows.Err()
	rows.Close()
	if err != nil {
		return err
	}
	for _, column := range []string{"published", "revision"} {
		if !columns[column] {
			if _, err := s.db.Exec("ALTER TABLE posts ADD COLUMN " + column + " INTEGER NOT NULL DEFAULT 1"); err != nil {
				return err
			}
		}
	}
	_, err = s.db.Exec(`
CREATE TABLE IF NOT EXISTS post_redirects (slug TEXT PRIMARY KEY, target TEXT NOT NULL);
CREATE TABLE IF NOT EXISTS images (
    filename TEXT PRIMARY KEY,
    original_name TEXT NOT NULL,
    width INTEGER NOT NULL,
    height INTEGER NOT NULL,
    size INTEGER NOT NULL,
    uploaded_at TEXT NOT NULL
);
`)
	return err
}

// ListPosts returns all published posts ordered by date descending.
// If tag is non-empty, results are filtered to posts containing that tag.
func (s *Store) ListPosts(tag string) ([]BlogPost, error) {
	var rows *sql.Rows
	var err error
	if tag == "" {
		rows, err = s.db.Query(`SELECT slug, title, date, tags, summary, content, published, revision FROM posts WHERE published = 1 ORDER BY date DESC`)
	} else {
		normalizedTag := strings.ToLower(strings.TrimSpace(tag))
		rows, err = s.db.Query(`SELECT slug, title, date, tags, summary, content, published, revision FROM posts WHERE published = 1 AND instr(lower(tags), ',' || ? || ',') > 0 ORDER BY date DESC`, normalizedTag)
	}
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var posts []BlogPost
	for rows.Next() {
		var slug, title, date, tags, summary, content string
		var published int
		var revision int64
		if err := rows.Scan(&slug, &title, &date, &tags, &summary, &content, &published, &revision); err != nil {
			return nil, err
		}
		post := BlogPost{
			Slug:         slug,
			Title:        title,
			Date:         date,
			Tags:         ParseTags(tags),
			Summary:      summary,
			Content:      content,
			Link:         "/blog/" + slug,
			Published:    published == 1,
			Revision:     revision,
			OriginalSlug: slug,
		}
		posts = append(posts, post)
	}
	return posts, rows.Err()
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
	var revision int64
	err := s.db.QueryRow(`SELECT title, date, tags, summary, content, published, revision FROM posts WHERE slug = ? AND published = 1`, slug).
		Scan(&title, &date, &tags, &summary, &content, &published, &revision)
	if err != nil {
		return BlogPost{}, err
	}
	return BlogPost{
		Slug:         slug,
		Title:        title,
		Date:         date,
		Tags:         ParseTags(tags),
		Summary:      summary,
		Content:      content,
		Link:         "/blog/" + slug,
		Published:    published == 1,
		Revision:     revision,
		OriginalSlug: slug,
	}, nil
}

// GetPostAny returns a post by slug regardless of published status (for admin).
func (s *Store) GetPostAny(slug string) (BlogPost, error) {
	var title, date, tags, summary, content string
	var published int
	var revision int64
	err := s.db.QueryRow(`SELECT title, date, tags, summary, content, published, revision FROM posts WHERE slug = ?`, slug).
		Scan(&title, &date, &tags, &summary, &content, &published, &revision)
	if err != nil {
		return BlogPost{}, err
	}
	return BlogPost{
		Slug:         slug,
		Title:        title,
		Date:         date,
		Tags:         ParseTags(tags),
		Summary:      summary,
		Content:      content,
		Link:         "/blog/" + slug,
		Published:    published == 1,
		Revision:     revision,
		OriginalSlug: slug,
	}, nil
}

// ListAllPosts returns every post (published and drafts) ordered by date descending.
func (s *Store) ListAllPosts() ([]BlogPost, error) {
	rows, err := s.db.Query(`SELECT slug, title, date, tags, summary, content, published, revision FROM posts ORDER BY date DESC`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var posts []BlogPost
	for rows.Next() {
		var slug, title, date, tags, summary, content string
		var published int
		var revision int64
		if err := rows.Scan(&slug, &title, &date, &tags, &summary, &content, &published, &revision); err != nil {
			return nil, err
		}
		posts = append(posts, BlogPost{
			Slug:         slug,
			Title:        title,
			Date:         date,
			Tags:         ParseTags(tags),
			Summary:      summary,
			Content:      content,
			Link:         "/blog/" + slug,
			Published:    published == 1,
			Revision:     revision,
			OriginalSlug: slug,
		})
	}
	return posts, rows.Err()
}

// ErrPostConflict means a slug is taken or the editor's revision is stale.
var ErrPostConflict = errors.New("post changed or slug is already in use; reload before saving")

// SavePost creates a new post when Revision is zero, otherwise updates the loaded
// OriginalSlug only if its revision still matches. Fetch the post again after saving.
func (s *Store) SavePost(p BlogPost) error {
	if msg := ValidateSlug(p.Slug); msg != "" {
		return fmt.Errorf("%s", msg)
	}
	if strings.TrimSpace(p.Title) == "" {
		return fmt.Errorf("title is required")
	}
	if p.Date != "" {
		if _, err := time.Parse("2006-01-02", p.Date); err != nil {
			return fmt.Errorf("use YYYY-MM-DD for the date")
		}
	}
	tags := FilterEmpty(p.Tags)
	for i, t := range tags {
		tags[i] = strings.ToLower(t)
	}
	tagString := "," + strings.Join(tags, ",") + ","
	tx, err := s.db.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()
	var occupied int
	if err := tx.QueryRow("SELECT count(*) FROM post_redirects WHERE slug = ?", p.Slug).Scan(&occupied); err != nil {
		return err
	}
	if occupied > 0 {
		return ErrPostConflict
	}
	if p.Revision == 0 {
		if p.OriginalSlug != "" {
			return ErrPostConflict
		}
		result, err := tx.Exec(`INSERT INTO posts (slug,title,date,tags,summary,content,published) VALUES (?,?,?,?,?,?,?) ON CONFLICT(slug) DO NOTHING`, p.Slug, p.Title, p.Date, tagString, p.Summary, p.Content, p.Published)
		if err != nil {
			return err
		}
		n, err := result.RowsAffected()
		if err != nil {
			return err
		}
		if n != 1 {
			return ErrPostConflict
		}
	} else {
		if p.OriginalSlug == "" {
			return ErrPostConflict
		}
		if p.Slug != p.OriginalSlug {
			if err := tx.QueryRow("SELECT count(*) FROM posts WHERE slug = ?", p.Slug).Scan(&occupied); err != nil {
				return err
			}
			if occupied > 0 {
				return ErrPostConflict
			}
		}
		result, err := tx.Exec(`UPDATE posts SET slug=?,title=?,date=?,tags=?,summary=?,content=?,published=?,revision=revision+1 WHERE slug=? AND revision=?`, p.Slug, p.Title, p.Date, tagString, p.Summary, p.Content, p.Published, p.OriginalSlug, p.Revision)
		if err != nil {
			return err
		}
		n, err := result.RowsAffected()
		if err != nil {
			return err
		}
		if n != 1 {
			return ErrPostConflict
		}
		if p.Slug != p.OriginalSlug {
			if _, err := tx.Exec("UPDATE post_redirects SET target=? WHERE target=?", p.Slug, p.OriginalSlug); err != nil {
				return err
			}
			if _, err := tx.Exec("INSERT INTO post_redirects (slug,target) VALUES (?,?)", p.OriginalSlug, p.Slug); err != nil {
				return err
			}
		}
	}
	return tx.Commit()
}

// ResolvePostRedirect returns the current slug only when the destination is published.
func (s *Store) ResolvePostRedirect(slug string) (string, error) {
	var target string
	err := s.db.QueryRow(`SELECT r.target FROM post_redirects r JOIN posts p ON p.slug=r.target WHERE r.slug=? AND p.published=1`, slug).Scan(&target)
	return target, err
}

// DeletePost removes a post and its previous URLs atomically.
func (s *Store) DeletePost(slug string) error {
	tx, err := s.db.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()
	if _, err := tx.Exec("DELETE FROM post_redirects WHERE target=?", slug); err != nil {
		return err
	}
	if _, err := tx.Exec("DELETE FROM posts WHERE slug=?", slug); err != nil {
		return err
	}
	return tx.Commit()
}

// SaveImage inserts image metadata into the database.
func (s *Store) SaveImage(img Image) error {
	_, err := s.db.Exec(`INSERT INTO images (filename, original_name, width, height, size, uploaded_at) VALUES (?, ?, ?, ?, ?, ?)`,
		img.Filename, img.OriginalName, img.Width, img.Height, img.Size, img.UploadedAt)
	return err
}

// ListImages returns all images ordered by upload time descending.
func (s *Store) ListImages() ([]Image, error) {
	rows, err := s.db.Query(`SELECT filename, original_name, width, height, size, uploaded_at FROM images ORDER BY uploaded_at DESC`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var images []Image
	for rows.Next() {
		var img Image
		if err := rows.Scan(&img.Filename, &img.OriginalName, &img.Width, &img.Height, &img.Size, &img.UploadedAt); err != nil {
			return nil, err
		}
		images = append(images, img)
	}
	return images, rows.Err()
}

// DeleteImage removes image metadata from the database.
func (s *Store) DeleteImage(filename string) error {
	_, err := s.db.Exec(`DELETE FROM images WHERE filename = ?`, filename)
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
