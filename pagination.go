package pubengine

import (
	"context"
	"github.com/labstack/echo/v4"
	"net/url"
	"strconv"
)

type Pagination struct{ Previous, Next string }
type paginationKey struct{}

func PaginationFromContext(ctx context.Context) Pagination {
	p, _ := ctx.Value(paginationKey{}).(Pagination)
	return p
}
func pageOffset(c echo.Context, size int) int {
	page, err := strconv.Atoi(c.QueryParam("page"))
	if err != nil || page < 1 || page > 1000000 {
		page = 1
	}
	return (page - 1) * size
}
func setPagination(c echo.Context, size, offset int, more bool) {
	if size <= 0 {
		return
	}
	link := func(page int) string {
		u := *c.Request().URL
		q := u.Query()
		q.Del("partial")
		q.Set("page", strconv.Itoa(page))
		u.RawQuery = q.Encode()
		return u.String()
	}
	p := Pagination{}
	if offset > 0 {
		p.Previous = link(offset / size)
	}
	if more {
		p.Next = link(offset/size + 2)
	}
	c.SetRequest(c.Request().WithContext(context.WithValue(c.Request().Context(), paginationKey{}, p)))
}

// PostPath returns the canonical public path.
func PostPath(slug string) string { return "/blog/" + url.PathEscape(slug) + "/" }
