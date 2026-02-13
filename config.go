package pubengine

import "time"

// SiteConfig holds all configuration for a pubengine site.
type SiteConfig struct {
	Name        string // Site name (default "Blog")
	URL         string // Canonical URL (default "http://localhost:3000")
	Description string // Site description for RSS and meta tags
	Author      string // Author name for JSON-LD

	Addr         string // Listen address (default ":3000")
	DatabasePath string // SQLite path (default "data/blog.db")

	AnalyticsEnabled      bool   // Enable analytics (default true)
	AnalyticsDatabasePath string // Analytics SQLite path (default "data/analytics.db")

	AdminPassword string // Required: admin login password
	SessionSecret string // Required: session encryption secret
	CookieSecure  bool   // Set true for HTTPS

	PostCacheTTL time.Duration // Post cache TTL (default 5min)
}

func (c *SiteConfig) setDefaults() {
	if c.Name == "" {
		c.Name = "Blog"
	}
	if c.URL == "" {
		c.URL = "http://localhost:3000"
	}
	if c.Addr == "" {
		c.Addr = ":3000"
	}
	if c.DatabasePath == "" {
		c.DatabasePath = "data/blog.db"
	}
	if c.AnalyticsDatabasePath == "" {
		c.AnalyticsDatabasePath = "data/analytics.db"
	}
	if c.PostCacheTTL == 0 {
		c.PostCacheTTL = 5 * time.Minute
	}
}

// Option configures additional App behavior.
type Option func(*App)

// WithCustomRoutes registers additional routes on the Echo instance.
// The callback receives the Echo instance before the server starts.
func WithCustomRoutes(fn func(*App)) Option {
	return func(a *App) {
		a.customRoutes = append(a.customRoutes, fn)
	}
}

// WithStaticDir sets the directory for user-owned static assets (default "public").
func WithStaticDir(dir string) Option {
	return func(a *App) {
		a.staticDir = dir
	}
}
