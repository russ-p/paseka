package runs_test

import (
	"strings"
	"testing"
	"time"

	"github.com/russ-p/paseka/internal/protocol"
	"github.com/russ-p/paseka/internal/runs"
)

// projectionRun writes the request every run needs and leaves the two outcome
// records to the test, so a fixture can describe exactly the pair on disk.
func projectionRun(t *testing.T, root, traceID, agentID string, started time.Time) runs.Dir {
	t.Helper()
	d := runs.Dir{ColonyRoot: root, TraceID: traceID, AgentID: agentID}
	if err := d.Prepare(); err != nil {
		t.Fatal(err)
	}
	if err := d.WriteRequest(protocol.Request{
		ProtocolVersion: protocol.Version,
		TraceID:         traceID,
		AgentID:         agentID,
		Bee:             "builder",
		Adapter:         "cursor",
		Workspace:       root,
		ColonyRoot:      root,
		CreatedAt:       started,
	}); err != nil {
		t.Fatal(err)
	}
	return d
}

func writeProjectionStatus(t *testing.T, d runs.Dir, state protocol.RunStatus, reason string, started time.Time) {
	t.Helper()
	if err := d.WriteStatusSnapshot(protocol.StatusSnapshot{
		ProtocolVersion: protocol.Version,
		State:           state,
		Reason:          reason,
		StartedAt:       started,
		FinishedAt:      started.Add(time.Minute),
	}); err != nil {
		t.Fatal(err)
	}
}

func writeProjectionResult(t *testing.T, d runs.Dir, result protocol.Result) {
	t.Helper()
	if err := d.WriteResult(result); err != nil {
		t.Fatal(err)
	}
}

func TestLoadRunMetaAgreeingRecordsExposeContractEvidence(t *testing.T) {
	root := t.TempDir()
	started := time.Now().UTC()
	d := projectionRun(t, root, "trace-agreeing", "agent-1", started)

	writeProjectionStatus(t, d, protocol.StatusFailed, protocol.ReasonCompletionContract, started)
	writeProjectionResult(t, d, protocol.Result{
		ProtocolVersion: protocol.Version,
		TraceID:         d.TraceID,
		AgentID:         d.AgentID,
		Status:          protocol.StatusFailed,
		Reason:          protocol.ReasonCompletionContract,
		FinishedAt:      started.Add(time.Minute),
		Contract: &protocol.ContractRecord{
			Status: protocol.ContractFailed,
			Rules: []protocol.ContractRule{{
				Type:      "VERIFICATION",
				KindOneOf: []string{string(protocol.VerificationSuccess)},
				Count:     1,
			}},
			Observations: []protocol.ContractObservation{{
				Type:      "VERIFICATION",
				KindOneOf: []string{string(protocol.VerificationSuccess)},
				Want:      1,
				Got:       0,
				Satisfied: false,
			}},
			Error: "runtime: no verification.success event",
		},
	})

	meta, err := runs.LoadRunMeta(d)
	if err != nil {
		t.Fatal(err)
	}
	if meta.State != string(protocol.StatusFailed) {
		t.Fatalf("state = %q, want failed", meta.State)
	}
	if meta.Records != nil {
		t.Fatalf("records = %+v, want no integrity marker for an agreeing pair", meta.Records)
	}
	if meta.Contract.Status != protocol.ContractFailed {
		t.Fatalf("contract status = %q, want failed", meta.Contract.Status)
	}
	if len(meta.Contract.Rules) != 1 || meta.Contract.Rules[0].Count != 1 {
		t.Fatalf("contract rules = %+v, want the effective declared rule", meta.Contract.Rules)
	}
	if len(meta.Contract.Observations) != 1 || meta.Contract.Observations[0].Satisfied {
		t.Fatalf("contract observations = %+v, want an unsatisfied observation", meta.Contract.Observations)
	}
	if !strings.Contains(meta.Contract.Error, "verification") {
		t.Fatalf("contract error = %q, want the evaluation message", meta.Contract.Error)
	}
}

