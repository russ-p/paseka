package runtime_test

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/russ-p/paseka/internal/adapters"
	"github.com/russ-p/paseka/internal/colony"
	"github.com/russ-p/paseka/internal/protocol"
	"github.com/russ-p/paseka/internal/runs"
	"github.com/russ-p/paseka/internal/runtime"
	"github.com/russ-p/paseka/internal/taskledger"
	"github.com/russ-p/paseka/internal/trailconfig"
)

func TestAttemptProvenanceSurvivesRetryAndRework(t *testing.T) {
	root := t.TempDir()
	bees := map[string]colony.Bee{
		"builder": {Role: "builder", Subscribes: []colony.SubscriptionRule{
			{EventRule: colony.EventRule{Type: "SIGNAL", Kind: "task.ready"}, Dispatch: colony.DispatchTask},
			{EventRule: colony.EventRule{Type: "VERIFICATION", Kind: "verification.failed"}, Dispatch: colony.DispatchDirect},
		}},
	}
	mustWriteTestColony(t, root, bees)

	plan, ready, rework, failed := taskAttemptEvents(t, "trace-attempt")
	r, rec := taskAttemptReactor(t, root, bees)
	if err := r.ProcessEvent(context.Background(), plan); err != nil {
		t.Fatal(err)
	}
	rec.result = &adapters.RunResult{Status: "completed", Summary: "waiting for review"}
	if err := r.ProcessEvent(context.Background(), ready); err != nil {
		t.Fatal(err)
	}

	attempts, err := trailconfig.Attempts(root, "trace-attempt")
	if err != nil {
		t.Fatal(err)
	}
	if len(attempts) != 1 {
		t.Fatalf("attempts = %d, want 1", len(attempts))
	}
	first := attempts[0]
	if first.State != "open" || first.Kind != trailconfig.AttemptKindTask {
		t.Fatalf("first attempt = %+v", first)
	}
	if first.ConfigRef == "" || first.ConfigRevision != 1 {
		t.Fatalf("first config = %q rev %d", first.ConfigRef, first.ConfigRevision)
	}
	if first.InitialCodeBaseSHA == "" {
		t.Fatal("first attempt did not record initial code base")
	}
	if first.TaskInput == nil || first.TaskInput.Digest == "" || first.TaskInput.Body != "do work" {
		t.Fatalf("task input evidence = %+v", first.TaskInput)
	}
	if first.SourceEventID != ready.ID || first.LastDispatchKey != first.AttemptID+"|"+ready.ID {
		t.Fatalf("first ownership source/dispatch = %q/%q", first.SourceEventID, first.LastDispatchKey)
	}
	if len(first.Agents) != 1 || first.Agents[0] != rec.lastReq.AgentID {
		t.Fatalf("first agents = %v, want %q", first.Agents, rec.lastReq.AgentID)
	}
	assertDispatchProvenance(t, root, "trace-attempt", rec, first)

	calls := rec.calls
	if err := r.ProcessEvent(context.Background(), rework); err != nil {
		t.Fatal(err)
	}
	if rec.calls != calls+1 {
		t.Fatalf("rework calls = %d, want %d", rec.calls, calls+1)
	}
	if rec.lastReq.AttemptID != first.AttemptID {
		t.Fatalf("rework attempt = %q, want %q", rec.lastReq.AttemptID, first.AttemptID)
	}
	attempts, err = trailconfig.Attempts(root, "trace-attempt")
	if err != nil {
		t.Fatal(err)
	}
	if len(attempts) != 1 {
		t.Fatalf("rework minted a new attempt: %d", len(attempts))
	}

	activateNewTrailConfig(t, root, "trace-attempt")
	reworkAfterActivation, err := protocol.NewEvent("trace-attempt", "guard", 5, protocol.EventVerification, protocol.VerificationPayload{
		Kind: protocol.VerificationFailed, TaskID: "task-1", Summary: "still failing",
	})
	if err != nil {
		t.Fatal(err)
	}
	if err := r.ProcessEvent(context.Background(), reworkAfterActivation); err != nil {
		t.Fatal(err)
	}
	if rec.calls != calls+2 {
		t.Fatalf("second rework calls = %d, want %d", rec.calls, calls+2)
	}
	if rec.lastReq.AttemptID != first.AttemptID || rec.lastReq.ConfigRevision != 1 {
		t.Fatalf("rework after activation ran under %q rev %d, want original %q rev 1", rec.lastReq.AttemptID, rec.lastReq.ConfigRevision, first.AttemptID)
	}
	if strings.Contains(rec.lastReq.Prompt, "REWRITTEN") {
		t.Fatalf("rework prompt was reinterpreted under the active config: %q", rec.lastReq.Prompt)
	}

	if err := r.ProcessEvent(context.Background(), failed); err != nil {
		t.Fatal(err)
	}
	attempts, err = trailconfig.Attempts(root, "trace-attempt")
	if err != nil {
		t.Fatal(err)
	}
	if len(attempts) != 1 || attempts[0].State != "closed" {
		t.Fatalf("attempts after failure = %+v", attempts)
	}

	retry := retryEvent(t, "trace-attempt")
	rec.result = &adapters.RunResult{Status: "completed", Summary: "retry done"}
	if err := r.ProcessEvent(context.Background(), retry); err != nil {
		t.Fatal(err)
	}
	attempts, err = trailconfig.Attempts(root, "trace-attempt")
	if err != nil {
		t.Fatal(err)
	}
	if len(attempts) != 2 {
		t.Fatalf("attempts after retry = %d, want 2", len(attempts))
	}
	retried := attempts[1]
	if retried.AttemptID == first.AttemptID || retried.State != "open" {
		t.Fatalf("retry attempt = %+v", retried)
	}
	manifest, err := trailconfig.Load(root, "trace-attempt")
	if err != nil {
		t.Fatal(err)
	}
	if retried.ConfigRef != manifest.ActiveConfigRef || retried.ConfigRevision != 2 {
		t.Fatalf("retry config = %q rev %d, want %q rev 2", retried.ConfigRef, retried.ConfigRevision, manifest.ActiveConfigRef)
	}
	assertDispatchProvenance(t, root, "trace-attempt", rec, retried)

	stale, err := protocol.NewEvent("trace-attempt", "old-builder", 6, protocol.EventVerification, protocol.TaskCompletedPayload{
		Kind: protocol.TaskEventCompleted, TaskID: "task-1", Status: protocol.TaskStatusCompleted, Summary: "old attempt done",
	})
	if err != nil {
		t.Fatal(err)
	}
	stale.Provenance = trailconfigAttemptProvenance(first)
	if err := r.ProcessEvent(context.Background(), stale); err != nil {
		t.Fatal(err)
	}
	attempts, err = trailconfig.Attempts(root, "trace-attempt")
	if err != nil {
		t.Fatal(err)
	}
	if attempts[1].AttemptID != retried.AttemptID || attempts[1].State != "open" {
		t.Fatalf("stale completion closed the active attempt: %+v", attempts[1])
	}
}

