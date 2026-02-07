# pubengine

A minimal blog engine built with Go, HTMX, and SQLite. One binary, no JS frameworks, just write and publish.

## Quick start

```bash
git clone https://github.com/eringen/pubengine.git
cd pubengine

# Install Go deps + templ CLI
go mod download
go install github.com/a-h/templ/cmd/templ@latest

# Install JS deps (Tailwind, esbuild)
npm install

# Start dev server on :3000 (live reload via air)
make run
```

Admin dashboard: `http://localhost:3000/admin/`

## Environment variables

| Variable | Required | Default | Description |
|---|---|---|---|
| `ADMIN_PASSWORD` | yes | — | Admin login password |
| `ADMIN_SESSION_SECRET` | yes | — | Session cookie encryption secret |
| `SITE_NAME` | no | `Blog` | Site name shown in nav, footer, and metadata |
| `SITE_URL` | no | `http://localhost:3000` | Canonical URL for sitemap, RSS, and OpenGraph |
| `SITE_DESCRIPTION` | no | — | Site description for RSS and meta tags |
| `SITE_AUTHOR` | no | — | Author name for JSON-LD structured data |
| `COOKIE_SECURE` | no | `false` | Set to `true` behind HTTPS |
| `DATABASE_PATH` | no | `data/blog.db` | SQLite database path |

## Commands

```bash
make run          # Dev server with live reload (auto-installs air)
make templ        # Regenerate *_templ.go from .templ files
make css          # Build Tailwind CSS
make js           # Minify app.js via esbuild
make prod         # Production build + run
make test         # Run all Go tests
make build-linux  # Cross-compile for Linux (amd64)
```

## Architecture

```
main.go               Routes, middleware, handlers, RSS/sitemap
store.go              SQLite data layer (single posts table)
cache.go              In-memory post cache with TTL
views/
  types.go            SiteConfig, PageMeta, BlogPost structs
  helpers.go          JSON-LD, tag utilities, URL helpers
  markdown.go         Custom markdown renderer
  nav.templ           Head, Nav, Footer components
  home.templ          Home page with blog listing
  post.templ          Single post page
  admin.templ         Admin login + dashboard
  notfound.templ      404 page
  servererror.templ   500 page
assets/
  app.js              HTMX navigation + dark mode toggle
  tailwind.css        Tailwind base styles
public/               Built CSS, JS, and static files
```

## Customization

All branding flows through `SiteConfig` — set the four `SITE_*` env vars and everything updates: nav, footer, RSS, sitemap, JSON-LD, OpenGraph tags, page titles.

To customize the theme, edit `assets/tailwind.css` and `tailwind.config.js`. The `ink` color (`#0f0f0f`) is used throughout for borders and text.

## Markdown

The blog uses a custom markdown renderer (`views/markdown.go`). Standard features plus:

- `[text](url)^` — appending `^` opens links in a new tab
- `![alt](url){style}` — curly braces for inline image styles
- Tables with `|---|---|` separator rows

## Data

SQLite database at `data/blog.db` (auto-created on first run). Single `posts` table with a `published` flag for draft support.