func TestLoadRunMetaReportsConflictingOutcomeRecords(t *testing.T) {
	started := time.Now().UTC()

	t.Run("a stale completed result cannot mask a recorded failure", func(t *testing.T) {
		root := t.TempDir()
		d := projectionRun(t, root, "trace-conflict", "agent-1", started)
		writeProjectionStatus(t, d, protocol.StatusFailed, protocol.ReasonCompletionContract, started)
		writeProjectionResult(t, d, protocol.Result{
			ProtocolVersion: protocol.Version,
			TraceID:         d.TraceID,
			AgentID:         d.AgentID,
			Status:          protocol.StatusCompleted,
			FinishedAt:      started.Add(time.Minute),
		})

		meta, err := runs.LoadRunMeta(d)
		if err != nil {
			t.Fatal(err)
		}
		if meta.State == string(protocol.StatusCompleted) {
			t.Fatalf("state = %q, want anything but completed", meta.State)
		}
		if meta.Records == nil || meta.Records.Status != runs.RecordConflict {
			t.Fatalf("records = %+v, want an explicit conflict marker", meta.Records)
		}
		if !strings.Contains(meta.Records.Detail, "failed") || !strings.Contains(meta.Records.Detail, "completed") {
			t.Fatalf("detail = %q, want both recorded states", meta.Records.Detail)
		}

		summary, err := runs.LoadTraceSummary(root, d.TraceID)
		if err != nil {
			t.Fatal(err)
		}
		if !summary.HasFailures {
			t.Fatalf("trail summary = %+v, want a conflicting run counted as a failure", summary)
		}
	})

	t.Run("a completed status cannot mask a recorded result failure", func(t *testing.T) {
		root := t.TempDir()
		d := projectionRun(t, root, "trace-conflict-reverse", "agent-1", started)
		writeProjectionStatus(t, d, protocol.StatusCompleted, "", started)
		writeProjectionResult(t, d, protocol.Result{
			ProtocolVersion: protocol.Version,
			TraceID:         d.TraceID,
			AgentID:         d.AgentID,
			Status:          protocol.StatusFailed,
			FinishedAt:      started.Add(time.Minute),
		})

		meta, err := runs.LoadRunMeta(d)
		if err != nil {
			t.Fatal(err)
		}
		if meta.State != string(protocol.StatusFailed) {
			t.Fatalf("state = %q, want failed", meta.State)
		}
		if meta.Records == nil || meta.Records.Status != runs.RecordConflict {
			t.Fatalf("records = %+v, want an explicit conflict marker", meta.Records)
		}
	})
}

func TestLoadRunMetaReportsIncompleteTerminalRecord(t *testing.T) {
	started := time.Now().UTC()

	t.Run("terminal status without a result record", func(t *testing.T) {
		root := t.TempDir()
		d := projectionRun(t, root, "trace-incomplete-status", "agent-1", started)
		writeProjectionStatus(t, d, protocol.StatusCompleted, "", started)

		meta, err := runs.LoadRunMeta(d)
		if err != nil {
			t.Fatal(err)
		}
		if meta.Records == nil || meta.Records.Status != runs.RecordIncomplete {
			t.Fatalf("records = %+v, want an incomplete marker", meta.Records)
		}
		if meta.State == string(protocol.StatusCompleted) {
			t.Fatalf("state = %q, want the unproven completion downgraded", meta.State)
		}

		summary, err := runs.LoadTraceSummary(root, d.TraceID)
		if err != nil {
			t.Fatal(err)
		}
		if !summary.HasFailures {
			t.Fatalf("trail summary = %+v, want an incomplete run counted as a failure", summary)
		}
	})

	t.Run("terminal result without a status record", func(t *testing.T) {
		root := t.TempDir()
		d := projectionRun(t, root, "trace-incomplete-result", "agent-1", started)
		writeProjectionResult(t, d, protocol.Result{
			ProtocolVersion: protocol.Version,
			TraceID:         d.TraceID,
			AgentID:         d.AgentID,
			Status:          protocol.StatusCompleted,
			FinishedAt:      started.Add(time.Minute),
		})

		meta, err := runs.LoadRunMeta(d)
		if err != nil {
			t.Fatal(err)
		}
		if meta.Records == nil || meta.Records.Status != runs.RecordIncomplete {
			t.Fatalf("records = %+v, want an incomplete marker", meta.Records)
		}
		if meta.State == string(protocol.StatusCompleted) {
			t.Fatalf("state = %q, want the unproven completion downgraded", meta.State)
		}
	})
}

