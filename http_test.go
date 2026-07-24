package pubengine

import (
	"context"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/a-h/templ"
	"github.com/labstack/echo/v4"
)

func testHTTPApp(t *testing.T) *App {
	t.Helper()
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "favicon.svg"), []byte("<svg/>"), 0o644); err != nil {
		t.Fatal(err)
	}
	a := New(SiteConfig{SessionSecret: strings.Repeat("a", 32)}, ViewFuncs{}, WithStaticDir(dir))
	a.setupMiddleware()
	a.setupRoutes()
	return a
}

func TestCanonicalRoutesAndCaching(t *testing.T) {
	a := testHTTPApp(t)
	for _, tc := range []struct {
		path     string
		code     int
		location string
	}{
		{"/blog", 308, "/blog/"}, {"/blog/", 301, "/"}, {"/favicon.svg", 200, ""}, {"/public/missing.js", 404, ""},
	} {
		r := httptest.NewRecorder()
		a.Echo.ServeHTTP(r, httptest.NewRequest("GET", tc.path, nil))
		if r.Code != tc.code || r.Header().Get("Location") != tc.location {
			t.Fatalf("%s => %d %s", tc.path, r.Code, r.Header().Get("Location"))
		}
		cache := r.Header().Get("Cache-Control")
		if strings.Contains(cache, "immutable") {
			t.Fatal(cache)
		}
		if r.Code >= 400 && cache != "no-store" {
			t.Fatal(cache)
		}
	}
	r := httptest.NewRecorder()
	a.Echo.ServeHTTP(r, httptest.NewRequest("POST", "/admin/save", strings.NewReader("content=hello")))
	if r.Code != http.StatusPermanentRedirect {
		t.Fatalf("mutation redirect=%d", r.Code)
	}
}

func TestBodyLimitsBeforeCSRFAndParsing(t *testing.T) {
	a := testHTTPApp(t)
	for _, tc := range []struct {
		path string
		size int
	}{
		{"/admin/login/", 17 << 10}, {"/admin/save/", (2 << 20) + 1}, {"/admin/images/upload/", maxUploadSize + (1 << 20) + 1}, {"/api/analytics/collect", 17 << 10},
	} {
		for _, chunked := range []bool{false, true} {
			req := httptest.NewRequest("POST", tc.path, strings.NewReader(strings.Repeat("x", tc.size)))
			req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
			if chunked {
				req.ContentLength = -1
			}
			r := httptest.NewRecorder()
			a.Echo.ServeHTTP(r, req)
			if r.Code != 413 {
				t.Fatalf("%s chunked=%v status=%d", tc.path, chunked, r.Code)
			}
		}
	}
}

func TestRenderFailuresDoNotCommitSuccess(t *testing.T) {
	for _, partial := range []bool{false, true} {
		a := testHTTPApp(t)
		a.Echo.GET("/broken/", func(c echo.Context) error {
			return Render(c, templ.ComponentFunc(func(_ context.Context, w io.Writer) error {
				if partial {
					_, _ = io.WriteString(w, "partial page")
				}
				return errors.New("template failed")
			}))
		})
		r := httptest.NewRecorder()
		a.Echo.ServeHTTP(r, httptest.NewRequest("GET", "/broken/", nil))
		if r.Code != 500 || strings.Contains(r.Body.String(), "partial page") || r.Header().Get("Cache-Control") != "no-store" {
			t.Fatalf("%d %s %v", r.Code, r.Body, r.Header())
		}
	}
	var buf renderBuffer
	if _, err := io.WriteString(&buf, strings.Repeat("x", maxRenderSize+1)); err == nil {
		t.Fatal("render limit bypassed")
	}
}

func TestConfiguredIPExtractorSurvivesMiddlewareSetup(t *testing.T) {
	a := New(SiteConfig{}, ViewFuncs{}, WithIPExtractor(echo.ExtractIPDirect()))
	a.setupMiddleware()
	req := httptest.NewRequest("GET", "/", nil)
	req.RemoteAddr = "192.0.2.10:1234"
	req.Header.Set("X-Forwarded-For", "203.0.113.22")
	if got := a.Echo.NewContext(req, httptest.NewRecorder()).RealIP(); got != "192.0.2.10" {
		t.Fatal(got)
	}
}

func TestCustomCachePoliciesAndLLMSArePreserved(t *testing.T) {
	a := testHTTPApp(t)
	a.Echo.GET("/private/", func(c echo.Context) error {
		c.Response().Header().Set("Cache-Control", "private, no-store")
		return c.String(200, "private")
	})
	a.Echo.GET("/llms.txt", func(c echo.Context) error { return c.String(200, "site info") })
	for path, want := range map[string]string{"/private/": "private, no-store", "/llms.txt": "public, max-age=86400"} {
		r := httptest.NewRecorder()
		a.Echo.ServeHTTP(r, httptest.NewRequest("GET", path, nil))
		if r.Code != 200 || r.Header().Get("Cache-Control") != want {
			t.Fatal(path, r.Code, r.Header())
		}
	}
}
