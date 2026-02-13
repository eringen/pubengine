package markdown

import (
	"bytes"
	"strings"
	"testing"
)

func TestFormatInlineBold(t *testing.T) {
	tests := []struct {
		input    string
		expected string
	}{
		{"**bold**", "<strong>bold</strong>"},
		{"__bold__", "<strong>bold</strong>"},
		{"text **bold** more", "text <strong>bold</strong> more"},
	}
	for _, tt := range tests {
		got := FormatInline(tt.input, new(int))
		if got != tt.expected {
			t.Errorf("FormatInline(%q) = %q, want %q", tt.input, got, tt.expected)
		}
	}
}

func TestFormatInlineItalic(t *testing.T) {
	tests := []struct {
		input    string
		expected string
	}{
		{"*italic*", "<em>italic</em>"},
		{"_italic_", "<em>italic</em>"},
		{"text *italic* more", "text <em>italic</em> more"},
	}
	for _, tt := range tests {
		got := FormatInline(tt.input, new(int))
		if got != tt.expected {
			t.Errorf("FormatInline(%q) = %q, want %q", tt.input, got, tt.expected)
		}
	}
}

func TestFormatInlineNested(t *testing.T) {
	tests := []struct {
		input    string
		expected string
	}{
		{"**bold *italic* text**", "<strong>bold <em>italic</em> text</strong>"},
		{"__bold _italic_ text__", "<strong>bold <em>italic</em> text</strong>"},
	}
	for _, tt := range tests {
		got := FormatInline(tt.input, new(int))
		if got != tt.expected {
			t.Errorf("FormatInline(%q) = %q, want %q", tt.input, got, tt.expected)
		}
	}
}

func TestFormatInlineBoldNotMatchedAsItalic(t *testing.T) {
	input := "**bold**"
	got := FormatInline(input, new(int))
	if strings.Contains(got, "<em>") {
		t.Errorf("FormatInline(%q) = %q, should not contain <em>", input, got)
	}
}

func TestRenderMarkdownCodeBlock(t *testing.T) {
	input := "```\ncode here\n```"
	var buf bytes.Buffer
	RenderMarkdown(&buf, input)
	got := buf.String()
	if !strings.Contains(got, "<pre") || !strings.Contains(got, "<code>") {
		t.Errorf("RenderMarkdown code block failed: %q", got)
	}
	if !strings.Contains(got, "code here") {
		t.Errorf("RenderMarkdown code block missing content: %q", got)
	}
}

func TestRenderMarkdownHeadings(t *testing.T) {
	tests := []struct {
		input    string
		expected string
	}{
		{"# Heading 1", "<h1>Heading 1</h1>"},
		{"## Heading 2", "<h2>Heading 2</h2>"},
		{"### Heading 3", "<h3>Heading 3</h3>"},
	}
	for _, tt := range tests {
		var buf bytes.Buffer
		RenderMarkdown(&buf, tt.input)
		got := buf.String()
		if got != tt.expected {
			t.Errorf("RenderMarkdown(%q) = %q, want %q", tt.input, got, tt.expected)
		}
	}
}

func TestFormatInlineLinkWithUnderscoresInURL(t *testing.T) {
	tests := []struct {
		input    string
		expected string
	}{
		{
			"[Wikipedia](https://en.wikipedia.org/wiki/Some_Article_Title)",
			`<a href="https://en.wikipedia.org/wiki/Some_Article_Title" class="underline decoration-2 underline-offset-4">Wikipedia</a>`,
		},
		{
			"Visit [link](https://example.com/my_page/sub_path) for info",
			`Visit <a href="https://example.com/my_page/sub_path" class="underline decoration-2 underline-offset-4">link</a> for info`,
		},
		{
			"[link](https://example.com/a_b_c/d_e)",
			`<a href="https://example.com/a_b_c/d_e" class="underline decoration-2 underline-offset-4">link</a>`,
		},
	}
	for _, tt := range tests {
		got := FormatInline(tt.input, new(int))
		if got != tt.expected {
			t.Errorf("FormatInline(%q)\n  got:  %q\n  want: %q", tt.input, got, tt.expected)
		}
	}
}

func TestFormatInlineLinkNewTab(t *testing.T) {
	tests := []struct {
		input    string
		expected string
	}{
		{
			"[Google](https://google.com)^",
			`<a href="https://google.com" class="underline decoration-2 underline-offset-4" target="_blank" rel="noopener noreferrer">Google</a>`,
		},
		{
			"[Google](https://google.com)",
			`<a href="https://google.com" class="underline decoration-2 underline-offset-4">Google</a>`,
		},
		{
			"Check [this](https://example.com)^ out",
			`Check <a href="https://example.com" class="underline decoration-2 underline-offset-4" target="_blank" rel="noopener noreferrer">this</a> out`,
		},
	}
	for _, tt := range tests {
		got := FormatInline(tt.input, new(int))
		if got != tt.expected {
			t.Errorf("FormatInline(%q)\n  got:  %q\n  want: %q", tt.input, got, tt.expected)
		}
	}
}

func TestRenderMarkdownList(t *testing.T) {
	input := "- item 1\n- item 2"
	var buf bytes.Buffer
	RenderMarkdown(&buf, input)
	got := buf.String()
	expected := "<ul><li>item 1</li><li>item 2</li></ul>"
	if got != expected {
		t.Errorf("RenderMarkdown(%q) = %q, want %q", input, got, expected)
	}
}
