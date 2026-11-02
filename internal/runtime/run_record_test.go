package runtime_test

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/russ-p/paseka/internal/adapters"
	"github.com/russ-p/paseka/internal/protocol"
	"github.com/russ-p/paseka/internal/runs"
	"github.com/russ-p/paseka/internal/runtime"
)

const contractBeeYAML = `role: builder
adapter: cursor
prompt_template: builder.md
run_summary: disabled
params:
  model: composer-2.5
  yolo: true
completion_contract:
  required:
    - type: VERIFICATION
      kind_one_of:
        - verification.success
        - verification.failed
      count: 1
`

const plainBeeYAML = `role: builder
adapter: cursor
prompt_template: builder.md
run_summary: disabled
params:
  model: composer-2.5
  yolo: true
`

var (
	contractRunStart = time.Date(2026, 2, 1, 10, 0, 0, 0, time.UTC)
	contractRunEnd   = time.Date(2026, 2, 1, 10, 5, 0, 0, time.UTC)
)

// recordWritingAdapter behaves like a real adapter: it appends the events it
// "emitted" and persists both outcome records before returning. Finalization is
// then correcting records that already exist, not fabricating them.
type recordWritingAdapter struct {
	status protocol.RunStatus
	events []protocol.Event
	// unreadableEvents replaces events.ndjson with a directory after the
	// adapter returns, which makes the run's event record unreadable.
	unreadableEvents bool
}

func (a *recordWritingAdapter) Name() string { return "cursor" }

func (a *recordWritingAdapter) Run(_ context.Context, req adapters.RunRequest) (*adapters.RunResult, error) {
	dir := runs.Dir{ColonyRoot: req.ColonyRoot, TraceID: req.TraceID, AgentID: req.AgentID}

	events := make([]protocol.Event, 0, len(a.events))
	for _, ev := range a.events {
		ev.TraceID = req.TraceID
		ev.AgentID = req.AgentID
		if err := dir.AppendEvent(ev); err != nil {
			return nil, err
		}
		events = append(events, ev)
	}

	result := protocol.Result{
		ProtocolVersion: protocol.Version,
		TraceID:         req.TraceID,
		AgentID:         req.AgentID,
		Status:          a.status,
		Summary:         "adapter summary",
		Artifacts:       []protocol.ArtifactRef{{Kind: "diff", Path: ".paseka/runs/x/diff.patch"}},
		Diagnostics: protocol.Diagnostics{
			ExitCode: 0,
			Stderr:   "adapter stderr kept",
		},
		Usage: &protocol.Usage{
			InputTokens:  123,
			OutputTokens: 45,
			Source:       protocol.UsageSourceCursorStreamJSON,
		},
		ProviderSessionID: "provider-session-42",
		FinishedAt:        contractRunEnd,
		Provenance: protocol.Provenance{
			AttemptID:          "attempt-1",
			ConfigRef:          "config-ref",
			ConfigRevision:     7,
			InitialCodeBaseSHA: "abc123",
		},
	}
	if err := dir.WriteResult(result); err != nil {
		return nil, err
	}
	if err := dir.WriteStatusSnapshot(protocol.StatusSnapshot{
		ProtocolVersion: protocol.Version,
		State:           a.status,
		ExitCode:        0,
		StartedAt:       contractRunStart,
		FinishedAt:      contractRunEnd,
		Provenance:      result.Provenance,
	}); err != nil {
		return nil, err
	}
	if a.unreadableEvents {
		if err := os.Remove(dir.EventsPath()); err != nil && !os.IsNotExist(err) {
			return nil, err
		}
		if err := os.Mkdir(dir.EventsPath(), 0o755); err != nil {
			return nil, err
		}
	}

	return &adapters.RunResult{
		Status:            string(a.status),
		Summary:           result.Summary,
		Output:            result.Summary,
		Artifacts:         []adapters.Artifact{{Kind: "diff", Path: ".paseka/runs/x/diff.patch", Content: "diff"}},
		Usage:             result.Usage,
		ProviderSessionID: result.ProviderSessionID,
		Events:            events,
	}, nil
}

func dispatchContractRun(t *testing.T, traceID, beeYAML string, adapter *recordWritingAdapter) (runs.Dir, *adapters.RunResult) {
	t.Helper()
	root := t.TempDir()
	writeColony(t, root)
	if err := os.WriteFile(filepath.Join(root, ".paseka/bees/builder.yaml"), []byte(beeYAML), 0o644); err != nil {
		t.Fatal(err)
	}

	d := runtime.NewDispatcher()
	d.RegisterAdapter("cursor", adapter)
	reg, err := runtime.BuildBeeRegistry(root)
	if err != nil {
		t.Fatal(err)
	}
	d.SetBeeRegistry(reg)

	result, err := d.Dispatch(context.Background(), runtime.DispatchRequest{
		ColonyRoot: root,
		Bee:        "builder",
		TraceID:    traceID,
		TaskID:     "task-1",
		Task:       "do the work",
	})
	if err != nil {
		t.Fatal(err)
	}
	return onlyRunDir(t, root, traceID), result
}

