package analytics

import (
	"context"
	"database/sql"
	"fmt"
	"strconv"
	"sync"
	"time"

	"github.com/eringen/pubengine/analytics/sqlcgen"
	"github.com/eringen/pubengine/internal/sqliteutil"
)

// Store provides database operations for analytics.
type Store struct {
	db     *sql.DB
	q      *sqlcgen.Queries
	saltMu sync.RWMutex
	salt   string
}

// NewStore creates a new analytics store.
func NewStore(dbPath string) (*Store, error) {
	db, err := sqliteutil.Open(dbPath)
	if err != nil {
		return nil, fmt.Errorf("open analytics db: %w", err)
	}

	s := &Store{
		db: db,
		q:  sqlcgen.New(db),
	}
	if err := s.ensureSchema(); err != nil {
		db.Close()
		return nil, fmt.Errorf("ensure schema: %w", err)
	}
	if err := s.migrate(); err != nil {
		db.Close()
		return nil, fmt.Errorf("migrate: %w", err)
	}

	if err := InitSalt(s); err != nil {
		db.Close()
		return nil, fmt.Errorf("initialize salt: %w", err)
	}
	return s, nil
}

// Close closes the database connection.
func (s *Store) Close() error {
	return s.db.Close()
}

// ensureSchema creates the necessary tables if they don't exist.
func (s *Store) ensureSchema() error {
	_, err := s.db.Exec(`
		CREATE TABLE IF NOT EXISTS visits (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			visitor_id TEXT NOT NULL,
			session_id TEXT NOT NULL,
			ip_hash TEXT NOT NULL,
			browser TEXT NOT NULL,
			os TEXT NOT NULL,
			device TEXT NOT NULL,
			path TEXT NOT NULL,
			referrer TEXT,
			screen_size TEXT,
			timestamp DATETIME NOT NULL,
			duration_sec INTEGER DEFAULT 0
		);

		CREATE TABLE IF NOT EXISTS bot_visits (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			bot_name TEXT NOT NULL,
			ip_hash TEXT NOT NULL,
			user_agent TEXT NOT NULL,
			path TEXT NOT NULL,
			timestamp DATETIME NOT NULL
		);

		CREATE INDEX IF NOT EXISTS idx_visits_timestamp ON visits(timestamp);
		CREATE INDEX IF NOT EXISTS idx_visits_visitor_id ON visits(visitor_id);
		CREATE INDEX IF NOT EXISTS idx_visits_path ON visits(path);
		CREATE INDEX IF NOT EXISTS idx_visits_browser ON visits(browser);
		CREATE INDEX IF NOT EXISTS idx_visits_os ON visits(os);
		CREATE INDEX IF NOT EXISTS idx_visits_device ON visits(device);

		CREATE INDEX IF NOT EXISTS idx_bot_visits_timestamp ON bot_visits(timestamp);
		CREATE INDEX IF NOT EXISTS idx_bot_visits_name ON bot_visits(bot_name);

		CREATE TABLE IF NOT EXISTS settings (
			key TEXT PRIMARY KEY,
			value TEXT NOT NULL
		);
	`)
	return err
}

// currentSchemaVersion is the latest schema version. Increment when adding migrations.
const currentSchemaVersion = 3

