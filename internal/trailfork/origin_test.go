package trailfork_test

import (
	"context"
	"testing"

	"github.com/russ-p/paseka/internal/protocol"
	"github.com/russ-p/paseka/internal/runs"
	"github.com/russ-p/paseka/internal/taskledger"
	"github.com/russ-p/paseka/internal/trailconfig"
	"github.com/russ-p/paseka/internal/trailfork"
)

func TestForkRecordsItsOwnOriginNotTheSourceTaskCue(t *testing.T) {
	root, _ := setupGitRepo(t)
	ctx := testContext(root)
	if _, err := trailconfig.Ensure(ctx, "trace-source"); err != nil {
		t.Fatal(err)
	}
	writeTask(t, root, "trace-source", taskledger.TaskSnapshot{
		TaskID: "task-1",
		Title:  "implement task-1",
		Body:   "body for task-1",
		Bee:    "builder",
		Status: protocol.TaskStatusCompleted,
		CueID:  "hotfix",
	})
	input := trailconfig.NewTaskInputEvidence(
		"task-1",
		"implement task-1",
		"body for task-1",
		"builder",
		"",
		"feature",
		string(protocol.TaskReviewNone),
		"hotfix",
	)
	if _, err := trailconfig.EnsureTaskAttempt(ctx, "trace-source", "task-1", "evt-task-1", input); err != nil {
		t.Fatal(err)
	}

	ledger := taskledger.NewMemoryLedger()
	result, err := trailfork.Create(context.Background(), &capturePublisher{}, ledger, trailfork.Options{
		Context:       ctx,
		SourceTraceID: "trace-source",
	})
	if err != nil {
		t.Fatal(err)
	}
	if result.Plan.Task.CueID != "hotfix" {
		t.Fatalf("fork did not copy the source task cue: %+v", result.Plan.Task)
	}

	origin, ok, err := runs.ReadTrailOrigin(root, result.Plan.TraceID)
	if err != nil || !ok {
		t.Fatalf("fork origin: ok=%v err=%v", ok, err)
	}
	if origin.Kind != runs.OriginKindFork {
		t.Fatalf("fork origin kind = %q", origin.Kind)
	}
	if origin.CueID != "" || origin.InvocationID != "" {
		t.Fatalf("fork origin invented a cue identity: %+v", origin)
	}
	if origin.SourceTraceID != "trace-source" || origin.ForkID != result.Plan.ForkID {
		t.Fatalf("fork origin lineage = %+v", origin)
	}
}
