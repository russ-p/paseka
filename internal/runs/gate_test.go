package runs_test

import (
	"testing"
	"time"

	"github.com/russ-p/paseka/internal/protocol"
	"github.com/russ-p/paseka/internal/runs"
)

func TestDirWriteGateRecordUpdatesStatusAndResult(t *testing.T) {
	root := t.TempDir()
	traceID := "trace-gate"
	agentID := "agent-1"
	d := runs.Dir{ColonyRoot: root, TraceID: traceID, AgentID: agentID}
	if err := d.Prepare(); err != nil {
		t.Fatal(err)
	}
	if err := d.WriteResult(protocol.Result{
		TraceID: traceID,
		AgentID: agentID,
		Status:  protocol.StatusCompleted,
		Summary: "done",
	}); err != nil {
		t.Fatal(err)
	}

	gate := protocol.GateResult{
		Command:    []string{"go", "test", "./..."},
		ExitCode:   1,
		DurationMs: 42,
		LogPath:    ".paseka/runs/trace-gate/agent-1/checks.log",
		Reason:     "exit",
		Summary:    "checks failed (exit 1): go test ./...",
	}
	if err := d.WriteGateRecord(gate, protocol.StatusFailed, protocol.ReasonChecksFailed, "boom"); err != nil {
		t.Fatal(err)
	}

	snap, err := d.ReadStatus()
	if err != nil {
		t.Fatal(err)
	}
	if snap.State != protocol.StatusFailed || snap.Reason != protocol.ReasonChecksFailed {
		t.Fatalf("status = %s/%s", snap.State, snap.Reason)
	}
	if snap.Gate == nil || snap.Gate.ExitCode != 1 || snap.Gate.LogPath != gate.LogPath {
		t.Fatalf("status gate = %+v", snap.Gate)
	}
	if snap.Error != "boom" {
		t.Fatalf("status error = %q", snap.Error)
	}

	res, err := d.ReadResultJSON()
	if err != nil {
		t.Fatal(err)
	}
	if res.Status != protocol.StatusFailed || res.Reason != protocol.ReasonChecksFailed {
		t.Fatalf("result = %s/%s", res.Status, res.Reason)
	}
	if res.Gate == nil || res.Gate.Summary != gate.Summary {
		t.Fatalf("result gate = %+v", res.Gate)
	}
}

func TestRecordedGateRunsOldestFirstAndSkipsUngated(t *testing.T) {
	root := t.TempDir()
	traceID := "trace-gates"

	writeRun := func(agentID, taskID, attemptID string, gate *protocol.GateResult) {
		t.Helper()
		d := runs.Dir{ColonyRoot: root, TraceID: traceID, AgentID: agentID}
		if err := d.Prepare(); err != nil {
			t.Fatal(err)
		}
		req := protocol.Request{
			TraceID:   traceID,
			AgentID:   agentID,
			TaskID:    taskID,
			AttemptID: attemptID,
			CreatedAt: timeFor(agentID),
		}
		if err := d.WriteRequest(req); err != nil {
			t.Fatal(err)
		}
		snap := protocol.StatusSnapshot{
			State:     protocol.StatusCompleted,
			StartedAt: timeFor(agentID),
			Gate:      gate,
		}
		if err := d.WriteStatusSnapshot(snap); err != nil {
			t.Fatal(err)
		}
	}

	failed := protocol.GateResult{ExitCode: 1}
	passed := protocol.GateResult{Passed: true}
	writeRun("agent-1", "task-1", "attempt-1", &failed)
	writeRun("agent-2", "task-1", "attempt-1", nil)
	writeRun("agent-3", "task-1", "attempt-1", &passed)

	got, err := runs.RecordedGateRuns(root, traceID)
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 2 {
		t.Fatalf("recorded = %d, want 2 (ungated skipped)", len(got))
	}
	if got[0].Gate.ExitCode != 1 || !got[1].Gate.Passed {
		t.Fatalf("order = %+v", got)
	}
	if got[0].TaskID != "task-1" || got[0].AttemptID != "attempt-1" {
		t.Fatalf("context = %+v", got[0])
	}
}

// timeFor gives each run a distinct deterministic start time so ordering does
// not depend on filesystem collision.
func timeFor(agentID string) time.Time {
	base := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	switch agentID {
	case "agent-1":
		return base
	case "agent-2":
		return base.Add(time.Second)
	default:
		return base.Add(2 * time.Second)
	}
}
