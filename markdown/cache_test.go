package markdown

import (
	"context"
	"io"
	"strings"
	"testing"
)

func TestCachedMarkdownChangesAndLimits(t *testing.T) {
	for _, text := range []string{"**before**", "**after**", "**before**"} {
		body, err := cachedMarkdown(text)
		if err != nil || !strings.Contains(string(body), "<strong>"+strings.Trim(text, "*")+"</strong>") {
			t.Fatal(string(body), err)
		}
	}
	if _, err := cachedMarkdown(strings.Repeat("x", maxInput+1)); err == nil {
		t.Fatal("oversized input accepted")
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if err := Markdown("text").Render(ctx, io.Discard); err == nil {
		t.Fatal("cancellation ignored")
	}
	var out boundedOutput
	if _, err := out.WriteString(strings.Repeat("x", maxOutput+1)); err == nil || out.Len() != 0 {
		t.Fatal("output limit failed")
	}
}

func BenchmarkMarkdown(b *testing.B) {
	text := strings.Repeat("## Heading\n\n**Text** [link](https://example.test) and `code`.\n\n", 100)
	component := Markdown(text)
	if err := component.Render(context.Background(), io.Discard); err != nil {
		b.Fatal(err)
	}
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		if err := component.Render(context.Background(), io.Discard); err != nil {
			b.Fatal(err)
		}
	}
}
