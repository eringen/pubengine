package views

import (
	"encoding/json"
	"net/url"
	"path"
	"strings"
)

// buildURL joins path segments onto a base URL, ensuring a trailing slash.
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

// FilterRelatedPosts returns posts that share at least one tag with the current post.
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

// PathEscape wraps url.PathEscape for use in templ expressions.
func PathEscape(s string) string {
	return url.PathEscape(s)
}

// TagClass returns CSS classes for a tag pill, with active variant.
func TagClass(active bool) string {
	base := "inline-flex items-center rounded border border-ink dark:border-white/30 bg-stone-100 dark:bg-neutral-700 px-2.5 py-1 text-[11px] font-semibold uppercase tracking-[0.12em] hover:-translate-y-0.5 hover:shadow-sm transition"
	if active {
		base += " bg-ink dark:bg-white text-white dark:text-ink"
	}
	return base
}

// JoinTags formats a tag slice as a comma-separated string for form fields.
func JoinTags(tags []string) string {
	return strings.Join(tags, ", ")
}

// WebsiteJsonLD produces a Schema.org WebSite JSON-LD block using cfg values.
func WebsiteJsonLD(cfg SiteConfig) string {
	data := map[string]interface{}{
		"@context": "https://schema.org",
		"@type":    "WebSite",
		"name":     cfg.Name,
		"url":      buildURL(cfg.URL),
	}
	if cfg.Description != "" {
		data["description"] = cfg.Description
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

// BlogPostingJsonLD produces a Schema.org BlogPosting JSON-LD block for a post.
func BlogPostingJsonLD(cfg SiteConfig, post BlogPost) string {
	postURL := buildURL(cfg.URL, "blog", post.Slug)
	data := map[string]interface{}{
		"@context":      "https://schema.org",
		"@type":         "BlogPosting",
		"headline":      post.Title,
		"description":   post.Summary,
		"datePublished": post.Date,
		"url":           postURL,
		"publisher": map[string]string{
			"@type": "Organization",
			"name":  cfg.Name,
		},
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
	if len(post.Tags) > 0 {
		data["keywords"] = strings.Join(post.Tags, ", ")
	}
	b, err := json.Marshal(data)
	if err != nil {
		return "{}"
	}
	return string(b)
}
