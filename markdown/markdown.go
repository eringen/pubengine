// Package markdown provides a simple Markdown-to-HTML renderer as a templ component.
package markdown

import (
	"bytes"
	"context"
	"html"
	"io"
	"net/url"
	"regexp"
	"strings"

	"github.com/a-h/templ"
)

var (
	reBold             = regexp.MustCompile(`\*\*(.+?)\*\*`)
	reBoldUnderscore   = regexp.MustCompile(`__(.+?)__`)
	reItalic           = regexp.MustCompile(`\*([^*]+)\*`)
	reItalicUnderscore = regexp.MustCompile(`_([^_]+)_`)
	reInlineCode       = regexp.MustCompile("`([^`]+)`")
	reLink             = regexp.MustCompile(`\[(.*?)\]\((.*?)\)(\^)?`)
	reOrderedList      = regexp.MustCompile(`^(\d+)\.\s`)
	// ![alt](url){style} or ![alt](url){style|width|height}
	reImg = regexp.MustCompile(`\!\[(.*?)\]\((.*?)\)\{([^|}]*?)(?:\|(\d+)\|(\d+))?\}`)
)

// Markdown returns a templ.Component that renders md as HTML.
func Markdown(content string) templ.Component {
	return templ.ComponentFunc(func(ctx context.Context, w io.Writer) error {
		var buf bytes.Buffer
		RenderMarkdown(&buf, content)
		_, err := w.Write(buf.Bytes())
		return err
	})
}

