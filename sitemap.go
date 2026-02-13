package pubengine

import (
	"encoding/xml"
	"net/http"

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

func (a *App) renderSitemap(c echo.Context, posts []BlogPost) error {
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
	c.Response().Header().Set(echo.HeaderContentType, "application/xml; charset=utf-8")
	c.Response().WriteHeader(http.StatusOK)
	c.Response().Write([]byte(xml.Header))
	return xml.NewEncoder(c.Response()).Encode(sitemap)
}
