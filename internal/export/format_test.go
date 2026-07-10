package export

import (
	"strings"
	"testing"

	"github.com/paseka/paseka/internal/console"
)

func TestFormatMarkdownPreservesNewlines(t *testing.T) {
	got := string(formatMarkdown("line one\nline two"))
	if !strings.Contains(got, "line one") || !strings.Contains(got, "line two") {
		t.Fatalf("formatMarkdown() = %q", got)
	}
	if !strings.Contains(got, "<br") {
		t.Fatalf("expected hard line break, got %q", got)
	}
}

func TestFormatMarkdownRendersBasicMarkdown(t *testing.T) {
	got := string(formatMarkdown("**bold** and `code`"))
	if !strings.Contains(got, "<strong>bold</strong>") {
		t.Fatalf("expected bold markdown, got %q", got)
	}
	if !strings.Contains(got, "<code>code</code>") {
		t.Fatalf("expected inline code, got %q", got)
	}
}

func TestRenderHTMLFormatsRunSummary(t *testing.T) {
	data := TraceExportData{
		Slug:       "demo",
		ColonyRoot: "/tmp",
		Trace: console.TraceDetailView{
			TraceSummaryView: console.TraceSummaryView{TraceID: "trace-1"},
		},
		Runs: []console.RunView{{
			AgentID: "agent-1",
			Bee:     "scout",
			State:   "completed",
			Summary: "Done.\n\n- item one\n- item two",
		}},
	}
	html, err := RenderHTML(data)
	if err != nil {
		t.Fatal(err)
	}
	body := string(html)
	if !strings.Contains(body, "item one") || !strings.Contains(body, "item two") {
		t.Fatalf("run summary list not rendered: %s", body)
	}
	if !strings.Contains(body, "formatted-text") {
		t.Fatalf("expected formatted-text class")
	}
}
