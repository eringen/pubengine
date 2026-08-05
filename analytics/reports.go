package analytics

import (
	"context"
	"github.com/eringen/pubengine/analytics/sqlcgen"
	"slices"
	"sync"
	"time"
)

type reportKey struct {
	from, to        time.Time
	hourly, monthly bool
}
type reportEntry[T any] struct {
	value *T
	until time.Time
}
type reportCache[T any] struct {
	mu      sync.Mutex
	entries map[reportKey]reportEntry[T]
	pending map[reportKey]chan struct{}
}

func (c *reportCache[T]) get(ctx context.Context, key reportKey, build func() (*T, error)) (*T, error) {
	for {
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		c.mu.Lock()
		if entry, ok := c.entries[key]; ok && time.Now().Before(entry.until) {
			c.mu.Unlock()
			return entry.value, nil
		}
		if done := c.pending[key]; done != nil {
			c.mu.Unlock()
			select {
			case <-done:
				continue
			case <-ctx.Done():
				return nil, ctx.Err()
			}
		}
		if c.pending == nil {
			c.pending = map[reportKey]chan struct{}{}
		}
		done := make(chan struct{})
		c.pending[key] = done
		c.mu.Unlock()
		value, err := build()
		c.mu.Lock()
		delete(c.pending, key)
		if err == nil {
			if c.entries == nil || len(c.entries) >= 16 {
				c.entries = map[reportKey]reportEntry[T]{}
			}
			c.entries[key] = reportEntry[T]{value, time.Now().Add(5 * time.Second)}
		}
		close(done)
		c.mu.Unlock()
		return value, err
	}
}

// Explicit BEGIN avoids the write reservation configured for mutations.
func (s *Store) readSnapshot(ctx context.Context, read func(*sqlcgen.Queries) error) error {
	conn, err := s.db.Conn(ctx)
	if err != nil {
		return err
	}
	defer conn.Close()
	if _, err := conn.ExecContext(ctx, "BEGIN"); err != nil {
		return err
	}
	defer func() {
		rollback, cancel := context.WithTimeout(context.Background(), time.Second)
		defer cancel()
		_, _ = conn.ExecContext(rollback, "ROLLBACK")
	}()
	return read(sqlcgen.New(conn))
}
func cloneStats(p *Stats) *Stats {
	r := *p
	r.TopPages = slices.Clone(p.TopPages)
	r.LatestPages = slices.Clone(p.LatestPages)
	r.BrowserStats = slices.Clone(p.BrowserStats)
	r.OSStats = slices.Clone(p.OSStats)
	r.DeviceStats = slices.Clone(p.DeviceStats)
	r.ReferrerStats = slices.Clone(p.ReferrerStats)
	r.DailyViews = slices.Clone(p.DailyViews)
	return &r
}
func cloneBotStats(p *BotStats) *BotStats {
	r := *p
	r.TopBots = slices.Clone(p.TopBots)
	r.TopPages = slices.Clone(p.TopPages)
	r.DailyVisits = slices.Clone(p.DailyVisits)
	return &r
}