// RenderMarkdown writes the HTML representation of md to buf.
func RenderMarkdown(buf *bytes.Buffer, md string) {
	imageCount := 0
	lines := strings.Split(md, "\n")
	inList := false
	inOrderedList := false
	inPara := false
	inQuote := false
	inCode := false
	codeLang := false // whether the current code block has a language badge
	inTable := false
	tableHeaderDone := false

	flushCode := func() {
		if inCode {
			buf.WriteString("</code></pre>")
			if codeLang {
				buf.WriteString("</div>")
				codeLang = false
			}
			inCode = false
			inPara = false
		}
	}
	flushPara := func() {
		if inPara {
			buf.WriteString("</p>")
			inPara = false
		}
	}
	flushQuote := func() {
		if inQuote {
			buf.WriteString("</blockquote>")
			inQuote = false
		}
	}
	flushList := func() {
		if inList {
			buf.WriteString("</ul>")
			inList = false
		}
	}
	flushOrderedList := func() {
		if inOrderedList {
			buf.WriteString("</ol>")
			inOrderedList = false
		}
	}
	flushTable := func() {
		if inTable {
			if tableHeaderDone {
				buf.WriteString("</tbody>")
			}
			buf.WriteString("</table>")
			inTable = false
			tableHeaderDone = false
		}
	}

	for _, raw := range lines {
		line := strings.TrimRight(raw, "\r")
		if strings.HasPrefix(line, "```") {
			if inCode {
				flushCode()
			} else {
				flushPara()
				flushList()
				flushOrderedList()
				flushQuote()
				flushTable()
				lang := strings.TrimSpace(line[3:])
				if lang != "" {
					codeLang = true
					escapedLang := html.EscapeString(lang)
					buf.WriteString("<div class=\"code-block-wrapper\"><span class=\"code-lang code-lang-" + escapedLang + "\">" + escapedLang + "</span>")
					buf.WriteString("<pre class=\"code-block\"><code class=\"language-" + escapedLang + "\">")
				} else {
					buf.WriteString("<pre class=\"code-block\"><code>")
				}
				inCode = true
			}
			continue
		}

		if inCode {
			buf.WriteString(html.EscapeString(line))
			buf.WriteString("\n")
			continue
		}

		if strings.TrimSpace(line) == "" {
			flushPara()
			flushList()
			flushOrderedList()
			flushQuote()
			flushTable()
			continue
		}

		switch {
		case strings.HasPrefix(line, "---"):
			flushPara()
			flushList()
			flushOrderedList()
			flushQuote()
			flushTable()
			buf.WriteString("<hr/>")
		case strings.HasPrefix(line, "# "):
			flushPara()
			flushList()
			flushOrderedList()
			flushQuote()
			flushTable()
			buf.WriteString("<h1>")
			buf.WriteString(FormatInline(strings.TrimSpace(line[2:]), &imageCount))
			buf.WriteString("</h1>")
		case strings.HasPrefix(line, "## "):
			flushPara()
			flushList()
			flushOrderedList()
			flushQuote()
			flushTable()
			buf.WriteString("<h2>")
			buf.WriteString(FormatInline(strings.TrimSpace(line[3:]), &imageCount))
			buf.WriteString("</h2>")
		case strings.HasPrefix(line, "### "):
			flushPara()
			flushList()
			flushOrderedList()
			flushQuote()
			flushTable()
			buf.WriteString("<h3>")
			buf.WriteString(FormatInline(strings.TrimSpace(line[4:]), &imageCount))
			buf.WriteString("</h3>")
		case strings.HasPrefix(line, "|"):
			if !inTable {
				flushPara()
				flushList()
				flushOrderedList()
				flushQuote()
				buf.WriteString("<table>")
				inTable = true
				// First row is the header
				buf.WriteString("<thead><tr>")
				for _, cell := range parseTableCells(line) {
					buf.WriteString("<th>")
					buf.WriteString(FormatInline(cell, &imageCount))
					buf.WriteString("</th>")
				}
				buf.WriteString("</tr></thead>")
			} else if isTableSeparator(line) {
				// Skip separator line like |---|---|
				if !tableHeaderDone {
					buf.WriteString("<tbody>")
					tableHeaderDone = true
				}
			} else {
				if !tableHeaderDone {
					buf.WriteString("<tbody>")
					tableHeaderDone = true
				}
				buf.WriteString("<tr>")
				for _, cell := range parseTableCells(line) {
					buf.WriteString("<td>")
					buf.WriteString(FormatInline(cell, &imageCount))
					buf.WriteString("</td>")
				}
				buf.WriteString("</tr>")
			}
		case strings.HasPrefix(line, "- "):
			if !inList {
				flushPara()
				flushOrderedList()
				flushQuote()
				flushTable()
				buf.WriteString("<ul>")
				inList = true
			}
			buf.WriteString("<li>")
			buf.WriteString(FormatInline(strings.TrimSpace(line[2:]), &imageCount))
			buf.WriteString("</li>")
		case reOrderedList.MatchString(line):
			if !inOrderedList {
				flushPara()
				flushList()
				flushQuote()
				flushTable()
				buf.WriteString("<ol>")
				inOrderedList = true
			}
			content := reOrderedList.ReplaceAllString(line, "")
			buf.WriteString("<li>")
			buf.WriteString(FormatInline(strings.TrimSpace(content), &imageCount))
			buf.WriteString("</li>")
		case strings.HasPrefix(line, "> "):
			if !inQuote {
				flushPara()
				flushList()
				flushOrderedList()
				flushTable()
				buf.WriteString("<blockquote>")
				inQuote = true
			}
			buf.WriteString(FormatInline(strings.TrimSpace(line[2:]), &imageCount))
		default:
			if !inPara {
				flushList()
				flushOrderedList()
				flushQuote()
				flushTable()
				buf.WriteString("<p>")
				inPara = true
			} else {
				buf.WriteString(" ")
			}
			buf.WriteString(FormatInline(strings.TrimSpace(line), &imageCount) + "\n")
		}
	}
	flushPara()
	flushList()
	flushOrderedList()
	flushQuote()
	flushTable()
	flushCode()
}

func parseTableCells(line string) []string {
	line = strings.TrimSpace(line)
	line = strings.Trim(line, "|")
	parts := strings.Split(line, "|")
	for i, p := range parts {
		parts[i] = strings.TrimSpace(p)
	}
	return parts
}

