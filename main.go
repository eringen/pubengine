// main.go — pubengine HTTP server
// Sets up Echo routes, middleware, and handlers for a blog publishing engine.
// All site branding comes from environment variables via SiteConfig.
package main

import (
	"database/sql"
	"encoding/xml"
	"fmt"
	"log"
	"net/http"
	"net/url"
	"os"
	"path"
	"strings"
	"sync"
	"time"

	"github.com/a-h/templ"
	"github.com/gorilla/sessions"
	"github.com/labstack/echo-contrib/session"
	"github.com/labstack/echo/v4"
	"github.com/labstack/echo/v4/middleware"
	_ "modernc.org/sqlite"

	"github.com/eringen/pubengine/views"
)

// app holds shared dependencies injected into every handler.
type app struct {
	cfg          views.SiteConfig
	store        *store
	cache        *postCache
	loginLimiter *loginLimiter
}

func main() {
	store, err := newStore(databasePath())
	if err != nil {
		log.Fatal(err)
	}
	defer store.Close()

	// Build site config once from env vars — templates read these values.
	cfg := views.SiteConfig{
		Name:        siteName(),
		URL:         siteURL(),
		Description: siteDescription(),
		Author:      siteAuthor(),
	}

	app := &app{
		cfg:          cfg,
		store:        store,
		cache:        newPostCache(store, 5*time.Minute),
		loginLimiter: newLoginLimiter(5, time.Minute),
	}

	e := echo.New()
	e.IPExtractor = echo.ExtractIPFromXFFHeader(
		echo.TrustLoopback(true),
		echo.TrustLinkLocal(false),
		echo.TrustPrivateNet(true),
	)

	// Custom error handler — renders styled 404/500 pages using SiteConfig.
	e.HTTPErrorHandler = func(err error, c echo.Context) {
		if c.Response().Committed {
			return
		}
		he, ok := err.(*echo.HTTPError)
		if ok && he.Code == http.StatusNotFound {
			_ = renderStatus(c, http.StatusNotFound, views.NotFound(cfg))
			return
		}
		code := http.StatusInternalServerError
		if ok {
			code = he.Code
		}
		if code >= 500 {
			c.Logger().Errorf("server error: %v", err)
			_ = renderStatus(c, code, views.ServerError(cfg))
			return
		}
		e.DefaultHTTPErrorHandler(err, c)
	}
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
	e.Use(middleware.SecureWithConfig(middleware.SecureConfig{
		XSSProtection:         "1; mode=block",
		ContentTypeNosniff:    "nosniff",
		XFrameOptions:         "DENY",
		ReferrerPolicy:        "strict-origin-when-cross-origin",
		ContentSecurityPolicy: "default-src 'self'; script-src 'self' 'unsafe-inline'; style-src 'self' 'unsafe-inline'; img-src 'self' https: data:; font-src 'self'; connect-src 'self'",
		HSTSMaxAge:            31536000,
		HSTSExcludeSubdomains: false,
	}))
	e.Use(session.Middleware(newSessionStore()))
	e.Use(middleware.CSRFWithConfig(middleware.CSRFConfig{
		ContextKey:  middleware.DefaultCSRFConfig.ContextKey,
		TokenLookup: "header:X-CSRF-Token,form:_csrf",
		CookieName:  "_csrf",
		CookiePath:  "/",
		CookieSameSite: func() http.SameSite {
			return http.SameSiteLaxMode
		}(),
		CookieSecure: cookieSecure(),
		ErrorHandler: func(err error, c echo.Context) error {
			return c.String(http.StatusForbidden, "Forbidden")
		},
	}))
	e.Use(middleware.AddTrailingSlashWithConfig(middleware.TrailingSlashConfig{
		RedirectCode: http.StatusMovedPermanently,
		Skipper: func(c echo.Context) bool {
			path := c.Request().URL.Path
			return strings.HasPrefix(path, "/public") || path == "/sitemap.xml" || path == "/feed.xml" || path == "/robots.txt"
		},
	}))

	e.Use(cacheControl)
	e.Static("/public", "public")
	e.GET("/favicon.svg", handleFavicon)
	e.GET("/robots.txt", app.handleRobots)
	e.GET("/sitemap.xml", app.handleSitemap)
	e.GET("/feed.xml", app.handleFeed)
	e.GET("/blog", handleBlogRedirect)
	e.GET("/", app.handleHome)
	e.GET("/blog/:slug/", app.handlePost)

	// Admin routes — password-protected dashboard for managing posts.
	e.GET("/admin/", app.handleAdmin)
	e.POST("/admin/login/", app.handleAdminLogin)
	e.POST("/admin/logout/", handleAdminLogout)
	e.GET("/admin/post/:slug/", app.handleAdminPost)
	e.POST("/admin/save/", app.handleAdminSave)
	e.DELETE("/admin/post/:slug/", app.handleAdminDelete)

	addr := ":3000"
	if err := e.Start(addr); err != nil && err != http.ErrServerClosed {
		log.Fatal(err)
	}
}

