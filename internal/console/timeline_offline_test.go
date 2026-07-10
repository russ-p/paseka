package console

import (
	"fmt"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/paseka/paseka/internal/colony"
	"github.com/paseka/paseka/internal/protocol"
	"github.com/paseka/paseka/internal/runs"
	"github.com/paseka/paseka/internal/taskledger"
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
