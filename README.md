# pubengine

A Go blog publishing framework. Ships blog CRUD, admin dashboard, privacy-first analytics, RSS, and sitemap out of the box. You own the templates, pubengine handles everything else.

Built with [Echo](https://echo.labstack.com/), [templ](https://templ.guide/), [HTMX](https://htmx.org/), [Tailwind CSS](https://tailwindcss.com/), and [SQLite](https://sqlite.org/).

## How it works

pubengine is a Go module, not a standalone app. You import it, provide your own templ templates via a `ViewFuncs` struct, and pubengine wires up all the handlers, middleware, database, caching, and analytics. Think of it like Django for Go blogs &mdash; convention over configuration with full template ownership.

```
+-----------------+       +-------------------+
|  Your Project   |       |    pubengine      |
|                 |       |                   |
|  main.go        |------>|  Handlers         |
|  views/*.templ  |       |  Middleware       |
|  assets/        |       |  Store (SQLite)   |
|  public/        |       |  Cache            |
|                 |       |  Analytics        |
|  ViewFuncs{     |       |  RSS / Sitemap    |
|    Home: ...,   |       |  Rate Limiter     |
|    Post: ...,   |       |  Session / CSRF   |
|  }              |       |  Markdown         |
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
│   ├── nav.templ         # Head, Nav, Footer, dark mode
│   ├── notfound.templ    # 404 page
│   ├── servererror.templ # 500 page
│   └── helpers.go        # Type aliases for BlogPost, PageMeta
├── assets/
│   └── tailwind.css      # Tailwind directives
├── public/
│   ├── robots.txt
│   └── favicon.svg
├── data/                 # SQLite databases (auto-created)
├── Makefile
├── package.json
├── tailwind.config.js
└── .env.example
```

### Run it

```bash
go mod tidy
npm install
make run
```

Your blog is running at `http://localhost:3000`. Admin dashboard at `/admin/`.

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
    app := pubengine.New(
        pubengine.SiteConfig{
            Name:          pubengine.EnvOr("SITE_NAME", "My Blog"),
            URL:           pubengine.EnvOr("SITE_URL", "http://localhost:3000"),
            Description:   pubengine.EnvOr("SITE_DESCRIPTION", "A blog about things"),
            Author:        pubengine.EnvOr("SITE_AUTHOR", "Your Name"),
            Addr:          pubengine.EnvOr("ADDR", ":3000"),
            DatabasePath:  pubengine.EnvOr("DATABASE_PATH", "data/blog.db"),
            AdminPassword: pubengine.MustEnv("ADMIN_PASSWORD"),
            SessionSecret: pubengine.MustEnv("ADMIN_SESSION_SECRET"),
            CookieSecure:  pubengine.EnvOr("COOKIE_SECURE", "") == "true",
        },
        pubengine.ViewFuncs{
            Home:             views.Home,
            HomePartial:      views.HomePartial,
            BlogSection:      views.BlogSection,
            Post:             views.Post,
            PostPartial:      views.PostPartial,
            AdminLogin:       views.AdminLogin,
            AdminDashboard:   views.AdminDashboard,
            AdminFormPartial: views.AdminFormPartial,
            NotFound:         views.NotFound,
            ServerError:      views.ServerError,
        },
    )
    defer app.Close()

    if err := app.Start(); err != nil {
        log.Fatal(err)
    }
}
```

### ViewFuncs

This is the core inversion-of-control mechanism. You provide templ components, pubengine calls them from its handlers:

```go
type ViewFuncs struct {
    // Full page renders (initial page load)
    Home             func(posts []BlogPost, activeTag string, tags []string, siteURL string) templ.Component
    Post             func(post BlogPost, posts []BlogPost, siteURL string) templ.Component

    // HTMX partial renders (SPA-like navigation)
    HomePartial      func(posts []BlogPost, activeTag string, tags []string, siteURL string) templ.Component
    BlogSection      func(posts []BlogPost, activeTag string, tags []string) templ.Component
    PostPartial      func(post BlogPost, posts []BlogPost, siteURL string) templ.Component

    // Admin pages
    AdminLogin       func(showError bool, csrfToken string) templ.Component
    AdminDashboard   func(posts []BlogPost, message string, csrfToken string) templ.Component
    AdminFormPartial func(post BlogPost, csrfToken string) templ.Component

    // Error pages
    NotFound         func() templ.Component
    ServerError      func() templ.Component
}
```

The framework handles when to call full vs. partial renders based on HTMX headers automatically.

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
| `AnalyticsEnabled` | `bool` | `true` | Enable built-in analytics |
| `AnalyticsDatabasePath` | `string` | `"data/analytics.db"` | Analytics SQLite path |
| `AdminPassword` | `string` | **required** | Admin login password |
| `SessionSecret` | `string` | **required** | Session cookie encryption secret |
| `CookieSecure` | `bool` | `false` | Set `true` when behind HTTPS |
| `PostCacheTTL` | `time.Duration` | `5m` | In-memory post cache TTL |

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

app.Config    // SiteConfig
app.Echo      // *echo.Echo - the HTTP server
app.Store     // *Store - SQLite operations
app.Cache     // *PostCache - in-memory cache
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
    Link      string     // "/blog/my-post" (auto-generated)
    Slug      string     // "my-post"
    Content   string     // Markdown source
    Published bool
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
| `GET` | `/` | Home page / blog listing |
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
| `GET` | `/admin/post/:slug/` | Edit post form (HTMX) |
| `POST` | `/admin/save/` | Create or update post |
| `DELETE` | `/admin/post/:slug/` | Delete post |

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
pubengine.PathEscape(tag)                   // URL-safe tag encoding
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
| `# Heading 1` | `<h1>` |
| `## Heading 2` | `<h2>` |
| `### Heading 3` | `<h3>` |
| `[text](url)` | Link (same tab) |
| `[text](url)^` | Link (new tab, adds `target="_blank"`) |
| `![alt](url){style}` | Image with inline CSS |
| `![alt](url){style\|w\|h}` | Image with dimensions |
| `- item` | Unordered list |
| `> quote` | Blockquote |
| `` ``` `` | Code block |
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

- All text is HTML-escaped before formatting
- Only `http`, `https`, `mailto`, and `tel` URL schemes are allowed
- Bold/italic regex runs only on text outside HTML tags to prevent URL corruption
- First image gets `fetchpriority="high"` for LCP optimization

## Analytics

pubengine includes a built-in, privacy-first analytics system. No cookies, no third-party scripts, no personal data stored.

### How it works

- IP addresses are hashed with a salted SHA-256 (salt rotates, stored in DB)
- Visitor IDs are derived from IP + User-Agent hash (no cookies)
- Bot traffic is detected and tracked separately
- Respects Do Not Track (DNT) header
- Data retention is configurable with automatic cleanup (default: 365 days)
- All data stays in your SQLite database

### Client-side

The framework ships `analytics.js` as an embedded asset. It's automatically served at `/public/analytics.js`. Include it in your `<head>`:

```html
<script src="/public/analytics.js" defer></script>
```

### Dashboard

The analytics dashboard is available at `/admin/analytics/` (requires admin login). It shows:

- Unique visitors and total page views
- Average time on page
- Top pages and latest visits
- Browser, OS, and device breakdown
- Referrer sources
- Daily view charts
- Bot traffic (separate tab)

### Disabling analytics

```go
pubengine.SiteConfig{
    AnalyticsEnabled: false,
    // ...
}
```

## Middleware

pubengine configures a production-ready middleware stack:

1. **NonWWWRedirect** - Redirects `www.` to bare domain
2. **RequestLogger** - Logs method, URI, status code, latency
3. **Recover** - Panic recovery with error logging
4. **Security headers** - CSP, HSTS, X-Frame-Options, X-Content-Type-Options, Referrer-Policy
5. **Session** - Cookie-based sessions (gorilla/sessions, 12-hour expiry)
6. **CSRF** - Token-based protection (skipped for analytics endpoint)
7. **Trailing slash** - Enforces consistent URL format
8. **Cache-Control** - Static assets: 1 year immutable; pages: 1 hour; admin: no-store

## Database

### Blog database

SQLite at `data/blog.db` (auto-created on first run).

```sql
CREATE TABLE posts (
    slug TEXT PRIMARY KEY,
    title TEXT NOT NULL,
    date TEXT NOT NULL,
    tags TEXT NOT NULL,          -- comma-delimited: ",go,web,"
    summary TEXT NOT NULL,
    content TEXT NOT NULL,
    published INTEGER NOT NULL DEFAULT 1
);
```

### Analytics database

Separate SQLite at `data/analytics.db`.

```sql
CREATE TABLE visits (
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

CREATE TABLE bot_visits (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
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

## Store API

The `Store` provides all blog CRUD operations:

```go
store, err := pubengine.NewStore("data/blog.db")
defer store.Close()

// Published posts (for public pages)
posts, _ := store.ListPosts("")          // all published, newest first
posts, _ := store.ListPosts("go")        // filtered by tag (case-insensitive)
post, _  := store.GetPost("my-slug")     // single published post
tags, _  := store.ListTags()             // unique tags from published posts

// All posts (for admin)
posts, _ := store.ListAllPosts()          // including drafts
post, _  := store.GetPostAny("my-slug")  // regardless of published status

// Write operations
store.SavePost(post)                      // insert or replace
store.DeletePost("my-slug")              // delete by slug
```

## Cache API

The `PostCache` wraps the store with an in-memory cache:

```go
cache := pubengine.NewPostCache(store, 5*time.Minute)

posts, _ := cache.ListPosts("")     // from cache if fresh, else DB
tags, _  := cache.ListTags()        // from cache
post, _  := cache.GetPost("slug")   // from cached post list

cache.Invalidate()                  // clear on write operations
```

## Project structure

```
pubengine/
├── pubengine.go           # App struct, New(), Start(), Close()
├── config.go              # SiteConfig, Option functions
├── types.go               # BlogPost, PageMeta
├── store.go               # SQLite blog CRUD
├── cache.go               # In-memory post cache
├── handlers.go            # Blog handlers (home, post, feed, sitemap)
├── admin.go               # Admin handlers (login, save, delete)
├── middleware.go           # Security headers, sessions, CSRF, cache
├── render.go              # Render helpers
├── helpers.go             # Slugify, BuildURL, JSON-LD, tag utils
├── limiter.go             # Login rate limiter
├── rss.go                 # RSS XML generation
├── sitemap.go             # Sitemap XML generation
├── embed.go               # Embedded static assets
├── embedded/              # htmx.min.js, analytics.js, dashboard.min.js
├── markdown/
│   ├── markdown.go        # Custom markdown renderer
│   └── markdown_test.go
├── analytics/
│   ├── analytics.go       # IP hashing, UA parsing, bot detection
│   ├── store.go           # Analytics SQLite operations
│   ├── handlers.go        # Collection + dashboard handlers
│   ├── sqlcgen/           # Generated SQL (sqlc)
│   └── templates/         # Analytics dashboard templates
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
- `{{.ProjectName}}` - directory name (e.g., `myblog`)
- `{{.ModuleName}}` - full module path (e.g., `github.com/yourname/myblog`)
- `{{.SiteName}}` - title-cased name (e.g., `Myblog`)

### pubengine version

```bash
pubengine version
```

## Scaffolded project commands

After `pubengine new`, the generated Makefile provides:

```bash
make run          # Dev server with live reload
make templ        # Regenerate templ templates
make css          # Build Tailwind CSS
make css-prod     # Production CSS (minified)
make test         # Run Go tests
make build-linux  # Cross-compile for Linux
```

## Environment variables

| Variable | Required | Default | Description |
|---|---|---|---|
| `ADMIN_PASSWORD` | yes | -- | Admin login password |
| `ADMIN_SESSION_SECRET` | yes | -- | Session encryption secret (32+ chars) |
| `SITE_NAME` | no | `Blog` | Site name for nav, RSS, JSON-LD |
| `SITE_URL` | no | `http://localhost:3000` | Canonical URL for sitemap and OpenGraph |
| `SITE_DESCRIPTION` | no | `""` | Description for RSS and meta tags |
| `SITE_AUTHOR` | no | `""` | Author name for JSON-LD |
| `COOKIE_SECURE` | no | `false` | Set `true` behind HTTPS |
| `DATABASE_PATH` | no | `data/blog.db` | Blog SQLite path |
| `ANALYTICS_DATABASE_PATH` | no | `data/analytics.db` | Analytics SQLite path |
| `ADDR` | no | `:3000` | Server listen address |

## Dependencies

| Package | Version | Purpose |
|---|---|---|
| [echo/v4](https://echo.labstack.com/) | v4.14.0 | HTTP framework |
| [templ](https://templ.guide/) | v0.3.960 | Type-safe HTML templates |
| [modernc.org/sqlite](https://pkg.go.dev/modernc.org/sqlite) | v1.44.2 | Pure-Go SQLite driver |
| [gorilla/sessions](https://github.com/gorilla/sessions) | v1.2.2 | Cookie session management |
| [echo-contrib](https://github.com/labstack/echo-contrib) | v0.17.1 | Echo session middleware |

No JavaScript framework dependencies. HTMX and the analytics script are embedded in the binary.

## Testing

```bash
# Run all tests
go test ./...

# Run with verbose output
go test -v ./...

# Run benchmarks
go test -bench=. ./...
```

Test coverage includes store operations, rate limiting, and markdown rendering.

## Deployment

pubengine compiles to a single binary. Deploy it with your `public/` directory and a `data/` directory for SQLite:

```bash
# Build for Linux
GOOS=linux GOARCH=amd64 go build -o mysite .

# On the server
./mysite
# Needs: public/ directory, data/ directory (auto-created), env vars set
```

The binary embeds HTMX, the analytics script, and the analytics dashboard. User assets (CSS, JS, fonts, images) must be in the `public/` directory alongside the binary.

## License

MIT
