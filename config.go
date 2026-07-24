package pubengine

import (
	"fmt"
	"strings"
	"time"

	"github.com/labstack/echo/v4"
)

// SiteConfig holds all configuration for a pubengine site.
type SiteConfig struct {
	Name        string // Site name (default "Blog")
	URL         string // Canonical URL (default "http://localhost:3000")
	Description string // Site description for RSS and meta tags
	Author      string // Author name for JSON-LD

	Addr         string // Listen address (default ":3000")
	DatabasePath string // SQLite path (default "data/blog.db")

	AnalyticsEnabled      bool   // Enable analytics (default false; scaffold sets true)
	AnalyticsDatabasePath string // Analytics SQLite path (default "data/analytics.db")

	AdminPassword string // Required: admin login password
	SessionSecret string // Required: cookie signing key, at least 32 bytes
	CookieSecure  bool   // Set true for HTTPS

	GoogleClientID     string // Google OAuth client ID (optional)
	GoogleClientSecret string // Google OAuth client secret (optional)
	GoogleAdminEmail   string // Allowed Google email for admin login (optional)

	ReadHeaderTimeout time.Duration // Default 5 seconds
	ReadTimeout       time.Duration // Default 30 seconds
	WriteTimeout      time.Duration // Default 30 seconds
	IdleTimeout       time.Duration // Default 60 seconds
	ShutdownTimeout   time.Duration // Default 10 seconds

	PostCacheTTL time.Duration // Post cache TTL (default 5min)
}

// GoogleAuthEnabled returns true when all three Google OAuth fields are configured.
func (c *SiteConfig) GoogleAuthEnabled() bool {
	return c.GoogleClientID != "" && c.GoogleClientSecret != "" && c.GoogleAdminEmail != ""
}

func (c *SiteConfig) setDefaults() {
	if c.ReadHeaderTimeout == 0 {
		c.ReadHeaderTimeout = 5 * time.Second
	}
	if c.ReadTimeout == 0 {
		c.ReadTimeout = 30 * time.Second
	}
	if c.WriteTimeout == 0 {
		c.WriteTimeout = 30 * time.Second
	}
	if c.IdleTimeout == 0 {
		c.IdleTimeout = time.Minute
	}
	if c.ShutdownTimeout == 0 {
		c.ShutdownTimeout = 10 * time.Second
	}
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

// WithIPExtractor configures which proxies may supply a client's IP address.
// Use echo.ExtractIPDirect() when the app is reachable without a trusted proxy.
func WithIPExtractor(extractor echo.IPExtractor) Option {
	return func(a *App) { a.Echo.IPExtractor = extractor }
}

func (c SiteConfig) validate() error {
	if strings.TrimSpace(c.AdminPassword) == "" || strings.EqualFold(strings.TrimSpace(c.AdminPassword), "changeme") {
		return fmt.Errorf("pubengine: set AdminPassword to a non-placeholder password")
	}
	if len(c.SessionSecret) < 32 || strings.Contains(strings.ToLower(c.SessionSecret), "changeme") {
		return fmt.Errorf("pubengine: SessionSecret must be a random signing key of at least 32 bytes")
	}
	if c.PostCacheTTL < 0 {
		return fmt.Errorf("pubengine: PostCacheTTL must not be negative")
	}
	if c.ReadHeaderTimeout < 0 || c.ReadTimeout < 0 || c.WriteTimeout < 0 || c.IdleTimeout < 0 || c.ShutdownTimeout < 0 {
		return fmt.Errorf("pubengine: server timeouts must not be negative")
	}
	return nil
}
