package markdown

import (
	"bytes"
	"strings"
	"testing"

	"golang.org/x/net/html"
)

func TestCodeAndImageAttributesStayLiteral(t *testing.T) {
	if got := FormatInline("`[x](https://example.com)`", new(int)); got != "<code>[x](https://example.com)</code>" {
		t.Fatal(got)
	}
	got := FormatInline(`![x](/x.jpg){font-family:[x](https://example.com)}`, new(int))
	doc, err := html.Parse(strings.NewReader(got))
	if err != nil {
		t.Fatal(err)
	}
	var visit func(*html.Node)
	found := false
	visit = func(n *html.Node) {
		if n.Type == html.ElementNode && n.Data == "a" {
			t.Fatal("link created in image attribute")
		}
		if n.Type == html.ElementNode && n.Data == "img" {
			found = true
			for _, a := range n.Attr {
				if a.Key == "style" && a.Val != "font-family:[x](https://example.com)" {
					t.Fatal(a.Val)
				}
			}
		}
		for c := n.FirstChild; c != nil; c = c.NextSibling {
			visit(c)
		}
	}
	visit(doc)
	if !found {
		t.Fatal("missing image")
	}
}

func TestFenceBlockBoundaries(t *testing.T) {
	for _, input := range []string{"```go\nx", "|header|\n|---|\n```go\nx\n```"} {
		var out bytes.Buffer
		RenderMarkdown(&out, input)
		got := out.String()
		if strings.Contains(got, "</p>") {
			t.Fatal(got)
		}
		if strings.HasPrefix(input, "|") && !strings.Contains(got, "</table><div") {
			t.Fatal(got)
		}
		if !strings.HasSuffix(got, "</code></pre></div>") {
			t.Fatal(got)
		}
	}
}

func TestDangerousURLsAndQuotesRemainInert(t *testing.T) {
	for _, md := range []string{`[x](javascript:alert)`, `![x](/a" onerror="alert){color:red}`, "`![x](/a){}`"} {
		out := FormatInline(md, new(int))
		doc, err := html.Parse(strings.NewReader(out))
		if err != nil {
			t.Fatal(err)
		}
		var visit func(*html.Node)
		visit = func(n *html.Node) {
			for _, a := range n.Attr {
				if strings.HasPrefix(a.Key, "on") || strings.HasPrefix(a.Val, "javascript:") {
					t.Fatal(out)
				}
			}
			for c := n.FirstChild; c != nil; c = c.NextSibling {
				visit(c)
			}
		}
		visit(doc)
	}
}
