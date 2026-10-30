package trailfork_test

import (
	"strings"
	"testing"

	"github.com/russ-p/paseka/internal/protocol"
	"github.com/russ-p/paseka/internal/taskledger"
	"github.com/russ-p/paseka/internal/trailfork"
)

func TestForkCopiesWorkTasksOnly(t *testing.T) {
	root, _ := setupGitRepo(t)
	ctx := testContext(root)
	writeSource(t, ctx, "trace-source", "task-1", nil)
	writeTask(t, root, "trace-source", taskledger.TaskSnapshot{
		TaskID: "closing-1",
		Phase:  protocol.TaskPhaseClosing,
		Status: protocol.TaskStatusCompleted,
		Bee:    "builder",
	})
	writeTask(t, root, "trace-source", taskledger.TaskSnapshot{
		TaskID: "_review",
		Review: protocol.TaskReviewFinal,
		Status: protocol.TaskStatusWaitingReview,
		Bee:    "builder",
	})

	// The closing round and the merge gate are not fork roots; the work task is.
	preview, err := trailfork.Preview(trailfork.Options{Context: ctx, SourceTraceID: "trace-source"})
	if err != nil {
		t.Fatal(err)
	}
	if preview.Plan.Task.SourceTaskID != "task-1" {
		t.Fatalf("fork copied %q, want the work task", preview.Plan.Task.SourceTaskID)
	}

	for _, taskID := range []string{"closing-1", "_review"} {
		_, err := trailfork.Preview(trailfork.Options{Context: ctx, SourceTraceID: "trace-source", TaskID: taskID})
		if err == nil {
			t.Fatalf("forking %q succeeded, want a refusal", taskID)
		}
		if taskID == "closing-1" && !strings.Contains(err.Error(), "closing-phase") {
			t.Fatalf("closing task error = %v", err)
		}
		if taskID == "_review" && !strings.Contains(err.Error(), "final-review") {
			t.Fatalf("merge task error = %v", err)
		}
	}
}
