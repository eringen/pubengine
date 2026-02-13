package pubengine

import (
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
	c.Response().Header().Set(echo.HeaderContentType, echo.MIMETextHTMLCharsetUTF8)
	c.Response().WriteHeader(code)
	return cmp.Render(c.Request().Context(), c.Response().Writer)
}