func handleFavicon(c echo.Context) error {
	return c.File("public/favicon.svg")
}

// handleRobots generates robots.txt dynamically using SITE_URL.
func (a *app) handleRobots(c echo.Context) error {
	body := fmt.Sprintf("User-agent: *\nAllow: /\nDisallow: /admin/\n\nSitemap: %s/sitemap.xml\n", a.cfg.URL)
	return c.String(http.StatusOK, body)
}

func handleBlogRedirect(c echo.Context) error {
	return c.Redirect(http.StatusMovedPermanently, "/")
}

func (a *app) handleSitemap(c echo.Context) error {
	posts, err := a.cache.ListPosts("")
	if err != nil {
		return err
	}
	return a.renderSitemap(c, posts)
}

func (a *app) handleFeed(c echo.Context) error {
	posts, err := a.cache.ListPosts("")
	if err != nil {
		return err
	}
	return a.renderRSS(c, posts)
}

// handleHome serves the blog listing page, with HTMX partial support.
func (a *app) handleHome(c echo.Context) error {
	tag := c.QueryParam("tag")
	posts, err := a.cache.ListPosts(tag)
	if err != nil {
		return err
	}
	tags, err := a.cache.ListTags()
	if err != nil {
		return err
	}
	if c.Request().Header.Get("HX-Request") == "true" {
		partial := c.QueryParam("partial")
		switch partial {
		case "blog":
			return render(c, views.BlogSectionPartial(posts, tag, tags))
		case "home":
			return render(c, views.HomePartial(a.cfg, posts, tag, tags))
		}
	}
	return render(c, views.Home(a.cfg, posts, tag, tags))
}

// handlePost serves a single blog post, with HTMX partial support.
func (a *app) handlePost(c echo.Context) error {
	slug := c.Param("slug")
	post, err := a.cache.GetPost(slug)
	if err != nil {
		if err == sql.ErrNoRows {
			return renderStatus(c, http.StatusNotFound, views.NotFound(a.cfg))
		}
		return err
	}
	posts, err := a.cache.ListPosts("")
	if err != nil {
		return err
	}
	if c.Request().Header.Get("HX-Request") == "true" && c.QueryParam("partial") == "post" {
		return render(c, views.PostPartial(a.cfg, post, posts))
	}
	return render(c, views.Post(a.cfg, post, posts))
}

func (a *app) handleAdmin(c echo.Context) error {
	if !isAdmin(c) {
		return render(c, views.AdminLogin(a.cfg, false, csrfToken(c)))
	}
	return a.renderAdminDashboard(c, c.QueryParam("msg"))
}

func (a *app) handleAdminPost(c echo.Context) error {
	if !isAdmin(c) {
		return c.Redirect(http.StatusSeeOther, "/admin/")
	}
	slug := c.Param("slug")
	post, err := a.store.GetPostAny(slug)
	if err != nil {
		if err == sql.ErrNoRows {
			return c.NoContent(http.StatusNotFound)
		}
		return err
	}
	return render(c, views.AdminFormPartial(post, csrfToken(c)))
}

func (a *app) handleAdminLogin(c echo.Context) error {
	if !a.loginLimiter.Allow(c.RealIP()) {
		return c.String(http.StatusTooManyRequests, "Too many login attempts. Try again later.")
	}
	pass := c.FormValue("password")
	if pass == adminPassword() {
		if err := setAdminSession(c); err != nil {
			return err
		}
		return c.Redirect(http.StatusSeeOther, "/admin/")
	}
	return render(c, views.AdminLogin(a.cfg, true, csrfToken(c)))
}

func handleAdminLogout(c echo.Context) error {
	if err := clearAdminSession(c); err != nil {
		return err
	}
	return c.Redirect(http.StatusSeeOther, "/admin/")
}

