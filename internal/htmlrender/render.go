package htmlrender

import (
	"bytes"
	"fmt"
	"net/http"
	"sync"

	"github.com/a-h/templ"
	"github.com/eringen/pubengine/internal/httpcache"
	"github.com/labstack/echo/v4"
)

// Retain ordinary page buffers, but let large article renders be collected.
// Reset each buffer before reuse; no rendered representation is cached.
const maxPooledCapacity = 64 << 10

var buffers = sync.Pool{New: func() any { return new(Buffer) }}

func releaseBuffer(buf *Buffer) {
	if buf.Cap() > maxPooledCapacity {
		return
	}
	buf.Reset()
	buffers.Put(buf)
}

// Render writes a templ component as an HTTP 200 HTML response.
func Render(c echo.Context, cmp templ.Component) error {
	return RenderStatus(c, http.StatusOK, cmp)
}

// RenderStatus writes a templ component with a specific HTTP status code.
func RenderStatus(c echo.Context, code int, cmp templ.Component) error {
	if cmp == nil {
		return fmt.Errorf("pubengine: missing view component")
	}
	buf := buffers.Get().(*Buffer)
	defer releaseBuffer(buf)
	if err := cmp.Render(c.Request().Context(), buf); err != nil {
		return err
	}
	return httpcache.Bytes(c, code, echo.MIMETextHTMLCharsetUTF8, buf.Bytes())
}

const MaxSize = 16 << 20

type Buffer struct{ bytes.Buffer }

func (b *Buffer) WriteString(s string) (int, error) {
	if len(s) > MaxSize-b.Len() {
		return 0, fmt.Errorf("pubengine: rendered page exceeds 16 MB")
	}
	return b.Buffer.WriteString(s)
}

func (b *Buffer) Write(p []byte) (int, error) {
	if len(p) > MaxSize-b.Len() {
		return 0, fmt.Errorf("pubengine: rendered page exceeds 16 MB")
	}
	return b.Buffer.Write(p)
}
