package pubengine

import (
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gorilla/sessions"
	"github.com/labstack/echo-contrib/session"
	"github.com/labstack/echo/v4"
)

func TestRejectDefaultCredentials(t *testing.T) {
	for _, cfg := range []SiteConfig{
		{AdminPassword: "changeme", SessionSecret: strings.Repeat("x", 32)},
		{AdminPassword: "password", SessionSecret: "changeme-secret"},
		{AdminPassword: "password", SessionSecret: "short"},
		{SessionSecret: strings.Repeat("x", 32)},
	} {
		if err := cfg.validate(); err == nil {
			t.Fatal("accepted invalid credentials")
		}
	}
	if err := (SiteConfig{AdminPassword: "a different password", SessionSecret: strings.Repeat("k", 32)}).validate(); err != nil {
		t.Fatal(err)
	}
}

func TestForeignSessionKeyCannotAuthenticate(t *testing.T) {
	a := New(SiteConfig{SessionSecret: strings.Repeat("a", 32)}, ViewFuncs{})
	request := httptest.NewRequest("GET", "/", nil)
	writer := httptest.NewRecorder()
	foreign := sessions.NewCookieStore([]byte(strings.Repeat("b", 32)))
	s, err := foreign.New(request, sessionName)
	if err != nil {
		t.Fatal(err)
	}
	s.Values["authenticated"] = true
	if err := s.Save(request, writer); err != nil {
		t.Fatal(err)
	}
	for _, cookie := range writer.Result().Cookies() {
		request.AddCookie(cookie)
	}
	c := a.Echo.NewContext(request, httptest.NewRecorder())
	err = session.Middleware(a.newSessionStore())(func(c echo.Context) error {
		if IsAdmin(c) {
			t.Error("foreign key authenticated")
		}
		return nil
	})(c)
	if err != nil {
		t.Fatal(err)
	}
}
