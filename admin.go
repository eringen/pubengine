package pubengine

import (
	"crypto/subtle"
	"database/sql"
	"net/http"
	"strings"
	"time"

	"github.com/labstack/echo/v4"
)

func (a *App) handleAdmin(c echo.Context) error {
	if !IsAdmin(c) {
		return Render(c, a.Views.AdminLogin(false, CsrfToken(c)))
	}
	return a.renderAdminDashboard(c, c.QueryParam("msg"))
}

func (a *App) handleAdminPost(c echo.Context) error {
	if !IsAdmin(c) {
		return c.Redirect(http.StatusSeeOther, "/admin/")
	}
	slug := c.Param("slug")
	post, err := a.Store.GetPostAny(slug)
	if err != nil {
		if err == sql.ErrNoRows {
			return c.NoContent(http.StatusNotFound)
		}
		return err
	}
	return Render(c, a.Views.AdminFormPartial(post, CsrfToken(c)))
}

func (a *App) handleAdminLogin(c echo.Context) error {
	if !a.loginLimiter.Allow(c.RealIP()) {
		return c.String(http.StatusTooManyRequests, "Too many login attempts. Try again later.")
	}
	pass := c.FormValue("password")
	if subtle.ConstantTimeCompare([]byte(pass), []byte(a.Config.AdminPassword)) == 1 {
		if err := setAdminSession(c); err != nil {
			return err
		}
		return c.Redirect(http.StatusSeeOther, "/admin/")
	}
	return Render(c, a.Views.AdminLogin(true, CsrfToken(c)))
}

func handleAdminLogout(c echo.Context) error {
	if err := clearAdminSession(c); err != nil {
		return err
	}
	return c.Redirect(http.StatusSeeOther, "/admin/")
}

func (a *App) handleAdminSave(c echo.Context) error {
	if !IsAdmin(c) {
		return c.Redirect(http.StatusSeeOther, "/admin/")
	}
	if err := c.Request().ParseForm(); err != nil {
		return err
	}
	title := strings.TrimSpace(c.FormValue("title"))
	slug := strings.TrimSpace(c.FormValue("slug"))
	if slug == "" {
		slug = Slugify(title)
	}
	if slug == "" {
		return c.Redirect(http.StatusSeeOther, "/admin/?msg=Slug+is+required.+Add+a+title+or+slug.")
	}
	date := strings.TrimSpace(c.FormValue("date"))
	if date == "" {
		date = time.Now().Format("2006-01-02")
	}
	if _, err := time.Parse("2006-01-02", date); err != nil {
		return c.Redirect(http.StatusSeeOther, "/admin/?msg=Invalid+date+format.+Use+YYYY-MM-DD.")
	}
	tags := strings.Split(c.FormValue("tags"), ",")
	for i := range tags {
		tags[i] = strings.TrimSpace(tags[i])
	}
	tags = FilterEmpty(tags)
	summary := c.FormValue("summary")
	content := c.FormValue("content")
	published := c.FormValue("published") != ""
	if err := a.Store.SavePost(BlogPost{
		Slug:      slug,
		Title:     title,
		Date:      date,
		Tags:      tags,
		Summary:   summary,
		Content:   content,
		Published: published,
	}); err != nil {
		return err
	}
	a.Cache.Invalidate()
	return a.renderAdminDashboard(c, "saved")
}

func (a *App) handleAdminDelete(c echo.Context) error {
	if !IsAdmin(c) {
		return c.Redirect(http.StatusSeeOther, "/admin/")
	}
	slug := c.Param("slug")
	if err := a.Store.DeletePost(slug); err != nil {
		return err
	}
	a.Cache.Invalidate()
	return a.renderAdminDashboard(c, "deleted")
}

func (a *App) renderAdminDashboard(c echo.Context, msg string) error {
	posts, err := a.Store.ListAllPosts()
	if err != nil {
		return err
	}
	return Render(c, a.Views.AdminDashboard(posts, msg, CsrfToken(c)))
}
