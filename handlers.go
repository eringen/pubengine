package pubengine

import (
	"database/sql"
	"net/http"

	"github.com/labstack/echo/v4"
)

func (a *App) handleHome(c echo.Context) error {
	tag := c.QueryParam("tag")
	posts, err := a.Cache.ListPosts(tag)
	if err != nil {
		return err
	}
	tags, err := a.Cache.ListTags()
	if err != nil {
		return err
	}
	if c.Request().Header.Get("HX-Request") == "true" {
		partial := c.QueryParam("partial")
		switch partial {
		case "blog":
			return Render(c, a.Views.BlogSection(posts, tag, tags))
		case "home":
			return Render(c, a.Views.HomePartial(posts, tag, tags, a.Config.URL))
		}
	}
	return Render(c, a.Views.Home(posts, tag, tags, a.Config.URL))
}

func (a *App) handlePost(c echo.Context) error {
	slug := c.Param("slug")
	post, err := a.Cache.GetPost(slug)
	if err != nil {
		if err == sql.ErrNoRows {
			return RenderStatus(c, http.StatusNotFound, a.Views.NotFound())
		}
		return err
	}
	posts, err := a.Cache.ListPosts("")
	if err != nil {
		return err
	}
	if c.Request().Header.Get("HX-Request") == "true" && c.QueryParam("partial") == "post" {
		return Render(c, a.Views.PostPartial(post, posts, a.Config.URL))
	}
	return Render(c, a.Views.Post(post, posts, a.Config.URL))
}

func (a *App) handleSitemap(c echo.Context) error {
	posts, err := a.Cache.ListPosts("")
	if err != nil {
		return err
	}
	return a.renderSitemap(c, posts)
}

func (a *App) handleFeed(c echo.Context) error {
	posts, err := a.Cache.ListPosts("")
	if err != nil {
		return err
	}
	return a.renderRSS(c, posts)
}

func handleBlogRedirect(c echo.Context) error {
	return c.Redirect(http.StatusMovedPermanently, "/")
}

func (a *App) handleFavicon(c echo.Context) error {
	return c.File(a.staticDir + "/favicon.svg")
}

func (a *App) handleRobots(c echo.Context) error {
	return c.File(a.staticDir + "/robots.txt")
}

func (a *App) httpErrorHandler(err error, c echo.Context) {
	if c.Response().Committed {
		return
	}
	he, ok := err.(*echo.HTTPError)
	if ok && he.Code == http.StatusNotFound {
		_ = RenderStatus(c, http.StatusNotFound, a.Views.NotFound())
		return
	}
	code := http.StatusInternalServerError
	if ok {
		code = he.Code
	}
	if code >= 500 {
		c.Logger().Errorf("server error: %v", err)
		_ = RenderStatus(c, code, a.Views.ServerError())
		return
	}
	a.Echo.DefaultHTTPErrorHandler(err, c)
}
