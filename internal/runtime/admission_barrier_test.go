package runtime_test

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/russ-p/paseka/internal/colony"
	"github.com/russ-p/paseka/internal/runtime"
	"github.com/russ-p/paseka/internal/trailconfig"
)

func TestBeeRunHoldsNewWorkWhenConfigurationApplyPending(t *testing.T) {
	repo := initBeeRunRepo(t)
	setupBeeRunHome(t, repo)
	ctx, err := colony.ResolveContext(repo)
	if err != nil {
		t.Fatal(err)
	}
	traceID := "trace-pending-cli"
	if _, err := trailconfig.Ensure(ctx, traceID); err != nil {
		t.Fatal(err)
	}
	promptPath := filepath.Join(repo, ".paseka", "prompts", "default.md")
	if err := os.WriteFile(promptPath, []byte("NEW {{.Task}}"), 0o644); err != nil {
		t.Fatal(err)
	}
	applied, err := trailconfig.Apply(trailconfig.ApplyOptions{
		Context: ctx,
		TraceID: traceID,
		Source:  trailconfig.SourceColony,
		Actor:   "operator",
		Blockers: []string{
			"task task-1 is running",
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	if applied.Change.Outcome != trailconfig.OutcomePending {
		t.Fatalf("apply outcome = %q", applied.Change.Outcome)
	}

	rec := &recordingAdapter{}
	d := runtime.NewDispatcher()
	d.RegisterAdapter("cursor", rec)
	for _, taskID := range []string{"", "task-1"} {
		_, err := d.BeeRun(context.Background(), runtime.BeeRunRequest{
			StartDir: repo,
			Bee:      "builder",
			TraceID:  traceID,
			TaskID:   taskID,
			Task:     "do not launch",
		})
		if err == nil || !strings.Contains(err.Error(), "pending configuration change") {
			t.Fatalf("task %q err = %v, want pending configuration refusal", taskID, err)
		}
	}
	if rec.calls != 0 {
		t.Fatalf("adapter launched %d times while configuration was pending", rec.calls)
	}
}
