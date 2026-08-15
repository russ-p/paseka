package hiveview

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/paseka/paseka/internal/colony"
	"github.com/paseka/paseka/internal/protocol"
	"github.com/paseka/paseka/internal/runs"
	"github.com/paseka/paseka/internal/runtime"
)

func TestBuildSnapshotAttentionLowEnergy(t *testing.T) {
	energy := SnapshotEnergy{
		Available: true,
		Traces: []SnapshotEnergyTrace{
			{TraceID: "t1", Remaining: 0, Budget: 10},
			{TraceID: "t2", Remaining: 5, Budget: 10},
			{TraceID: "t3", Remaining: 0, Budget: 0},
		},
	}
	att := buildSnapshotAttention(
		SnapshotRuntime{Status: runtime.RuntimeStatusRunning, Alive: true},
		SnapshotNATS{},
		TaskBoardView{},
		nil,
		energy,
	)
	if len(att.LowEnergyTraces) != 1 || att.LowEnergyTraces[0].TraceID != "t1" {
		t.Fatalf("lowEnergy = %+v", att.LowEnergyTraces)
	}
}

func TestBuildSnapshotAttentionNatsDown(t *testing.T) {
	att := buildSnapshotAttention(
		SnapshotRuntime{Status: runtime.RuntimeStatusRunning, Alive: true},
		SnapshotNATS{Configured: true, Connected: false},
		TaskBoardView{},
		nil,
		SnapshotEnergy{},
	)
	if !att.NatsDown {
		t.Fatal("expected natsDown")
	}
}

func TestTaskBoardFromFilesystem(t *testing.T) {
	repo := t.TempDir()
	home := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", home)
	slug := "fs-board"
	homeDir := filepath.Join(home, "paseka", slug)
	if err := os.MkdirAll(homeDir, 0o755); err != nil {
		t.Fatal(err)
	}
	cfg := "colony_root: " + repo + "\nslug: " + slug + "\nnats:\n  url: nats://127.0.0.1:59999\n"
	if err := os.WriteFile(filepath.Join(homeDir, "config.yaml"), []byte(cfg), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(homeDir, "state.json"), []byte("{}\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Join(repo, ".paseka"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(repo, ".paseka", "colony.yaml"), []byte("name: test\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	ctx := colony.Context{
		ColonyRoot: repo,
		Slug:       slug,
		Home: colony.HomeConfig{
			ColonyRoot: repo,
			Slug:       slug,
			NATS:       colony.NATSConfig{URL: "nats://127.0.0.1:59999"},
		},
	}

	traceID := "trace-fs"
	taskDir, err := runs.NewTaskDir(repo, traceID, "task-ready")
	if err != nil {
		t.Fatal(err)
	}
	if err := taskDir.WriteTask(runs.TaskFrontmatter{
		TraceID: traceID,
		TaskID:  "task-ready",
		Title:   "Ready task",
		Bee:     "scout",
		Status:  protocol.TaskStatusReady,
	}, "body"); err != nil {
		t.Fatal(err)
	}

	board, err := taskBoardForSnapshot(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if board.TaskCounts[string(protocol.TaskStatusReady)] != 1 {
		t.Fatalf("taskCounts = %+v", board.TaskCounts)
	}
}
