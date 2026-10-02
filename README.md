# pubengine

A Go blog publishing framework. Ships blog CRUD, admin dashboard, privacy-first analytics, RSS, and sitemap out of the box. You own the templates, pubengine handles everything else.

Built with [Echo](https://echo.labstack.com/), [templ](https://templ.guide/), [talkDOM](https://github.com/eringen/talkdom), [Tailwind CSS](https://tailwindcss.com/), and [SQLite](https://sqlite.org/).

## How it works

pubengine is a Go module, not a standalone app. You import it, provide your own templ templates via a `ViewFuncs` struct, and pubengine wires up all the handlers, middleware, database, caching, and analytics. Think of it like Django for Go blogs: convention over configuration with full template ownership.

```
+-----------------+       +-------------------+
|  Your Project   |       |    pubengine      |
|                 |       |                   |
|  main.go        |------>|  Handlers         |
|  views/*.templ  |       |  Middleware       |
|  assets/        |       |  Store (SQLite)   |
|  src/           |       |  Cache            |
|  public/        |       |  Analytics        |
|                 |       |  RSS / Sitemap    |
|  ViewFuncs{     |       |  Rate Limiter     |
|    Home: ...,   |       |  Session / CSRF   |
|    Post: ...,   |       |  Markdown         |
|  }              |       |  Image Library    |
+-----------------+       +-------------------+
```

## Quick start

### Install the CLI

```bash
go install github.com/eringen/pubengine/cmd/pubengine@latest
```

### Scaffold a new project

```bash
pubengine new github.com/yourname/myblog
cd myblog
```

This generates a complete project:

```
myblog/
├── main.go               # ~40 lines: config + ViewFuncs wiring
├── go.mod
├── views/
│   ├── home.templ        # Home page with blog listing
│   ├── post.templ        # Single post with related posts
│   ├── admin.templ       # Admin login + dashboard + editor
│   ├── nav.templ         # Head, Nav, Footer
│   ├── notfound.templ    # 404 page
│   ├── servererror.templ # 500 page
│   ├── config.go         # Runtime settings bound to views
│   └── helpers.go        # Type aliases for BlogPost, PageMeta
├── assets/
│   └── tailwind.css      # Tailwind directives
├── src/
│   └── app.js            # Custom JavaScript entry point
├── public/
│   ├── robots.txt
│   └── favicon.svg
├── data/                 # SQLite databases (auto created)
├── Makefile
├── package.json
├── tailwind.config.js
├── .env                 # Private generated credentials (ignored by Git)
└── .env.example         # Shareable settings with blank credentials
```

### Run it

```bash
go mod tidy
npm install
make run
# Unique admin credentials are in the generated .env.
```

Your blog is running at `http://localhost:3000`. Admin dashboard at `/admin/`.

## Upgrading existing sites

New projects pin PubEngine v0.5.0. To update an existing blog's runtime and embedded assets, run these commands inside its project:

```bash
go get github.com/eringen/pubengine@v0.5.0
go mod tidy
```

Update the site's templ generator to v0.3.1020, then rebuild and deploy its binary. Template, CSS, and custom JavaScript files belong to your blog and are not overwritten by a module update. To adopt the new typography, responsive layout, and navigation feedback, merge the updated scaffold files into your site and rebuild its assets.

The updated build libraries require Go 1.26 and Node.js 24.15+. Scaffolds use Tailwind CSS/CLI 4.3.3, typography 0.5.20, and esbuild 0.28.2. Tailwind 4 targets Safari 16.4+, Chrome 111+, and Firefox 128+. Existing sites migrating from Tailwind 3 should copy the updated CSS imports, CLI dependency, source directives, and config together; updating only the version will not build the styles.
The correctness fixes change a few integration points:

- Bind scaffold views with `views.New(cfg)` so runtime name, description, author, and analytics settings reach every page. Generated sites read `ANALYTICS_DATABASE_PATH` and `ANALYTICS_ENABLED` (default `true`).
- Set `PageSize: 20` to enable summary listings, pagination, and at most six related posts. Read page links with `PaginationFromContext(ctx)`. Zero preserves the existing full-content callback contract. `BlogPost.Link` retains its legacy format; use `PostPath(post.Slug)` for canonical links.
- Full-page editor errors use the optional `AdminEditor` callback; without it, the framework wraps `AdminFormPartial` in a basic document. Saves redirect with HTTP 303 and post deletion returns 204. Fetch-based image controls must check response status and authentication redirects before replacing their panel.
- Scaffold `HomePartial` and `PostPartial` now return complete documents. Updated talkDOM extracts the content and synchronizes title, canonical/OpenGraph metadata, and JSON-LD. Update custom navigation views to follow this contract when their metadata changes.
- Set a unique admin password and a random `SessionSecret` of at least 32 bytes. Known scaffold placeholders are rejected. Session cookies are **signed**, not encrypted. Rotating the signing key logs out existing sessions.
- New scaffolds create a private, ignored `.env` with unique credentials and a shareable `.env.example` with blank credential fields. Keep `.env` private; do not replace it with the blank example.
- `SavePost` now creates when `Revision == 0`. To edit, fetch the post with `GetPostAny`, modify it, and save its `OriginalSlug` and `Revision` unchanged. Fetch again after a successful save. Duplicate slugs and stale revisions return `ErrPostConflict`. Renames retain redirects to published destinations; previous slugs stay reserved until the post is deleted.
- Existing admin templates must submit hidden `original_slug` and `revision` fields and display `post.Error`. Copy the updated `AdminFormPartial` scaffold if needed. Validation failures return the submitted content; conflicts return HTTP 409.
- Deploy the updated analytics client and change its script URL to `/public/analytics.js?v=2`. The collector requires `event` (`view` or `duration`) and a `page_view_id`; older ambiguous beacons are rejected. If calling the analytics Go API directly, use `store.HashIP` and `store.GenerateVisitorID` so hashing is scoped to the installation.
- Replace query-value uses of `PathEscape` with `QueryEscape`. Update the scaffold's `JsonLD` component to use `templ.JSONScript` and include your custom `/public/app.min.js` bundle.
- Prefer `StartContext(ctx)` with a signal-aware context. `Shutdown(ctx)` drains requests; `Close()` uses the configured grace period. Both stop background workers and close databases. Create a new `App` after shutdown. HTTP timeouts are configurable through `SiteConfig`.
- For explicit proxy trust, pass `WithIPExtractor(...)`, for example `echo.ExtractIPDirect()` without a proxy, or an Echo extractor configured for your proxy CIDRs.

Database upgrades run automatically. Back up both databases and uploaded files before upgrading. The new schemas add post revisions/redirects and analytics page-view IDs; do not run older binaries against an upgraded analytics schema. Framework assets and public pages now revalidate instead of retaining long-lived cached copies. Copies already cached under the former policy cannot be recalled; version asset URLs when deploying this upgrade. `/llms.txt` retains a one-day cache policy.

## Usage

### The main.go pattern

Every pubengine site follows the same structure:

```go
package main

import (
    "log"

    "github.com/eringen/pubengine"
    "myblog/views"
)

func main() {
    cfg := pubengine.SiteConfig{
        Name:          pubengine.EnvOr("SITE_NAME", "My Blog"),
        URL:           pubengine.EnvOr("SITE_URL", "http://localhost:3000"),
        Description:   pubengine.EnvOr("SITE_DESCRIPTION", "A blog about things"),
        Author:        pubengine.EnvOr("SITE_AUTHOR", "Your Name"),
        Addr:          pubengine.EnvOr("ADDR", ":3000"),
        DatabasePath:  pubengine.EnvOr("DATABASE_PATH", "data/blog.db"),
        AdminPassword: pubengine.MustEnv("ADMIN_PASSWORD"),
        SessionSecret: pubengine.MustEnv("ADMIN_SESSION_SECRET"),
        CookieSecure:  pubengine.EnvOr("COOKIE_SECURE", "") == "true",
        PageSize:      20,
        AnalyticsEnabled: pubengine.EnvOr("ANALYTICS_ENABLED", "true") == "true",
        AnalyticsDatabasePath: pubengine.EnvOr("ANALYTICS_DATABASE_PATH", "data/analytics.db"),
    }
    app := pubengine.New(cfg, views.New(cfg))
    defer app.Close()

    if err := app.Start(); err != nil {
        log.Fatal(err)
    }
}
```

### ViewFuncs

This is the core inversion of control mechanism. You provide templ components, pubengine calls them from its handlers:

```go
type ViewFuncs struct {
    // Full page renders (initial page load)
    Home             func(posts []BlogPost, activeTag string, tags []string, siteURL string) templ.Component
    Post             func(post BlogPost, posts []BlogPost, siteURL string) templ.Component

    // talkDOM partial renders (SPA like navigation)
    HomePartial      func(posts []BlogPost, activeTag string, tags []string, siteURL string) templ.Component
    BlogSection      func(posts []BlogPost, activeTag string, tags []string) templ.Component
    PostPartial      func(post BlogPost, posts []BlogPost, siteURL string) templ.Component

    // Admin pages
    AdminLogin       func(errorMsg string, csrfToken string, googleLoginURL string) templ.Component
    AdminDashboard   func(posts []BlogPost, message string, csrfToken string) templ.Component
    AdminFormPartial func(post BlogPost, csrfToken string) templ.Component
    AdminEditor      func(post BlogPost, csrfToken string) templ.Component
    AdminImages      func(images []Image, csrfToken string) templ.Component

    // Error pages
    NotFound         func() templ.Component
    ServerError      func() templ.Component
}
```

The framework selects partial renders using the `partial` query parameter in the scaffold’s talkDOM requests.

`HomePartial` and `PostPartial` default to their full-page callbacks. Missing error views have basic built-in pages. Other missing callbacks return a render error and HTTP 500 instead of panicking. `AdminEditor` is optional as described above. Scaffold `views.New(cfg)` binds runtime settings without changing callback signatures.

### SiteConfig

All configuration in one struct:

| Field | Type | Default | Description |
|---|---|---|---|
| `Name` | `string` | `"Blog"` | Site name for nav, footer, RSS, JSON-LD |
| `URL` | `string` | `"http://localhost:3000"` | Canonical URL for sitemap, RSS, OpenGraph |
| `Description` | `string` | `""` | Site description for RSS and meta tags |
| `Author` | `string` | `""` | Author name for JSON-LD structured data |
| `Addr` | `string` | `":3000"` | Server listen address |
| `DatabasePath` | `string` | `"data/blog.db"` | SQLite database path |
| `AnalyticsEnabled` | `bool` | `false` | Enable built in analytics |
| `AnalyticsDatabasePath` | `string` | `"data/analytics.db"` | Analytics SQLite path |
| `AdminPassword` | `string` | **required** | Admin login password |
| `SessionSecret` | `string` | **required** | Session cookie signing key (minimum 32 bytes) |
| `CookieSecure` | `bool` | `false` | Set `true` when behind HTTPS |
| `GoogleClientID` | `string` | `""` | Google OAuth client ID (optional) |
| `GoogleClientSecret` | `string` | `""` | Google OAuth client secret (optional) |
| `GoogleAdminEmail` | `string` | `""` | Allowed Google email for admin login (optional) |
| `PostCacheTTL` | `time.Duration` | `5m` | In memory post cache TTL |
| `PageSize` | `int` | `0` | 1–200 enables paginated summaries; scaffold uses 20 |
| `MaxConcurrentUploads` | `int` | `2` | 1–8 simultaneous uploads; excess requests receive 503 and Retry-After |
| `ReadHeaderTimeout` | `time.Duration` | `5s` | Limit for reading HTTP headers |
| `ReadTimeout` | `time.Duration` | `30s` | Limit for reading an HTTP request |
| `WriteTimeout` | `time.Duration` | `30s` | Limit for writing an HTTP response |
| `IdleTimeout` | `time.Duration` | `1m` | Keep-alive idle timeout |
| `ShutdownTimeout` | `time.Duration` | `10s` | Grace period used by `Close` |

### Options

Configure additional behavior with option functions:

```go
// Add custom routes (runs after pubengine's routes)
pubengine.WithCustomRoutes(func(a *pubengine.App) {
    a.Echo.GET("/about/", handleAbout)
    a.Echo.Static("/portfolio", "portfolio")
})

// Change the static assets directory (default: "public")
pubengine.WithStaticDir("static")
```

### Accessing the App

The `App` struct exposes the underlying components for advanced use:

```go
app := pubengine.New(cfg, views)
// Store and Cache are initialized when Start or StartContext begins.

app.Config    // SiteConfig
app.Echo      // *echo.Echo, the HTTP server
app.Store     // *Store, SQLite operations
app.Cache     // *PostCache, in memory cache
app.Views     // ViewFuncs
```

## Core types

### BlogPost

```go
type BlogPost struct {
    Title     string
    Date      string     // "2024-01-15" format
    Tags      []string
    Summary   string
    Link      string     // "/blog/my-post" (auto generated)
    Slug      string     // "my-post"
    Content   string     // Markdown source
    Published bool
    OriginalSlug string // Loaded identity for edits
    Revision int64      // Concurrency token; zero for creates
    Error string        // Editor error, not persisted
}
```

### PageMeta

```go
type PageMeta struct {
    Title       string   // Page title and og:title
    Description string   // Meta description and og:description
    URL         string   // Canonical URL and og:url
    OGType      string   // "website" or "article"
}
```

## Routes

pubengine registers these routes automatically:

### Public

| Method | Path | Description |
|---|---|---|
| `GET` | `/` | Home page with blog listing |
| `GET` | `/blog/:slug/` | Single blog post |
| `GET` | `/feed.xml` | RSS feed |
| `GET` | `/sitemap.xml` | XML sitemap |
| `GET` | `/robots.txt` | Robots.txt (from static dir) |
| `GET` | `/favicon.svg` | Favicon (from static dir) |
| `GET` | `/public/*` | Static assets |

### Admin

| Method | Path | Description |
|---|---|---|
| `GET` | `/admin/` | Login page or dashboard |
| `POST` | `/admin/login/` | Process login |
| `POST` | `/admin/logout/` | Logout |
| `GET` | `/admin/post/:slug/` | Edit post form (talkDOM) |
| `POST` | `/admin/save/` | Create or update post |
| `DELETE` | `/admin/post/:slug/` | Delete post |
| `GET` | `/admin/images/` | Image library (talkDOM) |
| `POST` | `/admin/images/upload/` | Upload image |
| `DELETE` | `/admin/images/:filename/` | Delete image |

### Analytics (when enabled)

| Method | Path | Description |
|---|---|---|
| `POST` | `/api/analytics/collect` | Track page view |
| `GET` | `/admin/analytics/` | Analytics dashboard |
| `GET` | `/admin/analytics/api/stats` | Stats JSON |
| `GET` | `/admin/analytics/fragments/stats` | Stats HTML fragment |
| `GET` | `/admin/analytics/api/bot-stats` | Bot stats JSON |
| `GET` | `/admin/analytics/fragments/bot-stats` | Bot stats HTML fragment |

## Helper functions

pubengine exports utility functions for use in your templates:

```go
// URL and path helpers
pubengine.BuildURL(base, "blog", slug)     // "https://example.com/blog/my-post/"
pubengine.PostPath(slug)                   // "/blog/my-post/"
pubengine.PathEscape(tag)                   // URL safe tag encoding
pubengine.QueryEscape(tag)                  // Query parameter encoding
pubengine.Slugify("My Post Title")          // "my-post-title"

// Tag helpers
pubengine.JoinTags(tags)                    // "go, web, sqlite"
pubengine.FilterEmpty(tags)                 // Remove empty strings
pubengine.FilterRelatedPosts(current, all)  // Posts sharing tags

// JSON-LD structured data
pubengine.WebsiteJsonLD(cfg)                // WebSite schema
pubengine.BlogPostingJsonLD(post, cfg)      // BlogPosting schema

// Environment helpers (for main.go)
pubengine.EnvOr("KEY", "default")           // Get env var with fallback
pubengine.MustEnv("KEY")                    // Get env var or log.Fatal

// Template rendering
pubengine.Render(c, component)              // Render as HTTP 200
pubengine.RenderStatus(c, 404, component)   // Render with status code

// Auth helpers
pubengine.IsAdmin(c)                        // Check if session is authenticated
pubengine.CsrfToken(c)                      // Extract CSRF token from context
```

## Markdown

pubengine includes a custom markdown renderer (`pubengine/markdown` package) with no external dependencies.

### Supported syntax

| Syntax | Output |
|---|---|
| `**bold**` or `__bold__` | **bold** |
| `*italic*` or `_italic_` | *italic* |
| `` `code` `` | Inline code |
| `# Heading 1` | `<h1>` |
| `## Heading 2` | `<h2>` |
| `### Heading 3` | `<h3>` |
| `[text](url)` | Link (same tab) |
| `[text](url)^` | Link (new tab, adds `target="_blank"`) |
| `![alt](url)` | Image without assumed dimensions |
| `![alt](url){style}` | Image with inline CSS |
| `![alt](url){style\|w\|h}` | Image with dimensions |
| `- item` | Unordered list |
| `1. item` | Ordered list |
| `> quote` | Blockquote |
| `` ``` `` | Code block |
| `` ```lang `` | Code block with language badge |
| `\|col\|col\|` | Table |
| `---` | Horizontal rule |

### Usage in templates

```go
import "github.com/eringen/pubengine/markdown"

// In a templ component:
@markdown.Markdown(post.Content)
```

### Programmatic usage

```go
import "github.com/eringen/pubengine/markdown"

var buf bytes.Buffer
markdown.RenderMarkdown(&buf, "**hello** world")
// buf.String() == "<p><strong>hello</strong> world\n</p>"
```

### Security

All text is HTML escaped before formatting. Only `http`, `https`, `mailto`, and `tel` URL schemes are allowed. Bold/italic regex runs only on text outside HTML tags to prevent URL corruption. The first image gets `fetchpriority="high"`; later images use lazy loading. The image library copies actual dimensions. Inline code content is protected from bold/italic formatting.

Rendering accepts up to 2 MiB of Markdown and 16 MiB of output. A process-wide cache retains up to 256 rendered articles within a 16 MiB budget; outputs over 1 MiB bypass it. Content changes use a new cache key immediately.

## Analytics

pubengine includes a built in, privacy first analytics system. No cookies, no third party scripts, no personal data stored.

### How it works

IP addresses are hashed with a salted SHA-256. Each installation has its own persistent salt stored in its database; the salt does not rotate automatically. Visitor IDs are derived from IP + User Agent hash (no cookies). Bot traffic is detected and tracked separately. The system respects Do Not Track (DNT) headers. The App uses automatic cleanup with 365-day retention. Standalone analytics stores can configure their cleanup scheduler. All data stays in your SQLite database.

### Enabling analytics

```go
pubengine.SiteConfig{
    AnalyticsEnabled:      true,
    AnalyticsDatabasePath: "data/analytics.db",
    // ...
}
```

### Client side tracking

The framework ships `analytics.js` as an embedded asset, automatically served at `/public/analytics.js`. Include it in your template `<head>`:

```html
<script src="/public/analytics.js?v=2" defer></script>
```

The script tracks page views and visible-tab duration with explicit event types and page-view IDs, and handles committed talkDOM navigation. Hidden time is excluded. It uses `navigator.sendBeacon` for unload tracking, with a fetch fallback. Delivery is best effort. Admin paths are excluded by both client and collector. Installation is same-origin; cross-origin collection is not configured. The older `VisitRequest` and path-based `UpdateVisitDuration` APIs are deprecated.

### Dashboard

The analytics dashboard is available at `/admin/analytics/` (requires admin login). The admin nav bar includes a link to it. It shows:

- Viewed in last 5 min (distinct visitors who started a page view; not an active-reader heartbeat)
- Unique visitors and total page views
- Average visible-tab time on page
- Top pages and latest visits (last 10)
- Browser, OS, and device breakdown
- Top ten referrer sources plus Other, retaining the total
- Daily/hourly/monthly view charts
- Beacon bots (collector submissions, not every crawler page request)

The dashboard is fully self contained. Its CSS (`admin.css`) and JS (`dashboard.min.js`) are embedded in the binary alongside `talkdom.js`.

Reports use one read snapshot, honor request cancellation, and coalesce identical periods into a five-second cache with at most 16 period keys per report type. Recent-view counts refresh separately and query errors remain errors. Polling pauses while the tab is hidden. Collection remains synchronous; a successful response acknowledges persistence.

### Rate limiting

The analytics collect endpoint is rate limited to 60 requests per IP per minute to prevent flooding.

## Google OAuth login

pubengine supports an optional Google OAuth login for the admin panel. When configured, a "Sign in with Google" button appears on the login page alongside the password form. Password login always remains available as a fallback.

### Setup

1. Create OAuth credentials in the [Google Cloud Console](https://console.cloud.google.com/apis/credentials)
2. Set the authorized redirect URI to `https://yourdomain.com/admin/auth/google/callback`
3. Set the environment variables:

```bash
GOOGLE_CLIENT_ID=your-client-id.apps.googleusercontent.com
GOOGLE_CLIENT_SECRET=your-client-secret
GOOGLE_ADMIN_EMAIL=you@gmail.com
```

Or in your `SiteConfig`:

```go
pubengine.SiteConfig{
    GoogleClientID:     pubengine.EnvOr("GOOGLE_CLIENT_ID", ""),
    GoogleClientSecret: pubengine.EnvOr("GOOGLE_CLIENT_SECRET", ""),
    GoogleAdminEmail:   pubengine.EnvOr("GOOGLE_ADMIN_EMAIL", ""),
    // ...
}
```

All three fields must be set for Google login to be enabled. Only the email matching `GOOGLE_ADMIN_EMAIL` (case-insensitive) is allowed to log in.

## Middleware

pubengine configures a production ready middleware stack:

1. **NonWWWRedirect** redirects `www.` to bare domain
2. **RequestLogger** logs method, URI, status code, latency
3. **Recover** provides panic recovery with error logging
4. **Security headers** include CSP, HSTS, X-Frame-Options, X-Content-Type-Options, Referrer-Policy
5. **Session** uses cookie based sessions (gorilla/sessions, 12 hour expiry)
6. **CSRF** provides token based protection (skipped for analytics and static assets)
7. **Trailing slash** enforces consistent URL format
8. **Cache-Control** sets stable assets and public pages to revalidate, admin/API/error responses to no-store

Text assets are gzip compressed; compressed media is skipped. Public rendered HTML, RSS, sitemap, and embedded assets have ETags and support conditional responses. HTML still renders before its validator is calculated; feed and sitemap representations are reused until the post snapshot or site metadata changes. Stable `?v=2` URLs are not treated as content hashes or cached as immutable.

Uploads are admitted before body parsing, spooled to bounded temporary files, and limited to JPEG, PNG, or GIF input. Admission has no waiting queue. The concurrency setting bounds simultaneous processing, not total process memory; decoding large images still requires substantial memory.

## Database

### Blog database

SQLite at `data/blog.db` (auto created on first run).

```sql
CREATE TABLE posts (
    slug TEXT PRIMARY KEY,
    title TEXT NOT NULL,
    date TEXT NOT NULL,
    tags TEXT NOT NULL,          -- comma delimited: ",go,web,"
    summary TEXT NOT NULL,
    content TEXT NOT NULL,
    published INTEGER NOT NULL DEFAULT 1,
    revision INTEGER NOT NULL DEFAULT 1
);
```

### Analytics database

Separate SQLite at `data/analytics.db`.

```sql
CREATE TABLE visits (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    page_view_id TEXT,
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

CREATE TABLE bot_visits (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    page_view_id TEXT,
    bot_name TEXT NOT NULL,
    ip_hash TEXT NOT NULL,
    user_agent TEXT NOT NULL,
    path TEXT NOT NULL,
    timestamp DATETIME NOT NULL
);

CREATE TABLE settings (
    key TEXT PRIMARY KEY,
    value TEXT NOT NULL
);
```

Both databases use WAL mode with tuned pragmas (busy_timeout, synchronous=NORMAL, 8MB cache, 256MB mmap) for concurrent read performance.

These table outlines omit auxiliary tables and indexes. Runtime migrations also maintain post redirects and unique page-view indexes. Legacy referrers migrate in bounded primary-key batches within one transaction; retention cleanup deletes 1,000 rows per batch and supports cancellation.

## Store API

The `Store` provides all blog CRUD operations:

```go
store, err := pubengine.NewStore("data/blog.db")
defer store.Close()

// Published posts (for public pages)
posts, _ := store.ListPosts("")          // all published, newest first
posts, _ := store.ListPosts("go")        // filtered by tag (case insensitive)
post, _  := store.GetPost("my-slug")     // single published post
tags, _  := store.ListTags()             // unique tags from published posts

// All posts (for admin)
posts, _ := store.ListAllPosts()          // including drafts
post, _  := store.GetPostAny("my-slug")  // regardless of published status

// Write operations
store.SavePost(post)                      // create, or update the fetched revision
store.DeletePost("my-slug")              // delete by slug
```

Request handlers should use the corresponding `...Context(ctx, ...)` methods. Empty dates default to the current UTC date at the store boundary. Tags are trimmed, deduplicated, and normalized with Go Unicode lowercasing; posts sort by descending date and ascending slug. Direct store mutations require `cache.Invalidate()` when using a separate cache.

## Cache API

The `PostCache` wraps the store with an in memory cache:

```go
cache := pubengine.NewPostCache(store, 5*time.Minute)

posts, _ := cache.ListPosts("")     // full content; bulk DB read when bodies are missing
tags, _  := cache.ListTags()        // from cache
post, _  := cache.GetPost("slug")   // indexed lookup; body fetched on demand

posts, more, err := cache.ListPageContext(ctx, "go", 0, 20)
related, err := cache.RelatedContext(ctx, post, 6)

cache.Invalidate()                  // clear on write operations
```

The cache keeps an immutable archive summary and slug/tag indexes. Body retention is limited to 128 entries and 16 MiB of content, with oldest entries evicted individually. Summary memory still grows with archive size. Refresh I/O runs outside the cache lock; invalidation prevents an older in-flight refresh from publishing. Returned slices belong to the caller.

## Project structure

```
pubengine/
├── pubengine.go           # App struct, New(), Start(), Close()
├── config.go              # SiteConfig, Option functions
├── types.go               # BlogPost, PageMeta, Image
├── store.go               # SQLite blog CRUD
├── cache.go               # In memory post cache
├── handlers.go            # Blog handlers (home, post, feed, sitemap)
├── admin.go               # Admin handlers (login, save, delete, images)
├── middleware.go           # Security headers, sessions, CSRF, cache
├── render.go              # Render helpers
├── helpers.go             # Slugify, BuildURL, JSON-LD, tag utils
├── images.go              # Image upload, resize, library
├── limiter.go             # Login rate limiter
├── rss.go                 # RSS XML generation
├── sitemap.go             # Sitemap XML generation
├── embed.go               # Embedded static assets
├── embedded/
│   ├── talkdom.js          # talkDOM library
│   ├── analytics.js       # Client side tracking script
│   ├── dashboard.min.js   # Analytics dashboard JS
│   └── admin.css          # Analytics dashboard styles
├── markdown/
│   ├── markdown.go        # Custom markdown renderer
│   └── markdown_test.go
├── analytics/
│   ├── analytics.go       # IP hashing, UA parsing, bot detection
│   ├── store.go           # Analytics SQLite operations
│   ├── handlers.go        # Collection + dashboard handlers
│   ├── limiter.go         # Analytics rate limiter
│   ├── sqlcgen/           # Generated SQL (sqlc)
│   └── templates/         # Analytics dashboard templ templates
├── scaffold/
│   ├── scaffold.go        # embed.FS for templates
│   └── templates/         # Project scaffolding templates
├── cmd/pubengine/
│   ├── main.go            # CLI entry point
│   └── new.go             # Scaffold logic
├── store_test.go
├── limiter_test.go
└── go.mod
```

## CLI

### pubengine new

```bash
pubengine new github.com/yourname/myblog
```

Creates a new project directory with everything needed to run a blog. The last segment of the module path becomes the directory name (`myblog`).

Template variables:
- `{{.ProjectName}}` is the directory name (e.g., `myblog`)
- `{{.ModuleName}}` is the full module path (e.g., `github.com/yourname/myblog`)
- `{{.SiteName}}` is the title cased name (e.g., `Myblog`)

### pubengine version

```bash
pubengine version
```

## Scaffolded project commands

After `pubengine new`, the generated Makefile and package.json provide:

### Make targets

```bash
make run          # Generate templates, build CSS + JS, start server
make templ        # Regenerate templ templates
make css          # Build Tailwind CSS
make css-prod     # Production CSS (minified)
make js           # Bundle and minify src/app.js
make test         # Run Go tests
make build-linux  # Cross compile for Linux
```

### npm scripts

```bash
npm run css          # Build Tailwind CSS (minified)
npm run css:watch    # Watch mode for CSS
npm run js           # Bundle and minify src/app.js via esbuild
npm run js:watch     # Watch mode for JS
npm run build        # Build both CSS and JS
```

## Environment variables

| Variable | Required | Default | Description |
|---|---|---|---|
| `ADMIN_PASSWORD` | yes | | Admin login password |
| `ADMIN_SESSION_SECRET` | yes | | Random cookie signing key (at least 32 bytes) |
| `SITE_NAME` | no | Generated project name | Site name for nav, RSS, JSON-LD |
| `SITE_URL` | no | `http://localhost:3000` | Canonical URL for sitemap and OpenGraph |
| `SITE_DESCRIPTION` | no | `""` | Description for RSS and meta tags |
| `SITE_AUTHOR` | no | `""` | Author name for JSON-LD |
| `COOKIE_SECURE` | no | `false` | Set `true` behind HTTPS |
| `GOOGLE_CLIENT_ID` | no | `""` | Google OAuth client ID |
| `GOOGLE_CLIENT_SECRET` | no | `""` | Google OAuth client secret |
| `GOOGLE_ADMIN_EMAIL` | no | `""` | Allowed Google email for admin login |
| `DATABASE_PATH` | no | `data/blog.db` | Blog SQLite path |
| `ANALYTICS_DATABASE_PATH` | no | `data/analytics.db` | Analytics SQLite path |
| `ANALYTICS_ENABLED` | no | `true` | Generated site enables tracking only when set to `true` |
| `ADDR` | no | `:3000` | Server listen address |

## Dependencies

| Package | Version | Purpose |
|---|---|---|
| [echo/v4](https://echo.labstack.com/) | v4.16.0 | HTTP framework |
| [templ](https://templ.guide/) | v0.3.1020 | Type safe HTML templates |
| [modernc.org/sqlite](https://pkg.go.dev/modernc.org/sqlite) | v1.60.1 | Pure Go SQLite driver |
| [gorilla/sessions](https://github.com/gorilla/sessions) | v1.4.0 | Cookie session management |
| [echo-contrib](https://github.com/labstack/echo-contrib) | v0.50.1 | Echo session middleware |

No JavaScript framework dependencies. TalkDOM 0.5.0 and the analytics script are embedded in the binary. `npm run build:talkdom` reproducibly builds the pinned npm source with the integration in `scripts/build_talkdom.cjs`; `npm run check:talkdom` verifies the artifact. The generic cancellation and native-click fixes are also committed upstream as `c351966` and remain in the build integration until a published TalkDOM release includes them. PubEngine adds document metadata snapshots, authentication redirects, and a `talkdom:navigate` event emitted after URL commits and history restoration. Analytics listens for that event as well as legacy `talkdom:done` events.

The scaffold includes article typography, responsive admin forms, keyboard skip links and focus handling, and navigation loading/error announcements. Its JavaScript retains compatibility with v0.4.0's navigation events.

## Testing

Use Go 1.26 and Node.js 24.15 or newer for repository checks. Generators are pinned to templ v0.3.1020 and sqlc v1.30.0. Generated sites pin Tailwind CSS and esbuild; retain their generated `package-lock.json` for reproducible installs.

```bash
npm ci
npx playwright install chromium
make check-generated
make test
npm run test:browser
PUBENGINE_TEST_PUBLISHED=1 npm run test:browser  # Verify the scaffold's published dependency
make bench
```

`make test` runs Go race tests, vet, and Node regressions. Generated Go/templates are always compiled by `go test ./...`; the pinned generator may download dependencies on the first run. Browser checks additionally build the generated CSS/JS and exercise pagination, metadata, Back navigation, failed and out-of-order requests, edit conflicts, uploads, copied Markdown, and expired sessions. CI installs Chromium and runs these checks.

### Rendering allocation improvement

On Apple M4, darwin/arm64, Go 1.26.0, `go test ./internal/htmlrender -run '^$' -bench BenchmarkRender -benchmem -count=3` renders the same approximately 31 KiB article into a reused HTTP recorder. Before this change, public responses allocated about 33,008 B in 7 allocations and took 12.3–17.0 µs; afterward they allocated 176 B in 5 allocations and took 10.7–10.8 µs. Admin responses went from about 32,976 B / 5 allocations / 12.3–12.6 µs to 0 B / 0 allocations / 0.71–0.75 µs by also skipping unused ETag hashes. These isolate rendering costs, not complete request latency.

Render buffers with capacity up to 64 KiB are reused after the response finishes; larger buffers are discarded. This is a per-buffer retention cap, not a global memory limit. Render errors remain atomic and the 16 MiB output limit still applies.

### Performance samples

Local samples on Apple M4, darwin/arm64, Go 1.25.4, without an HTTP proxy:

| Fixture | Measured operation | Sample |
|---|---|---|
| 100 / 1,000 / 10,000 posts, in-memory SQLite, warm cache | Single-post lookup | 62–64 ns, 32 B, 1 allocation |
| Same fixtures | 20 summaries | 0.84–1.01 µs, 4,096 B, 21 allocations |
| Same fixtures | Six related summaries | 0.64–0.72 µs, 2,880 B, 11 allocations |
| 10,000 visits/referrers, file-backed SQLite, uncached year report | Complete report | 39.3 ms, 19,357 B, 417 allocations |
| 1,000 / 10,000 distinct legacy referrers, file-backed SQLite | Migration transaction | 3.37 / 32.1 ms, 0.52 / 5.19 MB allocated |
| 2,400 × 1,600 PNG resized to 800 pixels wide | Decode and resize | 44.3 ms, 58.4 MB allocated |

Reproduce with `go test ./... -run '^$' -bench . -benchmem -benchtime=100ms`; filter with `-bench BenchmarkPostCache`, `BenchmarkAnalyticsReport`, `BenchmarkLegacyMigration`, or `BenchmarkImageProcessing`. These are local microbenchmarks, not before/after speedups, HTTP throughput, p95 latency, or peak heap measurements. Benchmark production-sized data and concurrent collection before adding write queues or more analytics indexes.

## Deployment

pubengine compiles to a single binary. Deploy it with your `public/` directory and a `data/` directory for SQLite:

```bash
# Build for Linux
GOOS=linux GOARCH=amd64 go build -o mysite .

# On the server
./mysite
# Needs: public/ directory, data/ directory (auto created), env vars set
```

The binary embeds talkDOM, the analytics script, the analytics dashboard JS, and the admin CSS. User assets (CSS, JS, fonts, images) live in the `public/` directory alongside the binary.

## License

MIT [MIT](LICENSE)
