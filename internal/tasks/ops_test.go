package tasks_test

import (
	"testing"

	"github.com/paseka/paseka/internal/protocol"
	"github.com/paseka/paseka/internal/taskledger"
	"github.com/paseka/paseka/internal/tasks"
)

func TestDeriveTitle(t *testing.T) {
	if got := tasks.DeriveTitle("Explicit", "ignored body"); got != "Explicit" {
		t.Fatalf("title = %q", got)
	}
	if got := tasks.DeriveTitle("", "  \nFirst line\nsecond"); got != "First line" {
		t.Fatalf("derived = %q", got)
	}
}

func TestTasksToStart(t *testing.T) {
	snap := taskledger.TraceSnapshot{
		TraceID: "trace-1",
		Tasks: map[string]taskledger.TaskSnapshot{
			"a": {TaskID: "a", Status: protocol.TaskStatusPlanned, Bee: "builder"},
			"b": {TaskID: "b", Status: protocol.TaskStatusReady, Bee: "builder"},
		},
	}

	_, err := tasks.TasksToStart(snap, "b")
	if err == nil {
		t.Fatal("expected already ready error")
	}

	started, err := tasks.TasksToStart(snap, "a")
	if err != nil {
		t.Fatal(err)
	}
	if len(started) != 1 || started[0].TaskID != "a" {
		t.Fatalf("started = %+v", started)
	}

	started, err = tasks.TasksToStart(snap, "")
	if err != nil {
		t.Fatal(err)
	}
	if len(started) != 1 || started[0].TaskID != "a" {
		t.Fatalf("default start = %+v", started)
	}
}

func TestCanStartTask(t *testing.T) {
	snap := taskledger.TraceSnapshot{
		TraceID: "trace-1",
		Tasks: map[string]taskledger.TaskSnapshot{
			"blocked": {
				TaskID:    "blocked",
				Status:    protocol.TaskStatusPlanned,
				DependsOn: []string{"missing"},
			},
			"planned": {TaskID: "planned", Status: protocol.TaskStatusPlanned},
		},
	}
	if tasks.CanStartTask(snap, "blocked") {
		t.Fatal("blocked task should not be startable")
	}
	if !tasks.CanStartTask(snap, "planned") {
		t.Fatal("planned task should be startable")
	}
}
