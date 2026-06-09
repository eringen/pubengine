package pubengine

import (
	"bytes"
	"io"
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

	e.Pre(cacheControlMiddleware, requestBodyLimits, middleware.NonWWWRedirect())

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
		ContentSecurityPolicy: "default-src 'self'; script-src 'self' 'unsafe-inline' 'wasm-unsafe-eval' https://nanolytica.org https://www.googletagmanager.com blob:; style-src 'self' 'unsafe-inline'; img-src 'self' https: data:; font-src 'self'; connect-src 'self' data: blob: https://nanolytica.org https://www.google-analytics.com https://www.googletagmanager.com; worker-src 'self' blob:; media-src 'self' data:",
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
			path := c.Request().URL.Path
			return strings.HasPrefix(path, "/api/analytics/") ||
				path == "/admin/auth/google/callback"
		},
		ErrorHandler: func(err error, c echo.Context) error {
			return c.String(http.StatusForbidden, "Forbidden")
		},
	}))

	e.Pre(middleware.AddTrailingSlashWithConfig(middleware.TrailingSlashConfig{
		RedirectCode: http.StatusPermanentRedirect,
		Skipper: func(c echo.Context) bool {
			path := c.Request().URL.Path
			return strings.HasPrefix(path, "/public") ||
				strings.HasPrefix(path, "/workbench") ||
				strings.HasPrefix(path, "/api/") ||
				strings.HasPrefix(path, "/admin/analytics/api/") ||
				strings.HasPrefix(path, "/admin/analytics/fragments/") ||
				path == "/admin/auth/google/callback" ||
				path == "/favicon.svg" || path == "/sitemap.xml" || path == "/feed.xml" || path == "/robots.txt" ||
				path == "/llms.txt"
		},
	}))

}

func cacheControlMiddleware(next echo.HandlerFunc) echo.HandlerFunc {
	return func(c echo.Context) error {
		c.Response().Before(func() {
			path := c.Request().URL.Path
			status := c.Response().Status
			switch {
			case status >= 400, c.Request().Method != http.MethodGet && c.Request().Method != http.MethodHead,
				strings.HasPrefix(path, "/admin"), strings.HasPrefix(path, "/api/"):
				c.Response().Header().Set("Cache-Control", "no-store")
			case path == "/llms.txt":
				c.Response().Header().Set("Cache-Control", "public, max-age=86400")
			default:
				// Stable asset URLs and published content must revalidate after changes.
				c.Response().Header().Set("Cache-Control", "public, max-age=0, must-revalidate")
			}
		})
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
	// session.Get always returns a usable session even when the existing
	// cookie can't be decoded (e.g. secret changed). Ignore the decode error.
	sess, _ := session.Get(sessionName, c)
	sess.Values["authenticated"] = true
	return sess.Save(c.Request(), c.Response())
}

func clearAdminSession(c echo.Context) error {
	sess, _ := session.Get(sessionName, c)
	sess.Options.MaxAge = -1
	return sess.Save(c.Request(), c.Response())
}

// CsrfToken extracts the CSRF token from the Echo context.
func CsrfToken(c echo.Context) string {
	token, _ := c.Get(middleware.DefaultCSRFConfig.ContextKey).(string)
	return token
}

// Read bounded framework bodies before CSRF can parse forms or multipart data.
func requestBodyLimits(next echo.HandlerFunc) echo.HandlerFunc {
	return func(c echo.Context) error {
		req := c.Request()
		if req.Body == nil || req.Body == http.NoBody {
			return next(c)
		}
		path := strings.TrimSuffix(req.URL.Path, "/")
		var limit int64
		switch {
		case path == "/admin/images/upload":
			limit = maxUploadSize + (1 << 20)
		case path == "/admin/login":
			limit = 16 << 10
		case strings.HasPrefix(path, "/admin"):
			limit = 2 << 20
		case path == "/api/analytics/collect":
			limit = 16 << 10
		default:
			return next(c)
		}
		if req.ContentLength > limit {
			return echo.ErrStatusRequestEntityTooLarge
		}
		body, err := io.ReadAll(io.LimitReader(req.Body, limit+1))
		req.Body.Close()
		if err != nil {
			return echo.NewHTTPError(http.StatusBadRequest, "Invalid request body").SetInternal(err)
		}
		if int64(len(body)) > limit {
			return echo.ErrStatusRequestEntityTooLarge
		}
		req.Body = io.NopCloser(bytes.NewReader(body))
		return next(c)
	}
}