func isTableSeparator(line string) bool {
	line = strings.TrimSpace(line)
	line = strings.Trim(line, "|")
	for _, cell := range strings.Split(line, "|") {
		cell = strings.TrimSpace(cell)
		cleaned := strings.ReplaceAll(strings.ReplaceAll(cell, "-", ""), ":", "")
		if cleaned != "" {
			return false
		}
	}
	return true
}

// ApplyOutsideTags applies fn only to text segments outside HTML tags,
// so that formatting regexes never touch URLs inside href attributes, etc.
func ApplyOutsideTags(s string, fn func(string) string) string {
	var buf strings.Builder
	for len(s) > 0 {
		lt := strings.Index(s, "<")
		if lt < 0 {
			buf.WriteString(fn(s))
			break
		}
		if lt > 0 {
			buf.WriteString(fn(s[:lt]))
		}
		gt := strings.Index(s[lt:], ">")
		if gt < 0 {
			buf.WriteString(s[lt:])
			break
		}
		buf.WriteString(s[lt : lt+gt+1])
		s = s[lt+gt+1:]
	}
	return buf.String()
}

// FormatInline applies inline formatting (bold, italic, links, images) to s.
var reInlineToken = regexp.MustCompile(reInlineCode.String() + "|" + reImg.String() + "|" + reLink.String())

func FormatInline(s string, imageCount *int) string {
	var out strings.Builder
	offset := 0
	for _, loc := range reInlineToken.FindAllStringIndex(s, -1) {
		out.WriteString(formatText(s[offset:loc[0]]))
		token := s[loc[0]:loc[1]]
		switch {
		case strings.HasPrefix(token, "`"):
			out.WriteString("<code>" + html.EscapeString(token[1:len(token)-1]) + "</code>")
		case strings.HasPrefix(token, "!["):
			match := reImg.FindStringSubmatch(token)
			src := SafeURL(match[2])
			alt := html.EscapeString(match[1])
			if src == "" {
				out.WriteString(alt)
				break
			}
			width, height := "1024", "768"
			if match[4] != "" && match[5] != "" {
				width, height = match[4], match[5]
			}
			*imageCount++
			load := `loading="eager"`
			if *imageCount == 1 {
				load = `fetchpriority="high"`
			}
			out.WriteString(`<img ` + load + ` width="` + width + `" height="` + height + `" alt="` + alt + `" src="` + src + `" style="` + html.EscapeString(match[3]) + `" decoding="async"/>`)
		default:
			match := reLink.FindStringSubmatch(token)
			href := SafeURL(match[2])
			label := formatText(match[1])
			if href == "" {
				out.WriteString(label)
				break
			}
			attrs := `class="underline decoration-2 underline-offset-4"`
			if match[3] == "^" {
				attrs += ` target="_blank" rel="noopener noreferrer"`
			}
			out.WriteString(`<a href="` + href + `" ` + attrs + `>` + label + `</a>`)
		}
		offset = loc[1]
	}
	out.WriteString(formatText(s[offset:]))
	return out.String()
}

func formatText(s string) string {
	s = html.EscapeString(s)
	s = reBold.ReplaceAllString(s, "<strong>$1</strong>")
	s = reBoldUnderscore.ReplaceAllString(s, "<strong>$1</strong>")
	s = reItalic.ReplaceAllString(s, "<em>$1</em>")
	return reItalicUnderscore.ReplaceAllString(s, "<em>$1</em>")
}

// SafeURL validates and sanitizes a URL for use in HTML attributes.
func SafeURL(raw string) string {
	val := strings.TrimSpace(html.UnescapeString(raw))
	if val == "" {
		return ""
	}
	if strings.HasPrefix(val, "/") || strings.HasPrefix(val, "#") {
		return html.EscapeString(val)
	}
	parsed, err := url.Parse(val)
	if err != nil || parsed.Scheme == "" {
		return ""
	}
	switch strings.ToLower(parsed.Scheme) {
	case "http", "https", "mailto", "tel":
		return html.EscapeString(val)
	default:
		return ""
	}
}
