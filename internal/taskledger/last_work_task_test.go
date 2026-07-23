package taskledger_test

import (
	"testing"

	"github.com/paseka/paseka/internal/protocol"
	"github.com/paseka/paseka/internal/taskledger"
)

func TestIsLastWorkTask(t *testing.T) {
	trace := func(tasks map[string]taskledger.TaskSnapshot) taskledger.TraceSnapshot {
		return taskledger.TraceSnapshot{TraceID: "trace-1", Tasks: tasks}
	}

	cases := []struct {
		name   string
		trace  taskledger.TraceSnapshot
		taskID string
		want   bool
	}{
		{
			name: "sole incomplete work task",
			trace: trace(map[string]taskledger.TaskSnapshot{
				"task-1": {TaskID: "task-1", Status: protocol.TaskStatusCompleted},
				"task-2": {TaskID: "task-2", Status: protocol.TaskStatusReady},
			}),
			taskID: "task-2",
			want:   true,
		},
		{
			name: "two incomplete work tasks",
			trace: trace(map[string]taskledger.TaskSnapshot{
				"task-1": {TaskID: "task-1", Status: protocol.TaskStatusReady},
				"task-2": {TaskID: "task-2", Status: protocol.TaskStatusPlanned},
			}),
			taskID: "task-1",
			want:   false,
		},
		{
			name: "failed sibling keeps count above one",
			trace: trace(map[string]taskledger.TaskSnapshot{
				"task-1": {TaskID: "task-1", Status: protocol.TaskStatusFailed},
				"task-2": {TaskID: "task-2", Status: protocol.TaskStatusReady},
			}),
			taskID: "task-2",
			want:   false,
		},
		{
			name: "blocked sibling keeps count above one",
			trace: trace(map[string]taskledger.TaskSnapshot{
				"task-1": {TaskID: "task-1", Status: protocol.TaskStatusBlocked},
				"task-2": {TaskID: "task-2", Status: protocol.TaskStatusRunning},
			}),
			taskID: "task-2",
			want:   false,
		},
		{
			name: "final review current task",
			trace: trace(map[string]taskledger.TaskSnapshot{
				"task-1": {TaskID: "task-1", Status: protocol.TaskStatusCompleted},
				"review": {TaskID: "review", Review: protocol.TaskReviewFinal, Status: protocol.TaskStatusWaitingReview},
			}),
			taskID: "review",
			want:   false,
		},
		{
			name: "synthetic final review task",
			trace: trace(map[string]taskledger.TaskSnapshot{
				"task-1":  {TaskID: "task-1", Status: protocol.TaskStatusCompleted},
				"_review": {TaskID: taskledger.FinalReviewTaskID, Review: protocol.TaskReviewFinal, Status: protocol.TaskStatusWaitingReview},
			}),
			taskID: taskledger.FinalReviewTaskID,
			want:   false,
		},
		{
			name: "unset task id",
			trace: trace(map[string]taskledger.TaskSnapshot{
				"task-1": {TaskID: "task-1", Status: protocol.TaskStatusReady},
			}),
			taskID: "",
			want:   false,
		},
		{
			name: "review required gate counts as work task",
			trace: trace(map[string]taskledger.TaskSnapshot{
				"task-1": {TaskID: "task-1", Status: protocol.TaskStatusCompleted},
				"task-2": {TaskID: "task-2", Review: protocol.TaskReviewRequired, Status: protocol.TaskStatusWaitingReview},
			}),
			taskID: "task-2",
			want:   true,
		},
		{
			name: "all work tasks completed",
			trace: trace(map[string]taskledger.TaskSnapshot{
				"task-1": {TaskID: "task-1", Status: protocol.TaskStatusCompleted},
				"review": {TaskID: "review", Review: protocol.TaskReviewFinal, Status: protocol.TaskStatusPlanned},
			}),
			taskID: "task-1",
			want:   false,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := taskledger.IsLastWorkTask(tc.trace, tc.taskID)
			if got != tc.want {
				t.Fatalf("IsLastWorkTask() = %v, want %v", got, tc.want)
			}
		})
	}
}
