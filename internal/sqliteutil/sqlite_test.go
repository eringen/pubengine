package sqliteutil

import (
	"context"
	"database/sql"
	"fmt"
	"path/filepath"
	"sync"
	"testing"
)

func TestPoolSettingsAndConcurrentWrites(t *testing.T) {
	db, err := Open(filepath.Join(t.TempDir(), "nested", "test.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	var conns []*sql.Conn
	for i := 0; i < 4; i++ {
		c, err := db.Conn(context.Background())
		if err != nil {
			t.Fatal(err)
		}
		conns = append(conns, c)
		for pragma, want := range map[string]int{"busy_timeout": 5000, "synchronous": 1, "cache_size": -8000} {
			var got int
			if err := c.QueryRowContext(context.Background(), "PRAGMA "+pragma).Scan(&got); err != nil {
				t.Fatal(err)
			}
			if got != want {
				t.Fatalf("connection %d %s=%d, want %d", i, pragma, got, want)
			}
		}
	}
	for _, c := range conns {
		c.Close()
	}
	if _, err := db.Exec("CREATE TABLE writes (id TEXT PRIMARY KEY)"); err != nil {
		t.Fatal(err)
	}
	var wg sync.WaitGroup
	for i := 0; i < 4; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			for j := 0; j < 20; j++ {
				if _, err := db.Exec("INSERT INTO writes VALUES (?)", fmt.Sprintf("%d-%d", i, j)); err != nil {
					t.Error(err)
					return
				}
			}
		}(i)
	}
	wg.Wait()
	var count int
	if err := db.QueryRow("SELECT count(*) FROM writes").Scan(&count); err != nil {
		t.Fatal(err)
	}
	if count != 80 {
		t.Fatal(count)
	}
}

func TestMemoryAndEscapedPaths(t *testing.T) {
	for _, path := range []string{":memory:", "file:memory-test?mode=memory&cache=shared", filepath.Join(t.TempDir(), "a?#.db")} {
		db, err := Open(path)
		if err != nil {
			t.Fatal(err)
		}
		if _, err := db.Exec("CREATE TABLE example (id INTEGER)"); err != nil {
			t.Fatal(err)
		}
		db.Close()
	}
}
