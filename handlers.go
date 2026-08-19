package pubengine

import (
	"database/sql"
	"net/http"

	"github.com/labstack/echo/v4"
)

func (a *App) handleHome(c echo.Context) error {
	tag := normalizeTag(c.QueryParam("tag"))
	var posts []BlogPost
	var err error
	if a.Config.PageSize > 0 {
		offset := pageOffset(c, a.Config.PageSize)
		var more bool
		posts, more, err = a.Cache.ListPageContext(c.Request().Context(), tag, offset, a.Config.PageSize)
		setPagination(c, a.Config.PageSize, offset, more)
	} else {
		posts, err = a.Cache.ListPostsContext(c.Request().Context(), tag)
	}
	if err != nil {
		return err
	}
	tags, err := a.Cache.ListTagsContext(c.Request().Context())
	if err != nil {
		return err
	}
	partial := c.QueryParam("partial")
	switch partial {
	case "blog":
		return Render(c, a.Views.BlogSection(posts, tag, tags))
	case "home":
		return Render(c, a.Views.HomePartial(posts, tag, tags, a.Config.URL))
	}
	return Render(c, a.Views.Home(posts, tag, tags, a.Config.URL))
}

func (a *App) handlePost(c echo.Context) error {
	slug := c.Param("slug")
	post, err := a.Cache.GetPostContext(c.Request().Context(), slug)
	if err != nil {
		if err == sql.ErrNoRows {
			if target, redirectErr := a.Store.ResolvePostRedirectContext(c.Request().Context(), slug); redirectErr == nil {
				location := PostPath(target)
				if c.QueryParam("partial") == "post" {
					location += "?partial=post"
				}
				return c.Redirect(http.StatusMovedPermanently, location)
			} else if redirectErr != sql.ErrNoRows {
				return redirectErr
			}
			return RenderStatus(c, http.StatusNotFound, a.Views.NotFound())
		}
		return err
	}
	var posts []BlogPost
	if a.Config.PageSize > 0 {
		posts, err = a.Cache.RelatedContext(c.Request().Context(), post, 6)
	} else {
		posts, err = a.Cache.ListPostsContext(c.Request().Context(), "")
	}
	if err != nil {
		return err
	}
	if c.QueryParam("partial") == "post" {
		return Render(c, a.Views.PostPartial(post, posts, a.Config.URL))
	}
	return Render(c, a.Views.Post(post, posts, a.Config.URL))
}

func (a *App) handleSitemap(c echo.Context) error { return a.serveXML(c, "sitemap") }
func (a *App) handleFeed(c echo.Context) error    { return a.serveXML(c, "feed") }

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
	if ok && he.Code == http.StatusNotFound && a.Views.NotFound != nil {
		if renderErr := RenderStatus(c, http.StatusNotFound, a.Views.NotFound()); renderErr != nil {
			c.Logger().Error(renderErr)
			_ = c.String(500, "Internal Server Error")
		}
		return
	}
	code := http.StatusInternalServerError
	if ok {
		code = he.Code
	}
	if code >= 500 {
		c.Logger().Errorf("server error: %v", err)
		if a.Views.ServerError != nil {
			if renderErr := RenderStatus(c, code, a.Views.ServerError()); renderErr == nil {
				return
			} else {
				c.Logger().Error(renderErr)
			}
		}
		_ = c.String(code, "Internal Server Error")
		return
	}
	a.Echo.DefaultHTTPErrorHandler(err, c)
}
