package pubengine

import (
	"github.com/a-h/templ"
	"github.com/labstack/echo/v4"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func TestAdminFormResponseContract(t *testing.T) {
	a := testHTTPApp(t)
	s, err := NewStore(":memory:")
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	a.Store, a.Cache = s, NewPostCache(s, time.Hour)
	a.Views.AdminFormPartial = func(p BlogPost, csrf string) templ.Component { return templ.Raw("<form>" + p.Error + "</form>") }
	a.Echo.GET("/test-login/", func(c echo.Context) error {
		if err := setAdminSession(c); err != nil {
			return err
		}
		return c.NoContent(204)
	})
	login := httptest.NewRecorder()
	a.Echo.ServeHTTP(login, httptest.NewRequest("GET", "/test-login/", nil))
	send := func(body string) *httptest.ResponseRecorder {
		r := httptest.NewRequest("POST", "/admin/save/", strings.NewReader(body))
		r.Header.Set("Content-Type", "application/x-www-form-urlencoded")
		for _, cookie := range login.Result().Cookies() {
			r.AddCookie(cookie)
			if cookie.Name == "_csrf" {
				r.Header.Set("X-CSRF-Token", cookie.Value)
			}
		}
		w := httptest.NewRecorder()
		a.Echo.ServeHTTP(w, r)
		return w
	}
	bad := send("slug=post&content=keep")
	if bad.Code != http.StatusBadRequest || !strings.Contains(bad.Body.String(), "<!DOCTYPE html>") {
		t.Fatal(bad.Code, bad.Body)
	}
	good := send("slug=post&title=Post&published=1")
	if good.Code != http.StatusSeeOther || good.Header().Get("Location") != "/admin/?msg=saved" {
		t.Fatal(good.Code, good.Body)
	}
}
