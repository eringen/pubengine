package htmlrender

import (
	"bytes"
	"fmt"
	"net/http"

	"github.com/a-h/templ"
	"github.com/labstack/echo/v4"
)

// Render writes a templ component as an HTTP 200 HTML response.
func Render(c echo.Context, cmp templ.Component) error {
	return RenderStatus(c, http.StatusOK, cmp)
}

// RenderStatus writes a templ component with a specific HTTP status code.
func RenderStatus(c echo.Context, code int, cmp templ.Component) error {
	var buf Buffer
	if cmp == nil {
		return fmt.Errorf("pubengine: missing view component")
	}
	if err := cmp.Render(c.Request().Context(), &buf); err != nil {
		return err
	}
	return c.Blob(code, echo.MIMETextHTMLCharsetUTF8, buf.Bytes())
}

const MaxSize = 16 << 20

type Buffer struct{ bytes.Buffer }

func (b *Buffer) WriteString(s string) (int, error) { return b.Write([]byte(s)) }

func (b *Buffer) Write(p []byte) (int, error) {
	if len(p) > MaxSize-b.Len() {
		return 0, fmt.Errorf("pubengine: rendered page exceeds 16 MB")
	}
	return b.Buffer.Write(p)
}
