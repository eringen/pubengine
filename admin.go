package pubengine

import (
	"crypto/subtle"
	"database/sql"
	"errors"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/labstack/echo/v4"
)

func (a *App) handleAdmin(c echo.Context) error {
	if !IsAdmin(c) {
		errorMsg := ""
		switch c.QueryParam("error") {
		case "unauthorized_email":
			errorMsg = "Unauthorized Google account."
		case "invalid_state", "oauth_failed":
			errorMsg = "Google login failed. Please try again."
		}
		return Render(c, a.Views.AdminLogin(errorMsg, CsrfToken(c), a.googleLoginURL()))
	}
	return a.renderAdminDashboard(c, c.QueryParam("msg"))
}

func (a *App) handleAdminPost(c echo.Context) error {
	if !IsAdmin(c) {
		return c.Redirect(http.StatusSeeOther, "/admin/")
	}
	slug := c.Param("slug")
	if slug == "new" {
		return Render(c, a.Views.AdminFormPartial(BlogPost{}, CsrfToken(c)))
	}
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
	ip := c.RealIP()
	if !a.loginLimiter.Allow(ip) {
		return c.String(http.StatusTooManyRequests, "Too many login attempts. Try again later.")
	}
	pass := c.FormValue("password")
	if subtle.ConstantTimeCompare([]byte(pass), []byte(a.Config.AdminPassword)) == 1 {
		if err := setAdminSession(c); err != nil {
			return err
		}
		return c.Redirect(http.StatusSeeOther, "/admin/")
	}
	return Render(c, a.Views.AdminLogin("Invalid password.", CsrfToken(c), a.googleLoginURL()))
}

func (a *App) googleLoginURL() string {
	if a.Config.GoogleAuthEnabled() {
		return "/admin/auth/google/"
	}
	return ""
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
	p := BlogPost{
		Title: strings.TrimSpace(c.FormValue("title")), Slug: strings.TrimSpace(c.FormValue("slug")),
		OriginalSlug: c.FormValue("original_slug"), Date: strings.TrimSpace(c.FormValue("date")),
		Tags: FilterEmpty(strings.Split(c.FormValue("tags"), ",")), Summary: c.FormValue("summary"),
		Content: c.FormValue("content"), Published: c.FormValue("published") != "",
	}
	if p.Slug == "" {
		p.Slug = Slugify(p.Title)
	}
	if p.Date == "" {
		p.Date = time.Now().Format("2006-01-02")
	}
	if revision := c.FormValue("revision"); revision != "" {
		var err error
		p.Revision, err = strconv.ParseInt(revision, 10, 64)
		if err != nil || p.Revision < 0 {
			p.Error = "Invalid revision. Reload the editor."
			return RenderStatus(c, http.StatusBadRequest, a.Views.AdminFormPartial(p, CsrfToken(c)))
		}
	}
	if err := a.Store.SavePost(p); err != nil {
		status := http.StatusBadRequest
		if errors.Is(err, ErrPostConflict) {
			status = http.StatusConflict
		} else if ValidateSlug(p.Slug) == "" && p.Title != "" {
			if _, dateErr := time.Parse("2006-01-02", p.Date); dateErr == nil {
				return err
			}
		}
		p.Error = err.Error()
		return RenderStatus(c, status, a.Views.AdminFormPartial(p, CsrfToken(c)))
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
