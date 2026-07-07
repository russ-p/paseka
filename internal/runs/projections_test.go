package runs_test

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/paseka/paseka/internal/adapters"
	"github.com/paseka/paseka/internal/protocol"
	"github.com/paseka/paseka/internal/runs"
)

func TestScanRecentRuns(t *testing.T) {
	root := t.TempDir()
	older := time.Now().UTC().Add(-2 * time.Hour)
	newer := time.Now().UTC().Add(-30 * time.Minute)

	writeHeadlessRun(t, root, "trace-old", "agent-old", older, protocol.StatusCompleted, "old summary")
	writeHeadlessRun(t, root, "trace-new", "agent-new", newer, protocol.StatusRunning, "")

	list, err := runs.ScanRecentRuns(root, 10)
	if err != nil {
		t.Fatal(err)
	}
	if len(list) != 2 {
		t.Fatalf("runs = %+v", list)
	}
	if list[0].AgentID != "agent-new" || list[1].AgentID != "agent-old" {
		t.Fatalf("sort order = %+v", list)
	}
	if list[0].State != string(protocol.StatusRunning) {
		t.Fatalf("state = %q", list[0].State)
	}
}

func TestScanRecentRunsSkipsWithoutRequest(t *testing.T) {
	root := t.TempDir()
	dir := runs.Dir{ColonyRoot: root, TraceID: "trace-1", AgentID: "agent-1"}
	if err := dir.Prepare(); err != nil {
		t.Fatal(err)
	}

	list, err := runs.ScanRecentRuns(root, 10)
	if err != nil {
		t.Fatal(err)
	}
	if len(list) != 0 {
		t.Fatalf("expected empty list, got %+v", list)
	}
}

func TestScanRecentRunsSkipsTasksDir(t *testing.T) {
	root := t.TempDir()
	tasksDir := filepath.Join(root, ".paseka", "runs", "trace-1", "tasks", "task-1")
	if err := os.MkdirAll(tasksDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(tasksDir, "task.md"), []byte("task"), 0o644); err != nil {
		t.Fatal(err)
	}

	list, err := runs.ScanRecentRuns(root, 10)
	if err != nil {
		t.Fatal(err)
	}
	if len(list) != 0 {
		t.Fatalf("expected no runs, got %+v", list)
	}
}

func TestLoadRunMetaToleratesMissingStatusAndResult(t *testing.T) {
	root := t.TempDir()
	started := time.Now().UTC()
	d := runs.Dir{ColonyRoot: root, TraceID: "trace-1", AgentID: "agent-1"}
	if err := d.Prepare(); err != nil {
		t.Fatal(err)
	}
	if err := d.WriteRequest(protocol.Request{
		ProtocolVersion: protocol.Version,
		TraceID:         "trace-1",
		AgentID:         "agent-1",
		Bee:             "builder",
		Adapter:         "cursor",
		Workspace:       root,
		ColonyRoot:      root,
		TaskID:          "task-1",
		Task:            "do work",
		Intent:          "feature",
		CreatedAt:       started,
	}); err != nil {
		t.Fatal(err)
	}

	meta, err := runs.LoadRunMeta(d)
	if err != nil {
		t.Fatal(err)
	}
	if meta.State != string(protocol.StatusQueued) {
		t.Fatalf("state = %q", meta.State)
	}
	if meta.TaskID != "task-1" || meta.Intent != "feature" {
		t.Fatalf("meta = %+v", meta)
	}
}

func TestLoadRunMetaMarksSessionAndEvents(t *testing.T) {
	root := t.TempDir()
	started := time.Now().UTC()
	d := runs.Dir{ColonyRoot: root, TraceID: "trace-1", AgentID: "agent-1"}
	writeHeadlessRun(t, root, "trace-1", "agent-1", started, protocol.StatusCompleted, "done")

	if err := d.WriteSession(runs.SessionMeta{
		SessionID: "agent-1",
		TraceID:   "trace-1",
		AgentID:   "agent-1",
		State:     string(adapters.SessionCompleted),
		StartedAt: started,
	}); err != nil {
		t.Fatal(err)
	}
	ev, err := protocol.NewEvent("trace-1", "agent-1", 0, protocol.EventLog, map[string]string{"line": "ok"})
	if err != nil {
		t.Fatal(err)
	}
	if err := d.AppendEvent(ev); err != nil {
		t.Fatal(err)
	}

	meta, err := runs.LoadRunMeta(d)
	if err != nil {
		t.Fatal(err)
	}
	if !meta.HasSession || !meta.HasEvents {
		t.Fatalf("meta = %+v", meta)
	}
}

func TestFindRun(t *testing.T) {
	root := t.TempDir()
	started := time.Now().UTC()
	writeHeadlessRun(t, root, "trace-1", "agent-1", started, protocol.StatusCompleted, "ok")

	meta, ok, err := runs.FindRun(root, "trace-1", "agent-1")
	if err != nil || !ok {
		t.Fatalf("FindRun() = %+v, %v, %v", meta, ok, err)
	}
	if meta.Summary != "ok" {
		t.Fatalf("summary = %q", meta.Summary)
	}

	_, ok, err = runs.FindRun(root, "missing", "agent-1")
	if err != nil || ok {
		t.Fatalf("FindRun missing = %v, %v", ok, err)
	}
}

func TestReadEventsAfter(t *testing.T) {
	root := t.TempDir()
	d := runs.Dir{ColonyRoot: root, TraceID: "trace-1", AgentID: "agent-1"}
	if err := d.Prepare(); err != nil {
		t.Fatal(err)
	}
	for i := 1; i <= 3; i++ {
		ev, err := protocol.NewEvent("trace-1", "agent-1", 0, protocol.EventLog, map[string]int{"n": i})
		if err != nil {
			t.Fatal(err)
		}
		if err := d.AppendEvent(ev); err != nil {
			t.Fatal(err)
		}
	}

	page, next, err := d.ReadEventsAfter(1)
	if err != nil {
		t.Fatal(err)
	}
	if len(page) != 2 || next != 3 {
		t.Fatalf("page = %+v next = %d", page, next)
	}
}

func writeHeadlessRun(t *testing.T, root, traceID, agentID string, started time.Time, state protocol.RunStatus, summary string) {
	t.Helper()
	d := runs.Dir{ColonyRoot: root, TraceID: traceID, AgentID: agentID}
	if err := d.Prepare(); err != nil {
		t.Fatal(err)
	}
	if err := d.WriteRequest(protocol.Request{
		ProtocolVersion: protocol.Version,
		TraceID:         traceID,
		AgentID:         agentID,
		Bee:             "builder",
		Adapter:         "cursor",
		Workspace:       root,
		ColonyRoot:      root,
		CreatedAt:       started,
	}); err != nil {
		t.Fatal(err)
	}
	if err := d.WriteStatusSnapshot(protocol.StatusSnapshot{
		ProtocolVersion: protocol.Version,
		State:           state,
		StartedAt:       started,
	}); err != nil {
		t.Fatal(err)
	}
	if summary != "" {
		if err := d.WriteResult(protocol.Result{
			ProtocolVersion: protocol.Version,
			TraceID:         traceID,
			AgentID:         agentID,
			Status:          state,
			Summary:         summary,
			FinishedAt:      started.Add(time.Minute),
		}); err != nil {
			t.Fatal(err)
		}
	}
}
