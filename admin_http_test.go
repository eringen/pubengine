package pubengine

import (
	"bytes"
	"image"
	"image/png"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/a-h/templ"
	"github.com/labstack/echo/v4"
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
	a.Views.AdminImages = func([]Image, string) templ.Component { return templ.Raw("uploaded") }
	var body bytes.Buffer
	writer := multipart.NewWriter(&body)
	for _, cookie := range login.Result().Cookies() {
		if cookie.Name == "_csrf" {
			if err := writer.WriteField("_csrf", cookie.Value); err != nil {
				t.Fatal(err)
			}
		}
	}
	file, err := writer.CreateFormFile("image", "photo.png")
	if err != nil {
		t.Fatal(err)
	}
	if err := png.Encode(file, image.NewRGBA(image.Rect(0, 0, 12, 8))); err != nil {
		t.Fatal(err)
	}
	if err := writer.Close(); err != nil {
		t.Fatal(err)
	}
	req := httptest.NewRequest("POST", "/admin/images/upload/", &body)
	req.Header.Set("Content-Type", writer.FormDataContentType())
	for _, cookie := range login.Result().Cookies() {
		req.AddCookie(cookie)
	}
	uploaded := httptest.NewRecorder()
	a.Echo.ServeHTTP(uploaded, req)
	if uploaded.Code != 200 || uploaded.Body.String() != "uploaded" {
		t.Fatal(uploaded.Code, uploaded.Body)
	}
}
