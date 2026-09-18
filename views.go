package pubengine

import (
	"context"
	"fmt"
	"io"

	"github.com/a-h/templ"
)

func missingView(name string) templ.Component {
	return templ.ComponentFunc(func(context.Context, io.Writer) error { return fmt.Errorf("pubengine: missing %s view", name) })
}

func defaultViews(v ViewFuncs) ViewFuncs {
	if v.Home == nil {
		v.Home = func([]BlogPost, string, []string, string) templ.Component { return missingView("Home") }
	}
	if v.HomePartial == nil {
		v.HomePartial = v.Home
	}
	if v.BlogSection == nil {
		v.BlogSection = func([]BlogPost, string, []string) templ.Component { return missingView("BlogSection") }
	}
	if v.Post == nil {
		v.Post = func(BlogPost, []BlogPost, string) templ.Component { return missingView("Post") }
	}
	if v.PostPartial == nil {
		v.PostPartial = v.Post
	}
	if v.AdminLogin == nil {
		v.AdminLogin = func(string, string, string) templ.Component { return missingView("AdminLogin") }
	}
	if v.AdminDashboard == nil {
		v.AdminDashboard = func([]BlogPost, string, string) templ.Component { return missingView("AdminDashboard") }
	}
	if v.AdminFormPartial == nil {
		v.AdminFormPartial = func(BlogPost, string) templ.Component { return missingView("AdminFormPartial") }
	}
	if v.AdminImages == nil {
		v.AdminImages = func([]Image, string) templ.Component { return missingView("AdminImages") }
	}
	if v.NotFound == nil {
		v.NotFound = func() templ.Component { return templ.Raw("<!DOCTYPE html><title>Not found</title><h1>Not found</h1>") }
	}
	if v.ServerError == nil {
		v.ServerError = func() templ.Component {
			return templ.Raw("<!DOCTYPE html><title>Server error</title><h1>Server error</h1>")
		}
	}
	return v
}
