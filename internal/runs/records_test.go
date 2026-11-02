package runs_test

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/russ-p/paseka/internal/protocol"
	"github.com/russ-p/paseka/internal/runs"
)

func TestReconcileRunRecordsAppliesVerdictAndPreservesEvidence(t *testing.T) {
	root := t.TempDir()
	d := runs.Dir{ColonyRoot: root, TraceID: "trace-reconcile", AgentID: "agent-1"}
	if err := d.Prepare(); err != nil {
		t.Fatal(err)
	}

	started := time.Date(2026, 3, 1, 9, 0, 0, 0, time.UTC)
	finished := started.Add(4 * time.Minute)
	provenance := protocol.Provenance{AttemptID: "attempt-1", ConfigRef: "cfg", ConfigRevision: 3}
	if err := d.WriteResult(protocol.Result{
		TraceID:           "trace-reconcile",
		AgentID:           "agent-1",
		Status:            protocol.StatusCompleted,
		Summary:           "done",
		Artifacts:         []protocol.ArtifactRef{{Kind: "diff", Path: ".paseka/runs/x/diff.patch"}},
		Diagnostics:       protocol.Diagnostics{ExitCode: 0, Stderr: "kept"},
		Usage:             &protocol.Usage{InputTokens: 10, OutputTokens: 2},
		ProviderSessionID: "sess-1",
		FinishedAt:        finished,
		Provenance:        provenance,
	}); err != nil {
		t.Fatal(err)
	}
	if err := d.WriteStatusSnapshot(protocol.StatusSnapshot{
		State:      protocol.StatusCompleted,
		ExitCode:   0,
		StartedAt:  started,
		FinishedAt: finished,
		Provenance: provenance,
	}); err != nil {
		t.Fatal(err)
	}

	contract := &protocol.ContractRecord{
		Status:       protocol.ContractFailed,
		Rules:        []protocol.ContractRule{{Type: "VERIFICATION", KindOneOf: []string{"verification.success"}, Count: 1}},
		Observations: []protocol.ContractObservation{{Type: "VERIFICATION", Want: 1, Got: 0}},
		Error:        "runtime: completion contract: missing event",
	}
	if err := d.ReconcileRunRecords(runs.RunRecordReconciliation{
		State:    protocol.StatusFailed,
		Reason:   protocol.ReasonCompletionContract,
		Note:     contract.Error,
		Contract: contract,
	}); err != nil {
		t.Fatal(err)
	}

	snap, err := d.ReadStatus()
	if err != nil {
		t.Fatal(err)
	}
	if snap.State != protocol.StatusFailed || snap.Reason != protocol.ReasonCompletionContract {
		t.Fatalf("status = %s/%s", snap.State, snap.Reason)
	}
	if snap.Error != contract.Error {
		t.Fatalf("status error = %q, want %q", snap.Error, contract.Error)
	}
	if snap.ExitCode != 0 || !snap.StartedAt.Equal(started) || !snap.FinishedAt.Equal(finished) {
		t.Fatalf("status lost adapter evidence: %+v", snap)
	}
	if snap.Provenance != provenance {
		t.Fatalf("status provenance = %+v", snap.Provenance)
	}

	res, err := d.ReadResultJSON()
	if err != nil {
		t.Fatal(err)
	}
	if res.Status != protocol.StatusFailed || res.Reason != protocol.ReasonCompletionContract {
		t.Fatalf("result = %s/%s", res.Status, res.Reason)
	}
	if res.Diagnostics.Error != contract.Error || res.Diagnostics.ExitCode != 0 || res.Diagnostics.Stderr != "kept" {
		t.Fatalf("result diagnostics = %+v", res.Diagnostics)
	}
	if res.Usage == nil || res.Usage.InputTokens != 10 || res.ProviderSessionID != "sess-1" {
		t.Fatalf("result lost usage/session: %+v", res)
	}
	if len(res.Artifacts) != 1 || res.Artifacts[0].Kind != "diff" {
		t.Fatalf("result artifacts = %+v", res.Artifacts)
	}
	if !res.FinishedAt.Equal(finished) || res.Provenance != provenance {
		t.Fatalf("result lost timestamps/provenance: %+v", res)
	}
	if res.Contract == nil || res.Contract.Status != protocol.ContractFailed {
		t.Fatalf("contract = %+v", res.Contract)
	}
}

