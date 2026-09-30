package analytics

import (
	"fmt"
	"path/filepath"
	"testing"

	"github.com/eringen/pubengine/internal/sqliteutil"
)

func legacyStore(t testing.TB, count int) *Store {
	t.Helper()
	db, err := sqliteutil.Open(filepath.Join(t.TempDir(), "legacy.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { db.Close() })
	s := &Store{db: db}
	if err := s.ensureSchema(); err != nil {
		t.Fatal(err)
	}
	tx, err := db.Begin()
	if err != nil {
		t.Fatal(err)
	}
	defer tx.Rollback()
	if _, err := tx.Exec("INSERT INTO settings VALUES ('schema_version','1')"); err != nil {
		t.Fatal(err)
	}
	for i := 0; i < count; i++ {
		_, err := tx.Exec(`INSERT INTO visits(visitor_id,session_id,ip_hash,browser,os,device,path,referrer,timestamp)
VALUES ('v','s','ip','b','o','d','/',?,'2026-06-01')`, fmt.Sprintf("https://ref%d.test/path?q=old", i))
		if err != nil {
			t.Fatal(err)
		}
	}
	if err := tx.Commit(); err != nil {
		t.Fatal(err)
	}
	return s
}

func TestMigrationRollsBackAndRetries(t *testing.T) {
	s := legacyStore(t, 3)
	if _, err := s.db.Exec(`CREATE TRIGGER fail_normalization BEFORE UPDATE ON visits
WHEN OLD.id = 2 BEGIN SELECT RAISE(ABORT, 'interrupted migration'); END`); err != nil {
		t.Fatal(err)
	}
	if err := s.migrate(); err == nil {
		t.Fatal("migration ignored a failed update")
	}
	var version, referrer string
	if err := s.db.QueryRow("SELECT value FROM settings WHERE key='schema_version'").Scan(&version); err != nil || version != "1" {
		t.Fatal(version, err)
	}
	if err := s.db.QueryRow("SELECT referrer FROM visits WHERE id=1").Scan(&referrer); err != nil || referrer != "https://ref0.test/path?q=old" {
		t.Fatal("partial migration persisted", referrer, err)
	}
	if _, err := s.db.Exec("DROP TRIGGER fail_normalization"); err != nil {
		t.Fatal(err)
	}
	if err := s.migrate(); err != nil {
		t.Fatal(err)
	}
	if err := s.migrate(); err != nil {
		t.Fatal("restart failed", err)
	}
	if err := s.db.QueryRow("SELECT referrer FROM visits WHERE id=1").Scan(&referrer); err != nil || referrer != "ref0.test" {
		t.Fatal(referrer, err)
	}
}

func BenchmarkLegacyMigration(b *testing.B) {
	for _, count := range []int{1000, 10000} {
		b.Run(fmt.Sprint(count), func(b *testing.B) {
			b.ReportAllocs()
			for i := 0; i < b.N; i++ {
				b.StopTimer()
				s := legacyStore(b, count)
				b.StartTimer()
				if err := s.migrate(); err != nil {
					b.Fatal(err)
				}
				b.StopTimer()
				s.db.Close()
			}
		})
	}
}
