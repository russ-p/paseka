package trailconfig_test

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/russ-p/paseka/internal/protocol"
	"github.com/russ-p/paseka/internal/taskledger"
	"github.com/russ-p/paseka/internal/trailconfig"
)

func TestApplyDryRunCreatesNoConfigurationDirectory(t *testing.T) {
	root := writeColony(t, "slug: test\n")
	setupHome(t, "test")

	_, err := trailconfig.Apply(trailconfig.ApplyOptions{
		Context: testContext(root), TraceID: "trace-dry-unbound",
		Source: trailconfig.SourceColony, Actor: "operator", DryRun: true,
	})
	if err == nil || !strings.Contains(err.Error(), "no configuration binding") {
		t.Fatalf("dry run err = %v, want missing binding", err)
	}
	dir := filepath.Join(root, ".paseka", "runs", "trace-dry-unbound", "config")
	if _, err := os.Stat(dir); !os.IsNotExist(err) {
		t.Fatalf("dry run created %s (stat err = %v)", dir, err)
	}
}

func TestApplyDryRunReportsBlockersWithoutRecording(t *testing.T) {
	root := writeColony(t, "slug: test\n")
	setupHome(t, "test")
	ctx := testContext(root)
	manifest, err := trailconfig.Ensure(ctx, "trace-dry-blocked")
	if err != nil {
		t.Fatal(err)
	}
	writeRemainingTask(t, root, "trace-dry-blocked", taskledger.TaskSnapshot{
		TaskID: "task-1", Title: "implement", Bee: "builder", Status: protocol.TaskStatusRunning,
	})
	if err := os.WriteFile(filepath.Join(root, ".paseka", "prompts", "builder.md"), []byte("blocked change\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	expected := manifest.ConfigRevision
	result, err := trailconfig.Apply(trailconfig.ApplyOptions{
		Context: ctx, TraceID: "trace-dry-blocked", Source: trailconfig.SourceColony,
		Actor: "operator", ExpectedRevision: &expected, DryRun: true,
	})
	if err != nil {
		t.Fatal(err)
	}
	if result.Change.Outcome != trailconfig.OutcomePending {
		t.Fatalf("outcome = %q, want pending", result.Change.Outcome)
	}
	if !strings.Contains(strings.Join(result.Change.Blockers, ","), "task-1") {
		t.Fatalf("blockers = %v", result.Change.Blockers)
	}

	reloaded, err := trailconfig.Load(root, "trace-dry-blocked")
	if err != nil {
		t.Fatal(err)
	}
	if reloaded.ConfigRevision != manifest.ConfigRevision || reloaded.Pending != nil || len(reloaded.History) != len(manifest.History) {
		t.Fatalf("dry run recorded a change: %+v", reloaded)
	}
	assertSnapshotCount(t, root, "trace-dry-blocked", 1)
}

func TestApplyDryRunPredictsImmediateApply(t *testing.T) {
	root := writeColony(t, "slug: test\n")
	setupHome(t, "test")
	ctx := testContext(root)
	manifest, err := trailconfig.Ensure(ctx, "trace-dry-ready")
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, ".paseka", "prompts", "builder.md"), []byte("ready change\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	expected := manifest.ConfigRevision
	result, err := trailconfig.Apply(trailconfig.ApplyOptions{
		Context: ctx, TraceID: "trace-dry-ready", Source: trailconfig.SourceColony,
		Actor: "operator", ExpectedRevision: &expected, DryRun: true,
	})
	if err != nil {
		t.Fatal(err)
	}
	if result.Change.Outcome != trailconfig.OutcomeApplied {
		t.Fatalf("outcome = %q, want predicted applied", result.Change.Outcome)
	}
	if len(result.Change.Blockers) != 0 {
		t.Fatalf("blockers = %v", result.Change.Blockers)
	}
	if result.Change.AppliedAt != nil {
		t.Fatal("dry run marked the change as applied")
	}
	reloaded, err := trailconfig.Load(root, "trace-dry-ready")
	if err != nil {
		t.Fatal(err)
	}
	if reloaded.ConfigRevision != manifest.ConfigRevision || reloaded.ActiveConfigRef != manifest.ActiveConfigRef {
		t.Fatalf("dry run changed the binding: %+v", reloaded)
	}
	assertSnapshotCount(t, root, "trace-dry-ready", 1)
}

func TestApplyDryRunReportsUnchangedCandidate(t *testing.T) {
	root := writeColony(t, "slug: test\n")
	setupHome(t, "test")
	ctx := testContext(root)
	manifest, err := trailconfig.Ensure(ctx, "trace-dry-same")
	if err != nil {
		t.Fatal(err)
	}

	expected := manifest.ConfigRevision
	result, err := trailconfig.Apply(trailconfig.ApplyOptions{
		Context: ctx, TraceID: "trace-dry-same", Source: trailconfig.SourceColony,
		Actor: "operator", ExpectedRevision: &expected, DryRun: true,
	})
	if err != nil {
		t.Fatal(err)
	}
	if result.Change.Outcome != trailconfig.OutcomeUnchanged {
		t.Fatalf("outcome = %q, want unchanged", result.Change.Outcome)
	}
	if result.NewRef != manifest.ActiveConfigRef {
		t.Fatalf("new ref = %q, want %q", result.NewRef, manifest.ActiveConfigRef)
	}
	reloaded, err := trailconfig.Load(root, "trace-dry-same")
	if err != nil {
		t.Fatal(err)
	}
	if len(reloaded.History) != len(manifest.History) {
		t.Fatalf("dry run appended history: %d entries", len(reloaded.History))
	}
}

func assertSnapshotCount(t *testing.T, root, traceID string, want int) {
	t.Helper()
	entries, err := os.ReadDir(filepath.Join(trailconfig.ConfigDir(root, traceID), "snapshots"))
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) != want {
		t.Fatalf("snapshots = %d, want %d", len(entries), want)
	}
}
