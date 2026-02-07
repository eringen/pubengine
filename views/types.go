package views

// SiteConfig holds site-wide settings populated from environment variables.
// Every handler passes this to templates so nothing is hardcoded.
type SiteConfig struct {
	Name        string // SITE_NAME  (default "Blog")
	URL         string // SITE_URL   (default "http://localhost:3000")
	Description string // SITE_DESCRIPTION
	Author      string // SITE_AUTHOR
}

// PageMeta carries per-page OpenGraph and SEO metadata into the <head> template.
type PageMeta struct {
	Title       string
	Description string
	URL         string // canonical + og:url
	OGType      string // "website" or "article"
}

// BlogPost is the core content type stored in SQLite and rendered by templates.
type BlogPost struct {
	Title     string
	Date      string
	Tags      []string
	Summary   string
	Link      string
	Slug      string
	Content   string
	Published bool
}
