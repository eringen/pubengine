// Package sqliteutil configures the database connections used by both stores.
package sqliteutil

import (
	"database/sql"
	"net/url"
	"os"
	"path/filepath"
	"strings"

	_ "modernc.org/sqlite"
)

// Open applies connection-local settings to every connection, including replacements.
func Open(path string) (*sql.DB, error) {
	var u *url.URL
	var err error
	if strings.HasPrefix(path, "file:") {
		u, err = url.Parse(path)
	} else if path == ":memory:" {
		u = &url.URL{Scheme: "file", Opaque: ":memory:"}
	} else {
		path, err = filepath.Abs(path)
		u = &url.URL{Scheme: "file", Path: filepath.ToSlash(path)}
	}
	if err != nil {
		return nil, err
	}
	q := u.Query()
	q.Set("_txlock", "immediate")
	memory := u.Opaque == ":memory:" || u.Path == ":memory:" || q.Get("mode") == "memory"
	if !memory && q.Get("mode") != "ro" {
		filename := u.Path
		if filename == "" {
			filename = u.Opaque
		}
		if err := os.MkdirAll(filepath.Dir(filename), 0o755); err != nil {
			return nil, err
		}
	}
	for _, pragma := range []string{"busy_timeout=5000", "synchronous=NORMAL", "cache_size=-8000", "mmap_size=268435456"} {
		q.Add("_pragma", pragma)
	}
	u.RawQuery = q.Encode()
	db, err := sql.Open("sqlite", u.String())
	if err != nil {
		return nil, err
	}
	db.SetMaxOpenConns(4)
	db.SetMaxIdleConns(4)
	// Private in-memory databases must not split into independent pooled databases.
	if memory {
		db.SetMaxOpenConns(1)
		db.SetMaxIdleConns(1)
	}
	if _, err := db.Exec("PRAGMA journal_mode=WAL"); err != nil {
		db.Close()
		return nil, err
	}
	return db, nil
}
