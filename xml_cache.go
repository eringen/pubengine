package pubengine

import (
	"github.com/eringen/pubengine/internal/httpcache"
	"github.com/labstack/echo/v4"
)

type xmlDocument struct {
	snapshot               *postSnapshot
	name, url, description string
	body                   []byte
	tag                    string
}

func (a *App) serveXML(c echo.Context, kind string) error {
	snapshot, err := a.Cache.snapshotContext(c.Request().Context())
	if err != nil {
		return err
	}
	a.xmlMu.Lock()
	doc := a.xmlDocuments[kind]
	if doc.snapshot != snapshot || doc.name != a.Config.Name || doc.url != a.Config.URL || doc.description != a.Config.Description {
		if kind == "feed" {
			doc.body, err = a.buildRSS(snapshot.posts)
		} else {
			doc.body, err = a.buildSitemap(snapshot.posts)
		}
		if err == nil {
			doc.snapshot = snapshot
			doc.name = a.Config.Name
			doc.url = a.Config.URL
			doc.description = a.Config.Description
			doc.tag = httpcache.ETag(doc.body)
			if a.xmlDocuments == nil {
				a.xmlDocuments = map[string]xmlDocument{}
			}
			a.xmlDocuments[kind] = doc
		}
	}
	a.xmlMu.Unlock()
	if err != nil {
		return err
	}
	contentType := "application/xml; charset=utf-8"
	if kind == "feed" {
		contentType = "application/rss+xml; charset=utf-8"
	}
	return httpcache.TaggedBytes(c, 200, contentType, doc.body, doc.tag)
}
