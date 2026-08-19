package pubengine

import (
	"compress/gzip"
	"io"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func TestEmbeddedCompressionAndValidators(t *testing.T) {
	a := testHTTPApp(t)
	r := httptest.NewRequest("GET", "/public/talkdom.js", nil)
	r.Header.Set("Accept-Encoding", "gzip")
	w := httptest.NewRecorder()
	a.Echo.ServeHTTP(w, r)
	if w.Code != 200 || w.Header().Get("Content-Encoding") != "gzip" || w.Header().Get("ETag") == "" {
		t.Fatal(w.Code, w.Header())
	}
	z, err := gzip.NewReader(w.Body)
	if err != nil {
		t.Fatal(err)
	}
	body, err := io.ReadAll(z)
	z.Close()
	if err != nil || !strings.Contains(string(body), "talkDOM") {
		t.Fatal(err)
	}
	for _, method := range []string{"GET", "HEAD"} {
		req := httptest.NewRequest(method, "/public/talkdom.js", nil)
		req.Header.Set("If-None-Match", `"other", `+w.Header().Get("ETag"))
		req.Header.Set("Accept-Encoding", "gzip")
		res := httptest.NewRecorder()
		a.Echo.ServeHTTP(res, req)
		if res.Code != 304 || res.Body.Len() != 0 {
			t.Fatal(res.Code, res.Body)
		}
	}
}

func TestFeedCacheInvalidation(t *testing.T) {
	a := testHTTPApp(t)
	s, err := NewStore(":memory:")
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	a.Store = s
	a.Cache = NewPostCache(s, time.Hour)
	if err := s.SavePost(BlogPost{Slug: "post", Title: "Before", Published: true}); err != nil {
		t.Fatal(err)
	}
	request := func(tag string) *httptest.ResponseRecorder {
		req := httptest.NewRequest("GET", "/feed.xml", nil)
		req.Header.Set("If-None-Match", tag)
		res := httptest.NewRecorder()
		a.Echo.ServeHTTP(res, req)
		return res
	}
	first := request("")
	if first.Code != 200 {
		t.Fatal(first.Code, first.Body)
	}
	tag := first.Header().Get("ETag")
	if res := request(tag); res.Code != 304 {
		t.Fatal(res.Code)
	}
	post, _ := s.GetPost("post")
	post.Title = "After"
	if err := s.SavePost(post); err != nil {
		t.Fatal(err)
	}
	a.Cache.Invalidate()
	res := request(tag)
	if res.Code != 200 || !strings.Contains(res.Body.String(), "After") || res.Header().Get("ETag") == tag {
		t.Fatal(res.Code, res.Body)
	}
	a.Config.Name = "Renamed"
	res = request(res.Header().Get("ETag"))
	if res.Code != 200 || !strings.Contains(res.Body.String(), "Renamed") {
		t.Fatal(res.Code, res.Body)
	}
}
