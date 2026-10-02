package httpcache

import (
	"crypto/sha256"
	"fmt"
	"github.com/labstack/echo/v4"
	"net/http"
	"strings"
)

func ETag(body []byte) string { return fmt.Sprintf(`W/"%x"`, sha256.Sum256(body)) }
func Bytes(c echo.Context, code int, contentType string, body []byte) error {
	tag := ""
	if cacheable(c, code) {
		tag = ETag(body)
	}
	return TaggedBytes(c, code, contentType, body, tag)
}
func TaggedBytes(c echo.Context, code int, contentType string, body []byte, tag string) error {
	method := c.Request().Method
	if tag != "" && cacheable(c, code) {
		c.Response().Header().Set("ETag", tag)
		for _, candidate := range strings.Split(c.Request().Header.Get("If-None-Match"), ",") {
			candidate = strings.TrimSpace(candidate)
			if candidate == "*" || strings.TrimPrefix(candidate, "W/") == strings.TrimPrefix(tag, "W/") {
				return c.NoContent(http.StatusNotModified)
			}
		}
	}
	if method == http.MethodHead {
		c.Response().Header().Set("Content-Type", contentType)
		return c.NoContent(code)
	}
	return c.Blob(code, contentType, body)
}

func cacheable(c echo.Context, code int) bool {
	method, path := c.Request().Method, c.Request().URL.Path
	return code == http.StatusOK && (method == http.MethodGet || method == http.MethodHead) &&
		!strings.HasPrefix(path, "/admin") && !strings.HasPrefix(path, "/api/") &&
		!strings.Contains(c.Response().Header().Get("Cache-Control"), "no-store")
}
