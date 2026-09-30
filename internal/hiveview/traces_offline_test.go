package hiveview

import (
	"fmt"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/russ-p/paseka/internal/colony"
	"github.com/russ-p/paseka/internal/protocol"
	"github.com/russ-p/paseka/internal/runs"
	"github.com/russ-p/paseka/internal/taskledger"
)

type failingLedger struct{}

func (failingLedger) Snapshot(string) (taskledger.TraceSnapshot, error) {
	return taskledger.TraceSnapshot{}, fmt.Errorf("kv unavailable")
}

func (failingLedger) Apply(protocol.Event) (taskledger.ApplyResult, error) {
	return taskledger.ApplyResult{}, fmt.Errorf("kv unavailable")
}

func (failingLedger) SeedEnergy(string, int) error {
	return fmt.Errorf("kv unavailable")
}

func TestResolveTraceTasksFallsBackOnLoadTraceError(t *testing.T) {
	repo := t.TempDir()
	traceID := "trace-kv-fail"
	taskDir, err := runs.NewTaskDir(repo, traceID, "task-fs")
	if err != nil {
		t.Fatal(err)
	}
	if err := taskDir.WriteTask(runs.TaskFrontmatter{
		TraceID: traceID,
		TaskID:  "task-fs",
		Title:   "From filesystem",
		Bee:     "scout",
		Status:  protocol.TaskStatusReady,
	}, "body"); err != nil {
		t.Fatal(err)
	}

	ctx := colony.Context{ColonyRoot: repo, Slug: "test"}
	snap, err := resolveTraceTasks(ctx, failingLedger{}, traceID)
	if err != nil {
		t.Fatalf("resolveTraceTasks: %v", err)
	}
	if len(snap.Tasks) != 1 {
		t.Fatalf("tasks = %+v", snap.Tasks)
	}
	if _, ok := snap.Tasks["task-fs"]; !ok {
		t.Fatalf("expected filesystem task, got %+v", snap.Tasks)
	}
}

func TestGetTraceAlignsTaskCountWithLoadedTasks(t *testing.T) {
	repo := t.TempDir()
	slug := "align-count"
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
	traceID := "trace-count"
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

	for _, taskID := range []string{"task-a", "task-b"} {
		taskDir, err := runs.NewTaskDir(repo, traceID, taskID)
		if err != nil {
			t.Fatal(err)
		}
		if err := taskDir.WriteTask(runs.TaskFrontmatter{
			TraceID: traceID,
			TaskID:  taskID,
			Title:   taskID,
			Bee:     "scout",
			Status:  protocol.TaskStatusReady,
		}, "body"); err != nil {
			t.Fatal(err)
		}
	}

	view, ok, err := GetTrace(ctx, traceID)
	if err != nil || !ok {
		t.Fatalf("GetTrace() = ok=%v err=%v", ok, err)
	}
	if len(view.Tasks) != 2 {
		t.Fatalf("tasks = %+v", view.Tasks)
	}
	if view.TaskCount != len(view.Tasks) {
		t.Fatalf("TaskCount=%d len(Tasks)=%d", view.TaskCount, len(view.Tasks))
	}
}

// The console iterates the trace detail's tasks and runs and the board's
// groups, and JSON-marshals each. A view built by appending onto a nil slice
// reaches the browser as `null`, which is not iterable — so an empty half of
// any of these must still be an empty list.
func TestEmptyProjectionsAreEmptyListsNotNil(t *testing.T) {
	repo := t.TempDir()
	slug := "empty-projections"
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

	// A trace with a run but no tasks: runs has one entry, tasks has none.
	traceID := "trace-half-empty"
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

	view, ok, err := GetTrace(ctx, traceID)
	if err != nil || !ok {
		t.Fatalf("GetTrace() = ok=%v err=%v", ok, err)
	}
	if view.Tasks == nil {
		t.Fatal("trace with no tasks has nil Tasks, want an empty list")
	}
	if view.Runs == nil {
		t.Fatal("trace with runs has nil Runs, want a list")
	}
	if len(view.Runs) != 1 {
		t.Fatalf("runs = %+v, want the one headless run", view.Runs)
	}

	// A board over no tasks at all.
	board := buildTaskBoard(nil)
	if board.Groups == nil {
		t.Fatal("empty task board has nil Groups, want an empty list")
	}

	// A colony that has published no narrative insight.
	insights, err := CollectRecentInsights(ctx, 10)
	if err != nil {
		t.Fatal(err)
	}
	if insights == nil {
		t.Fatal("no insights is nil, want an empty list")
	}
}

func TestCollectRecentInsightsExcludesTraceSummary(t *testing.T) {
	repo := t.TempDir()
	traceID := "trace-insights"
	started := time.Now().UTC()

	d := runs.Dir{ColonyRoot: repo, TraceID: traceID, AgentID: "agent-1"}
	if err := d.Prepare(); err != nil {
		t.Fatal(err)
	}
	if err := d.WriteRequest(protocol.Request{
		ProtocolVersion: protocol.Version,
		TraceID:         traceID,
		AgentID:         "agent-1",
		Bee:             "builder",
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

	traceSummary, err := protocol.NewEvent(traceID, "builder", 1, protocol.EventInsight, protocol.TraceSummaryPayload{
		Kind:    protocol.InsightTraceSummary,
		Summary: "Trail outcome description",
	})
	if err != nil {
		t.Fatal(err)
	}
	traceSummary.CreatedAt = started
	if err := d.AppendEvent(traceSummary); err != nil {
		t.Fatal(err)
	}

	runSummary, err := protocol.NewEvent(traceID, "builder", 2, protocol.EventInsight, protocol.NarrativeInsightPayload{
		Kind:    protocol.InsightRunSummary,
		Summary: "Task run narrative",
		TaskID:  "task-1",
	})
	if err != nil {
		t.Fatal(err)
	}
	runSummary.CreatedAt = started.Add(time.Second)
	if err := d.AppendEvent(runSummary); err != nil {
		t.Fatal(err)
	}

	ctx := colony.Context{ColonyRoot: repo, Slug: "test"}
	insights, err := CollectRecentInsights(ctx, 10)
	if err != nil {
		t.Fatal(err)
	}
	if len(insights) != 1 {
		t.Fatalf("insights = %+v, want 1 narrative highlight", insights)
	}
	if insights[0].PayloadKind != string(protocol.InsightRunSummary) {
		t.Fatalf("payloadKind = %q, want run.summary", insights[0].PayloadKind)
	}
}
