package trailconfig_test

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/russ-p/paseka/internal/protocol"
	"github.com/russ-p/paseka/internal/runs"
	"github.com/russ-p/paseka/internal/taskledger"
	"github.com/russ-p/paseka/internal/trailconfig"
)

func TestApplyColonyAdvancesConfigurationAndPreservesWork(t *testing.T) {
	root := writeColony(t, "slug: test\nmodel_aliases:\n  builder: vendor/builder\n")
	setupHome(t, "test")
	ctx := testContext(root)
	manifest, err := trailconfig.Ensure(ctx, "trace-apply")
	if err != nil {
		t.Fatal(err)
	}
	writeRemainingTask(t, root, "trace-apply", taskledger.TaskSnapshot{
		TaskID: "task-1", Title: "implement", Bee: "builder", Status: protocol.TaskStatusPlanned,
	})
	codePath := filepath.Join(root, "main.go")
	if err := os.WriteFile(codePath, []byte("package main\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	taskPath := filepath.Join(root, ".paseka", "runs", "trace-apply", "tasks", "task-1", runs.TaskFileName)
	taskBefore, err := os.ReadFile(taskPath)
	if err != nil {
		t.Fatal(err)
	}

	if err := os.WriteFile(filepath.Join(root, ".paseka", "prompts", "builder.md"), []byte("new instructions\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	expected := manifest.ConfigRevision
	result, err := trailconfig.Apply(trailconfig.ApplyOptions{
		Context: ctx, TraceID: "trace-apply", Source: trailconfig.SourceColony,
		Actor: "operator", Reason: "try new procedure", RequestID: "request-1",
		ExpectedRevision: &expected,
	})
	if err != nil {
		t.Fatal(err)
	}
	if result.Change.Outcome != trailconfig.OutcomeApplied {
		t.Fatalf("outcome = %q", result.Change.Outcome)
	}
	if result.Manifest.ConfigRevision != 2 || result.Manifest.ActiveConfigRef != result.NewRef {
		t.Fatalf("binding = revision %d active %s new %s", result.Manifest.ConfigRevision, result.Manifest.ActiveConfigRef, result.NewRef)
	}
	if result.NewRef == manifest.ActiveConfigRef {
		t.Fatal("configuration ref did not change")
	}
	if len(result.Diff) == 0 {
		t.Fatal("expected a preview diff")
	}
	last := result.Manifest.History[len(result.Manifest.History)-1]
	if last.ChangeID == "" || last.Actor != "operator" || last.Source != trailconfig.SourceColony ||
		last.OldConfigRef != manifest.ActiveConfigRef || last.NewConfigRef != result.NewRef ||
		last.ExpectedRevision != 1 || last.Outcome != trailconfig.OutcomeApplied || last.AppliedAt == nil {
		t.Fatalf("history entry = %+v", last)
	}
	if last.Reason != "try new procedure" || last.RequestID != "request-1" {
		t.Fatalf("history provenance = %+v", last)
	}
	if data, err := os.ReadFile(codePath); err != nil || string(data) != "package main\n" {
		t.Fatalf("code changed: %q %v", data, err)
	}
	if data, err := os.ReadFile(taskPath); err != nil || string(data) != string(taskBefore) {
		t.Fatalf("task progress changed: %q %v", data, err)
	}
	if _, err := os.Stat(filepath.Join(root, ".paseka", "runs", "trace-apply", "config", "snapshots", result.NewRef, "tree", ".paseka", "prompts", "builder.md")); err != nil {
		t.Fatalf("candidate snapshot missing: %v", err)
	}
}

func TestApplyIdenticalContentRecordsUnchanged(t *testing.T) {
	root := writeColony(t, "slug: test\n")
	setupHome(t, "test")
	ctx := testContext(root)
	manifest, err := trailconfig.Ensure(ctx, "trace-unchanged")
	if err != nil {
		t.Fatal(err)
	}
	expected := manifest.ConfigRevision
	result, err := trailconfig.Apply(trailconfig.ApplyOptions{
		Context: ctx, TraceID: "trace-unchanged", Source: trailconfig.SourceColony,
		Actor: "operator", ExpectedRevision: &expected,
	})
	if err != nil {
		t.Fatal(err)
	}
	if result.Change.Outcome != trailconfig.OutcomeUnchanged {
		t.Fatalf("outcome = %q", result.Change.Outcome)
	}
	if result.Manifest.ConfigRevision != manifest.ConfigRevision || result.Manifest.ActiveConfigRef != manifest.ActiveConfigRef {
		t.Fatalf("unchanged advanced the binding: %+v", result.Manifest)
	}
	if len(result.Manifest.History) != 2 {
		t.Fatalf("history = %d, want 2", len(result.Manifest.History))
	}
	if result.Manifest.History[1].AppliedAt != nil {
		t.Fatal("unchanged recorded an applied time")
	}
}

func TestApplyRejectsStaleExpectedRevision(t *testing.T) {
	root := writeColony(t, "slug: test\n")
	setupHome(t, "test")
	ctx := testContext(root)
	manifest, err := trailconfig.Ensure(ctx, "trace-stale")
	if err != nil {
		t.Fatal(err)
	}
	stale := manifest.ConfigRevision + 5
	result, err := trailconfig.Apply(trailconfig.ApplyOptions{
		Context: ctx, TraceID: "trace-stale", Source: trailconfig.SourceColony,
		Actor: "operator", ExpectedRevision: &stale,
	})
	if err != nil {
		t.Fatal(err)
	}
	if result.Change.Outcome != trailconfig.OutcomeRejected {
		t.Fatalf("outcome = %q", result.Change.Outcome)
	}
	if !strings.Contains(result.Change.Detail, "expected revision") {
		t.Fatalf("detail = %q", result.Change.Detail)
	}
	reloaded, err := trailconfig.Load(root, "trace-stale")
	if err != nil {
		t.Fatal(err)
	}
	if reloaded.ConfigRevision != manifest.ConfigRevision || reloaded.ActiveConfigRef != manifest.ActiveConfigRef {
		t.Fatalf("stale request changed the binding: %+v", reloaded)
	}
	if len(reloaded.History) != 2 || reloaded.History[1].Outcome != trailconfig.OutcomeRejected {
		t.Fatalf("rejection not recorded: %+v", reloaded.History)
	}
}

func TestApplyRefusesCompetingPendingRequest(t *testing.T) {
	root := writeColony(t, "slug: test\n")
	setupHome(t, "test")
	ctx := testContext(root)
	manifest, err := trailconfig.Ensure(ctx, "trace-pending")
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, ".paseka", "prompts", "builder.md"), []byte("pending procedure\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	expected := manifest.ConfigRevision
	first, err := trailconfig.Apply(trailconfig.ApplyOptions{
		Context: ctx, TraceID: "trace-pending", Source: trailconfig.SourceColony,
		Actor: "operator", ExpectedRevision: &expected, Blockers: []string{"task task-1 is running"},
	})
	if err != nil {
		t.Fatal(err)
	}
	if first.Change.Outcome != trailconfig.OutcomePending || first.Manifest.Pending == nil {
		t.Fatalf("first apply = %+v", first)
	}
	second, err := trailconfig.Apply(trailconfig.ApplyOptions{
		Context: ctx, TraceID: "trace-pending", Source: trailconfig.SourceColony,
		Actor: "operator", ExpectedRevision: &expected,
	})
	if err != nil {
		t.Fatal(err)
	}
	if second.Change.Outcome != trailconfig.OutcomeRejected {
		t.Fatalf("competing outcome = %q", second.Change.Outcome)
	}
	if second.Manifest.ConfigRevision != manifest.ConfigRevision {
		t.Fatalf("competing request advanced revision to %d", second.Manifest.ConfigRevision)
	}

	activated, changed, err := trailconfig.TryActivate(root, "trace-pending", nil)
	if err != nil {
		t.Fatal(err)
	}
	if !changed || activated.ConfigRevision != 2 {
		t.Fatalf("pending change did not activate: %+v", activated)
	}
}

func TestApplyDuplicateRequestReturnsRecordedResult(t *testing.T) {
	root := writeColony(t, "slug: test\n")
	setupHome(t, "test")
	ctx := testContext(root)
	manifest, err := trailconfig.Ensure(ctx, "trace-idempotent")
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, ".paseka", "prompts", "builder.md"), []byte("first change\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	expected := manifest.ConfigRevision
	first, err := trailconfig.Apply(trailconfig.ApplyOptions{
		Context: ctx, TraceID: "trace-idempotent", Source: trailconfig.SourceColony,
		Actor: "operator", RequestID: "same-request", ExpectedRevision: &expected,
	})
	if err != nil {
		t.Fatal(err)
	}
	if first.Change.Outcome != trailconfig.OutcomeApplied {
		t.Fatalf("first outcome = %q", first.Change.Outcome)
	}

	if err := os.WriteFile(filepath.Join(root, ".paseka", "prompts", "builder.md"), []byte("second change\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	next := first.Manifest.ConfigRevision
	second, err := trailconfig.Apply(trailconfig.ApplyOptions{
		Context: ctx, TraceID: "trace-idempotent", Source: trailconfig.SourceColony,
		Actor: "operator", RequestID: "same-request", ExpectedRevision: &next,
	})
	if err != nil {
		t.Fatal(err)
	}
	if second.Change.Outcome != trailconfig.OutcomeApplied || second.Change.NewConfigRef != first.Change.NewConfigRef {
		t.Fatalf("duplicate returned %+v, want recorded %+v", second.Change, first.Change)
	}
	if second.Manifest.ConfigRevision != first.Manifest.ConfigRevision {
		t.Fatalf("duplicate advanced revision to %d", second.Manifest.ConfigRevision)
	}

	_, err = trailconfig.Apply(trailconfig.ApplyOptions{
		Context: ctx, TraceID: "trace-idempotent", Source: trailconfig.SourceSnapshot,
		SnapshotRef: first.Manifest.InitialConfigRef, Actor: "operator",
		RequestID: "same-request", ExpectedRevision: &next,
	})
	if err == nil || !strings.Contains(err.Error(), "different configuration request") {
		t.Fatalf("identity reuse err = %v", err)
	}
}

func TestApplyRejectsRemainingWorkMissingBee(t *testing.T) {
	root := writeColony(t, "slug: test\n")
	setupHome(t, "test")
	ctx := testContext(root)
	if _, err := trailconfig.Ensure(ctx, "trace-remaining"); err != nil {
		t.Fatal(err)
	}
	writeRemainingTask(t, root, "trace-remaining", taskledger.TaskSnapshot{
		TaskID: "task-2", Title: "review", Bee: "guard", Status: protocol.TaskStatusReady,
	})
	_, err := trailconfig.Apply(trailconfig.ApplyOptions{
		Context: ctx, TraceID: "trace-remaining", Source: trailconfig.SourceColony, Actor: "operator",
	})
	if err == nil || !strings.Contains(err.Error(), "removes bee") {
		t.Fatalf("remaining-work err = %v", err)
	}
	manifest, err := trailconfig.Load(root, "trace-remaining")
	if err != nil {
		t.Fatal(err)
	}
	if manifest.ConfigRevision != 1 {
		t.Fatalf("invalid candidate advanced revision to %d", manifest.ConfigRevision)
	}
}

func TestApplyDryRunMakesNoDurableChanges(t *testing.T) {
	root := writeColony(t, "slug: test\n")
	setupHome(t, "test")
	ctx := testContext(root)
	manifest, err := trailconfig.Ensure(ctx, "trace-dry")
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, ".paseka", "prompts", "builder.md"), []byte("dry change\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	expected := manifest.ConfigRevision
	result, err := trailconfig.Apply(trailconfig.ApplyOptions{
		Context: ctx, TraceID: "trace-dry", Source: trailconfig.SourceColony,
		Actor: "operator", ExpectedRevision: &expected, DryRun: true,
	})
	if err != nil {
		t.Fatal(err)
	}
	if !result.DryRun || result.NewRef == "" || len(result.Diff) == 0 {
		t.Fatalf("dry run result = %+v", result)
	}
	reloaded, err := trailconfig.Load(root, "trace-dry")
	if err != nil {
		t.Fatal(err)
	}
	if reloaded.ConfigRevision != manifest.ConfigRevision || reloaded.ActiveConfigRef != manifest.ActiveConfigRef {
		t.Fatalf("dry run changed binding: %+v", reloaded)
	}
	entries, err := os.ReadDir(filepath.Join(root, ".paseka", "runs", "trace-dry", "config", "snapshots"))
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) != 1 {
		t.Fatalf("dry run wrote snapshots: %d", len(entries))
	}
}

func TestApplySnapshotRollbackKeepsFutureOnly(t *testing.T) {
	root := writeColony(t, "slug: test\n")
	setupHome(t, "test")
	ctx := testContext(root)
	manifest, err := trailconfig.Ensure(ctx, "trace-rollback")
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, ".paseka", "prompts", "builder.md"), []byte("changed\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	expected := manifest.ConfigRevision
	applied, err := trailconfig.Apply(trailconfig.ApplyOptions{
		Context: ctx, TraceID: "trace-rollback", Source: trailconfig.SourceColony,
		Actor: "operator", ExpectedRevision: &expected,
	})
	if err != nil {
		t.Fatal(err)
	}
	if applied.Manifest.ConfigRevision != 2 {
		t.Fatalf("revision = %d", applied.Manifest.ConfigRevision)
	}
	rollbackExpected := applied.Manifest.ConfigRevision
	rolled, err := trailconfig.Apply(trailconfig.ApplyOptions{
		Context: ctx, TraceID: "trace-rollback", Source: trailconfig.SourceSnapshot,
		SnapshotRef: applied.Manifest.InitialConfigRef, Actor: "operator", ExpectedRevision: &rollbackExpected,
	})
	if err != nil {
		t.Fatal(err)
	}
	if rolled.Change.Outcome != trailconfig.OutcomeApplied || rolled.Manifest.ConfigRevision != 3 {
		t.Fatalf("rollback = %+v", rolled)
	}
	if rolled.Manifest.ActiveConfigRef != applied.Manifest.InitialConfigRef {
		t.Fatalf("rollback active = %s, want %s", rolled.Manifest.ActiveConfigRef, applied.Manifest.InitialConfigRef)
	}
}

func TestApplyRefusesPinnedTrail(t *testing.T) {
	root := writeColony(t, "slug: test\n")
	setupHome(t, "test")
	ctx := testContext(root)
	manifest, err := trailconfig.Ensure(ctx, "trace-pinned")
	if err != nil {
		t.Fatal(err)
	}
	manifest.Policy = trailconfig.PolicyPinned
	if err := writeManifestForTest(root, "trace-pinned", manifest); err != nil {
		t.Fatal(err)
	}
	if _, err := trailconfig.Apply(trailconfig.ApplyOptions{
		Context: ctx, TraceID: "trace-pinned", Source: trailconfig.SourceColony, Actor: "operator",
	}); err == nil || !strings.Contains(err.Error(), "pinned") {
		t.Fatalf("pinned apply err = %v", err)
	}
}

func writeRemainingTask(t *testing.T, root, traceID string, task taskledger.TaskSnapshot) {
	t.Helper()
	if err := runs.WriteTaskSnapshot(root, traceID, task); err != nil {
		t.Fatal(err)
	}
}

func writeManifestForTest(root, traceID string, manifest trailconfig.Manifest) error {
	data, err := jsonMarshalIndent(manifest)
	if err != nil {
		return err
	}
	return os.WriteFile(trailconfig.ManifestPath(root, traceID), data, 0o644)
}

func jsonMarshalIndent(value any) ([]byte, error) {
	data, err := json.MarshalIndent(value, "", "  ")
	if err != nil {
		return nil, err
	}
	return append(data, '\n'), nil
}

func TestApplyOverridesCandidateKeepsCodeAndIdentity(t *testing.T) {
	root := writeColony(t, "slug: test\n")
	setupHome(t, "test")
	ctx := testContext(root)
	manifest, err := trailconfig.Ensure(ctx, "trace-overrides")
	if err != nil {
		t.Fatal(err)
	}
	expected := manifest.ConfigRevision
	result, err := trailconfig.Apply(trailconfig.ApplyOptions{
		Context: ctx, TraceID: "trace-overrides", Source: trailconfig.SourceOverrides,
		Actor: "operator", ExpectedRevision: &expected,
		Overrides: []trailconfig.Override{{Role: "builder", Key: "timeout", Value: "12m"}},
	})
	if err != nil {
		t.Fatal(err)
	}
	if result.Change.Outcome != trailconfig.OutcomeApplied || result.Manifest.ConfigRevision != 2 {
		t.Fatalf("override apply = %+v", result)
	}
	if result.Manifest.InitialConfigRef != manifest.InitialConfigRef {
		t.Fatalf("initial ref changed to %s", result.Manifest.InitialConfigRef)
	}
	data, err := os.ReadFile(filepath.Join(root, ".paseka", "runs", "trace-overrides", "config", "snapshots", result.NewRef, "tree", ".paseka", "bees", "builder.yaml"))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(data), "12m") {
		t.Fatalf("override not materialized: %s", data)
	}
	live, err := os.ReadFile(filepath.Join(root, ".paseka", "bees", "builder.yaml"))
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(live), "12m") {
		t.Fatal("override leaked into the colony")
	}
}

func TestApplyOverridesRefusesPolicyKey(t *testing.T) {
	root := writeColony(t, "slug: test\n")
	setupHome(t, "test")
	ctx := testContext(root)
	if _, err := trailconfig.Ensure(ctx, "trace-forbidden"); err != nil {
		t.Fatal(err)
	}
	_, err := trailconfig.Apply(trailconfig.ApplyOptions{
		Context: ctx, TraceID: "trace-forbidden", Source: trailconfig.SourceOverrides,
		Actor: "operator", Overrides: []trailconfig.Override{{Role: "builder", Key: "adapter", Value: "evil"}},
	})
	if err == nil || !strings.Contains(err.Error(), "cannot change colony policy") {
		t.Fatalf("forbidden override err = %v", err)
	}
}
