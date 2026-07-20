package runs

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/paseka/paseka/internal/protocol"
)

func TestResolveTraceTitleFromInsight(t *testing.T) {
	root := t.TempDir()
	traceID := "trace-title-insight"
	d := Dir{ColonyRoot: root, TraceID: traceID, AgentID: "agent-1"}
	if err := d.Prepare(); err != nil {
		t.Fatal(err)
	}

	feature, err := protocol.NewEvent(traceID, "beekeeper", 0, protocol.EventSignal, map[string]any{
		"kind":  featureRequestedKind,
		"title": "Entry title",
		"body":  "body",
	})
	if err != nil {
		t.Fatal(err)
	}
	feature.CreatedAt = time.Now().UTC().Add(-time.Minute)
	if err := d.AppendEvent(feature); err != nil {
		t.Fatal(err)
	}

	titleEv, err := protocol.NewEvent(traceID, "scout", 1, protocol.EventInsight, protocol.TraceTitlePayload{
		Kind:  protocol.InsightTraceTitle,
		Title: "Resolved trail title",
	})
	if err != nil {
		t.Fatal(err)
	}
	titleEv.CreatedAt = time.Now().UTC()
	if err := d.AppendEvent(titleEv); err != nil {
		t.Fatal(err)
	}

	got, err := ResolveTraceTitle(root, traceID)
	if err != nil {
		t.Fatal(err)
	}
	if got != "Resolved trail title" {
		t.Fatalf("title = %q, want Resolved trail title", got)
	}
}

func TestResolveTraceTitleFromFeatureRequested(t *testing.T) {
	root := t.TempDir()
	traceID := "trace-title-feature"
	d := Dir{ColonyRoot: root, TraceID: traceID, AgentID: "telegram"}
	if err := d.Prepare(); err != nil {
		t.Fatal(err)
	}

	ev, err := protocol.NewEvent(traceID, "telegram", 0, protocol.EventSignal, map[string]any{
		"kind":  featureRequestedKind,
		"title": "Live bees header",
		"body":  "Show bees in header",
	})
	if err != nil {
		t.Fatal(err)
	}
	if err := d.AppendEvent(ev); err != nil {
		t.Fatal(err)
	}

	got, err := ResolveTraceTitle(root, traceID)
	if err != nil {
		t.Fatal(err)
	}
	if got != "Live bees header" {
		t.Fatalf("title = %q, want Live bees header", got)
	}
}

func TestResolveTraceTitleFromTaskMarkdown(t *testing.T) {
	root := t.TempDir()
	traceID := "trace-title-task"
	taskDir := filepath.Join(root, ".paseka", "runs", traceID, "tasks", "001-add-endpoint")
	if err := os.MkdirAll(taskDir, 0o755); err != nil {
		t.Fatal(err)
	}
	content := "---\ntraceId: trace-title-task\ntaskId: 001-add-endpoint\ntitle: Add endpoint\nstatus: planned\n---\n\nBody\n"
	if err := os.WriteFile(filepath.Join(taskDir, "task.md"), []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}

	got, err := ResolveTraceTitle(root, traceID)
	if err != nil {
		t.Fatal(err)
	}
	if got != "Add endpoint" {
		t.Fatalf("title = %q, want Add endpoint", got)
	}
}

func TestResolveTraceTitleEmpty(t *testing.T) {
	root := t.TempDir()
	got, err := ResolveTraceTitle(root, "trace-missing")
	if err != nil {
		t.Fatal(err)
	}
	if got != "" {
		t.Fatalf("title = %q, want empty", got)
	}
}