func TestFinalizationContractMissFailsBothRecords(t *testing.T) {
	dir, result := dispatchContractRun(t, "trace-contract-miss", contractBeeYAML, &recordWritingAdapter{
		status: protocol.StatusCompleted,
	})

	if result.Status != string(protocol.StatusFailed) || result.Reason != protocol.ReasonCompletionContract {
		t.Fatalf("result = %s/%s, want failed/%s", result.Status, result.Reason, protocol.ReasonCompletionContract)
	}

	snap, err := dir.ReadStatus()
	if err != nil {
		t.Fatal(err)
	}
	if snap.State != protocol.StatusFailed || snap.Reason != protocol.ReasonCompletionContract {
		t.Fatalf("status record = %s/%s, want failed/%s", snap.State, snap.Reason, protocol.ReasonCompletionContract)
	}
	if !strings.Contains(snap.Error, "completion contract") {
		t.Fatalf("status error = %q, want contract message", snap.Error)
	}
	if snap.ExitCode != 0 || !snap.StartedAt.Equal(contractRunStart) || !snap.FinishedAt.Equal(contractRunEnd) {
		t.Fatalf("status record lost adapter evidence: %+v", snap)
	}
	if snap.AttemptID != "attempt-1" || snap.ConfigRevision != 7 {
		t.Fatalf("status provenance = %+v", snap.Provenance)
	}

	res, err := dir.ReadResultJSON()
	if err != nil {
		t.Fatal(err)
	}
	if res.Status != protocol.StatusFailed || res.Reason != protocol.ReasonCompletionContract {
		t.Fatalf("result record = %s/%s, want failed/%s", res.Status, res.Reason, protocol.ReasonCompletionContract)
	}
	if !strings.Contains(res.Diagnostics.Error, "completion contract") {
		t.Fatalf("result diagnostics.error = %q, want contract message", res.Diagnostics.Error)
	}
	if res.Diagnostics.ExitCode != 0 || res.Diagnostics.Stderr != "adapter stderr kept" {
		t.Fatalf("result diagnostics lost adapter evidence: %+v", res.Diagnostics)
	}
	if res.Usage == nil || res.Usage.InputTokens != 123 || res.Usage.OutputTokens != 45 {
		t.Fatalf("result usage = %+v, want preserved", res.Usage)
	}
	if res.ProviderSessionID != "provider-session-42" {
		t.Fatalf("provider session = %q, want preserved", res.ProviderSessionID)
	}
	if len(res.Artifacts) != 1 || res.Artifacts[0].Kind != "diff" {
		t.Fatalf("artifacts = %+v, want preserved", res.Artifacts)
	}
	if !res.FinishedAt.Equal(contractRunEnd) {
		t.Fatalf("finishedAt = %s, want preserved %s", res.FinishedAt, contractRunEnd)
	}
	if res.InitialCodeBaseSHA != "abc123" {
		t.Fatalf("result provenance = %+v", res.Provenance)
	}
	if res.Contract == nil || res.Contract.Status != protocol.ContractFailed {
		t.Fatalf("contract record = %+v, want failed", res.Contract)
	}
	if len(res.Contract.Rules) != 1 || res.Contract.Rules[0].Type != "VERIFICATION" || res.Contract.Rules[0].Count != 1 {
		t.Fatalf("contract rules = %+v, want effective declared rule", res.Contract.Rules)
	}
	if len(res.Contract.Observations) != 1 {
		t.Fatalf("observations = %+v, want one", res.Contract.Observations)
	}
	obs := res.Contract.Observations[0]
	if obs.Want != 1 || obs.Got != 0 || obs.Satisfied {
		t.Fatalf("observation = %+v, want want=1 got=0 unsatisfied", obs)
	}

	summary, err := runs.LoadTraceSummary(dir.ColonyRoot, "trace-contract-miss")
	if err != nil {
		t.Fatal(err)
	}
	if !summary.HasFailures {
		t.Fatalf("trail scan = %+v, want failure", summary)
	}
}