func TestReconcileRunRecordsIsIdempotent(t *testing.T) {
	root := t.TempDir()
	d := runs.Dir{ColonyRoot: root, TraceID: "trace-repeat", AgentID: "agent-1"}
	if err := d.Prepare(); err != nil {
		t.Fatal(err)
	}
	if err := d.WriteResult(protocol.Result{
		TraceID: "trace-repeat",
		AgentID: "agent-1",
		Status:  protocol.StatusCompleted,
	}); err != nil {
		t.Fatal(err)
	}
	if err := d.WriteStatus(protocol.StatusCompleted, "", 0, time.Time{}, time.Time{}, ""); err != nil {
		t.Fatal(err)
	}

	update := runs.RunRecordReconciliation{
		State:  protocol.StatusFailed,
		Reason: protocol.ReasonCompletionContract,
		Note:   "runtime: completion contract: missing event",
	}
	if err := d.ReconcileRunRecords(update); err != nil {
		t.Fatal(err)
	}
	statusFirst, err := os.ReadFile(d.StatusPath())
	if err != nil {
		t.Fatal(err)
	}
	resultFirst, err := os.ReadFile(d.ResultJSONPath())
	if err != nil {
		t.Fatal(err)
	}
	if err := d.ReconcileRunRecords(update); err != nil {
		t.Fatal(err)
	}
	statusSecond, err := os.ReadFile(d.StatusPath())
	if err != nil {
		t.Fatal(err)
	}
	resultSecond, err := os.ReadFile(d.ResultJSONPath())
	if err != nil {
		t.Fatal(err)
	}
	if string(statusFirst) != string(statusSecond) || string(resultFirst) != string(resultSecond) {
		t.Fatalf("repeated reconciliation changed records:\nstatus before %s\nafter %s\nresult before %s\nafter %s",
			statusFirst, statusSecond, resultFirst, resultSecond)
	}
}

func TestReconcileRunRecordsSurfacesUnreadableRecord(t *testing.T) {
	root := t.TempDir()
	d := runs.Dir{ColonyRoot: root, TraceID: "trace-broken", AgentID: "agent-1"}
	if err := d.Prepare(); err != nil {
		t.Fatal(err)
	}
	if err := os.Remove(d.StatusPath()); err != nil && !os.IsNotExist(err) {
		t.Fatal(err)
	}
	if err := os.Mkdir(d.StatusPath(), 0o755); err != nil {
		t.Fatal(err)
	}

	err := d.ReconcileRunRecords(runs.RunRecordReconciliation{State: protocol.StatusFailed, Reason: protocol.ReasonCompletionContract})
	if err == nil {
		t.Fatal("expected a finalization error for an unreadable status record")
	}
}

func TestReadResultWithoutContractStaysReadableAsUnknown(t *testing.T) {
	root := t.TempDir()
	d := runs.Dir{ColonyRoot: root, TraceID: "trace-legacy", AgentID: "agent-legacy"}
	if err := d.Prepare(); err != nil {
		t.Fatal(err)
	}
	legacy := []byte(`{
  "protocolVersion": "0.0.1",
  "traceId": "trace-legacy",
  "agentId": "agent-legacy",
  "status": "completed",
  "summary": "old run",
  "diagnostics": {"exitCode": 0}
}`)
	if err := os.WriteFile(d.ResultJSONPath(), legacy, 0o644); err != nil {
		t.Fatal(err)
	}

	res, err := d.ReadResultJSON()
	if err != nil {
		t.Fatalf("legacy result record is unreadable: %v", err)
	}
	if res.Status != protocol.StatusCompleted {
		t.Fatalf("recorded status = %q, want completed", res.Status)
	}
	// An absent contract proves neither the absence nor the success of one;
	// readers must treat it as unknown rather than as a pass.
	if res.Contract != nil {
		t.Fatalf("legacy contract = %+v, want unknown (nil)", res.Contract)
	}
}

