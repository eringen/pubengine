// Package pubengine is a blog publishing engine built with Go, Echo, and templ.
// It provides blog CRUD, admin dashboard, analytics, RSS, and sitemap out of the box.
//
// Users provide their own templ templates via the ViewFuncs struct,
// and pubengine handles all the handler logic, middleware, and database operations.
package pubengine

import (
	"fmt"
	"io/fs"
	"log"
	"net/http"
	"os"
	"time"

	"github.com/a-h/templ"
	"github.com/labstack/echo/v4"

	"github.com/eringen/pubengine/analytics"
)

// ViewFuncs holds user-provided templ components that the framework calls
// when rendering pages. This is the inversion-of-control mechanism that
// lets users own and customize all templates.
type ViewFuncs struct {
	Home             func(posts []BlogPost, activeTag string, tags []string, siteURL string) templ.Component
	HomePartial      func(posts []BlogPost, activeTag string, tags []string, siteURL string) templ.Component
	BlogSection      func(posts []BlogPost, activeTag string, tags []string) templ.Component
	Post             func(post BlogPost, posts []BlogPost, siteURL string) templ.Component
	PostPartial      func(post BlogPost, posts []BlogPost, siteURL string) templ.Component
	AdminLogin       func(showError bool, csrfToken string) templ.Component
	AdminDashboard   func(posts []BlogPost, message string, csrfToken string) templ.Component
	AdminFormPartial func(post BlogPost, csrfToken string) templ.Component
	AdminImages      func(images []Image, csrfToken string) templ.Component
	NotFound         func() templ.Component
	ServerError      func() templ.Component
}

// App is the central pubengine application. It wires together the store,
// cache, handlers, middleware, and user-provided templates.
type App struct {
	Config SiteConfig
	Echo   *echo.Echo
	Store  *Store
	Cache  *PostCache
	Views  ViewFuncs

	loginLimiter   *LoginLimiter
	analyticsStore *analytics.Store
	customRoutes   []func(*App)
	staticDir      string
}

// New creates a new pubengine App with the given configuration and view functions.
func New(cfg SiteConfig, views ViewFuncs, opts ...Option) *App {
	cfg.setDefaults()

	a := &App{
		Config:    cfg,
		Echo:      echo.New(),
		Views:     views,
		staticDir: "public",
	}

	for _, opt := range opts {
		opt(a)
	}

	return a
}

// Start initializes the database, cache, middleware, routes, and starts the server.
func (a *App) Start() error {
	// Validate required config
	if a.Config.AdminPassword == "" {
		return fmt.Errorf("pubengine: AdminPassword is required")
	}
	if a.Config.SessionSecret == "" {
		return fmt.Errorf("pubengine: SessionSecret is required")
	}

	// Initialize store
	store, err := NewStore(a.Config.DatabasePath)
	if err != nil {
		return fmt.Errorf("pubengine: init store: %w", err)
	}
	a.Store = store

	// Initialize cache
	a.Cache = NewPostCache(a.Store, a.Config.PostCacheTTL)

	// Initialize login limiter
	a.loginLimiter = NewLoginLimiter(5, time.Minute)

	// Initialize analytics if enabled
	if a.Config.AnalyticsEnabled {
		analyticsStore, err := analytics.NewStore(a.Config.AnalyticsDatabasePath)
		if err != nil {
			return fmt.Errorf("pubengine: init analytics: %w", err)
		}
		a.analyticsStore = analyticsStore
		if err := analytics.InitSalt(analyticsStore); err != nil {
			return fmt.Errorf("pubengine: init analytics salt: %w", err)
		}
		stopCleanup := analyticsStore.StartCleanupScheduler(365, 24*time.Hour)
		defer stopCleanup()
	}

	// Setup middleware
	a.setupMiddleware()

	// Setup routes
	a.setupRoutes()

	// Apply custom routes
	for _, fn := range a.customRoutes {
		fn(a)
	}

	// Start server
	if err := a.Echo.Start(a.Config.Addr); err != nil && err != http.ErrServerClosed {
		return err
	}
	return nil
}

func (a *App) setupRoutes() {
	e := a.Echo

	// Serve embedded framework assets (htmx.min.js, analytics.js, dashboard.min.js)
	// These are served under /public/ and fall through to the user's static dir.
	embeddedFS, _ := fs.Sub(EmbeddedAssets, "embedded")
	embeddedHandler := http.FileServer(http.FS(embeddedFS))
	e.GET("/public/htmx.min.js", echo.WrapHandler(http.StripPrefix("/public/", embeddedHandler)))
	e.GET("/public/analytics.js", echo.WrapHandler(http.StripPrefix("/public/", embeddedHandler)))
	e.GET("/public/dashboard.min.js", echo.WrapHandler(http.StripPrefix("/public/", embeddedHandler)))

	// User's static assets
	e.Static("/public", a.staticDir)
	e.GET("/favicon.svg", a.handleFavicon)
	e.GET("/robots.txt", a.handleRobots)

	// Public routes
	e.GET("/sitemap.xml", a.handleSitemap)
	e.GET("/feed.xml", a.handleFeed)
	e.GET("/blog", handleBlogRedirect)
	e.GET("/", a.handleHome)
	e.GET("/blog/:slug/", a.handlePost)

	// Admin routes
	e.GET("/admin/", a.handleAdmin)
	e.POST("/admin/login/", a.handleAdminLogin)
	e.POST("/admin/logout/", handleAdminLogout)
	e.GET("/admin/post/:slug/", a.handleAdminPost)
	e.POST("/admin/save/", a.handleAdminSave)
	e.DELETE("/admin/post/:slug/", a.handleAdminDelete)
	e.GET("/admin/images/", a.handleImageList)
	e.POST("/admin/images/upload/", a.handleImageUpload)
	e.DELETE("/admin/images/:filename/", a.handleImageDelete)

	// Analytics routes
	if a.Config.AnalyticsEnabled && a.analyticsStore != nil {
		analyticsHandler := analytics.NewHandler(a.analyticsStore)
		analyticsAuthMiddleware := func(next echo.HandlerFunc) echo.HandlerFunc {
			return func(c echo.Context) error {
				if !IsAdmin(c) {
					return c.Redirect(http.StatusSeeOther, "/admin/")
				}
				return next(c)
			}
		}
		publicGroup := e.Group("")
		analyticsHandler.RegisterRoutes(e, publicGroup, analyticsAuthMiddleware)
		e.GET("/admin/analytics/", func(c echo.Context) error {
			if !IsAdmin(c) {
				return c.Redirect(http.StatusSeeOther, "/admin/")
			}
			return analyticsHandler.DashboardHTML(c)
		})
	}
}

// Close cleans up resources. Call this when the app is shutting down.
func (a *App) Close() error {
	if a.Store != nil {
		a.Store.Close()
	}
	if a.analyticsStore != nil {
		a.analyticsStore.Close()
	}
	return nil
}

// EnvOr returns the value of the environment variable key, or fallback if empty.
// This is a convenience function for use in scaffolded main.go files.
func EnvOr(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}

// MustEnv returns the value of the environment variable key, or fatally exits if empty.
func MustEnv(key string) string {
	v := os.Getenv(key)
	if v == "" {
		log.Fatalf("pubengine: required environment variable %s is not set", key)
	}
	return v
}