func TestFinalizationContractPassStaysCompleted(t *testing.T) {
	success, err := protocol.NewEvent("", "", 1, protocol.EventVerification, protocol.VerificationPayload{
		Kind: protocol.VerificationSuccess, Summary: "ok",
	})
	if err != nil {
		t.Fatal(err)
	}
	dir, result := dispatchContractRun(t, "trace-contract-pass", contractBeeYAML, &recordWritingAdapter{
		status: protocol.StatusCompleted,
		events: []protocol.Event{success},
	})

	if result.Status != string(protocol.StatusCompleted) || result.Reason != "" {
		t.Fatalf("result = %s/%s, want completed with no reason", result.Status, result.Reason)
	}
	res, err := dir.ReadResultJSON()
	if err != nil {
		t.Fatal(err)
	}
	if res.Status != protocol.StatusCompleted || res.Reason != "" {
		t.Fatalf("result record = %s/%s, want completed", res.Status, res.Reason)
	}
	if res.Contract == nil || res.Contract.Status != protocol.ContractPassed {
		t.Fatalf("contract record = %+v, want passed", res.Contract)
	}
	if len(res.Contract.Observations) != 1 || !res.Contract.Observations[0].Satisfied || res.Contract.Observations[0].Got != 1 {
		t.Fatalf("observations = %+v, want satisfied got=1", res.Contract.Observations)
	}
	snap, err := dir.ReadStatus()
	if err != nil {
		t.Fatal(err)
	}
	if snap.State != protocol.StatusCompleted {
		t.Fatalf("status record = %s, want completed", snap.State)
	}
}

func TestFinalizationWithoutContractOmitsField(t *testing.T) {
	dir, result := dispatchContractRun(t, "trace-no-contract", plainBeeYAML, &recordWritingAdapter{
		status: protocol.StatusCompleted,
	})
	if result.Status != string(protocol.StatusCompleted) {
		t.Fatalf("result = %s, want completed", result.Status)
	}
	if result.Contract != nil {
		t.Fatalf("result contract = %+v, want nil", result.Contract)
	}
	raw, err := os.ReadFile(dir.ResultJSONPath())
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(raw), `"contract"`) {
		t.Fatalf("result.json carries a contract field without a declared contract: %s", raw)
	}

	res, err := dir.ReadResultJSON()
	if err != nil {
		t.Fatal(err)
	}
	if res.Contract != nil {
		t.Fatalf("decoded contract = %+v, want nil", res.Contract)
	}
}

func TestFinalizationUnreadableEventsRecordsUnavailable(t *testing.T) {
	dir, result := dispatchContractRun(t, "trace-contract-unreadable", contractBeeYAML, &recordWritingAdapter{
		status:           protocol.StatusCompleted,
		unreadableEvents: true,
	})
	if result.Status != string(protocol.StatusFailed) || result.Reason != protocol.ReasonCompletionContract {
		t.Fatalf("result = %s/%s, want failed/%s", result.Status, result.Reason, protocol.ReasonCompletionContract)
	}
	res, err := dir.ReadResultJSON()
	if err != nil {
		t.Fatal(err)
	}
	if res.Contract == nil || res.Contract.Status != protocol.ContractUnavailable {
		t.Fatalf("contract record = %+v, want unavailable", res.Contract)
	}
	if res.Contract.Error == "" {
		t.Fatal("unavailable contract carries no evaluation error")
	}
	if res.Contract.Status == protocol.ContractPassed {
		t.Fatal("unreadable event record was presented as a pass")
	}
}

func TestFinalizationGateRejectionKeepsChecksFailedWithFailedContract(t *testing.T) {
	gateBee := contractBeeYAML + `checks:
  command: ["sh", "-c", "exit 1"]
`
	dir, result := dispatchContractRun(t, "trace-gate-contract", gateBee, &recordWritingAdapter{
		status: protocol.StatusCompleted,
	})
	if result.Reason != protocol.ReasonChecksFailed {
		t.Fatalf("result reason = %q, want %q", result.Reason, protocol.ReasonChecksFailed)
	}
	res, err := dir.ReadResultJSON()
	if err != nil {
		t.Fatal(err)
	}
	if res.Reason != protocol.ReasonChecksFailed {
		t.Fatalf("result record reason = %q, want %q", res.Reason, protocol.ReasonChecksFailed)
	}
	if res.Contract == nil || res.Contract.Status != protocol.ContractFailed {
		t.Fatalf("contract record = %+v, want failed observation recorded", res.Contract)
	}
	snap, err := dir.ReadStatus()
	if err != nil {
		t.Fatal(err)
	}
	if snap.Reason != protocol.ReasonChecksFailed {
		t.Fatalf("status record reason = %q, want %q", snap.Reason, protocol.ReasonChecksFailed)
	}
}

