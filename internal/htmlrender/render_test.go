package htmlrender

import (
	"context"
	"errors"
	"io"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/a-h/templ"
	"github.com/labstack/echo/v4"
)

func TestRenderAtomicAndIsolated(t *testing.T) {
	e := echo.New()
	render := func(body string, fail bool) *httptest.ResponseRecorder {
		w := httptest.NewRecorder()
		c := e.NewContext(httptest.NewRequest("GET", "/", nil), w)
		err := Render(c, templ.ComponentFunc(func(_ context.Context, out io.Writer) error {
			if _, err := io.WriteString(out, body); err != nil {
				return err
			}
			if fail {
				return errors.New("render failed")
			}
			return nil
		}))
		if (err != nil) != fail {
			t.Fatalf("unexpected error: %v", err)
		}
		if fail && (w.Body.Len() != 0 || c.Response().Committed) {
			t.Fatal("partial render was sent")
		}
		return w
	}
	render("private partial content", true)
	if w := render("public", false); w.Body.String() != "public" {
		t.Fatal(w.Body.String())
	}
	render(strings.Repeat("x", 256<<10), false)
	if w := render("small", false); w.Body.String() != "small" {
		t.Fatal(w.Body.String())
	}
}

func TestRenderLimit(t *testing.T) {
	for _, useString := range []bool{false, true} {
		var b Buffer
		b.Grow(MaxSize)
		b.Buffer.Write(make([]byte, MaxSize-1))
		var err error
		if useString {
			_, err = b.WriteString("xx")
		} else {
			_, err = b.Write([]byte("xx"))
		}
		if err == nil || b.Len() != MaxSize-1 {
			t.Fatal("render limit bypassed")
		}
	}
}

func BenchmarkRender(b *testing.B) {
	body := strings.Repeat("<p>Readable article content.</p>", 1024)
	cmp := templ.ComponentFunc(func(_ context.Context, w io.Writer) error { _, err := io.WriteString(w, body); return err })
	for _, path := range []string{"/blog/post/", "/admin/"} {
		b.Run(path, func(b *testing.B) {
			e := echo.New()
			req := httptest.NewRequest("GET", path, nil)
			w := httptest.NewRecorder()
			c := e.NewContext(req, w)
			b.ReportAllocs()
			b.ResetTimer()
			for i := 0; i < b.N; i++ {
				w.Body.Reset()
				c.Reset(req, w)
				if err := Render(c, cmp); err != nil {
					b.Fatal(err)
				}
			}
		})
	}
}
