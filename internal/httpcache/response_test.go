package httpcache

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/labstack/echo/v4"
)

func TestConditionalResponseEligibility(t *testing.T) {
	for _, tc := range []struct {
		name, method, path, policy string
		status                     int
		cached                     bool
	}{
		{"public", "GET", "/blog/post/", "", 200, true},
		{"head", "HEAD", "/blog/post/", "", 200, true},
		{"admin", "GET", "/admin/", "", 200, false},
		{"api", "GET", "/api/data", "", 200, false},
		{"private", "GET", "/preview/", "no-store", 200, false},
		{"write", "POST", "/save/", "", 200, false},
		{"error", "GET", "/missing/", "", 404, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			r := httptest.NewRequest(tc.method, tc.path, nil)
			r.Header.Set("If-None-Match", "*")
			w := httptest.NewRecorder()
			c := echo.New().NewContext(r, w)
			c.Response().Header().Set("Cache-Control", tc.policy)
			if err := Bytes(c, tc.status, "text/plain", []byte("body")); err != nil {
				t.Fatal(err)
			}
			if tc.cached {
				if w.Code != http.StatusNotModified || w.Header().Get("ETag") == "" || w.Body.Len() != 0 {
					t.Fatal(w)
				}
			} else if w.Code != tc.status || w.Header().Get("ETag") != "" || w.Body.String() != "body" {
				t.Fatal(w)
			}
		})
	}
}
