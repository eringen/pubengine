package pubengine

import (
	"github.com/a-h/templ"
	"github.com/eringen/pubengine/internal/htmlrender"
	"github.com/labstack/echo/v4"
)

func Render(c echo.Context, cmp templ.Component) error { return htmlrender.Render(c, cmp) }
func RenderStatus(c echo.Context, code int, cmp templ.Component) error {
	return htmlrender.RenderStatus(c, code, cmp)
}

const maxRenderSize = htmlrender.MaxSize

type renderBuffer = htmlrender.Buffer
