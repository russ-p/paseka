package main

import (
	"strings"
	"testing"

	"github.com/paseka/paseka/internal/protocol"
	"github.com/paseka/paseka/internal/taskledger"
)

func TestResolveTaskCreateBodyPrecedence(t *testing.T) {
	_, err := resolveTaskCreateBody("inline", "file.txt", true, strings.NewReader("stdin"))
	if err == nil {
		t.Fatal("expected error for multiple body sources")
	}

	got, err := resolveTaskCreateBody("inline body", "", false, nil)
	if err != nil || got != "inline body" {
		t.Fatalf("body = %q, err = %v", got, err)
	}

	got, err = resolveTaskCreateBody("", "", true, strings.NewReader("from stdin\n"))
	if err != nil || got != "from stdin\n" {
		t.Fatalf("stdin body = %q, err = %v", got, err)
	}
}

func TestDeriveTaskTitle(t *testing.T) {
	if got := deriveTaskTitle("Explicit title", "body"); got != "Explicit title" {
		t.Fatalf("title = %q", got)
	}
	if got := deriveTaskTitle("", "First line\nSecond line"); got != "First line" {
		t.Fatalf("derived title = %q", got)
	}
}

func TestParseDependsOn(t *testing.T) {
	got := parseDependsOn([]string{"task-1", "task-2, task-3"})
	want := []string{"task-1", "task-2", "task-3"}
	if len(got) != len(want) {
		t.Fatalf("depends = %#v", got)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("depends[%d] = %q, want %q", i, got[i], want[i])
		}
	}
}

func TestTaskPlanEvent(t *testing.T) {
	ev, err := taskPlanEvent("trace-1", protocol.TaskSpec{
		TaskID: "task-1",
		Title:  "Add endpoint",
		Bee:    "builder",
	})
	if err != nil {
		t.Fatal(err)
	}
	if ev.Type != protocol.EventInsight || ev.TraceID != "trace-1" {
		t.Fatalf("event = %+v", ev)
	}
}

func TestTasksToStartSingleTask(t *testing.T) {
	snap := taskledger.TraceSnapshot{
		TraceID: "trace-1",
		Tasks: map[string]taskledger.TaskSnapshot{
			"task-1": {TaskID: "task-1", Status: protocol.TaskStatusPlanned},
		},
	}
	tasks, err := tasksToStart(snap, "task-1")
	if err != nil {
		t.Fatal(err)
	}
	if len(tasks) != 1 || tasks[0].TaskID != "task-1" {
		t.Fatalf("tasks = %+v", tasks)
	}
}

func TestTasksToStartEligibleBatch(t *testing.T) {
	snap := taskledger.TraceSnapshot{
		TraceID: "trace-1",
		Tasks: map[string]taskledger.TaskSnapshot{
			"task-1": {TaskID: "task-1", Status: protocol.TaskStatusCompleted},
			"task-2": {TaskID: "task-2", Status: protocol.TaskStatusPlanned, DependsOn: []string{"task-1"}},
			"task-3": {TaskID: "task-3", Status: protocol.TaskStatusPlanned, DependsOn: []string{"task-2"}},
		},
	}
	tasks, err := tasksToStart(snap, "")
	if err != nil {
		t.Fatal(err)
	}
	if len(tasks) != 1 || tasks[0].TaskID != "task-2" {
		t.Fatalf("tasks = %+v", tasks)
	}
}