func trailconfigAttemptProvenance(a trailconfig.Attempt) protocol.Provenance {
	return protocol.Provenance{
		AttemptID:            a.AttemptID,
		ConfigRef:            a.ConfigRef,
		ConfigRevision:       a.ConfigRevision,
		InitialCodeBaseSHA:   a.InitialCodeBaseSHA,
		InitialCodeBaseDirty: a.InitialCodeBaseDirty,
	}
}

func TestClosedDirectAttemptCannotReopenUnderNewConfiguration(t *testing.T) {
	root := t.TempDir()
	bees := map[string]colony.Bee{
		"builder": {Role: "builder", Subscribes: []colony.SubscriptionRule{
			{EventRule: colony.EventRule{Type: "SIGNAL", Kind: "feature.requested"}, Dispatch: colony.DispatchDirect},
		}},
		"guard": {Role: "guard", Subscribes: []colony.SubscriptionRule{
			{EventRule: colony.EventRule{Type: "MUTATION", Kind: "code.proposal.root"}, Dispatch: colony.DispatchDirect},
		}},
	}
	mustWriteTestColony(t, root, bees)

	ev := featureEvent(t, "trace-closed", "scout", "original")
	r, rec := directAttemptReactor(t, root, bees)
	if err := r.ProcessEvent(context.Background(), ev); err != nil {
		t.Fatal(err)
	}
	if rec.calls != 1 {
		t.Fatalf("calls = %d, want 1", rec.calls)
	}
	attempts, err := trailconfig.Attempts(root, "trace-closed")
	if err != nil {
		t.Fatal(err)
	}
	if len(attempts) != 1 || attempts[0].Kind != trailconfig.AttemptKindDirect {
		t.Fatalf("chain attempts = %+v", attempts)
	}
	chain := attempts[0]
	proposal, err := protocol.NewEvent("trace-closed", rec.lastReq.AgentID, 1, protocol.EventMutation, protocol.MutationPayload{
		Kind: protocol.MutationCodeProposalRoot, Summary: "fix", Diff: "+fix", TaskID: "task-1",
	})
	if err != nil {
		t.Fatal(err)
	}
	proposal.Provenance = trailconfigAttemptProvenance(chain)
	if err := r.ProcessEvent(context.Background(), proposal); err != nil {
		t.Fatal(err)
	}
	if rec.calls != 2 || rec.lastReq.Bee != "guard" {
		t.Fatalf("guard calls = %d bee %q, want same-chain dispatch", rec.calls, rec.lastReq.Bee)
	}
	if rec.lastReq.AttemptID != chain.AttemptID || rec.lastReq.ConfigRevision != chain.ConfigRevision {
		t.Fatalf("guard attempt = %q rev %d, want %q rev %d", rec.lastReq.AttemptID, rec.lastReq.ConfigRevision, chain.AttemptID, chain.ConfigRevision)
	}

	if err := trailconfig.CloseTraceAttempts(root, "trace-closed", "test drain"); err != nil {
		t.Fatal(err)
	}
	activateNewTrailConfig(t, root, "trace-closed")

	r2, rec2 := directAttemptReactor(t, root, bees)
	if err := r2.ProcessEvent(context.Background(), ev); err != nil {
		t.Fatal(err)
	}
	if rec2.calls != 0 {
		t.Fatalf("replayed closed direct event calls = %d, want 0", rec2.calls)
	}
	replayed := ev
	replayed.ID = "replayed-" + ev.ID
	replayed.Seq = ev.Seq + 10
	replayed.Provenance = protocol.Provenance{AttemptID: chain.AttemptID}
	if err := r2.ProcessEvent(context.Background(), replayed); err != nil {
		t.Fatal(err)
	}
	if rec2.calls != 0 {
		t.Fatalf("closed attempt event calls = %d, want 0", rec2.calls)
	}
}

