package runs_test

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/paseka/paseka/internal/protocol"
	"github.com/paseka/paseka/internal/runs"
	"github.com/paseka/paseka/internal/taskledger"
)

func TestTaskMarkdownRoundTripPreservesIntent(t *testing.T) {
	fm := runs.TaskFrontmatter{
		TraceID: "trace-1",
		TaskID:  "task-1",
		Title:   "Fix login",
		Bee:     "builder",
		Intent:  "bugfix",
		Status:  protocol.TaskStatusPlanned,
	}
	content := runs.MarshalTaskMarkdown(fm, "fix null pointer")
	gotFM, _, err := runs.ParseTaskMarkdown(content)
	if err != nil {
		t.Fatal(err)
	}
	if gotFM.Intent != "bugfix" {
		t.Fatalf("intent = %q", gotFM.Intent)
	}
}

func TestTaskMarkdownRoundTrip(t *testing.T) {
	fm := runs.TaskFrontmatter{
		TraceID:   "trace-1",
		TaskID:    "task-1",
		Title:     "Add endpoint",
		Bee:       "builder",
		Status:    protocol.TaskStatusPlanned,
		DependsOn: []string{"task-0"},
		UpdatedAt: "2026-07-07T06:00:00Z",
	}
	body := "POST /api/auth/login with JWT"

	content := runs.MarshalTaskMarkdown(fm, body)
	gotFM, gotBody, err := runs.ParseTaskMarkdown(content)
	if err != nil {
		t.Fatal(err)
	}
	if gotFM.TaskID != "task-1" || gotFM.Title != "Add endpoint" {
		t.Fatalf("frontmatter = %+v", gotFM)
	}
	if gotBody != body {
		t.Fatalf("body = %q", gotBody)
	}
}

func TestWriteTaskSnapshotAndRuns(t *testing.T) {
	root := t.TempDir()
	traceID := "trace-1"
	task := taskledger.TaskSnapshot{
		TaskID:    "task-1",
		Title:     "Add endpoint",
		Body:      "POST /api/auth/login with JWT",
		Bee:       "builder",
		Status:    protocol.TaskStatusReady,
		DependsOn: []string{},
		UpdatedAt: time.Date(2026, 7, 7, 6, 0, 0, 0, time.UTC),
	}
	if err := runs.WriteTaskSnapshot(root, traceID, task); err != nil {
		t.Fatal(err)
	}

	taskPath := filepath.Join(root, ".paseka", "runs", traceID, "tasks", "task-1", runs.TaskFileName)
	if _, err := os.Stat(taskPath); err != nil {
		t.Fatalf("task.md missing: %v", err)
	}

	d, err := runs.NewTaskDir(root, traceID, "task-1")
	if err != nil {
		t.Fatal(err)
	}
	fm, body, err := d.ReadTask()
	if err != nil {
		t.Fatal(err)
	}
	if fm.Status != protocol.TaskStatusReady || body != task.Body {
		t.Fatalf("task = %+v body=%q", fm, body)
	}

	started := time.Date(2026, 7, 7, 6, 1, 0, 0, time.UTC)
	if err := d.AppendTaskRun(runs.TaskRunEntry{
		AgentID:   "agent-a",
		Bee:       "builder",
		RunDir:    filepath.Join(root, ".paseka", "runs", traceID, "agent-a"),
		StartedAt: started,
		RunStatus: "running",
	}); err != nil {
		t.Fatal(err)
	}

	finished := time.Date(2026, 7, 7, 6, 5, 0, 0, time.UTC)
	if err := d.UpdateTaskRunStatus("agent-a", "completed", finished); err != nil {
		t.Fatal(err)
	}
	entries, err := d.ReadTaskRuns()
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) != 1 || entries[0].RunStatus != "completed" {
		t.Fatalf("entries = %+v", entries)
	}
}

func TestSyncTraceTasksAndLoadFromFS(t *testing.T) {
	root := t.TempDir()
	trace := taskledger.TraceSnapshot{
		TraceID: "trace-1",
		Tasks: map[string]taskledger.TaskSnapshot{
			"task-1": {TaskID: "task-1", Title: "first", Status: protocol.TaskStatusPlanned, Intent: "feature", Sector: "frontend"},
			"task-2": {TaskID: "task-2", Title: "second", Status: protocol.TaskStatusPlanned, DependsOn: []string{"task-1"}},
		},
	}
	if err := runs.SyncTraceTasks(root, trace); err != nil {
		t.Fatal(err)
	}
	got, err := runs.LoadTraceTasksFromFS(root, "trace-1")
	if err != nil {
		t.Fatal(err)
	}
	if len(got.Tasks) != 2 {
		t.Fatalf("tasks = %d", len(got.Tasks))
	}
	if got.Tasks["task-1"].Intent != "feature" {
		t.Fatalf("intent = %q", got.Tasks["task-1"].Intent)
	}
	if got.Tasks["task-1"].Sector != "frontend" {
		t.Fatalf("sector = %q", got.Tasks["task-1"].Sector)
	}
}

func TestTaskProjectionFromLedgerEvents(t *testing.T) {
	root := t.TempDir()
	traceID := "trace-1"

	plan, err := protocol.NewEvent(traceID, "scout", 0, protocol.EventInsight, protocol.TaskPlanPayload{
		Kind: protocol.TaskEventPlan,
		Tasks: []protocol.TaskSpec{{
			TaskID: "task-1",
			Title:  "Add endpoint",
			Body:   "POST /api/auth/login",
			Bee:    "builder",
		}},
	})
	if err != nil {
		t.Fatal(err)
	}
	ready, err := protocol.NewEvent(traceID, "cli", 0, protocol.EventSignal, protocol.TaskReadyPayload{
		Kind:   protocol.TaskEventReady,
		TaskID: "task-1",
		Bee:    "builder",
	})
	if err != nil {
		t.Fatal(err)
	}

	ledger := taskledger.NewMemoryLedger()
	for _, ev := range []protocol.Event{plan, ready} {
		res, err := ledger.Apply(ev)
		if err != nil {
			t.Fatal(err)
		}
		if err := runs.SyncTraceTasks(root, res.Trace); err != nil {
			t.Fatal(err)
		}
	}

	got, err := runs.LoadTraceTasksFromFS(root, traceID)
	if err != nil {
		t.Fatal(err)
	}
	task := got.Tasks["task-1"]
	if task.Status != protocol.TaskStatusReady || task.Body != "POST /api/auth/login" {
		t.Fatalf("task = %+v", task)
	}
}