// migrate applies incremental schema migrations based on a version stored in the settings table.
func (s *Store) migrate() error {
	tx, err := s.db.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()
	var verStr string
	err = tx.QueryRow("SELECT value FROM settings WHERE key='schema_version'").Scan(&verStr)
	if err != nil && err != sql.ErrNoRows {
		return err
	}
	version := 0
	if verStr != "" {
		version, err = strconv.Atoi(verStr)
		if err != nil {
			return err
		}
	}
	if version > currentSchemaVersion || version < 0 {
		return fmt.Errorf("unsupported analytics schema version %d", version)
	}
	if version < 2 {
		rows, err := tx.Query("SELECT DISTINCT referrer FROM visits")
		if err != nil {
			return err
		}
		var refs []sql.NullString
		for rows.Next() {
			var ref sql.NullString
			if err := rows.Scan(&ref); err != nil {
				rows.Close()
				return err
			}
			refs = append(refs, ref)
		}
		err = rows.Err()
		rows.Close()
		if err != nil {
			return err
		}
		for _, ref := range refs {
			if _, err := tx.Exec("UPDATE visits SET referrer=? WHERE referrer IS ?", CleanReferrer(ref.String), ref); err != nil {
				return err
			}
		}
	}
	if version < 3 {
		if _, err := tx.Exec(`ALTER TABLE visits ADD COLUMN page_view_id TEXT;
   CREATE UNIQUE INDEX idx_visits_page_view ON visits(visitor_id,page_view_id);
   ALTER TABLE bot_visits ADD COLUMN page_view_id TEXT;
   CREATE UNIQUE INDEX idx_bot_page_view ON bot_visits(ip_hash,page_view_id);`); err != nil {
			return err
		}
	}
	if _, err := tx.Exec("INSERT INTO settings (key,value) VALUES ('schema_version',?) ON CONFLICT(key) DO UPDATE SET value=excluded.value", strconv.Itoa(currentSchemaVersion)); err != nil {
		return err
	}
	return tx.Commit()
}

// GetSetting retrieves a setting value by key. Returns empty string if not found.
func (s *Store) GetSetting(key string) (string, error) {
	val, err := s.q.GetSetting(context.Background(), key)
	if err == sql.ErrNoRows {
		return "", nil
	}
	return val, err
}

// SetSetting stores a setting value by key (upsert).
func (s *Store) SetSetting(key, value string) error {
	return s.q.UpsertSetting(context.Background(), key, value)
}

// SaveVisit stores a new visit in the database.
func (s *Store) SaveVisit(v *Visit) error { return s.SaveVisitContext(context.Background(), v) }

func (s *Store) SaveVisitContext(ctx context.Context, v *Visit) error {
	return s.q.InsertVisit(ctx, sqlcgen.InsertVisitParams{
		VisitorID:   v.VisitorID,
		PageViewID:  sql.NullString{String: v.PageViewID, Valid: v.PageViewID != ""},
		SessionID:   v.SessionID,
		IpHash:      v.IPHash,
		Browser:     v.Browser,
		Os:          v.OS,
		Device:      v.Device,
		Path:        v.Path,
		Referrer:    sql.NullString{String: CleanReferrer(v.Referrer), Valid: true},
		ScreenSize:  sql.NullString{String: v.ScreenSize, Valid: true},
		Timestamp:   v.Timestamp.UTC(),
		DurationSec: sql.NullInt64{Int64: int64(v.DurationSec), Valid: true},
	})
}

// UpdateVisitDuration updates the duration of the most recent visit for a visitor+path.
func (s *Store) UpdateVisitDuration(visitorID, path string, durationSec int) error {
	return s.q.UpdateVisitDuration(context.Background(), sqlcgen.UpdateVisitDurationParams{
		DurationSec: sql.NullInt64{Int64: int64(durationSec), Valid: true},
		VisitorID:   visitorID,
		Path:        path,
	})
}

// SaveBotVisit stores a new bot visit in the database.
func (s *Store) SaveBotVisit(bv *BotVisit) error {
	return s.SaveBotVisitContext(context.Background(), bv)
}

func (s *Store) SaveBotVisitContext(ctx context.Context, bv *BotVisit) error {
	return s.q.InsertBotVisit(ctx, sqlcgen.InsertBotVisitParams{
		BotName:    bv.BotName,
		PageViewID: sql.NullString{String: bv.PageViewID, Valid: bv.PageViewID != ""},
		IpHash:     bv.IPHash,
		UserAgent:  bv.UserAgent,
		Path:       bv.Path,
		Timestamp:  bv.Timestamp.UTC(),
	})
}

