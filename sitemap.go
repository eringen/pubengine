package pubengine

import (
	"encoding/xml"
	"github.com/eringen/pubengine/internal/httpcache"

	"github.com/labstack/echo/v4"
)

type sitemapURLSet struct {
	XMLName xml.Name     `xml:"urlset"`
	XMLNS   string       `xml:"xmlns,attr"`
	URLs    []sitemapURL `xml:"url"`
}

type sitemapURL struct {
	Loc     string `xml:"loc"`
	LastMod string `xml:"lastmod,omitempty"`
}

func (a *App) buildSitemap(posts []BlogPost) ([]byte, error) {
	base := a.Config.URL
	urls := []sitemapURL{
		{Loc: BuildURL(base)},
	}
	for _, p := range posts {
		urls = append(urls, sitemapURL{
			Loc:     BuildURL(base, "blog", p.Slug),
			LastMod: p.Date,
		})
	}
	sitemap := sitemapURLSet{
		XMLNS: "http://www.sitemaps.org/schemas/sitemap/0.9",
		URLs:  urls,
	}
	var buf renderBuffer
	if _, err := buf.WriteString(xml.Header); err != nil {
		return nil, err
	}
	if err := xml.NewEncoder(&buf).Encode(sitemap); err != nil {
		return nil, err
	}
	return buf.Bytes(), nil
}

func (a *App) renderSitemap(c echo.Context, posts []BlogPost) error {
	body, err := a.buildSitemap(posts)
	if err != nil {
		return err
	}
	return httpcache.Bytes(c, 200, "application/xml; charset=utf-8", body)
}
