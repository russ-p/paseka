package export

import (
	"bytes"
	"html/template"
	"strings"

	"github.com/yuin/goldmark"
	"github.com/yuin/goldmark/extension"
	"github.com/yuin/goldmark/parser"
	"github.com/yuin/goldmark/renderer/html"
)

var markdown = goldmark.New(
	goldmark.WithExtensions(extension.GFM),
	goldmark.WithParserOptions(parser.WithAutoHeadingID()),
	goldmark.WithRendererOptions(
		html.WithHardWraps(),
		html.WithXHTML(),
	),
)

// formatMarkdown renders plain text or markdown for HTML export.
// Single newlines are preserved; common markdown (GFM) is supported.
func formatMarkdown(text string) template.HTML {
	text = strings.TrimSpace(text)
	if text == "" {
		return ""
	}
	var buf bytes.Buffer
	if err := markdown.Convert([]byte(text), &buf); err != nil {
		return template.HTML(template.HTMLEscapeString(text))
	}
	return template.HTML(buf.String()) //nolint:gosec // goldmark escapes unsafe HTML
}