func (a *app) handleAdminSave(c echo.Context) error {
	if !isAdmin(c) {
		return c.Redirect(http.StatusSeeOther, "/admin/")
	}
	if err := c.Request().ParseForm(); err != nil {
		return err
	}
	title := strings.TrimSpace(c.FormValue("title"))
	slug := strings.TrimSpace(c.FormValue("slug"))
	if slug == "" {
		slug = slugify(title)
	}
	if slug == "" {
		return c.Redirect(http.StatusSeeOther, "/admin/?msg=Slug+is+required.+Add+a+title+or+slug.")
	}
	date := strings.TrimSpace(c.FormValue("date"))
	if date == "" {
		date = time.Now().Format("2006-01-02")
	}
	if _, err := time.Parse("2006-01-02", date); err != nil {
		return c.Redirect(http.StatusSeeOther, "/admin/?msg=Invalid+date+format.+Use+YYYY-MM-DD.")
	}
	tags := strings.Split(c.FormValue("tags"), ",")
	for i := range tags {
		tags[i] = strings.TrimSpace(tags[i])
	}
	tags = filterEmpty(tags)
	summary := c.FormValue("summary")
	content := c.FormValue("content")
	published := c.FormValue("published") != ""
	if err := a.store.SavePost(views.BlogPost{
		Slug:      slug,
		Title:     title,
		Date:      date,
		Tags:      tags,
		Summary:   summary,
		Content:   content,
		Published: published,
	}); err != nil {
		return err
	}
	a.cache.Invalidate()
	return a.renderAdminDashboard(c, "saved")
}

func (a *app) handleAdminDelete(c echo.Context) error {
	if !isAdmin(c) {
		return c.Redirect(http.StatusSeeOther, "/admin/")
	}
	slug := c.Param("slug")
	if err := a.store.DeletePost(slug); err != nil {
		return err
	}
	a.cache.Invalidate()
	return a.renderAdminDashboard(c, "deleted")
}

func (a *app) renderAdminDashboard(c echo.Context, msg string) error {
	posts, err := a.store.ListAllPosts()
	if err != nil {
		return err
	}
	return render(c, views.AdminDashboard(a.cfg, posts, msg, csrfToken(c)))
}

// render writes a templ component as an HTTP 200 HTML response.
func render(c echo.Context, cmp templ.Component) error {
	return renderStatus(c, http.StatusOK, cmp)
}

// renderStatus writes a templ component with a specific HTTP status code.
func renderStatus(c echo.Context, code int, cmp templ.Component) error {
	c.Response().Header().Set(echo.HeaderContentType, echo.MIMETextHTMLCharsetUTF8)
	c.Response().WriteHeader(code)
	return cmp.Render(c.Request().Context(), c.Response().Writer)
}

