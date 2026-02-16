package pubengine

import (
	"net/http"
	"strings"

	"github.com/gorilla/sessions"
	"github.com/labstack/echo-contrib/session"
	"github.com/labstack/echo/v4"
	"github.com/labstack/echo/v4/middleware"
)

const sessionName = "admin_session"

func (a *App) setupMiddleware() {
	e := a.Echo

	e.IPExtractor = echo.ExtractIPFromXFFHeader(
		echo.TrustLoopback(true),
		echo.TrustLinkLocal(false),
		echo.TrustPrivateNet(true),
	)

	e.HTTPErrorHandler = a.httpErrorHandler

	e.Pre(middleware.NonWWWRedirect())

	e.Use(middleware.RequestLoggerWithConfig(middleware.RequestLoggerConfig{
		LogStatus:  true,
		LogURI:     true,
		LogMethod:  true,
		LogLatency: true,
		LogValuesFunc: func(c echo.Context, v middleware.RequestLoggerValues) error {
			c.Logger().Infof("%s %s -> %d (%s)", v.Method, v.URI, v.Status, v.Latency)
			return nil
		},
	}))

	e.Use(middleware.Recover())

	e.Use(middleware.GzipWithConfig(middleware.GzipConfig{
		Level: 5,
		Skipper: func(c echo.Context) bool {
			return strings.HasPrefix(c.Request().URL.Path, "/public/")
		},
	}))

	e.Use(middleware.SecureWithConfig(middleware.SecureConfig{
		XSSProtection:         "1; mode=block",
		ContentTypeNosniff:    "nosniff",
		XFrameOptions:         "DENY",
		ReferrerPolicy:        "strict-origin-when-cross-origin",
		ContentSecurityPolicy: "default-src 'self'; script-src 'self' 'unsafe-inline' 'wasm-unsafe-eval' blob:; style-src 'self' 'unsafe-inline'; img-src 'self' https: data:; font-src 'self'; connect-src 'self' data: blob:; worker-src 'self' blob:; media-src 'self' data:",
		HSTSMaxAge:            31536000,
		HSTSExcludeSubdomains: false,
	}))

	e.Use(session.Middleware(a.newSessionStore()))

	e.Use(middleware.CSRFWithConfig(middleware.CSRFConfig{
		ContextKey:  middleware.DefaultCSRFConfig.ContextKey,
		TokenLookup: "header:X-CSRF-Token,form:_csrf",
		CookieName:  "_csrf",
		CookiePath:  "/",
		CookieSameSite: func() http.SameSite {
			return http.SameSiteLaxMode
		}(),
		CookieSecure: a.Config.CookieSecure,
		Skipper: func(c echo.Context) bool {
			return strings.HasPrefix(c.Request().URL.Path, "/api/analytics/")
		},
		ErrorHandler: func(err error, c echo.Context) error {
			return c.String(http.StatusForbidden, "Forbidden")
		},
	}))

	e.Use(middleware.AddTrailingSlashWithConfig(middleware.TrailingSlashConfig{
		RedirectCode: http.StatusMovedPermanently,
		Skipper: func(c echo.Context) bool {
			path := c.Request().URL.Path
			return strings.HasPrefix(path, "/public") ||
				strings.HasPrefix(path, "/workbench") ||
				strings.HasPrefix(path, "/api/") ||
				strings.HasPrefix(path, "/admin/analytics/api/") ||
				strings.HasPrefix(path, "/admin/analytics/fragments/") ||
				path == "/sitemap.xml" || path == "/feed.xml" || path == "/robots.txt"
		},
	}))

	e.Use(cacheControlMiddleware)
}

func cacheControlMiddleware(next echo.HandlerFunc) echo.HandlerFunc {
	return func(c echo.Context) error {
		path := c.Request().URL.Path
		switch {
		case strings.HasPrefix(path, "/public/"):
			c.Response().Header().Set("Cache-Control", "public, max-age=31536000, immutable")
		case path == "/sitemap.xml" || path == "/feed.xml" || path == "/robots.txt":
			c.Response().Header().Set("Cache-Control", "public, max-age=86400")
		case strings.HasPrefix(path, "/admin"):
			c.Response().Header().Set("Cache-Control", "no-store")
		default:
			c.Response().Header().Set("Cache-Control", "public, max-age=3600")
		}
		return next(c)
	}
}

func (a *App) newSessionStore() *sessions.CookieStore {
	store := sessions.NewCookieStore([]byte(a.Config.SessionSecret))
	store.Options = &sessions.Options{
		Path:     "/",
		HttpOnly: true,
		MaxAge:   60 * 60 * 12,
		SameSite: http.SameSiteLaxMode,
		Secure:   a.Config.CookieSecure,
	}
	return store
}

// IsAdmin checks if the current session is authenticated.
func IsAdmin(c echo.Context) bool {
	sess, err := session.Get(sessionName, c)
	if err != nil {
		return false
	}
	auth, ok := sess.Values["authenticated"].(bool)
	return ok && auth
}

func setAdminSession(c echo.Context) error {
	sess, err := session.Get(sessionName, c)
	if err != nil {
		return err
	}
	sess.Values["authenticated"] = true
	return sess.Save(c.Request(), c.Response())
}

func clearAdminSession(c echo.Context) error {
	sess, err := session.Get(sessionName, c)
	if err != nil {
		return err
	}
	sess.Options.MaxAge = -1
	return sess.Save(c.Request(), c.Response())
}

// CsrfToken extracts the CSRF token from the Echo context.
func CsrfToken(c echo.Context) string {
	token, _ := c.Get(middleware.DefaultCSRFConfig.ContextKey).(string)
	return token
}
