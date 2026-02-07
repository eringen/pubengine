package views

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
	reLink             = regexp.MustCompile(`\[(.*?)\]\((.*?)\)(\^)?`)
	// ![alt](url){style} or ![alt](url){style|width|height}
	reImg = regexp.MustCompile(`\!\[(.*?)\]\((.*?)\)\{([^|}]*?)(?:\|(\d+)\|(\d+))?\}`)

)

func Markdown(content string) templ.Component {
	return templ.ComponentFunc(func(ctx context.Context, w io.Writer) error {
		var buf bytes.Buffer
		renderMarkdown(&buf, content)
		_, err := w.Write(buf.Bytes())
		return err
	})
}

func renderMarkdown(buf *bytes.Buffer, md string) {
	imageCount := 0
	lines := strings.Split(md, "\n")
	inList := false
	inPara := false
	inQuote := false
	inCode := false
	inTable := false
	tableHeaderDone := false

	flushCode := func() {
		if inCode {
			buf.WriteString("</code></pre>")
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
				flushQuote()
				buf.WriteString("<pre class=\"code-block\"><code>")
				inCode = true
				inPara = true
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
			flushQuote()
			flushTable()
			continue
		}

		switch {
		case strings.HasPrefix(line, "---"):
			flushPara()
			flushList()
			flushQuote()
			flushTable()
			buf.WriteString("<hr/>")
		case strings.HasPrefix(line, "# "):
			flushPara()
			flushList()
			flushQuote()
			flushTable()
			buf.WriteString("<h1>")
			buf.WriteString(formatInline(strings.TrimSpace(line[2:]), &imageCount))
			buf.WriteString("</h1>")
		case strings.HasPrefix(line, "## "):
			flushPara()
			flushList()
			flushQuote()
			flushTable()
			buf.WriteString("<h2>")
			buf.WriteString(formatInline(strings.TrimSpace(line[3:]), &imageCount))
			buf.WriteString("</h2>")
		case strings.HasPrefix(line, "### "):
			flushPara()
			flushList()
			flushQuote()
			flushTable()
			buf.WriteString("<h3>")
			buf.WriteString(formatInline(strings.TrimSpace(line[4:]), &imageCount))
			buf.WriteString("</h3>")
		case strings.HasPrefix(line, "|"):
			if !inTable {
				flushPara()
				flushList()
				flushQuote()
				buf.WriteString("<table>")
				inTable = true
				// First row is the header
				buf.WriteString("<thead><tr>")
				for _, cell := range parseTableCells(line) {
					buf.WriteString("<th>")
					buf.WriteString(formatInline(cell, &imageCount))
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
					buf.WriteString(formatInline(cell, &imageCount))
					buf.WriteString("</td>")
				}
				buf.WriteString("</tr>")
			}
		case strings.HasPrefix(line, "- "):
			if !inList {
				flushPara()
				flushQuote()
				flushTable()
				buf.WriteString("<ul>")
				inList = true
			}
			buf.WriteString("<li>")
			buf.WriteString(formatInline(strings.TrimSpace(line[2:]), &imageCount))
			buf.WriteString("</li>")
		case strings.HasPrefix(line, "> "):
			if !inQuote {
				flushPara()
				flushList()
				flushTable()
				buf.WriteString("<blockquote>")
				inQuote = true
			}
			buf.WriteString(formatInline(strings.TrimSpace(line[2:]), &imageCount))
		default:
			if !inPara {
				flushList()
				flushQuote()
				flushTable()
				buf.WriteString("<p>")
				inPara = true
			} else {
				buf.WriteString(" ")
			}
			buf.WriteString(formatInline(strings.TrimSpace(line), &imageCount) + "\n")
		}
	}
	flushPara()
	flushList()
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

// applyOutsideTags applies fn only to text segments outside HTML tags,
// so that formatting regexes never touch URLs inside href attributes, etc.
func applyOutsideTags(s string, fn func(string) string) string {
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

func formatInline(s string, imageCount *int) string {
	escaped := html.EscapeString(s)
	// ![alt](url){style} or ![alt](url){style|width|height}
	escaped = reImg.ReplaceAllStringFunc(escaped, func(m string) string {
		match := reImg.FindStringSubmatch(m)
		if len(match) < 4 {
			return m
		}
		src := safeURL(match[2])
		if src == "" {
			return match[1]
		}

		alt := match[1]
		style := match[3]
		width := "1024"
		height := "768"
		if len(match) >= 6 && match[4] != "" && match[5] != "" {
			width = match[4]
			height = match[5]
		}

		*imageCount++
		var loadAttr string
		if *imageCount == 1 {
			loadAttr = `fetchpriority="high"`
		} else {
			loadAttr = `loading="eager"`
		}

		return `<img ` + loadAttr + ` width="` + width + `" height="` + height + `" alt="` + alt + `" src="` + src + `" style="` + style + `" decoding="async"/>`
	})
	escaped = reLink.ReplaceAllStringFunc(escaped, func(m string) string {
		match := reLink.FindStringSubmatch(m)
		if len(match) < 3 {
			return m
		}
		href := safeURL(match[2])
		if href == "" {
			return match[1]
		}
		attrs := `class="underline decoration-2 underline-offset-4"`
		if len(match) >= 4 && match[3] == "^" {
			attrs += ` target="_blank" rel="noopener noreferrer"`
		}
		return `<a href="` + href + `" ` + attrs + `>` + match[1] + `</a>`
	})
	// Apply bold/italic only outside HTML tags so URLs in href are not corrupted
	escaped = applyOutsideTags(escaped, func(seg string) string {
		seg = reBold.ReplaceAllString(seg, "<strong>$1</strong>")
		seg = reBoldUnderscore.ReplaceAllString(seg, "<strong>$1</strong>")
		seg = reItalic.ReplaceAllString(seg, "<em>$1</em>")
		seg = reItalicUnderscore.ReplaceAllString(seg, "<em>$1</em>")
		return seg
	})
	return escaped
}

func safeURL(raw string) string {
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
