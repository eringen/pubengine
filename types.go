package pubengine

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

// PageMeta carries per-page OpenGraph and SEO metadata into the <head> template.
type PageMeta struct {
	Title       string
	Description string
	URL         string // canonical + og:url
	OGType      string // "website" or "article"
}
