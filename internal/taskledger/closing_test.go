package taskledger

import (
	"testing"

	"github.com/russ-p/paseka/internal/protocol"
)

func TestDeriveTrailStage(t *testing.T) {
	work := TaskSnapshot{TaskID: "task-1", Status: protocol.TaskStatusCompleted}
	openWork := TaskSnapshot{TaskID: "task-2", Status: protocol.TaskStatusRunning}
	closingPlanned := TaskSnapshot{TaskID: "closing-1", Phase: protocol.TaskPhaseClosing, Status: protocol.TaskStatusPlanned}
	closingRunning := TaskSnapshot{TaskID: "closing-1", Phase: protocol.TaskPhaseClosing, Status: protocol.TaskStatusRunning}
	closingDone := TaskSnapshot{TaskID: "closing-1", Phase: protocol.TaskPhaseClosing, Status: protocol.TaskStatusCompleted}
	mergeWaiting := TaskSnapshot{TaskID: "_review", Review: protocol.TaskReviewFinal, Status: protocol.TaskStatusWaitingReview}
	mergeDone := TaskSnapshot{TaskID: "_review", Review: protocol.TaskReviewFinal, Status: protocol.TaskStatusCompleted}

	cases := []struct {
		name  string
		tasks []TaskSnapshot
		want  TrailStage
	}{
		{name: "no closing task", tasks: []TaskSnapshot{work}, want: TrailStageWork},
		{name: "empty trail", tasks: nil, want: TrailStageWork},
		{name: "closing round planned", tasks: []TaskSnapshot{work, closingPlanned}, want: TrailStageClosing},
		{name: "closing round running", tasks: []TaskSnapshot{work, closingRunning}, want: TrailStageClosing},
		{name: "closing failed", tasks: []TaskSnapshot{work, {TaskID: "closing-1", Phase: protocol.TaskPhaseClosing, Status: protocol.TaskStatusFailed}}, want: TrailStageClosing},
		{name: "closing completed, work still open", tasks: []TaskSnapshot{openWork, closingDone}, want: TrailStageWork},
		{name: "merge waits with nothing else open", tasks: []TaskSnapshot{work, closingDone, mergeWaiting}, want: TrailStageGate},
		{name: "merge waits with rework open", tasks: []TaskSnapshot{openWork, mergeWaiting}, want: TrailStageWork},
		{name: "merge completed", tasks: []TaskSnapshot{work, closingDone, mergeDone}, want: TrailStageDelivered},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			trace := TraceSnapshot{TraceID: "trace-stage", Tasks: map[string]TaskSnapshot{}}
			for _, task := range tc.tasks {
				trace.Tasks[task.TaskID] = task
			}
			if got := DeriveTrailStage(trace); got != tc.want {
				t.Fatalf("stage = %q, want %q", got, tc.want)
			}
		})
	}
}

func TestMergeGateWaitingRequiresEveryOtherTaskCompleted(t *testing.T) {
	waiting := TaskSnapshot{TaskID: "_review", Review: protocol.TaskReviewFinal, Status: protocol.TaskStatusWaitingReview}
	open := TaskSnapshot{TaskID: "task-1", Status: protocol.TaskStatusBlocked}
	done := TaskSnapshot{TaskID: "task-1", Status: protocol.TaskStatusCompleted}

	if !MergeGateWaiting(TraceSnapshot{Tasks: map[string]TaskSnapshot{"_review": waiting, "task-1": done}}) {
		t.Fatal("gate should be waiting when the merge task waits and the work is complete")
	}
	if MergeGateWaiting(TraceSnapshot{Tasks: map[string]TaskSnapshot{"_review": waiting, "task-1": open}}) {
		t.Fatal("gate must not read as waiting while another task is open")
	}
	if MergeGateWaiting(TraceSnapshot{Tasks: map[string]TaskSnapshot{"task-1": done}}) {
		t.Fatal("gate must not read as waiting without a merge task")
	}
}
