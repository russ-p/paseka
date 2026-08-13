package export

import (
	"fmt"
	"strings"
)

// Format selects the trace export renderer.
type Format string

const (
	FormatHTML     Format = "html"
	FormatMarkdown Format = "md"
)

// ParseFormat normalizes a CLI --format value.
func ParseFormat(s string) (Format, error) {
	switch strings.ToLower(strings.TrimSpace(s)) {
	case "", "html":
		return FormatHTML, nil
	case "md", "markdown":
		return FormatMarkdown, nil
	default:
		return "", fmt.Errorf("invalid --format %q (supported: html, md)", s)
	}
}
