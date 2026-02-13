package pubengine

import (
	"encoding/json"
	"net/url"
	"path"
	"strings"
)

// Slugify converts a title to a URL-safe slug.
func Slugify(s string) string {
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

// BuildURL joins a base URL with path segments, ensuring a trailing slash.
func BuildURL(base string, pathSegments ...string) string {
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

// FilterEmpty removes empty/whitespace-only strings from a slice.
func FilterEmpty(vals []string) []string {
	var out []string
	for _, v := range vals {
		if s := strings.TrimSpace(v); s != "" {
			out = append(out, s)
		}
	}
	return out
}

// FilterRelatedPosts finds posts that share at least one tag with current.
func FilterRelatedPosts(current BlogPost, posts []BlogPost) []BlogPost {
	tagSet := make(map[string]struct{})
	for _, t := range current.Tags {
		tag := strings.ToLower(strings.TrimSpace(t))
		if tag != "" {
			tagSet[tag] = struct{}{}
		}
	}
	var related []BlogPost
	for _, p := range posts {
		if p.Slug == current.Slug {
			continue
		}
		for _, t := range p.Tags {
			tag := strings.ToLower(strings.TrimSpace(t))
			if _, ok := tagSet[tag]; ok {
				related = append(related, p)
				break
			}
		}
	}
	return related
}

// JoinTags joins tags with ", ".
func JoinTags(tags []string) string {
	return strings.Join(tags, ", ")
}

// PathEscape escapes a string for use in a URL path.
func PathEscape(s string) string {
	return url.PathEscape(s)
}

// WebsiteJsonLD returns a JSON-LD string for a WebSite schema using SiteConfig.
func WebsiteJsonLD(cfg SiteConfig) string {
	data := map[string]interface{}{
		"@context":    "https://schema.org",
		"@type":       "WebSite",
		"name":        cfg.Name,
		"url":         BuildURL(cfg.URL),
		"description": cfg.Description,
	}
	if cfg.Author != "" {
		data["author"] = map[string]string{
			"@type": "Person",
			"name":  cfg.Author,
		}
	}
	b, err := json.Marshal(data)
	if err != nil {
		return "{}"
	}
	return string(b)
}

// BlogPostingJsonLD returns a JSON-LD string for a BlogPosting schema.
func BlogPostingJsonLD(post BlogPost, cfg SiteConfig) string {
	postURL := BuildURL(cfg.URL, "blog", post.Slug)
	data := map[string]interface{}{
		"@context":      "https://schema.org",
		"@type":         "BlogPosting",
		"headline":      post.Title,
		"description":   post.Summary,
		"datePublished": post.Date,
		"url":           postURL,
		"mainEntityOfPage": map[string]string{
			"@type": "WebPage",
			"@id":   postURL,
		},
	}
	if cfg.Author != "" {
		data["author"] = map[string]string{
			"@type": "Person",
			"name":  cfg.Author,
		}
	}
	if cfg.Name != "" {
		data["publisher"] = map[string]string{
			"@type": "Organization",
			"name":  cfg.Name,
		}
	}
	if len(post.Tags) > 0 {
		data["keywords"] = strings.Join(post.Tags, ", ")
	}
	b, err := json.Marshal(data)
	if err != nil {
		return "{}"
	}
	return string(b)
}
