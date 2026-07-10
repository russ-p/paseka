package export

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/paseka/paseka/internal/colony"
	"github.com/paseka/paseka/internal/console"
	"github.com/paseka/paseka/internal/protocol"
	"github.com/paseka/paseka/internal/runs"
)

func TestOutputFilename(t *testing.T) {
	got := OutputFilename("my-project", "trace-abc123")
	want := "paseka-export-my-project-trace-abc123.html"
	if got != want {
		t.Fatalf("OutputFilename() = %q, want %q", got, want)
	}
}

func TestOutputFilenameSanitizesUnsafeChars(t *testing.T) {
	got := OutputFilename("org/repo", "trace:bad")
	if strings.Contains(got, "/") || strings.Contains(got, ":") {
		t.Fatalf("OutputFilename() = %q, expected sanitized", got)
	}
}

func TestRenderHTMLContainsTraceData(t *testing.T) {
	data := TraceExportData{
		Slug:       "demo-hive",
		ColonyRoot: "/tmp/colony",
		ExportedAt: time.Date(2026, 7, 10, 12, 0, 0, 0, time.UTC),
		Trace: console.TraceDetailView{
			TraceSummaryView: console.TraceSummaryView{
				TraceID:        "trace-test-1",
				LastActivityAt: time.Date(2026, 7, 10, 11, 0, 0, 0, time.UTC),
				RunCount:       1,
				TaskCount:      1,
			},
			Tasks: []console.TaskSummaryView{{
				TaskID: "task-1",
				Title:  "Survey codebase",
				Status: "ready",
				Bee:    "scout",
			}},
			Runs: []console.RunView{{
				TraceID:   "trace-test-1",
				AgentID:   "agent-1",
				Bee:       "scout",
				State:     "completed",
				StartedAt: time.Date(2026, 7, 10, 10, 0, 0, 0, time.UTC),
			}},
		},
		Events: []console.EventFeedItem{{
			Type:        protocol.EventSignal,
			PayloadKind: "task.ready",
			CreatedAt:   time.Date(2026, 7, 10, 10, 5, 0, 0, time.UTC),
			TraceID:     "trace-test-1",
			AgentID:     "agent-1",
			Bee:         "scout",
			Summary:     "Task ready: Survey codebase",
		}},
	}

	html, err := RenderHTML(data)
	if err != nil {
		t.Fatalf("RenderHTML: %v", err)
	}
	body := string(html)
	for _, want := range []string{
		"Paseka export 🐝",
		"trace-test-1",
		"demo-hive",
		"SIGNAL",
		"Survey codebase",
		"fonts.googleapis.com",
		"JetBrains Mono",
		"Show raw JSON",
	} {
		if !strings.Contains(body, want) {
			t.Fatalf("HTML missing %q", want)
		}
	}
}

func TestExportTraceWritesFile(t *testing.T) {
	repo := t.TempDir()
	slug := "export-test"
	home := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", home)
	homeDir := filepath.Join(home, "paseka", slug)
	if err := os.MkdirAll(homeDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(homeDir, "config.yaml"), []byte("colony_root: "+repo+"\nslug: "+slug+"\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(homeDir, "state.json"), []byte("{}\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	ctx := colony.Context{ColonyRoot: repo, Slug: slug}
	traceID := "trace-export"
	started := time.Now().UTC().Add(-time.Minute)
	d := runs.Dir{ColonyRoot: repo, TraceID: traceID, AgentID: "agent-1"}
	if err := d.Prepare(); err != nil {
		t.Fatal(err)
	}
	if err := d.WriteRequest(protocol.Request{
		ProtocolVersion: protocol.Version,
		TraceID:         traceID,
		AgentID:         "agent-1",
		Bee:             "scout",
		Adapter:         "cursor",
		Workspace:       repo,
		ColonyRoot:      repo,
		CreatedAt:       started,
	}); err != nil {
		t.Fatal(err)
	}
	if err := d.WriteStatusSnapshot(protocol.StatusSnapshot{
		ProtocolVersion: protocol.Version,
		State:           protocol.StatusCompleted,
		StartedAt:       started,
		FinishedAt:      started.Add(time.Second),
	}); err != nil {
		t.Fatal(err)
	}
	if err := d.AppendEvent(protocol.Event{
		ProtocolVersion: protocol.Version,
		TraceID:         traceID,
		AgentID:         "agent-1",
		Seq:             1,
		Type:            protocol.EventInsight,
		CreatedAt:       started.Add(2 * time.Second),
		Payload:         []byte(`{"kind":"narrative","text":"hello export"}`),
	}); err != nil {
		t.Fatal(err)
	}

	outDir := t.TempDir()
	path, err := ExportTrace(ctx, exportOptions(traceID, outDir))
	if err != nil {
		t.Fatalf("ExportTrace: %v", err)
	}
	if !strings.HasSuffix(path, "paseka-export-"+slug+"-"+traceID+".html") {
		t.Fatalf("unexpected path %q", path)
	}
	if _, err := os.Stat(path); err != nil {
		t.Fatalf("stat output: %v", err)
	}
	body, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(body), traceID) {
		t.Fatalf("export file missing trace id")
	}
	if !strings.Contains(string(body), "INSIGHT") {
		t.Fatalf("export file missing event type")
	}
}

func exportOptions(traceID, outDir string) Options {
	return Options{TraceID: traceID, OutputDir: outDir}
}