func TestLoadRunMetaDoesNotFlagInFlightRun(t *testing.T) {
	root := t.TempDir()
	started := time.Now().UTC()
	d := projectionRun(t, root, "trace-in-flight", "agent-1", started)
	writeProjectionStatus(t, d, protocol.StatusRunning, "", started)

	meta, err := runs.LoadRunMeta(d)
	if err != nil {
		t.Fatal(err)
	}
	if meta.Records != nil {
		t.Fatalf("records = %+v, want no marker for an in-flight run without a terminal result", meta.Records)
	}
	if meta.State != string(protocol.StatusRunning) {
		t.Fatalf("state = %q, want running", meta.State)
	}
	if meta.Contract.Status != runs.ContractUnknown {
		t.Fatalf("contract status = %q, want unknown", meta.Contract.Status)
	}

	summary, err := runs.LoadTraceSummary(root, d.TraceID)
	if err != nil {
		t.Fatal(err)
	}
	if summary.HasFailures {
		t.Fatalf("trail summary = %+v, want no failure for an in-flight run", summary)
	}
	if !summary.HasActive {
		t.Fatalf("trail summary = %+v, want the run counted as active", summary)
	}
}

func TestLoadRunMetaKeepsLegacyRecordShape(t *testing.T) {
	started := time.Now().UTC()

	t.Run("legacy text result without result.json", func(t *testing.T) {
		root := t.TempDir()
		d := projectionRun(t, root, "trace-legacy-text", "agent-1", started)
		writeProjectionStatus(t, d, protocol.StatusCompleted, "", started)
		if err := d.WriteResultText("legacy run summary"); err != nil {
			t.Fatal(err)
		}

		meta, err := runs.LoadRunMeta(d)
		if err != nil {
			t.Fatal(err)
		}
		if meta.Records != nil {
			t.Fatalf("records = %+v, want a legacy record read as it always was", meta.Records)
		}
		if meta.State != string(protocol.StatusCompleted) {
			t.Fatalf("state = %q, want the recorded completed", meta.State)
		}
		if meta.Summary != "legacy run summary" {
			t.Fatalf("summary = %q, want the legacy text", meta.Summary)
		}
		if meta.Contract.Status != runs.ContractUnknown {
			t.Fatalf("contract status = %q, want unknown", meta.Contract.Status)
		}
	})

	t.Run("request-only run directory", func(t *testing.T) {
		root := t.TempDir()
		d := projectionRun(t, root, "trace-legacy-request", "agent-1", started)

		meta, err := runs.LoadRunMeta(d)
		if err != nil {
			t.Fatal(err)
		}
		if meta.Records != nil {
			t.Fatalf("records = %+v, want no marker", meta.Records)
		}
		if meta.State != string(protocol.StatusQueued) {
			t.Fatalf("state = %q, want the existing queued shape", meta.State)
		}
		if meta.Contract.Status != runs.ContractUnknown {
			t.Fatalf("contract status = %q, want unknown", meta.Contract.Status)
		}
	})

	t.Run("result without a contract field", func(t *testing.T) {
		root := t.TempDir()
		d := projectionRun(t, root, "trace-legacy-contract", "agent-1", started)
		writeProjectionStatus(t, d, protocol.StatusCompleted, "", started)
		writeProjectionResult(t, d, protocol.Result{
			ProtocolVersion: protocol.Version,
			TraceID:         d.TraceID,
			AgentID:         d.AgentID,
			Status:          protocol.StatusCompleted,
			FinishedAt:      started.Add(time.Minute),
		})

		meta, err := runs.LoadRunMeta(d)
		if err != nil {
			t.Fatal(err)
		}
		if meta.Records != nil {
			t.Fatalf("records = %+v, want no marker for an agreeing pair", meta.Records)
		}
		if meta.Contract.Status != runs.ContractUnknown {
			t.Fatalf("contract status = %q, want unknown rather than passed", meta.Contract.Status)
		}
		if len(meta.Contract.Rules) != 0 || len(meta.Contract.Observations) != 0 {
			t.Fatalf("contract evidence = %+v, want no assumed rules", meta.Contract)
		}
	})
}