func TestReconcileRunRecordsDoesNotCreateRecordsForContractOnly(t *testing.T) {
	root := t.TempDir()
	d := runs.Dir{ColonyRoot: root, TraceID: "trace-no-records", AgentID: "agent-1"}
	if err := os.MkdirAll(d.Root(), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := d.ReconcileRunRecords(runs.RunRecordReconciliation{Contract: &protocol.ContractRecord{Status: protocol.ContractPassed}}); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(d.StatusPath()); !os.IsNotExist(err) {
		t.Fatalf("status.json created for a run with no record: %v", err)
	}
	if _, err := os.Stat(filepath.Join(d.Root(), runs.ResultJSONFileName)); !os.IsNotExist(err) {
		t.Fatalf("result.json created for a run with no record: %v", err)
	}
}

func TestReconcileRunRecordsNeverRelabelsRecordedPrimaryFailure(t *testing.T) {
	root := t.TempDir()
	d := runs.Dir{ColonyRoot: root, TraceID: "trace-precedence", AgentID: "agent-1"}
	if err := d.Prepare(); err != nil {
		t.Fatal(err)
	}
	if err := d.WriteResult(protocol.Result{
		TraceID:     "trace-precedence",
		AgentID:     "agent-1",
		Status:      protocol.StatusCompleted,
		Diagnostics: protocol.Diagnostics{ExitCode: 0},
	}); err != nil {
		t.Fatal(err)
	}
	if err := d.WriteStatus(protocol.StatusCompleted, "", 0, time.Time{}, time.Time{}, ""); err != nil {
		t.Fatal(err)
	}

	gate := protocol.GateResult{
		Command:    []string{"sh", "-c", "exit 1"},
		ExitCode:   1,
		Reason:     "exit",
		Summary:    "checks failed (exit 1): sh -c exit 1",
		LogPath:    ".paseka/runs/trace-precedence/agent-1/checks.log",
		DurationMs: 12,
	}
	if err := d.WriteGateRecord(gate, protocol.StatusFailed, protocol.ReasonChecksFailed, gate.Summary); err != nil {
		t.Fatal(err)
	}

	contract := &protocol.ContractRecord{
		Status:       protocol.ContractFailed,
		Error:        "runtime: completion contract: expected at least 1 VERIFICATION event(s), got 0",
		Observations: []protocol.ContractObservation{{Type: "VERIFICATION", Want: 1, Got: 0}},
	}
	// A later contract pass asking to rewrite the outcome must lose to the
	// recorded gate rejection: precedence is enforced by the shared operation,
	// not by the caller remembering to skip it.
	if err := d.ReconcileRunRecords(runs.RunRecordReconciliation{
		State:    protocol.StatusFailed,
		Reason:   protocol.ReasonCompletionContract,
		Note:     contract.Error,
		Contract: contract,
	}); err != nil {
		t.Fatal(err)
	}

	snap, err := d.ReadStatus()
	if err != nil {
		t.Fatal(err)
	}
	if snap.State != protocol.StatusFailed || snap.Reason != protocol.ReasonChecksFailed {
		t.Fatalf("status = %s/%s, want failed/%s", snap.State, snap.Reason, protocol.ReasonChecksFailed)
	}
	if snap.Gate == nil || snap.Gate.Passed || snap.Gate.ExitCode != 1 || snap.Gate.LogPath != gate.LogPath {
		t.Fatalf("status gate = %+v, want the recorded rejection preserved", snap.Gate)
	}
	if strings.Contains(snap.Error, "completion contract") {
		t.Fatalf("status error = %q, want the gate message only", snap.Error)
	}

	res, err := d.ReadResultJSON()
	if err != nil {
		t.Fatal(err)
	}
	if res.Status != protocol.StatusFailed || res.Reason != protocol.ReasonChecksFailed {
		t.Fatalf("result = %s/%s, want failed/%s", res.Status, res.Reason, protocol.ReasonChecksFailed)
	}
	if res.Gate == nil || res.Gate.Passed || res.Gate.ExitCode != 1 {
		t.Fatalf("result gate = %+v, want the recorded rejection preserved", res.Gate)
	}
	if strings.Contains(res.Diagnostics.Error, "completion contract") {
		t.Fatalf("result diagnostics.error = %q, want the gate message only", res.Diagnostics.Error)
	}
	if res.Contract == nil || res.Contract.Status != protocol.ContractFailed || res.Contract.Error == "" {
		t.Fatalf("contract = %+v, want failed recorded with its own message", res.Contract)
	}
}
