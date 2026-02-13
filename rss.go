package pubengine

import (
	"encoding/xml"
	"net/http"
	"time"

	"github.com/labstack/echo/v4"
)

type rssXML struct {
	XMLName xml.Name   `xml:"rss"`
	Version string     `xml:"version,attr"`
	Channel rssChannel `xml:"channel"`
}

type rssChannel struct {
	Title       string    `xml:"title"`
	Link        string    `xml:"link"`
	Description string    `xml:"description"`
	Items       []rssItem `xml:"item"`
}

type rssItem struct {
	Title       string `xml:"title"`
	Link        string `xml:"link"`
	Description string `xml:"description"`
	PubDate     string `xml:"pubDate"`
	GUID        string `xml:"guid"`
}

func (a *App) renderRSS(c echo.Context, posts []BlogPost) error {
	base := a.Config.URL
	items := make([]rssItem, 0, len(posts))
	for _, p := range posts {
		pubDate := ""
		if t, err := time.Parse("2006-01-02", p.Date); err == nil {
			pubDate = t.Format(time.RFC1123Z)
		}
		postURL := BuildURL(base, "blog", p.Slug)
		items = append(items, rssItem{
			Title:       p.Title,
			Link:        postURL,
			Description: p.Summary,
			PubDate:     pubDate,
			GUID:        postURL,
		})
	}
	feed := rssXML{
		Version: "2.0",
		Channel: rssChannel{
			Title:       a.Config.Name,
			Link:        base,
			Description: a.Config.Description,
			Items:       items,
		},
	}
	c.Response().Header().Set(echo.HeaderContentType, "application/rss+xml; charset=utf-8")
	c.Response().WriteHeader(http.StatusOK)
	c.Response().Write([]byte(xml.Header))
	return xml.NewEncoder(c.Response()).Encode(feed)
}