// GetStats returns aggregated statistics for the given time period.
func (s *Store) GetStats(from, to time.Time, hourly, monthly bool) (*Stats, error) {
	ctx := context.Background()
	stats := &Stats{
		Period:        from.Format("2006-01-02") + " to " + to.Format("2006-01-02"),
		TopPages:      []PageStat{},
		LatestPages:   []LatestPageVisit{},
		BrowserStats:  []DimensionStat{},
		OSStats:       []DimensionStat{},
		DeviceStats:   []DimensionStat{},
		ReferrerStats: []DimensionStat{},
		DailyViews:    []DailyView{},
	}

	var mu sync.Mutex
	var wg sync.WaitGroup
	var firstErr error

	setErr := func(err error) {
		mu.Lock()
		if firstErr == nil {
			firstErr = err
		}
		mu.Unlock()
	}

	// Total views
	wg.Add(1)
	go func() {
		defer wg.Done()
		count, err := s.q.CountVisits(ctx, from, to)
		if err != nil {
			setErr(fmt.Errorf("count views: %w", err))
			return
		}
		mu.Lock()
		stats.TotalViews = int(count)
		mu.Unlock()
	}()

	// Unique visitors
	wg.Add(1)
	go func() {
		defer wg.Done()
		count, err := s.q.CountUniqueVisitors(ctx, from, to)
		if err != nil {
			setErr(fmt.Errorf("count unique visitors: %w", err))
			return
		}
		mu.Lock()
		stats.UniqueVisitors = int(count)
		mu.Unlock()
	}()

	// Average duration
	wg.Add(1)
	go func() {
		defer wg.Done()
		avg, err := s.q.AvgDuration(ctx, from, to)
		if err != nil {
			setErr(fmt.Errorf("avg duration: %w", err))
			return
		}
		if avg.Valid {
			mu.Lock()
			stats.AvgDuration = int(avg.Float64)
			mu.Unlock()
		}
	}()

	// Top pages
	wg.Add(1)
	go func() {
		defer wg.Done()
		rows, err := s.q.TopPages(ctx, from, to)
		if err != nil {
			setErr(fmt.Errorf("top pages: %w", err))
			return
		}
		pages := make([]PageStat, len(rows))
		for i, r := range rows {
			pages[i] = PageStat{Path: r.Path, Views: int(r.Views)}
		}
		mu.Lock()
		stats.TopPages = pages
		mu.Unlock()
	}()

	// Latest pages
	wg.Add(1)
	go func() {
		defer wg.Done()
		rows, err := s.q.LatestPages(ctx, from, to)
		if err != nil {
			setErr(fmt.Errorf("latest pages: %w", err))
			return
		}
		latest := make([]LatestPageVisit, len(rows))
		for i, r := range rows {
			latest[i] = LatestPageVisit{
				Path:      r.Path,
				Timestamp: r.Timestamp.Format("2006-01-02 15:04:05"),
				Browser:   r.Browser,
			}
		}
		mu.Lock()
		stats.LatestPages = latest
		mu.Unlock()
	}()

	// Browser stats
	wg.Add(1)
	go func() {
		defer wg.Done()
		rows, err := s.q.BrowserStats(ctx, from, to)
		if err != nil {
			setErr(fmt.Errorf("browser stats: %w", err))
			return
		}
		result := make([]DimensionStat, len(rows))
		for i, r := range rows {
			result[i] = DimensionStat{Name: r.Name, Count: int(r.Count)}
		}
		mu.Lock()
		stats.BrowserStats = result
		mu.Unlock()
	}()

	// OS stats
	wg.Add(1)
	go func() {
		defer wg.Done()
		rows, err := s.q.OSStats(ctx, from, to)
		if err != nil {
			setErr(fmt.Errorf("os stats: %w", err))
			return
		}
		result := make([]DimensionStat, len(rows))
		for i, r := range rows {
			result[i] = DimensionStat{Name: r.Name, Count: int(r.Count)}
		}
		mu.Lock()
		stats.OSStats = result
		mu.Unlock()
	}()

	// Device stats
	wg.Add(1)
	go func() {
		defer wg.Done()
		rows, err := s.q.DeviceStats(ctx, from, to)
		if err != nil {
			setErr(fmt.Errorf("device stats: %w", err))
			return
		}
		result := make([]DimensionStat, len(rows))
		for i, r := range rows {
			result[i] = DimensionStat{Name: r.Name, Count: int(r.Count)}
		}
		mu.Lock()
		stats.DeviceStats = result
		mu.Unlock()
	}()

	// Referrer stats
	wg.Add(1)
	go func() {
		defer wg.Done()
		rows, err := s.q.ReferrerStats(ctx, from, to)
		if err != nil {
			setErr(fmt.Errorf("referrer stats: %w", err))
			return
		}
		result := make([]DimensionStat, len(rows))
		for i, r := range rows {
			result[i] = DimensionStat{Name: r.Name, Count: int(r.Count)}
		}
		mu.Lock()
		stats.ReferrerStats = result
		mu.Unlock()
	}()

	// Daily/hourly/monthly views
	wg.Add(1)
	go func() {
		defer wg.Done()
		var result []DailyView
		if hourly {
			rows, err := s.q.HourlyViews(ctx, from, to)
			if err != nil {
				setErr(fmt.Errorf("hourly views: %w", err))
				return
			}
			sparse := make([]DailyView, len(rows))
			for i, r := range rows {
				sparse[i] = DailyView{Date: r.Date, Views: int(r.Views)}
			}
			result = fillHourlyGaps(from, sparse)
		} else if monthly {
			rows, err := s.q.MonthlyViews(ctx, from, to)
			if err != nil {
				setErr(fmt.Errorf("monthly views: %w", err))
				return
			}
			result = make([]DailyView, len(rows))
			for i, r := range rows {
				result[i] = DailyView{Date: r.Date, Views: int(r.Views)}
			}
		} else {
			rows, err := s.q.DailyViews(ctx, from, to)
			if err != nil {
				setErr(fmt.Errorf("daily views: %w", err))
				return
			}
			result = make([]DailyView, len(rows))
			for i, r := range rows {
				result[i] = DailyView{Date: r.Date, Views: int(r.Views)}
			}
		}
		mu.Lock()
		if !hourly {
			result = fillCalendarGaps(from, to, result, monthly)
		}
		stats.DailyViews = result
		mu.Unlock()
	}()

	wg.Wait()

	if firstErr != nil {
		return nil, firstErr
	}

	return stats, nil
}

