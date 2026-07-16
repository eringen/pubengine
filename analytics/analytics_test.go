package analytics

import (
	"context"
	"database/sql"
	"encoding/json"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/labstack/echo/v4"
)

func testStore(t *testing.T) *Store {
	t.Helper()
	s, err := NewStore(filepath.Join(t.TempDir(), "nested", "analytics.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { s.Close() })
	return s
}

func TestReferrersAndCalendarBuckets(t *testing.T) {
	s := testStore(t)
	now := time.Date(2026, 7, 8, 12, 0, 0, 0, time.UTC)
	for i, ref := range []string{"", "https://www.google.com/search", "https://github.com/repo", "https://example.com/google.com"} {
		if err := s.SaveVisit(&Visit{VisitorID: string(rune('a' + i)), Referrer: ref, Timestamp: now}); err != nil {
			t.Fatal(err)
		}
	}
	from, to := periodTimeRangeAt(now, 7, false)
	if to.Sub(from) != 7*24*time.Hour {
		t.Fatal(from, to)
	}
	stats, err := s.GetStats(from, to, false, false)
	if err != nil {
		t.Fatal(err)
	}
	counts := map[string]int{}
	for _, ref := range stats.ReferrerStats {
		counts[ref.Name] = ref.Count
	}
	for _, name := range []string{"Direct", "Google", "GitHub", "example.com"} {
		if counts[name] != 1 {
			t.Fatal(counts)
		}
	}
	if len(stats.DailyViews) != 7 || stats.DailyViews[6].Views != 4 || stats.DailyViews[0].Views != 0 {
		t.Fatal(stats.DailyViews)
	}
	for _, days := range []int{7, 30, 365} {
		from, to := periodTimeRangeAt(now, days, false)
		if int(to.Sub(from).Hours()) != days*24 {
			t.Fatal(days, from, to)
		}
	}
}

func TestSaltIsolationPersistenceAndRetry(t *testing.T) {
	a, b := testStore(t), testStore(t)
	if a.HashIP("ip") == b.HashIP("ip") {
		t.Fatal("stores share a salt")
	}
	value, err := a.GetSetting("hash_salt")
	if err != nil {
		t.Fatal(err)
	}
	a.saltMu.Lock()
	a.salt = ""
	a.saltMu.Unlock()
	if err := InitSalt(a); err != nil {
		t.Fatal(err)
	}
	if a.salt != value {
		t.Fatal("salt changed")
	}
	// A failed database read must not permanently consume an initialization guard.
	db, err := sql.Open("sqlite", ":memory:")
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	db.SetMaxOpenConns(1)
	broken := &Store{db: db}
	if err := InitSalt(broken); err == nil {
		t.Fatal("expected missing settings failure")
	}
	if _, err := db.Exec("CREATE TABLE settings (key TEXT PRIMARY KEY,value TEXT NOT NULL)"); err != nil {
		t.Fatal(err)
	}
	if err := InitSalt(broken); err != nil || broken.salt == "" {
		t.Fatal(err)
	}
}

func TestPageViewIdentityAndOutOfOrderEvents(t *testing.T) {
	s := testStore(t)
	h := NewHandler(s)
	t.Cleanup(h.Close)
	e := echo.New()
	send := func(event, id, path string, duration int) {
		t.Helper()
		body, _ := json.Marshal(CollectRequest{Event: event, PageViewID: id, Path: path, DurationSec: duration, UserAgent: "Mozilla Chrome"})
		req := httptest.NewRequest("POST", "/api/analytics/collect", strings.NewReader(string(body)))
		req.Header.Set("Content-Type", "application/json")
		r := httptest.NewRecorder()
		if err := h.Collect(e.NewContext(req, r)); err != nil {
			t.Fatal(err)
		}
		if r.Code != 204 {
			t.Fatalf("%d %s", r.Code, r.Body)
		}
	}
	id1, id2 := "aaaaaaaaaaaaaaaa", "bbbbbbbbbbbbbbbb"
	send("duration", id1, "/a/", 10)
	send("view", id1, "/a/", 0)
	send("duration", id1, "/a/", 3)
	send("view", id2, "/a/", 0)
	send("duration", id2, "/a/", 0)
	send("view", id2, "/a/", 0)
	rows, err := s.db.Query("SELECT page_view_id,duration_sec FROM visits ORDER BY page_view_id")
	if err != nil {
		t.Fatal(err)
	}
	defer rows.Close()
	got := map[string]int{}
	for rows.Next() {
		var id string
		var duration int
		if err := rows.Scan(&id, &duration); err != nil {
			t.Fatal(err)
		}
		got[id] = duration
	}
	if err := rows.Err(); err != nil {
		t.Fatal(err)
	}
	if len(got) != 2 || got[id1] != 10 || got[id2] != 0 {
		t.Fatal(got)
	}
}

func TestCollectRejectsInvalidIdentityAndHeaderFallback(t *testing.T) {
	s := testStore(t)
	h := NewHandler(s)
	t.Cleanup(h.Close)
	e := echo.New()
	for _, body := range []string{`{"path":"/","duration_sec":0}`, `{"event":"view","page_view_id":"aaaaaaaaaaaaaaaa","path":"https://example.com/"}`} {
		r := httptest.NewRecorder()
		req := httptest.NewRequest("POST", "/", strings.NewReader(body))
		if err := h.Collect(e.NewContext(req, r)); err != nil {
			t.Fatal(err)
		}
		if r.Code != 400 {
			t.Fatal(r.Code)
		}
	}
	r := httptest.NewRecorder()
	req := httptest.NewRequest("POST", "/", strings.NewReader(`{"event":"view","page_view_id":"aaaaaaaaaaaaaaaa","path":"/"}`))
	req.Header.Set("User-Agent", strings.Repeat("x", 513))
	if err := h.Collect(e.NewContext(req, r)); err != nil {
		t.Fatal(err)
	}
	if r.Code != 400 {
		t.Fatal(r.Code)
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if err := s.SaveVisitContext(ctx, &Visit{}); err == nil {
		t.Fatal("canceled insert succeeded")
	}
}

func TestAnalyticsMigrationNormalizesLegacyRows(t *testing.T) {
	path := filepath.Join(t.TempDir(), "old.db")
	// Create an actual v1 database without the page-view columns.
	db, err := sql.Open("sqlite", path)
	if err != nil {
		t.Fatal(err)
	}
	old := &Store{db: db}
	if err := old.ensureSchema(); err != nil {
		t.Fatal(err)
	}
	_, err = db.Exec(`INSERT INTO settings VALUES ('schema_version','1'); INSERT INTO visits (visitor_id,session_id,ip_hash,browser,os,device,path,referrer,timestamp) VALUES ('v','s','ip','b','o','d','/','https://www.google.com/search','2026-07-01')`)
	db.Close()
	if err != nil {
		t.Fatal(err)
	}
	upgraded, err := NewStore(path)
	if err != nil {
		t.Fatal(err)
	}
	defer upgraded.Close()
	var ref string
	if err := upgraded.db.QueryRow("SELECT referrer FROM visits").Scan(&ref); err != nil || ref != "Google" {
		t.Fatal(ref, err)
	}
	if err := upgraded.SaveVisit(&Visit{VisitorID: "new", PageViewID: "aaaaaaaaaaaaaaaa", Timestamp: time.Now()}); err != nil {
		t.Fatal(err)
	}
}
