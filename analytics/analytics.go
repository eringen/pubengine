// Package analytics provides privacy-first website analytics.
package analytics

import (
	"crypto/rand"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"net/url"
	"strings"
	"time"
)

// InitSalt initializes this store's persistent hashing key. Failed attempts can retry.
// NewStore calls it before returning; repeated calls on that store are harmless.
func InitSalt(store *Store) error {
	store.saltMu.Lock()
	defer store.saltMu.Unlock()
	if store.salt != "" {
		return nil
	}
	tx, err := store.db.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()
	var value string
	err = tx.QueryRow("SELECT value FROM settings WHERE key='hash_salt'").Scan(&value)
	if err != nil && err != sql.ErrNoRows {
		return err
	}
	if value == "" {
		b := make([]byte, 32)
		if _, err := rand.Read(b); err != nil {
			return err
		}
		value = hex.EncodeToString(b)
		if _, err := tx.Exec("INSERT INTO settings (key,value) VALUES ('hash_salt',?) ON CONFLICT(key) DO UPDATE SET value=excluded.value", value); err != nil {
			return err
		}
	}
	if err := tx.Commit(); err != nil {
		return err
	}
	store.salt = value
	return nil
}

// Visit represents a single page view.
type Visit struct {
	PageViewID  string    `json:"page_view_id"`
	ID          int64     `json:"-"`
	VisitorID   string    `json:"visitor_id"`  // Anonymous fingerprint hash
	SessionID   string    `json:"session_id"`  // Session identifier
	IPHash      string    `json:"-"`           // Hashed IP address
	Browser     string    `json:"browser"`     // Browser name
	OS          string    `json:"os"`          // Operating system
	Device      string    `json:"device"`      // desktop, mobile, tablet
	Path        string    `json:"path"`        // Page path
	Referrer    string    `json:"referrer"`    // Referrer URL
	ScreenSize  string    `json:"screen_size"` // e.g., "1920x1080"
	Timestamp   time.Time `json:"timestamp"`
	DurationSec int       `json:"duration_sec"` // Time spent on page (0 if not available)
}

// BotVisit represents a single bot/crawler page view.
type BotVisit struct {
	PageViewID string    `json:"page_view_id"`
	ID         int64     `json:"-"`
	BotName    string    `json:"bot_name"`   // Name of the bot (e.g., "Googlebot")
	IPHash     string    `json:"-"`          // Hashed IP address
	UserAgent  string    `json:"user_agent"` // Full user agent string
	Path       string    `json:"path"`       // Page path
	Timestamp  time.Time `json:"timestamp"`
}

// VisitRequest is the data sent from client.
type VisitRequest struct {
	Path       string `json:"path"`
	Referrer   string `json:"referrer"`
	ScreenSize string `json:"screen_size"`
	UserAgent  string `json:"user_agent"`
	Language   string `json:"language"`
}

// Stats holds aggregated analytics data.
type Stats struct {
	Period         string            `json:"period"`
	UniqueVisitors int               `json:"unique_visitors"`
	TotalViews     int               `json:"total_views"`
	AvgDuration    int               `json:"avg_duration_sec"`
	TopPages       []PageStat        `json:"top_pages"`
	LatestPages    []LatestPageVisit `json:"latest_pages"`
	BrowserStats   []DimensionStat   `json:"browsers"`
	OSStats        []DimensionStat   `json:"os"`
	DeviceStats    []DimensionStat   `json:"devices"`
	ReferrerStats  []DimensionStat   `json:"referrers"`
	DailyViews     []DailyView       `json:"daily_views"`
}

// BotStats holds aggregated bot analytics data.
type BotStats struct {
	Period      string          `json:"period"`
	TotalVisits int             `json:"total_visits"`
	TopBots     []DimensionStat `json:"top_bots"`
	TopPages    []PageStat      `json:"top_pages"`
	DailyVisits []DailyView     `json:"daily_visits"`
}

// PageStat represents page view statistics.
type PageStat struct {
	Path  string `json:"path"`
	Views int    `json:"views"`
}

// LatestPageVisit represents a single recent page visit.
type LatestPageVisit struct {
	Path      string `json:"path"`
	Timestamp string `json:"timestamp"`
	Browser   string `json:"browser"`
}

// DimensionStat represents a dimension breakdown (browser, OS, etc.).
type DimensionStat struct {
	Name  string `json:"name"`
	Count int    `json:"count"`
}

// DailyView represents views per day.
type DailyView struct {
	Date  string `json:"date"`
	Views int    `json:"views"`
}

// HashIP creates a hash scoped to this analytics installation.
func (s *Store) HashIP(ip string) string { return s.hashIdentity(ip) }

// GenerateVisitorID creates a persistent installation-scoped visitor identifier.
func (s *Store) GenerateVisitorID(ip, userAgent string) string {
	return s.hashIdentity(ip + "|" + userAgent)
}

func (s *Store) hashIdentity(value string) string {
	s.saltMu.RLock()
	key := s.salt
	s.saltMu.RUnlock()
	sum := sha256.Sum256([]byte(key + value))
	return hex.EncodeToString(sum[:])[:16]
}

