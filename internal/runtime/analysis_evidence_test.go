package runtime_test

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/russ-p/paseka/internal/colony"
	"github.com/russ-p/paseka/internal/protocol"
	"github.com/russ-p/paseka/internal/runs"
	"github.com/russ-p/paseka/internal/runtime"
)

// TestAnalysisDispatchReadsRetainedEvidence pins the analysis input contract:
// the run is told which milestone it studies and where the retained checkpoint,
// result, metrics snapshot, and patch live. It exercises the real dispatch path,
// not just the path helper.
func TestAnalysisDispatchReadsRetainedEvidence(t *testing.T) {
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	root := t.TempDir()
	mustWriteTestColony(t, root, map[string]colony.Bee{"analyst": {Role: "analyst", Worktree: true}})
	if err := os.WriteFile(filepath.Join(root, ".paseka", "prompts", "default.md"), []byte(
		"task={{.Task}}\nmilestone={{.AnalysisOf}}\n{{range .RetainedEvidence}}evidence={{.}}\n{{end}}",
	), 0o644); err != nil {
		t.Fatal(err)
	}

	traceID := "trace-analysis-evidence"
	completionID := "completion-r1-analysis"
	if _, err := runs.WriteWorkCompletion(root, traceID, runs.WorkCompletionCheckpoint{
		CompletionID: completionID,
		WorkRevision: 1,
		CompletedAt:  time.Now().UTC(),
	}, runs.WorkCompletionResult{}, runs.TrailMetrics{ObservedAt: time.Now().UTC()}); err != nil {
		t.Fatal(err)
	}

	rec := &recordingAdapter{}
	d := runtime.NewDispatcher()
	d.RegisterAdapter("cursor", rec)
	res, err := d.DispatchColonyBee(context.Background(), colony.Context{ColonyRoot: root, Slug: "test"}, runtime.ColonyDispatchRequest{
		Bee:        "analyst",
		TraceID:    traceID,
		Task:       "study the result",
		Purpose:    protocol.PurposeAnalysis,
		AnalysisOf: completionID,
	}, runtime.DispatchModeCLI)
	if err != nil {
		t.Fatal(err)
	}
	if res.Workspace != root {
		t.Fatalf("analysis workspace = %q, want the colony root %q", res.Workspace, root)
	}
	if rec.lastReq.Workspace != root {
		t.Fatalf("adapter workspace = %q, want the colony root", rec.lastReq.Workspace)
	}
	prompt := rec.lastReq.Prompt
	if !strings.Contains(prompt, "milestone="+completionID) {
		t.Fatalf("analysis prompt does not name its milestone:\n%s", prompt)
	}
	for _, name := range []string{"completion.json", "result.json", "metrics.json", "patch.diff"} {
		path := filepath.Join(runs.WorkCompletionDir(root, traceID, completionID), name)
		if name == "patch.diff" {
			continue // a non-code completion has no patch file
		}
		if !strings.Contains(prompt, "evidence="+path) {
			t.Fatalf("analysis prompt does not expose %s:\n%s", name, prompt)
		}
	}
}
