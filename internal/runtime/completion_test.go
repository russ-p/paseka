package runtime

import (
	"os"
	"testing"
	"time"

	"github.com/russ-p/paseka/internal/adapters"
	"github.com/russ-p/paseka/internal/colony"
	"github.com/russ-p/paseka/internal/protocol"
	"github.com/russ-p/paseka/internal/runs"
)

func contractTestBee() colony.Bee {
	return colony.Bee{
		Role: "guard",
		CompletionContract: colony.CompletionContract{
			Required: []colony.CompletionRule{{
				Type:      "VERIFICATION",
				KindOneOf: []string{"verification.success", "verification.failed"},
				Count:     1,
			}},
		},
	}
}

func newContractTestRunDir(t *testing.T) runs.Dir {
	t.Helper()
	root := t.TempDir()
	dir := runs.Dir{ColonyRoot: root, TraceID: "trace-contract", AgentID: "agent-contract"}
	if err := os.MkdirAll(dir.Root(), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := dir.WriteResult(protocol.Result{
		ProtocolVersion: protocol.Version,
		TraceID:         dir.TraceID,
		AgentID:         dir.AgentID,
		Status:          protocol.StatusCompleted,
		Diagnostics:     protocol.Diagnostics{ExitCode: 0},
	}); err != nil {
		t.Fatal(err)
	}
	if err := dir.WriteStatus(protocol.StatusCompleted, "", 0, time.Time{}, time.Time{}, ""); err != nil {
		t.Fatal(err)
	}
	return dir
}

func TestEnforceCompletionContractMarksFailed(t *testing.T) {
	d := NewDispatcher()
	dir := newContractTestRunDir(t)
	result := &adapters.RunResult{Status: string(protocol.StatusCompleted)}
	if err := d.enforceCompletionContract(dir, contractTestBee(), nil, nil, result); err != nil {
		t.Fatal(err)
	}
	if result.Status != string(protocol.StatusFailed) {
		t.Fatalf("status = %q, want failed", result.Status)
	}
	if result.Reason != protocol.ReasonCompletionContract {
		t.Fatalf("reason = %q, want %q", result.Reason, protocol.ReasonCompletionContract)
	}
	if result.Contract == nil || result.Contract.Status != protocol.ContractFailed {
		t.Fatalf("contract = %+v, want failed", result.Contract)
	}

	snap, err := dir.ReadStatus()
	if err != nil {
		t.Fatal(err)
	}
	if snap.State != protocol.StatusFailed || snap.Reason != protocol.ReasonCompletionContract {
		t.Fatalf("status record = %s/%s, want failed/%s", snap.State, snap.Reason, protocol.ReasonCompletionContract)
	}
	res, err := dir.ReadResultJSON()
	if err != nil {
		t.Fatal(err)
	}
	if res.Status != protocol.StatusFailed || res.Reason != protocol.ReasonCompletionContract {
		t.Fatalf("result record = %s/%s, want failed/%s", res.Status, res.Reason, protocol.ReasonCompletionContract)
	}
	if res.Diagnostics.Error == "" {
		t.Fatal("result diagnostics.error is empty, want contract message")
	}
}

func TestEnforceCompletionContractPasses(t *testing.T) {
	d := NewDispatcher()
	dir := newContractTestRunDir(t)
	ev, err := protocol.NewEvent("t", "a", 1, protocol.EventVerification, protocol.VerificationPayload{
		Kind: protocol.VerificationSuccess, Summary: "ok",
	})
	if err != nil {
		t.Fatal(err)
	}
	result := &adapters.RunResult{Status: string(protocol.StatusCompleted)}
	if err := d.enforceCompletionContract(dir, contractTestBee(), []protocol.Event{ev}, nil, result); err != nil {
		t.Fatal(err)
	}
	if result.Status != string(protocol.StatusCompleted) {
		t.Fatalf("status = %q, want completed", result.Status)
	}
	if result.Contract == nil || result.Contract.Status != protocol.ContractPassed {
		t.Fatalf("contract = %+v, want passed", result.Contract)
	}
	res, err := dir.ReadResultJSON()
	if err != nil {
		t.Fatal(err)
	}
	if res.Contract == nil || res.Contract.Status != protocol.ContractPassed {
		t.Fatalf("recorded contract = %+v, want passed", res.Contract)
	}
	if res.Status != protocol.StatusCompleted {
		t.Fatalf("recorded status = %q, want completed", res.Status)
	}
}

func TestEnforceCompletionContractUnavailableFailsRun(t *testing.T) {
	d := NewDispatcher()
	dir := newContractTestRunDir(t)
	result := &adapters.RunResult{Status: string(protocol.StatusCompleted)}
	readErr := os.ErrInvalid
	if err := d.enforceCompletionContract(dir, contractTestBee(), nil, readErr, result); err != nil {
		t.Fatal(err)
	}
	if result.Status != string(protocol.StatusFailed) || result.Reason != protocol.ReasonCompletionContract {
		t.Fatalf("result = %s/%s, want failed/%s", result.Status, result.Reason, protocol.ReasonCompletionContract)
	}
	if result.Contract == nil || result.Contract.Status != protocol.ContractUnavailable || result.Contract.Error == "" {
		t.Fatalf("contract = %+v, want unavailable with error", result.Contract)
	}
}

func TestEnforceCompletionContractKeepsGateReason(t *testing.T) {
	d := NewDispatcher()
	dir := newContractTestRunDir(t)
	result := &adapters.RunResult{
		Status: string(protocol.StatusFailed),
		Reason: protocol.ReasonChecksFailed,
	}
	if err := d.enforceCompletionContract(dir, contractTestBee(), nil, nil, result); err != nil {
		t.Fatal(err)
	}
	if result.Reason != protocol.ReasonChecksFailed {
		t.Fatalf("reason = %q, want %q", result.Reason, protocol.ReasonChecksFailed)
	}
	if result.Contract == nil || result.Contract.Status != protocol.ContractFailed {
		t.Fatalf("contract = %+v, want failed observation recorded", result.Contract)
	}
	snap, err := dir.ReadStatus()
	if err != nil {
		t.Fatal(err)
	}
	if snap.Reason == protocol.ReasonCompletionContract {
		t.Fatal("contract relabelled a gate rejection reason")
	}
}

func TestEnforceCompletionContractNoContractRecordsNothing(t *testing.T) {
	d := NewDispatcher()
	dir := newContractTestRunDir(t)
	result := &adapters.RunResult{Status: string(protocol.StatusCompleted)}
	if err := d.enforceCompletionContract(dir, colony.Bee{Role: "guard"}, nil, nil, result); err != nil {
		t.Fatal(err)
	}
	if result.Contract != nil {
		t.Fatalf("contract = %+v, want nil", result.Contract)
	}
	res, err := dir.ReadResultJSON()
	if err != nil {
		t.Fatal(err)
	}
	if res.Contract != nil {
		t.Fatalf("recorded contract = %+v, want nil", res.Contract)
	}
}