func TestLoadRunMetaDoesNotFlagInteractiveSessionWithoutResultRecord(t *testing.T) {
	root := t.TempDir()
	started := time.Now().UTC()
	d := projectionRun(t, root, "trace-session", "agent-1", started)
	writeProjectionStatus(t, d, protocol.StatusCompleted, "", started)
	if err := d.WriteSession(runs.SessionMeta{
		SessionID: "agent-1",
		TraceID:   d.TraceID,
		AgentID:   d.AgentID,
		State:     "completed",
		StartedAt: started,
	}); err != nil {
		t.Fatal(err)
	}

	meta, err := runs.LoadRunMeta(d)
	if err != nil {
		t.Fatal(err)
	}
	if meta.Records != nil {
		t.Fatalf("records = %+v, want an interactive session's expected absence read as no interruption", meta.Records)
	}
	if meta.State != string(protocol.StatusCompleted) {
		t.Fatalf("state = %q, want completed", meta.State)
	}
	if meta.Contract.Status != runs.ContractUnknown {
		t.Fatalf("contract status = %q, want unknown for an ungated session", meta.Contract.Status)
	}
}

func TestContractDowngradedRunStillScansAsFailed(t *testing.T) {
	root := t.TempDir()
	started := time.Now().UTC()
	d := projectionRun(t, root, "trace-contract-downgrade", "agent-1", started)
	writeProjectionStatus(t, d, protocol.StatusFailed, protocol.ReasonCompletionContract, started)
	writeProjectionResult(t, d, protocol.Result{
		ProtocolVersion: protocol.Version,
		TraceID:         d.TraceID,
		AgentID:         d.AgentID,
		Status:          protocol.StatusFailed,
		Reason:          protocol.ReasonCompletionContract,
		FinishedAt:      started.Add(time.Minute),
		Contract: &protocol.ContractRecord{
			Status: protocol.ContractUnavailable,
			Rules:  []protocol.ContractRule{{Type: "VERIFICATION", Count: 1}},
			Error:  "runtime: completion contract unavailable: unreadable event record",
		},
	})

	meta, err := runs.LoadRunMeta(d)
	if err != nil {
		t.Fatal(err)
	}
	if meta.State != string(protocol.StatusFailed) {
		t.Fatalf("state = %q, want failed", meta.State)
	}
	if meta.Contract.Status != protocol.ContractUnavailable {
		t.Fatalf("contract status = %q, want unavailable", meta.Contract.Status)
	}
	if !strings.Contains(meta.Contract.Error, "unavailable") {
		t.Fatalf("contract error = %q, want the evaluation error", meta.Contract.Error)
	}

	summary, err := runs.LoadTraceSummary(root, d.TraceID)
	if err != nil {
		t.Fatal(err)
	}
	if !summary.HasFailures {
		t.Fatalf("trail summary = %+v, want the contract-downgraded run counted as a failure", summary)
	}
}