// ParseUserAgent extracts browser, OS, and device from User-Agent string.
func ParseUserAgent(ua string) (browser, os, device string) {
	ua = strings.ToLower(ua)

	// Detect browser (order matters: more specific patterns before generic ones)
	switch {
	case strings.Contains(ua, "firefox"):
		browser = "Firefox"
	case strings.Contains(ua, "opera") || strings.Contains(ua, "opr"):
		browser = "Opera"
	case strings.Contains(ua, "edg"):
		browser = "Edge"
	case strings.Contains(ua, "chrome"):
		browser = "Chrome"
	case strings.Contains(ua, "safari"):
		browser = "Safari"
	default:
		browser = "Other"
	}

	// Detect OS (order matters: Android before Linux since Android UA contains "linux")
	switch {
	case strings.Contains(ua, "windows"):
		os = "Windows"
	case strings.Contains(ua, "android"):
		os = "Android"
	case strings.Contains(ua, "iphone") || strings.Contains(ua, "ipad"):
		os = "iOS"
	case strings.Contains(ua, "macintosh") || strings.Contains(ua, "mac os"):
		os = "macOS"
	case strings.Contains(ua, "linux"):
		os = "Linux"
	default:
		os = "Other"
	}

	// Detect device type (order matters: iPad contains "mobile" in UA, check tablet first)
	switch {
	case strings.Contains(ua, "tablet") || strings.Contains(ua, "ipad"):
		device = "Tablet"
	case strings.Contains(ua, "mobile"):
		device = "Mobile"
	default:
		device = "Desktop"
	}

	return
}

// IsBot checks if the User-Agent is likely a bot/crawler.
func IsBot(ua string) bool {
	ua = strings.ToLower(ua)
	bots := []string{
		"bot", "crawler", "spider", "crawl", "slurp", "scrape",
		"googlebot", "bingbot", "yandex", "baidu", "duckduckbot",
		"facebookexternalhit", "twitterbot", "linkedinbot",
		"ahrefsbot", "semrushbot", "mj12bot", "dotbot",
	}
	for _, bot := range bots {
		if strings.Contains(ua, bot) {
			return true
		}
	}
	return false
}

// ExtractBotName extracts the bot name from User-Agent string.
func ExtractBotName(ua string) string {
	ua = strings.ToLower(ua)

	// Known bot patterns
	botPatterns := map[string]string{
		"googlebot":           "Googlebot",
		"bingbot":             "Bingbot",
		"yandex":              "Yandex",
		"baidu":               "Baidu",
		"duckduckbot":         "DuckDuckBot",
		"facebookexternalhit": "Facebook",
		"twitterbot":          "Twitterbot",
		"linkedinbot":         "LinkedIn",
		"ahrefsbot":           "Ahrefs",
		"semrushbot":          "SEMrush",
		"mj12bot":             "Majestic",
		"dotbot":              "Moz",
		"slurp":               "Yahoo Slurp",
		"crawler":             "Generic Crawler",
		"spider":              "Generic Spider",
	}

	for pattern, name := range botPatterns {
		if strings.Contains(ua, pattern) {
			return name
		}
	}

	// Generic bot detection
	if strings.Contains(ua, "bot") {
		return "Other Bot"
	}

	return "Unknown"
}

// CleanReferrer normalizes raw URLs and historical stored labels to one source.
func CleanReferrer(ref string) string {
	ref = strings.TrimSpace(ref)
	if ref == "" {
		return "Direct"
	}
	for _, label := range []string{"Direct", "Google", "Bing", "DuckDuckGo", "Yahoo", "GitHub", "Other"} {
		if strings.EqualFold(ref, label) {
			return label
		}
	}
	if !strings.Contains(ref, "://") {
		ref = "https://" + ref
	}
	u, err := url.Parse(ref)
	if err != nil || (u.Scheme != "http" && u.Scheme != "https") || u.Hostname() == "" {
		return "Other"
	}
	host := strings.TrimPrefix(strings.ToLower(u.Hostname()), "www.")
	for _, source := range []struct{ domain, label string }{
		{"google.com", "Google"}, {"google.co.uk", "Google"}, {"google.com.tr", "Google"}, {"google.de", "Google"},
		{"bing.com", "Bing"}, {"duckduckgo.com", "DuckDuckGo"}, {"yahoo.com", "Yahoo"}, {"github.com", "GitHub"},
	} {
		if host == source.domain || strings.HasSuffix(host, "."+source.domain) {
			return source.label
		}
	}
	return host
}

// TruncateDate returns the date truncated to the specified period.
func TruncateDate(t time.Time, period string) time.Time {
	switch period {
	case "day":
		return time.Date(t.Year(), t.Month(), t.Day(), 0, 0, 0, 0, t.Location())
	case "week":
		// Go to Monday of the week
		wd := int(t.Weekday())
		if wd == 0 {
			wd = 7
		}
		return time.Date(t.Year(), t.Month(), t.Day()-wd+1, 0, 0, 0, 0, t.Location())
	case "month":
		return time.Date(t.Year(), t.Month(), 1, 0, 0, 0, 0, t.Location())
	default:
		return t
	}
}