// GetBotStats returns aggregated bot statistics for the given time period.
func (s *Store) GetBotStats(from, to time.Time, hourly, monthly bool) (*BotStats, error) {
	ctx := context.Background()
	stats := &BotStats{
		Period:      from.Format("2006-01-02") + " to " + to.Format("2006-01-02"),
		TopBots:     []DimensionStat{},
		TopPages:    []PageStat{},
		DailyVisits: []DailyView{},
	}

	// Total bot visits
	count, err := s.q.CountBotVisits(ctx, from, to)
	if err != nil {
		return nil, fmt.Errorf("count bot visits: %w", err)
	}
	stats.TotalVisits = int(count)

	// Top bots
	topBots, err := s.q.TopBots(ctx, from, to)
	if err != nil {
		return nil, fmt.Errorf("top bots: %w", err)
	}
	for _, r := range topBots {
		stats.TopBots = append(stats.TopBots, DimensionStat{Name: r.Name, Count: int(r.Count)})
	}

	// Top pages
	topPages, err := s.q.TopBotPages(ctx, from, to)
	if err != nil {
		return nil, fmt.Errorf("top bot pages: %w", err)
	}
	for _, r := range topPages {
		stats.TopPages = append(stats.TopPages, PageStat{Path: r.Path, Views: int(r.Views)})
	}

	// Daily/hourly/monthly bot visits
	if hourly {
		rows, err := s.q.HourlyBotVisits(ctx, from, to)
		if err != nil {
			return nil, fmt.Errorf("bot views: %w", err)
		}
		sparse := make([]DailyView, len(rows))
		for i, r := range rows {
			sparse[i] = DailyView{Date: r.Date, Views: int(r.Views)}
		}
		stats.DailyVisits = fillHourlyGaps(from, sparse)
	} else if monthly {
		rows, err := s.q.MonthlyBotVisits(ctx, from, to)
		if err != nil {
			return nil, fmt.Errorf("bot views: %w", err)
		}
		for _, r := range rows {
			stats.DailyVisits = append(stats.DailyVisits, DailyView{Date: r.Date, Views: int(r.Views)})
		}
	} else {
		rows, err := s.q.DailyBotVisits(ctx, from, to)
		if err != nil {
			return nil, fmt.Errorf("bot views: %w", err)
		}
		for _, r := range rows {
			stats.DailyVisits = append(stats.DailyVisits, DailyView{Date: r.Date, Views: int(r.Views)})
		}
	}

	if !hourly {
		stats.DailyVisits = fillCalendarGaps(from, to, stats.DailyVisits, monthly)
	}
	return stats, nil
}