func taskAttemptEvents(t *testing.T, traceID string) (protocol.Event, protocol.Event, protocol.Event, protocol.Event) {
	t.Helper()
	plan, err := protocol.NewEvent(traceID, "scout", 0, protocol.EventInsight, protocol.TaskPlanPayload{
		Kind: protocol.TaskEventPlan,
		Tasks: []protocol.TaskSpec{{
			TaskID: "task-1", Title: "implement", Body: "do work", Bee: "builder", Intent: "feature",
			Review: protocol.TaskReviewRequired,
		}},
	})
	if err != nil {
		t.Fatal(err)
	}
	ready, err := protocol.NewEvent(traceID, "scout", 1, protocol.EventSignal, protocol.TaskReadyPayload{
		Kind: protocol.TaskEventReady, TaskID: "task-1", Bee: "builder",
	})
	if err != nil {
		t.Fatal(err)
	}
	rework, err := protocol.NewEvent(traceID, "guard", 2, protocol.EventVerification, protocol.VerificationPayload{
		Kind: protocol.VerificationFailed, TaskID: "task-1", Summary: "tests failed",
	})
	if err != nil {
		t.Fatal(err)
	}
	failed, err := protocol.NewEvent(traceID, "runtime", 3, protocol.EventVerification, protocol.TaskCompletedPayload{
		Kind: protocol.TaskEventCompleted, TaskID: "task-1", Status: protocol.TaskStatusFailed, Summary: "give up",
	})
	if err != nil {
		t.Fatal(err)
	}
	return plan, ready, rework, failed
}

func retryEvent(t *testing.T, traceID string) protocol.Event {
	t.Helper()
	ev, err := protocol.NewEvent(traceID, "cli", 4, protocol.EventSignal, protocol.TaskReadyPayload{
		Kind: protocol.TaskEventReady, TaskID: "task-1", Bee: "builder",
	})
	if err != nil {
		t.Fatal(err)
	}
	return ev
}

func taskAttemptReactor(t *testing.T, root string, bees map[string]colony.Bee) (*runtime.Reactor, *recordingAdapter) {
	t.Helper()
	d := runtime.NewDispatcher()
	reg := runtime.NewBeeRegistryFromBees(bees)
	d.SetBeeRegistry(reg)
	rec := &recordingAdapter{}
	d.RegisterAdapter("cursor", rec)
	r := runtime.NewTestReactor(runtime.TestReactorOptions{
		ColonyRoot: root,
		Dispatcher: d,
		Registry:   reg,
		Ledger:     taskledger.NewMemoryLedger(),
	})
	return r, rec
}

func directAttemptReactor(t *testing.T, root string, bees map[string]colony.Bee) (*runtime.Reactor, *recordingAdapter) {
	return taskAttemptReactor(t, root, bees)
}

