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

// Image represents an uploaded image stored in the uploads directory.
type Image struct {
	Filename     string // e.g. "my-photo.jpg"
	OriginalName string
	Width        int
	Height       int
	Size         int    // bytes
	UploadedAt   string // RFC3339
}

// PageMeta carries per-page OpenGraph and SEO metadata into the <head> template.
type PageMeta struct {
	Title       string
	Description string
	URL         string // canonical + og:url
	OGType      string // "website" or "article"
}