// cacheControl sets Cache-Control headers based on the request path.
func cacheControl(next echo.HandlerFunc) echo.HandlerFunc {
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

// slugify converts a title to a URL-safe slug.
func slugify(s string) string {
	s = strings.ToLower(strings.TrimSpace(s))
	var b strings.Builder
	prev := false
	for _, r := range s {
		switch {
		case r >= 'a' && r <= 'z', r >= '0' && r <= '9':
			b.WriteRune(r)
			prev = false
		default:
			if !prev && b.Len() > 0 {
				b.WriteByte('-')
				prev = true
			}
		}
	}
	return strings.TrimRight(b.String(), "-")
}

// --- Environment variable helpers ---

func databasePath() string {
	if v := os.Getenv("DATABASE_PATH"); v != "" {
		return v
	}
	return "data/blog.db"
}

func adminPassword() string {
	v := os.Getenv("ADMIN_PASSWORD")
	if v == "" {
		log.Fatal("ADMIN_PASSWORD environment variable is required")
	}
	return v
}

func siteName() string {
	if v := os.Getenv("SITE_NAME"); v != "" {
		return v
	}
	return "Blog"
}

func siteURL() string {
	if v := os.Getenv("SITE_URL"); v != "" {
		return strings.TrimSuffix(v, "/")
	}
	return "http://localhost:3000"
}

func siteDescription() string {
	if v := os.Getenv("SITE_DESCRIPTION"); v != "" {
		return v
	}
	return ""
}

func siteAuthor() string {
	if v := os.Getenv("SITE_AUTHOR"); v != "" {
		return v
	}
	return ""
}

// --- Session management ---

func isAdmin(c echo.Context) bool {
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

func filterEmpty(vals []string) []string {
	var out []string
	for _, v := range vals {
		if s := strings.TrimSpace(v); s != "" {
			out = append(out, s)
		}
	}
	return out
}

const sessionName = "admin_session"

func newSessionStore() *sessions.CookieStore {
	store := sessions.NewCookieStore([]byte(sessionSecret()))
	store.Options = &sessions.Options{
		Path:     "/",
		HttpOnly: true,
		MaxAge:   60 * 60 * 12,
		SameSite: http.SameSiteLaxMode,
		Secure:   cookieSecure(),
	}
	return store
}

func sessionSecret() string {
	v := os.Getenv("ADMIN_SESSION_SECRET")
	if v == "" {
		log.Fatal("ADMIN_SESSION_SECRET environment variable is required")
	}
	return v
}

func cookieSecure() bool {
	return strings.EqualFold(os.Getenv("COOKIE_SECURE"), "true")
}

func csrfToken(c echo.Context) string {
	token, _ := c.Get(middleware.DefaultCSRFConfig.ContextKey).(string)
	return token
}

// --- Rate limiter ---

// loginLimiter implements a simple per-IP rate limiter for admin login attempts.
type loginLimiter struct {
	mu       sync.Mutex
	attempts map[string][]time.Time
	max      int
	window   time.Duration
}

func newLoginLimiter(max int, window time.Duration) *loginLimiter {
	l := &loginLimiter{
		attempts: make(map[string][]time.Time),
		max:      max,
		window:   window,
	}
	go l.cleanup()
	return l
}

func (l *loginLimiter) cleanup() {
	ticker := time.NewTicker(l.window)
	for range ticker.C {
		cutoff := time.Now().Add(-l.window)
		l.mu.Lock()
		for ip, hits := range l.attempts {
			kept := hits[:0]
			for _, t := range hits {
				if t.After(cutoff) {
					kept = append(kept, t)
				}
			}
			if len(kept) == 0 {
				delete(l.attempts, ip)
			} else {
				l.attempts[ip] = kept
			}
		}
		l.mu.Unlock()
	}
}

// Allow returns true if the IP has not exceeded the rate limit within the window.
func (l *loginLimiter) Allow(ip string) bool {
	now := time.Now()
	cutoff := now.Add(-l.window)

	l.mu.Lock()
	defer l.mu.Unlock()

	hits := l.attempts[ip]
	kept := hits[:0]
	for _, t := range hits {
		if t.After(cutoff) {
			kept = append(kept, t)
		}
	}
	if len(kept) >= l.max {
		l.attempts[ip] = kept
		return false
	}
	kept = append(kept, now)
	l.attempts[ip] = kept
	return true
}

// --- Sitemap ---

type sitemapURLSet struct {
	XMLName xml.Name     `xml:"urlset"`
	XMLNS   string       `xml:"xmlns,attr"`
	URLs    []sitemapURL `xml:"url"`
}

type sitemapURL struct {
	Loc     string `xml:"loc"`
	LastMod string `xml:"lastmod,omitempty"`
}

func (a *app) renderSitemap(c echo.Context, posts []views.BlogPost) error {
	base := a.cfg.URL
	urls := []sitemapURL{
		{Loc: buildURL(base)},
	}
	for _, p := range posts {
		urls = append(urls, sitemapURL{
			Loc:     buildURL(base, "blog", p.Slug),
			LastMod: p.Date,
		})
	}
	sitemap := sitemapURLSet{
		XMLNS: "http://www.sitemaps.org/schemas/sitemap/0.9",
		URLs:  urls,
	}
	c.Response().Header().Set(echo.HeaderContentType, "application/xml; charset=utf-8")
	c.Response().WriteHeader(http.StatusOK)
	c.Response().Write([]byte(xml.Header))
	return xml.NewEncoder(c.Response()).Encode(sitemap)
}

func buildURL(base string, pathSegments ...string) string {
	u, err := url.Parse(base)
	if err != nil {
		return base
	}
	u.Path = path.Join(u.Path, path.Join(pathSegments...))
	if len(pathSegments) > 0 && !strings.HasSuffix(u.Path, "/") {
		u.Path += "/"
	}
	return u.String()
}

// --- RSS feed ---

type rss struct {
	XMLName xml.Name   `xml:"rss"`
	Version string     `xml:"version,attr"`
	Channel rssChannel `xml:"channel"`
}

type rssChannel struct {
	Title       string    `xml:"title"`
	Link        string    `xml:"link"`
	Description string    `xml:"description"`
	Items       []rssItem `xml:"item"`
}

type rssItem struct {
	Title       string `xml:"title"`
	Link        string `xml:"link"`
	Description string `xml:"description"`
	PubDate     string `xml:"pubDate"`
	GUID        string `xml:"guid"`
}

func (a *app) renderRSS(c echo.Context, posts []views.BlogPost) error {
	base := a.cfg.URL
	items := make([]rssItem, 0, len(posts))
	for _, p := range posts {
		pubDate := ""
		if t, err := time.Parse("2006-01-02", p.Date); err == nil {
			pubDate = t.Format(time.RFC1123Z)
		}
		postURL := buildURL(base, "blog", p.Slug)
		items = append(items, rssItem{
			Title:       p.Title,
			Link:        postURL,
			Description: p.Summary,
			PubDate:     pubDate,
			GUID:        postURL,
		})
	}
	feed := rss{
		Version: "2.0",
		Channel: rssChannel{
			Title:       a.cfg.Name,
			Link:        base,
			Description: a.cfg.Description,
			Items:       items,
		},
	}
	c.Response().Header().Set(echo.HeaderContentType, "application/rss+xml; charset=utf-8")
	c.Response().WriteHeader(http.StatusOK)
	c.Response().Write([]byte(xml.Header))
	return xml.NewEncoder(c.Response()).Encode(feed)
}