// fillHourlyGaps ensures all 24 hours are present in the result, filling gaps with 0.
// Hours are ordered starting from the 'from' time, so the chart shows a rolling 24h window.
func fillHourlyGaps(from time.Time, sparse []DailyView) []DailyView {
	viewsByHour := make(map[string]int, len(sparse))
	for _, v := range sparse {
		viewsByHour[v.Date] = v.Views
	}

	result := make([]DailyView, 24)
	for i := 0; i < 24; i++ {
		hour := from.Add(time.Duration(i) * time.Hour)
		label := fmt.Sprintf("%02d:00", hour.Hour())
		result[i] = DailyView{Date: label, Views: viewsByHour[label]}
	}
	return result
}

// CleanupOldVisits removes visits and bot visits older than the retention period.
func (s *Store) CleanupOldVisits(retentionDays int) error {
	return s.cleanupOldVisits(context.Background(), retentionDays)
}

func (s *Store) cleanupOldVisits(ctx context.Context, retentionDays int) error {
	cutoff := time.Now().UTC().AddDate(0, 0, -retentionDays)
	if err := s.q.DeleteOldVisits(ctx, cutoff); err != nil {
		return fmt.Errorf("cleanup visits: %w", err)
	}
	if err := s.q.DeleteOldBotVisits(ctx, cutoff); err != nil {
		return fmt.Errorf("cleanup bot_visits: %w", err)
	}
	return nil
}

// StartCleanupScheduler runs periodic cleanup of old data. Returns a stop function.
func (s *Store) StartCleanupScheduler(retentionDays int, interval time.Duration) func() {
	ticker := time.NewTicker(interval)
	ctx, cancel := context.WithCancel(context.Background())
	stopped := make(chan struct{})
	go func() {
		defer ticker.Stop()
		defer close(stopped)
		for {
			select {
			case <-ticker.C:
				if err := s.cleanupOldVisits(ctx, retentionDays); err != nil && ctx.Err() == nil {
					fmt.Printf("cleanup error: %v\n", err)
				}
			case <-ctx.Done():
				return
			}
		}
	}()
	return func() { cancel(); <-stopped }
}

// GetRealtimeVisitors returns the number of unique visitors in the last 5 minutes.
func (s *Store) GetRealtimeVisitors() (int, error) {
	cutoff := time.Now().UTC().Add(-5 * time.Minute)
	count, err := s.q.CountRealtimeVisitors(context.Background(), cutoff)
	return int(count), err
}

func fillCalendarGaps(from, to time.Time, sparse []DailyView, monthly bool) []DailyView {
	counts := make(map[string]int, len(sparse))
	for _, v := range sparse {
		counts[v.Date] = v.Views
	}
	format := "2006-01-02"
	step := func(t time.Time) time.Time { return t.AddDate(0, 0, 1) }
	if monthly {
		format = "2006-01"
		from = time.Date(from.Year(), from.Month(), 1, 0, 0, 0, 0, time.UTC)
		step = func(t time.Time) time.Time { return t.AddDate(0, 1, 0) }
	}
	result := []DailyView{}
	for current := from; current.Before(to); current = step(current) {
		label := current.Format(format)
		result = append(result, DailyView{Date: label, Views: counts[label]})
	}
	return result
}
