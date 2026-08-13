package export

import (
	"strings"
	"testing"
	"time"

	"github.com/paseka/paseka/internal/hiveview"
	"github.com/paseka/paseka/internal/protocol"
)

func TestParseFormat(t *testing.T) {
	tests := []struct {
		in      string
		want    Format
		wantErr bool
	}{
		{"", FormatHTML, false},
		{"html", FormatHTML, false},
		{"md", FormatMarkdown, false},
		{"markdown", FormatMarkdown, false},
		{"MD", FormatMarkdown, false},
		{"pdf", "", true},
	}
	for _, tc := range tests {
		got, err := ParseFormat(tc.in)
		if tc.wantErr {
			if err == nil {
				t.Fatalf("ParseFormat(%q) expected error", tc.in)
			}
			if !strings.Contains(err.Error(), "invalid --format") {
				t.Fatalf("ParseFormat(%q) error = %v", tc.in, err)
			}
			continue
		}
		if err != nil {
			t.Fatalf("ParseFormat(%q): %v", tc.in, err)
		}
		if got != tc.want {
			t.Fatalf("ParseFormat(%q) = %q, want %q", tc.in, got, tc.want)
		}
	}
}

func TestRenderMarkdownContainsTraceData(t *testing.T) {
	data := TraceExportData{
		Slug:       "demo-hive",
		ColonyRoot: "/tmp/colony",
		ExportedAt: time.Date(2026, 7, 10, 12, 0, 0, 0, time.UTC),
		Trace: hiveview.TraceDetailView{
			TraceSummaryView: hiveview.TraceSummaryView{
				TraceID:        "trace-test-1",
				LastActivityAt: time.Date(2026, 7, 10, 11, 0, 0, 0, time.UTC),
				RunCount:       1,
				TaskCount:      1,
			},
			Tasks: []hiveview.TaskSummaryView{{
				TaskID: "task-1",
				Title:  "Survey codebase",
				Status: "ready",
				Bee:    "scout",
			}},
		},
		Runs: []hiveview.RunView{{
			TraceID:   "trace-test-1",
			AgentID:   "agent-1",
			Bee:       "scout",
			State:     "completed",
			StartedAt: time.Date(2026, 7, 10, 10, 0, 0, 0, time.UTC),
			Summary:   "Done.\n\n- item one\n- item two",
		}},
		Events: []hiveview.EventFeedItem{{
			Type:        protocol.EventSignal,
			PayloadKind: "task.ready",
			CreatedAt:   time.Date(2026, 7, 10, 10, 5, 0, 0, time.UTC),
			TraceID:     "trace-test-1",
			AgentID:     "agent-1",
			Bee:         "scout",
			Summary:     "Task ready: Survey codebase",
			Raw: protocol.Event{
				TraceID: "trace-test-1",
				AgentID: "agent-1",
				Type:    protocol.EventSignal,
			},
		}},
	}

	md, err := RenderMarkdown(data)
	if err != nil {
		t.Fatalf("RenderMarkdown: %v", err)
	}
	body := string(md)
	for _, want := range []string{
		"# Paseka export",
		"trace-test-1",
		"demo-hive",
		"## Overview",
		"## Tasks",
		"Survey codebase",
		"## Runs",
		"item one",
		"item two",
		"SIGNAL · task.ready",
		"Task ready: Survey codebase",
		"```json",
		`"traceId": "trace-test-1"`,
	} {
		if !strings.Contains(body, want) {
			t.Fatalf("markdown missing %q in:\n%s", want, body)
		}
	}
	if strings.Contains(body, "<strong>") || strings.Contains(body, "<p>") {
		t.Fatalf("markdown should not HTML-convert summaries: %s", body)
	}
}

func TestRenderMarkdownPreservesRunSummaryVerbatim(t *testing.T) {
	summary := "**bold** and `code`"
	data := TraceExportData{
		Slug: "demo",
		Trace: hiveview.TraceDetailView{
			TraceSummaryView: hiveview.TraceSummaryView{TraceID: "trace-1"},
		},
		Runs: []hiveview.RunView{{
			AgentID: "agent-1",
			Bee:     "scout",
			State:   "completed",
			Summary: summary,
		}},
	}
	md, err := RenderMarkdown(data)
	if err != nil {
		t.Fatal(err)
	}
	body := string(md)
	if !strings.Contains(body, summary) {
		t.Fatalf("run summary not preserved verbatim: %s", body)
	}
}
