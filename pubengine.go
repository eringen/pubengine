// Package pubengine is a blog publishing engine built with Go, Echo, and templ.
// It provides blog CRUD, admin dashboard, analytics, RSS, and sitemap out of the box.
//
// Users provide their own templ templates via the ViewFuncs struct,
// and pubengine handles all the handler logic, middleware, and database operations.
package pubengine

import (
	"context"
	"errors"
	"fmt"
	"io/fs"
	"log"
	"net"
	"net/http"
	"os"
	"sync"
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
	AdminLogin       func(errorMsg string, csrfToken string, googleLoginURL string) templ.Component
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

	loginLimiter     *LoginLimiter
	analyticsStore   *analytics.Store
	customRoutes     []func(*App)
	staticDir        string
	imageMu          sync.Mutex
	lifecycleMu      sync.Mutex
	started          bool
	closed           bool
	closeOnce        sync.Once
	closeErr         error
	analyticsHandler *analytics.Handler
	stopCleanup      func()
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

// Start serves until Close or Shutdown is called.
func (a *App) Start() error { return a.StartContext(context.Background()) }

// StartContext initializes the app and shuts it down when ctx is canceled.
// An App may be started once; create a new App after shutdown.
func (a *App) StartContext(ctx context.Context) (err error) {
	a.lifecycleMu.Lock()
	if a.started || a.closed {
		a.lifecycleMu.Unlock()
		return fmt.Errorf("pubengine: app already started or closed")
	}
	a.started = true
	listener, err := a.initialize(ctx)
	a.lifecycleMu.Unlock()
	defer func() { err = errors.Join(err, a.Close()) }()
	if err != nil {
		return err
	}
	done := make(chan struct{})
	defer close(done)
	go func() {
		select {
		case <-ctx.Done():
			_ = a.Close()
		case <-done:
		}
	}()
	err = a.Echo.Server.Serve(listener)
	if errors.Is(err, http.ErrServerClosed) {
		return nil
	}
	return err
}

func (a *App) initialize(ctx context.Context) (net.Listener, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	if err := a.Config.validate(); err != nil {
		return nil, err
	}
	store, err := NewStore(a.Config.DatabasePath)
	if err != nil {
		return nil, fmt.Errorf("pubengine: init store: %w", err)
	}
	a.Store = store
	a.Cache = NewPostCache(store, a.Config.PostCacheTTL)
	a.loginLimiter = NewLoginLimiter(5, time.Minute)
	if a.Config.AnalyticsEnabled {
		a.analyticsStore, err = analytics.NewStore(a.Config.AnalyticsDatabasePath)
		if err != nil {
			return nil, fmt.Errorf("pubengine: init analytics: %w", err)
		}
		a.stopCleanup = a.analyticsStore.StartCleanupScheduler(365, 24*time.Hour)
	}
	a.setupMiddleware()
	a.setupRoutes()
	for _, fn := range a.customRoutes {
		fn(a)
	}
	a.Echo.Server.Addr = a.Config.Addr
	a.Echo.Server.Handler = a.Echo
	a.Echo.Server.ReadHeaderTimeout = a.Config.ReadHeaderTimeout
	a.Echo.Server.ReadTimeout = a.Config.ReadTimeout
	a.Echo.Server.WriteTimeout = a.Config.WriteTimeout
	a.Echo.Server.IdleTimeout = a.Config.IdleTimeout
	listener, err := net.Listen("tcp", a.Config.Addr)
	if err != nil {
		return nil, err
	}
	a.Echo.Listener = listener
	return listener, nil
}

func (a *App) setupRoutes() {
	e := a.Echo

	// Serve embedded framework assets (talkdom.js, analytics.js, dashboard.min.js)
	// These are served under /public/ and fall through to the user's static dir.
	embeddedFS, _ := fs.Sub(EmbeddedAssets, "embedded")
	embeddedHandler := http.FileServer(http.FS(embeddedFS))
	e.GET("/public/talkdom.js", echo.WrapHandler(http.StripPrefix("/public/", embeddedHandler)))
	e.GET("/public/analytics.js", echo.WrapHandler(http.StripPrefix("/public/", embeddedHandler)))
	e.GET("/public/dashboard.min.js", echo.WrapHandler(http.StripPrefix("/public/", embeddedHandler)))
	e.GET("/public/admin.css", echo.WrapHandler(http.StripPrefix("/public/", embeddedHandler)))

	// User's static assets
	e.Static("/public", a.staticDir)
	e.GET("/favicon.svg", a.handleFavicon)
	e.GET("/robots.txt", a.handleRobots)

	// Public routes
	e.GET("/sitemap.xml", a.handleSitemap)
	e.GET("/feed.xml", a.handleFeed)
	e.GET("/blog/", handleBlogRedirect)
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

	// Google OAuth routes
	if a.Config.GoogleAuthEnabled() {
		e.GET("/admin/auth/google/", a.handleGoogleLogin)
		e.GET("/admin/auth/google/callback", a.handleGoogleCallback)
	}

	// Analytics routes
	if a.Config.AnalyticsEnabled && a.analyticsStore != nil {
		analyticsHandler := analytics.NewHandler(a.analyticsStore)
		a.analyticsHandler = analyticsHandler
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

// Shutdown drains HTTP requests before stopping workers and closing databases.
// When the deadline expires, remaining connections are closed forcibly.
func (a *App) Shutdown(ctx context.Context) error {
	a.closeOnce.Do(func() {
		a.lifecycleMu.Lock()
		a.closed = true
		a.lifecycleMu.Unlock()
		err := a.Echo.Shutdown(ctx)
		if err != nil {
			err = errors.Join(err, a.Echo.Close())
		}
		// Shutdown can precede Serve registering the listener.
		if a.Echo.Listener != nil {
			closeErr := a.Echo.Listener.Close()
			if closeErr != nil && !errors.Is(closeErr, net.ErrClosed) {
				err = errors.Join(err, closeErr)
			}
		}
		if a.stopCleanup != nil {
			a.stopCleanup()
		}
		if a.analyticsHandler != nil {
			a.analyticsHandler.Close()
		}
		if a.loginLimiter != nil {
			a.loginLimiter.Close()
		}
		if a.Store != nil {
			err = errors.Join(err, a.Store.Close())
		}
		if a.analyticsStore != nil {
			err = errors.Join(err, a.analyticsStore.Close())
		}
		a.closeErr = err
	})
	return a.closeErr
}

// Close shuts down the app using its configured grace period. It is idempotent.
func (a *App) Close() error {
	ctx, cancel := context.WithTimeout(context.Background(), a.Config.ShutdownTimeout)
	defer cancel()
	return a.Shutdown(ctx)
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