func assertDispatchProvenance(t *testing.T, root, traceID string, rec *recordingAdapter, attempt trailconfig.Attempt) {
	t.Helper()
	if rec.lastReq.AttemptID != attempt.AttemptID ||
		rec.lastReq.ConfigRef != attempt.ConfigRef ||
		rec.lastReq.ConfigRevision != attempt.ConfigRevision ||
		rec.lastReq.InitialCodeBaseSHA != attempt.InitialCodeBaseSHA {
		t.Fatalf("run request provenance = %+v, want attempt %+v", rec.lastReq.Provenance, attempt)
	}
	runDir := runs.Dir{ColonyRoot: root, TraceID: traceID, AgentID: rec.lastReq.AgentID}
	req, err := runDir.ReadRequest()
	if err != nil {
		t.Fatal(err)
	}
	if req.AttemptID != attempt.AttemptID || req.ConfigRef != attempt.ConfigRef || req.ConfigRevision != attempt.ConfigRevision {
		t.Fatalf("request provenance = %+v", req)
	}
	status, err := runDir.ReadStatus()
	if err != nil {
		t.Fatal(err)
	}
	if status.AttemptID != attempt.AttemptID || status.ConfigRef != attempt.ConfigRef || status.ConfigRevision != attempt.ConfigRevision {
		t.Fatalf("status provenance = %+v", status.Provenance)
	}
}

// activateNewTrailConfig appends a revision whose default prompt is REWRITTEN.
// It mirrors what a future config activation does so tests can prove an old
// attempt keeps its original snapshot.
func activateNewTrailConfig(t *testing.T, root, traceID string) trailconfig.Manifest {
	t.Helper()
	manifest, err := trailconfig.Load(root, traceID)
	if err != nil {
		t.Fatal(err)
	}
	oldDir := filepath.Join(trailconfig.SnapshotDir(root, traceID), manifest.ActiveConfigRef)
	var inventory trailconfig.Inventory
	data, err := os.ReadFile(filepath.Join(oldDir, trailconfig.InventoryFileName))
	if err != nil {
		t.Fatal(err)
	}
	if err := json.Unmarshal(data, &inventory); err != nil {
		t.Fatal(err)
	}
	content := []byte("REWRITTEN {{.Task}}")
	const rel = ".paseka/prompts/default.md"
	sum := sha256.Sum256(content)
	found := false
	for i, entry := range inventory.Entries {
		if filepath.ToSlash(entry.Path) != rel {
			continue
		}
		inventory.Entries[i].SHA256 = hex.EncodeToString(sum[:])
		inventory.Entries[i].Size = int64(len(content))
		found = true
	}
	if !found {
		t.Fatalf("snapshot has no %s entry", rel)
	}
	hashInput := inventory
	hashInput.Resolved.ProfileLayers = trailconfig.ProfileLayers{}
	encoded, err := json.Marshal(hashInput)
	if err != nil {
		t.Fatal(err)
	}
	refSum := sha256.Sum256(encoded)
	ref := "sha256:" + hex.EncodeToString(refSum[:])
	newDir := filepath.Join(trailconfig.SnapshotDir(root, traceID), ref)
	if err := copyDir(oldDir, newDir); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(newDir, trailconfig.SnapshotTreeDirName, filepath.FromSlash(rel)), content, 0o644); err != nil {
		t.Fatal(err)
	}
	inventoryData, err := json.MarshalIndent(inventory, "", "  ")
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(newDir, trailconfig.InventoryFileName), append(inventoryData, '\n'), 0o644); err != nil {
		t.Fatal(err)
	}

	next := manifest.ConfigRevision + 1
	manifest.ActiveConfigRef = ref
	manifest.ConfigRevision = next
	manifest.UpdatedAt = manifest.UpdatedAt.Add(1)
	manifest.Snapshots = append(manifest.Snapshots, trailconfig.Snapshot{
		ConfigRef: ref,
		Path:      filepath.ToSlash(filepath.Join("snapshots", ref)),
		Files:     len(inventory.Entries),
	})
	manifest.History = append(manifest.History, trailconfig.Revision{
		Revision:         next,
		InitialConfigRef: manifest.InitialConfigRef,
		ActiveConfigRef:  ref,
	})
	manifestData, err := json.MarshalIndent(manifest, "", "  ")
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(trailconfig.ManifestPath(root, traceID), append(manifestData, '\n'), 0o644); err != nil {
		t.Fatal(err)
	}
	return manifest
}

func copyDir(src, dst string) error {
	return filepath.Walk(src, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		rel, err := filepath.Rel(src, path)
		if err != nil {
			return err
		}
		target := filepath.Join(dst, rel)
		if info.IsDir() {
			return os.MkdirAll(target, info.Mode())
		}
		in, err := os.Open(path)
		if err != nil {
			return err
		}
		defer in.Close()
		out, err := os.OpenFile(target, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, info.Mode())
		if err != nil {
			return err
		}
		if _, err := io.Copy(out, in); err != nil {
			out.Close()
			return err
		}
		return out.Close()
	})
}
