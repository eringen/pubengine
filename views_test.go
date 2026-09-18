package pubengine

import (
	"context"
	"io"
	"strings"
	"testing"

	"github.com/a-h/templ"
)

func TestOptionalViews(t *testing.T) {
	a := New(SiteConfig{}, ViewFuncs{})
	err := a.Views.Home(nil, "", nil, "").Render(context.Background(), io.Discard)
	if err == nil || !strings.Contains(err.Error(), "missing Home view") {
		t.Fatal(err)
	}
	for _, component := range []templ.Component{a.Views.NotFound(), a.Views.ServerError()} {
		if err := component.Render(context.Background(), io.Discard); err != nil {
			t.Fatal(err)
		}
	}
	called := false
	v := defaultViews(ViewFuncs{Home: func([]BlogPost, string, []string, string) templ.Component {
		called = true
		return templ.Raw("home")
	}})
	v.HomePartial(nil, "", nil, "")
	if !called {
		t.Fatal("partial did not use the full view")
	}
}