func TestFinalizationContractFailurePreservesPassingGateEvidence(t *testing.T) {
	gateBee := contractBeeYAML + `checks:
  command: ["sh", "-c", "exit 0"]
`
	dir, result := dispatchContractRun(t, "trace-pass-gate-fail-contract", gateBee, &recordWritingAdapter{
		status: protocol.StatusCompleted,
	})
	if result.Status != string(protocol.StatusFailed) || result.Reason != protocol.ReasonCompletionContract {
		t.Fatalf("result = %s/%s, want failed/%s", result.Status, result.Reason, protocol.ReasonCompletionContract)
	}
	res, err := dir.ReadResultJSON()
	if err != nil {
		t.Fatal(err)
	}
	if res.Gate == nil || !res.Gate.Passed {
		t.Fatalf("result gate = %+v, want preserved pass", res.Gate)
	}
	if res.Contract == nil || res.Contract.Status != protocol.ContractFailed {
		t.Fatalf("contract record = %+v, want failed", res.Contract)
	}
	snap, err := dir.ReadStatus()
	if err != nil {
		t.Fatal(err)
	}
	if snap.Gate == nil || !snap.Gate.Passed {
		t.Fatalf("status gate = %+v, want preserved pass", snap.Gate)
	}
	if snap.Reason != protocol.ReasonCompletionContract {
		t.Fatalf("status reason = %q, want %q", snap.Reason, protocol.ReasonCompletionContract)
	}
}

func TestFinalizationReconciliationIsIdempotent(t *testing.T) {
	dir, _ := dispatchContractRun(t, "trace-contract-idempotent", contractBeeYAML, &recordWritingAdapter{
		status: protocol.StatusCompleted,
	})

	res, err := dir.ReadResultJSON()
	if err != nil {
		t.Fatal(err)
	}
	snap, err := dir.ReadStatus()
	if err != nil {
		t.Fatal(err)
	}
	statusBefore, err := os.ReadFile(dir.StatusPath())
	if err != nil {
		t.Fatal(err)
	}
	resultBefore, err := os.ReadFile(dir.ResultJSONPath())
	if err != nil {
		t.Fatal(err)
	}

	update := runs.RunRecordReconciliation{
		State:    protocol.StatusFailed,
		Reason:   protocol.ReasonCompletionContract,
		Note:     snap.Error,
		Contract: res.Contract,
	}
	if err := dir.ReconcileRunRecords(update); err != nil {
		t.Fatal(err)
	}
	if err := dir.ReconcileRunRecords(update); err != nil {
		t.Fatal(err)
	}

	statusAfter, err := os.ReadFile(dir.StatusPath())
	if err != nil {
		t.Fatal(err)
	}
	resultAfter, err := os.ReadFile(dir.ResultJSONPath())
	if err != nil {
		t.Fatal(err)
	}
	if string(statusBefore) != string(statusAfter) {
		t.Fatalf("repeated reconciliation changed status.json:\nbefore:\n%s\nafter:\n%s", statusBefore, statusAfter)
	}
	if string(resultBefore) != string(resultAfter) {
		t.Fatalf("repeated reconciliation changed result.json:\nbefore:\n%s\nafter:\n%s", resultBefore, resultAfter)
	}
}

func TestFinalizationSynthesizesMissingResultRecord(t *testing.T) {
	root := t.TempDir()
	writeColony(t, root)
	if err := os.WriteFile(filepath.Join(root, ".paseka/bees/builder.yaml"), []byte(contractBeeYAML), 0o644); err != nil {
		t.Fatal(err)
	}
	d := runtime.NewDispatcher()
	// A fake adapter that persists no records at all: finalization still owns
	// the outcome and must find a file to correct.
	d.RegisterAdapter("cursor", &mutatingAdapter{})
	reg, err := runtime.BuildBeeRegistry(root)
	if err != nil {
		t.Fatal(err)
	}
	d.SetBeeRegistry(reg)

	result, err := d.Dispatch(context.Background(), runtime.DispatchRequest{
		ColonyRoot: root,
		Bee:        "builder",
		TraceID:    "trace-synthesized-result",
		TaskID:     "task-1",
		Task:       "do the work",
	})
	if err != nil {
		t.Fatal(err)
	}
	if result.Status != string(protocol.StatusFailed) {
		t.Fatalf("result = %s, want failed", result.Status)
	}
	dir := onlyRunDir(t, root, "trace-synthesized-result")
	res, err := dir.ReadResultJSON()
	if err != nil {
		t.Fatal(err)
	}
	if res.Status != protocol.StatusFailed || res.Reason != protocol.ReasonCompletionContract {
		t.Fatalf("result record = %s/%s, want failed/%s", res.Status, res.Reason, protocol.ReasonCompletionContract)
	}
	if res.Contract == nil || res.Contract.Status != protocol.ContractFailed {
		t.Fatalf("contract record = %+v, want failed", res.Contract)
	}
}
