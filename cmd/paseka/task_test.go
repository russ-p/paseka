package main

import (
	"testing"

	"github.com/paseka/paseka/internal/protocol"
	"github.com/paseka/paseka/internal/taskledger"
)

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
