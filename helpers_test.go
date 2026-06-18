package pubengine

import (
	"net/url"
	"testing"
)

func TestQueryEscapingRoundTrip(t *testing.T) {
	for _, tag := range []string{"c++", "a&b", "hello world", "Türkçe", "x=y"} {
		u, err := url.Parse("/?tag=" + QueryEscape(tag))
		if err != nil {
			t.Fatal(err)
		}
		if u.Query().Get("tag") != tag {
			t.Fatal(tag, u)
		}
	}
}
