package analytics

import (
	"context"
	"errors"
	"fmt"
	"path/filepath"
	"sync"
	"testing"
	"time"

	"github.com/eringen/pubengine/analytics/sqlcgen"
)

func TestReportBoundsOwnershipAndCancellation(t *testing.T) {
	s := testStore(t)
	now := time.Now().UTC()
	for i := 0; i < 100; i++ {
		if err := s.SaveVisit(&Visit{VisitorID: fmt.Sprint(i), Referrer: fmt.Sprintf("https://ref%d.test", i), Timestamp: now, DurationSec: 10}); err != nil {
			t.Fatal(err)
		}
	}
	from, to := now.Add(-time.Hour), now.Add(time.Hour)
	var wg sync.WaitGroup
	for i := 0; i < 8; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			stats, err := s.GetStatsContext(context.Background(), from, to, false, false)
			if err != nil {
				t.Error(err)
				return
			}
			total := 0
			for _, r := range stats.ReferrerStats {
				total += r.Count
			}
			if total != 100 || len(stats.ReferrerStats) > 11 || stats.TotalViews != 100 {
				t.Error(stats)
			}
			stats.ReferrerStats[0].Count = -1
		}()
	}
	wg.Wait()
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := s.GetStatsContext(ctx, from, to, false, false); !errors.Is(err, context.Canceled) {
		t.Fatal(err)
	}
	if _, err := s.GetBotStatsContext(ctx, from, to, false, false); !errors.Is(err, context.Canceled) {
		t.Fatal(err)
	}
	if _, err := s.GetRealtimeVisitorsContext(ctx); !errors.Is(err, context.Canceled) {
		t.Fatal(err)
	}
	if err := s.SaveVisit(&Visit{VisitorID: "after", Timestamp: now}); err != nil {
		t.Fatal("read transaction leaked", err)
	}
}

func TestReportSnapshotAllowsWrites(t *testing.T) {
	s, err := NewStore(filepath.Join(t.TempDir(), "analytics.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	now := time.Now().UTC()
	from, to := now.Add(-time.Hour), now.Add(time.Hour)
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	err = s.readSnapshot(ctx, func(q *sqlcgen.Queries) error {
		before, err := q.CountVisits(ctx, from, to)
		if err != nil {
			return err
		}
		if err := s.SaveVisitContext(ctx, &Visit{VisitorID: "during-read", Timestamp: now}); err != nil {
			return fmt.Errorf("snapshot blocked collection: %w", err)
		}
		after, err := q.CountVisits(ctx, from, to)
		if err == nil && before != after {
			return fmt.Errorf("inconsistent snapshot: %d then %d", before, after)
		}
		return err
	})
	if err != nil {
		t.Fatal(err)
	}
	count, err := s.q.CountVisits(ctx, from, to)
	if err != nil || count != 1 {
		t.Fatal(count, err)
	}
	canceled, stop := context.WithCancel(ctx)
	err = s.readSnapshot(canceled, func(q *sqlcgen.Queries) error {
		stop()
		_, err := q.CountVisits(canceled, from, to)
		return err
	})
	if !errors.Is(err, context.Canceled) {
		t.Fatal(err)
	}
	if err := s.readSnapshot(ctx, func(q *sqlcgen.Queries) error {
		_, err := q.CountVisits(ctx, from, to)
		return err
	}); err != nil {
		t.Fatal("canceled report leaked its transaction", err)
	}
}

func TestReportWaiterCanCancel(t *testing.T) {
	var cache reportCache[Stats]
	started, release, done := make(chan struct{}), make(chan struct{}), make(chan struct{})
	go func() {
		defer close(done)
		_, _ = cache.get(context.Background(), reportKey{}, func() (*Stats, error) { close(started); <-release; return &Stats{}, nil })
	}()
	<-started
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Millisecond)
	defer cancel()
	_, err := cache.get(ctx, reportKey{}, func() (*Stats, error) { t.Error("duplicate build"); return &Stats{}, nil })
	close(release)
	<-done
	if !errors.Is(err, context.DeadlineExceeded) {
		t.Fatal(err)
	}
}

func BenchmarkAnalyticsReport(b *testing.B) {
	s, err := NewStore(b.TempDir() + "/analytics.db")
	if err != nil {
		b.Fatal(err)
	}
	defer s.Close()
	now := time.Now().UTC()
	tx, err := s.db.Begin()
	if err != nil {
		b.Fatal(err)
	}
	for i := 0; i < 10000; i++ {
		if _, err := tx.Exec(`INSERT INTO visits(visitor_id,session_id,ip_hash,browser,os,device,path,referrer,timestamp) VALUES (?,'s','ip','Chrome','Linux','Desktop','/',?,?)`, fmt.Sprint(i%1000), fmt.Sprintf("https://r%d.test", i), now.Add(-time.Duration(i)*time.Minute)); err != nil {
			b.Fatal(err)
		}
	}
	if err := tx.Commit(); err != nil {
		b.Fatal(err)
	}
	from, to := now.AddDate(0, 0, -365), now.Add(time.Hour)
	b.ResetTimer()
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		s.statsReports.mu.Lock()
		s.statsReports.entries = nil
		s.statsReports.mu.Unlock()
		if _, err := s.GetStatsContext(context.Background(), from, to, false, true); err != nil {
			b.Fatal(err)
		}
	}
}
