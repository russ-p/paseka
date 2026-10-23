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
	writeRemainingTask(t, root, "trace-rollback", taskledger.TaskSnapshot{
		TaskID: "task-1", Title: "finished", Bee: "builder", Status: protocol.TaskStatusCompleted,
	})
	taskPath := filepath.Join(root, ".paseka", "runs", "trace-rollback", "tasks", "task-1", runs.TaskFileName)
	taskBefore, err := os.ReadFile(taskPath)
	if err != nil {
		t.Fatal(err)
	}
	codePath := filepath.Join(root, "main.go")
	if err := os.WriteFile(codePath, []byte("package main\n"), 0o644); err != nil {
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
	if len(rolled.EffectiveDiff) == 0 {
		t.Fatal("rollback preview did not report the effective change")
	}
	prompt, err := os.ReadFile(filepath.Join(root, ".paseka", "runs", "trace-rollback", "config", "snapshots", rolled.Manifest.ActiveConfigRef, "tree", ".paseka", "prompts", "builder.md"))
	if err != nil {
		t.Fatal(err)
	}
	if string(prompt) != "build the thing\n" {
		t.Fatalf("rollback did not restore the complete selection: %q", prompt)
	}
	if data, err := os.ReadFile(codePath); err != nil || string(data) != "package main\n" {
		t.Fatalf("rollback changed completed code: %q %v", data, err)
	}
	if data, err := os.ReadFile(taskPath); err != nil || string(data) != string(taskBefore) {
		t.Fatalf("rollback changed completed task progress: %q %v", data, err)
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

func TestApplyOverridesPreviewReportsEffectiveShadowedAndRemainingWork(t *testing.T) {
	root := writeColony(t, "slug: test\n")
	setupHome(t, "test")
	beePath := filepath.Join(root, ".paseka", "bees", "builder.yaml")
	if err := os.WriteFile(beePath, []byte("role: builder\nadapter: cursor\nprompt_template: builder.md\nparams:\n  model: old\n  yolo: true\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	ctx := testContext(root)
	manifest, err := trailconfig.Ensure(ctx, "trace-preview")
	if err != nil {
		t.Fatal(err)
	}
	writeRemainingTask(t, root, "trace-preview", taskledger.TaskSnapshot{
		TaskID: "task-1", Title: "implement", Bee: "builder", Status: protocol.TaskStatusReady,
	})
	expected := manifest.ConfigRevision
	result, err := trailconfig.Apply(trailconfig.ApplyOptions{
		Context: ctx, TraceID: "trace-preview", Source: trailconfig.SourceOverrides,
		Actor: "operator", ExpectedRevision: &expected,
		Overrides: []trailconfig.Override{
			{Role: "builder", Key: "model", Value: "first"},
			{Role: "builder", Key: "model", Value: "second"},
			{Role: "builder", Key: "yolo", Value: "true"},
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	if result.Change.Outcome != trailconfig.OutcomeApplied || result.NewRef == manifest.ActiveConfigRef {
		t.Fatalf("override apply = %+v", result)
	}
	effective := strings.Join(result.EffectiveDiff, "\n")
	if !strings.Contains(effective, "bee builder param model: old -> second") {
		t.Fatalf("effective diff = %v", result.EffectiveDiff)
	}
	shadowed := strings.Join(result.ShadowedDiff, "\n")
	if !strings.Contains(shadowed, "shadowed by a later override") {
		t.Fatalf("superseded override not reported: %v", result.ShadowedDiff)
	}
	if !strings.Contains(shadowed, "already true") {
		t.Fatalf("no-op override not reported: %v", result.ShadowedDiff)
	}
	if !result.RemainingWork.Evaluated || !result.RemainingWork.Compatible {
		t.Fatalf("remaining work = %+v", result.RemainingWork)
	}
	if strings.Join(result.RemainingWork.Checked, ",") != "task-1" {
		t.Fatalf("checked tasks = %v", result.RemainingWork.Checked)
	}
}

func TestApplyOverridesRefuseProtectedCategories(t *testing.T) {
	root := writeColony(t, "slug: test\n")
	setupHome(t, "test")
	ctx := testContext(root)
	if _, err := trailconfig.Ensure(ctx, "trace-protected"); err != nil {
		t.Fatal(err)
	}
	keys := []string{
		"slug", "identity", "nats", "nats_url", "subject_prefix", "transport",
		"binary", "env", "env_mode", "api_key_env", "credentials",
		"energy", "energy_budget", "budget", "budget_ceiling",
		"delivery", "delivery_policy", "policy", "config_policy",
	}
	for _, key := range keys {
		_, err := trailconfig.Apply(trailconfig.ApplyOptions{
			Context: ctx, TraceID: "trace-protected", Source: trailconfig.SourceOverrides,
			Actor: "operator", Overrides: []trailconfig.Override{{Role: "builder", Key: key, Value: "x"}},
		})
		if err == nil || !strings.Contains(err.Error(), "cannot change") {
			t.Fatalf("override %q err = %v, want policy refusal", key, err)
		}
	}
}

func TestApplyRefusesUnretainedSnapshot(t *testing.T) {
	root := writeColony(t, "slug: test\n")
	setupHome(t, "test")
	ctx := testContext(root)
	if _, err := trailconfig.Ensure(ctx, "trace-unretained"); err != nil {
		t.Fatal(err)
	}
	unknown := "sha256:" + strings.Repeat("0", 64)
	_, err := trailconfig.Apply(trailconfig.ApplyOptions{
		Context: ctx, TraceID: "trace-unretained", Source: trailconfig.SourceSnapshot,
		SnapshotRef: unknown, Actor: "operator",
	})
	if err == nil || !strings.Contains(err.Error(), "not retained") {
		t.Fatalf("unretained snapshot err = %v", err)
	}
}

func TestApplyPendingBarrierActivatesOnceAtSafeBoundary(t *testing.T) {
	root := writeColony(t, "slug: test\n")
	setupHome(t, "test")
	ctx := testContext(root)
	manifest, err := trailconfig.Ensure(ctx, "trace-barrier")
	if err != nil {
		t.Fatal(err)
	}
	writeRemainingTask(t, root, "trace-barrier", taskledger.TaskSnapshot{
		TaskID: "task-1", Title: "implement", Bee: "builder", Status: protocol.TaskStatusRunning,
	})
	if err := os.WriteFile(filepath.Join(root, ".paseka", "prompts", "builder.md"), []byte("barrier\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	expected := manifest.ConfigRevision
	result, err := trailconfig.Apply(trailconfig.ApplyOptions{
		Context: ctx, TraceID: "trace-barrier", Source: trailconfig.SourceColony,
		Actor: "operator", ExpectedRevision: &expected,
	})
	if err != nil {
		t.Fatal(err)
	}
	if result.Change.Outcome != trailconfig.OutcomePending {
		t.Fatalf("outcome = %q, want pending", result.Change.Outcome)
	}
	if result.Manifest.ActiveConfigRef != manifest.ActiveConfigRef || result.Manifest.ConfigRevision != manifest.ConfigRevision {
		t.Fatalf("pending action changed the active binding: %+v", result.Manifest)
	}
	if len(result.Change.Blockers) == 0 || !strings.Contains(strings.Join(result.Change.Blockers, ","), "task-1") {
		t.Fatalf("blockers = %v", result.Change.Blockers)
	}

	writeRemainingTask(t, root, "trace-barrier", taskledger.TaskSnapshot{
		TaskID: "task-1", Title: "implement", Bee: "builder", Status: protocol.TaskStatusCompleted,
	})
	activated, changed, err := trailconfig.TryActivate(root, "trace-barrier", nil)
	if err != nil {
		t.Fatal(err)
	}
	if !changed || activated.ActiveConfigRef != result.NewRef || activated.ConfigRevision != manifest.ConfigRevision+1 {
		t.Fatalf("activation = %+v (changed %v)", activated, changed)
	}
	again, changedAgain, err := trailconfig.TryActivate(root, "trace-barrier", nil)
	if err != nil {
		t.Fatal(err)
	}
	if changedAgain || again.ConfigRevision != activated.ConfigRevision {
		t.Fatalf("duplicate activation applied twice: %+v", again)
	}
}

func TestApplyCancelPendingLeavesActiveAndRefusesAfterActivation(t *testing.T) {
	root := writeColony(t, "slug: test\n")
	setupHome(t, "test")
	ctx := testContext(root)
	manifest, err := trailconfig.Ensure(ctx, "trace-cancel")
	if err != nil {
		t.Fatal(err)
	}
	writeRemainingTask(t, root, "trace-cancel", taskledger.TaskSnapshot{
		TaskID: "task-1", Title: "implement", Bee: "builder", Status: protocol.TaskStatusRunning,
	})
	if err := os.WriteFile(filepath.Join(root, ".paseka", "prompts", "builder.md"), []byte("cancel me\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	expected := manifest.ConfigRevision
	pending, err := trailconfig.Apply(trailconfig.ApplyOptions{
		Context: ctx, TraceID: "trace-cancel", Source: trailconfig.SourceColony,
		Actor: "operator", ExpectedRevision: &expected,
	})
	if err != nil {
		t.Fatal(err)
	}
	cancelled, err := trailconfig.Cancel(root, "trace-cancel", pending.Change.ChangeID, "operator", "stay on old")
	if err != nil {
		t.Fatal(err)
	}
	if cancelled.Pending != nil || cancelled.ActiveConfigRef != manifest.ActiveConfigRef || cancelled.ConfigRevision != manifest.ConfigRevision {
		t.Fatalf("cancel changed the active binding: %+v", cancelled)
	}
	if _, err := trailconfig.Cancel(root, "trace-cancel", pending.Change.ChangeID, "operator", ""); err == nil {
		t.Fatal("second cancel unexpectedly succeeded")
	}

	writeRemainingTask(t, root, "trace-cancel", taskledger.TaskSnapshot{
		TaskID: "task-1", Title: "implement", Bee: "builder", Status: protocol.TaskStatusCompleted,
	})
	applied, err := trailconfig.Apply(trailconfig.ApplyOptions{
		Context: ctx, TraceID: "trace-cancel", Source: trailconfig.SourceColony,
		Actor: "operator", ExpectedRevision: &expected,
	})
	if err != nil {
		t.Fatal(err)
	}
	if applied.Change.Outcome != trailconfig.OutcomeApplied {
		t.Fatalf("apply after cancel = %q", applied.Change.Outcome)
	}
	if _, err := trailconfig.Cancel(root, "trace-cancel", applied.Change.ChangeID, "operator", ""); err == nil {
		t.Fatal("cancel after activation unexpectedly succeeded")
	}
}

func TestApplyDuplicateDeliveryRecoversPendingBarrier(t *testing.T) {
	root := writeColony(t, "slug: test\n")
	setupHome(t, "test")
	ctx := testContext(root)
	manifest, err := trailconfig.Ensure(ctx, "trace-recover")
	if err != nil {
		t.Fatal(err)
	}
	writeRemainingTask(t, root, "trace-recover", taskledger.TaskSnapshot{
		TaskID: "task-1", Title: "implement", Bee: "builder", Status: protocol.TaskStatusRunning,
	})
	if err := os.WriteFile(filepath.Join(root, ".paseka", "prompts", "builder.md"), []byte("recover\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	expected := manifest.ConfigRevision
	first, err := trailconfig.Apply(trailconfig.ApplyOptions{
		Context: ctx, TraceID: "trace-recover", Source: trailconfig.SourceColony,
		Actor: "operator", RequestID: "recover-request", ExpectedRevision: &expected,
	})
	if err != nil {
		t.Fatal(err)
	}
	if first.Change.Outcome != trailconfig.OutcomePending {
		t.Fatalf("first outcome = %q", first.Change.Outcome)
	}
	duplicate, err := trailconfig.Apply(trailconfig.ApplyOptions{
		Context: ctx, TraceID: "trace-recover", Source: trailconfig.SourceColony,
		Actor: "operator", RequestID: "recover-request", ExpectedRevision: &expected,
	})
	if err != nil {
		t.Fatal(err)
	}
	if duplicate.Change.ChangeID != first.Change.ChangeID || duplicate.Manifest.Pending == nil {
		t.Fatalf("duplicate did not return the pending action: %+v", duplicate.Change)
	}
	traceIDs, err := trailconfig.PendingTraceIDs(root)
	if err != nil {
		t.Fatal(err)
	}
	if len(traceIDs) != 1 || traceIDs[0] != "trace-recover" {
		t.Fatalf("pending traces = %v", traceIDs)
	}
	writeRemainingTask(t, root, "trace-recover", taskledger.TaskSnapshot{
		TaskID: "task-1", Title: "implement", Bee: "builder", Status: protocol.TaskStatusCompleted,
	})
	activated, changed, err := trailconfig.TryActivate(root, "trace-recover", nil)
	if err != nil {
		t.Fatal(err)
	}
	if !changed || activated.ConfigRevision != manifest.ConfigRevision+1 {
		t.Fatalf("pending barrier did not recover: %+v", activated)
	}
	after, err := trailconfig.Apply(trailconfig.ApplyOptions{
		Context: ctx, TraceID: "trace-recover", Source: trailconfig.SourceColony,
		Actor: "operator", RequestID: "recover-request", ExpectedRevision: &expected,
	})
	if err != nil {
		t.Fatal(err)
	}
	if after.Change.Outcome != trailconfig.OutcomeApplied || after.Manifest.ConfigRevision != activated.ConfigRevision {
		t.Fatalf("duplicate after activation applied again: %+v", after)
	}
}

func TestTryActivateRejectsCandidateIncompatibleWithNewRemainingWork(t *testing.T) {
	root := writeColony(t, "slug: test\n")
	setupHome(t, "test")
	ctx := testContext(root)
	manifest, err := trailconfig.Ensure(ctx, "trace-revalidate")
	if err != nil {
		t.Fatal(err)
	}
	writeRemainingTask(t, root, "trace-revalidate", taskledger.TaskSnapshot{
		TaskID: "task-1", Title: "implement", Bee: "builder", Status: protocol.TaskStatusRunning,
	})
	if err := os.WriteFile(filepath.Join(root, ".paseka", "prompts", "builder.md"), []byte("candidate\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	expected := manifest.ConfigRevision
	pending, err := trailconfig.Apply(trailconfig.ApplyOptions{
		Context: ctx, TraceID: "trace-revalidate", Source: trailconfig.SourceColony,
		Actor: "operator", ExpectedRevision: &expected,
	})
	if err != nil {
		t.Fatal(err)
	}
	if pending.Change.Outcome != trailconfig.OutcomePending {
		t.Fatalf("apply outcome = %q", pending.Change.Outcome)
	}
	writeRemainingTask(t, root, "trace-revalidate", taskledger.TaskSnapshot{
		TaskID: "task-2", Title: "review", Bee: "reviewer", Status: protocol.TaskStatusPlanned,
	})
	writeRemainingTask(t, root, "trace-revalidate", taskledger.TaskSnapshot{
		TaskID: "task-1", Title: "implement", Bee: "builder", Status: protocol.TaskStatusCompleted,
	})
	after, changed, err := trailconfig.TryActivate(root, "trace-revalidate", nil)
	if err != nil {
		t.Fatal(err)
	}
	if changed || after.Pending != nil {
		t.Fatalf("incompatible candidate was activated: %+v", after)
	}
	if after.ActiveConfigRef != manifest.ActiveConfigRef || after.ConfigRevision != manifest.ConfigRevision {
		t.Fatalf("rejection changed active config: %+v", after)
	}
	last := after.History[len(after.History)-1]
	if last.Outcome != trailconfig.OutcomeRejected || !strings.Contains(last.Detail, "reviewer") {
		t.Fatalf("rejection not recorded: %+v", last)
	}
}
