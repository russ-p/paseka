package taskledger_test

import (
	"testing"
	"time"

	"github.com/russ-p/paseka/internal/protocol"
	"github.com/russ-p/paseka/internal/taskledger"
)

// The cue ingress observation is a platform record: replaying it must not
// create a task, move honey, or make work ready.
func TestCueIngressObservationLeavesLedgerUntouched(t *testing.T) {
	trace := taskledger.TraceSnapshot{
		TraceID:         "trace-1",
		EnergyBudget:    5,
		EnergyRemaining: 3,
		Tasks: map[string]taskledger.TaskSnapshot{
			"task-1": {TaskID: "task-1", Title: "Existing", Status: protocol.TaskStatusPlanned},
		},
	}
	ev, err := protocol.NewEvent("trace-1", "cli", 0, protocol.EventSignal, protocol.CueIngressPayload{
		Kind:         protocol.SignalCueIngress,
		InvocationID: "cueinv-1",
		CueID:        "feature",
		Mode:         protocol.CueIngressModeSignal,
		InvokedAt:    time.Now().UTC(),
	})
	if err != nil {
		t.Fatal(err)
	}
	res, err := taskledger.ApplyEvent(trace, ev)
	if err != nil {
		t.Fatal(err)
	}
	if res.Changed {
		t.Fatal("cue.ingress changed ledger state")
	}
	if len(res.Ready) != 0 {
		t.Fatalf("ready = %+v, want none", res.Ready)
	}
	if len(res.Trace.Tasks) != 1 {
		t.Fatalf("tasks = %+v, want only the existing task", res.Trace.Tasks)
	}
	if res.Trace.EnergyBudget != 5 || res.Trace.EnergyRemaining != 3 {
		t.Fatalf("honey changed: %+v", res.Trace)
	}
}

// A task.plan written before invocation provenance existed still decodes; the
// absent identity stays unknown rather than being invented.
func TestLegacyTaskPlanWithoutCueInvocationDecodes(t *testing.T) {
	ev := protocol.Event{
		ID:        "evt-legacy",
		TraceID:   "trace-legacy",
		AgentID:   "cli",
		Type:      protocol.EventInsight,
		CreatedAt: time.Now().UTC(),
		Payload:   []byte(`{"kind":"task.plan","tasks":[{"taskId":"task-1","title":"Legacy","bee":"builder","cueId":"old-cue"}]}`),
	}
	res, err := taskledger.ApplyEvent(taskledger.TraceSnapshot{TraceID: "trace-legacy"}, ev)
	if err != nil {
		t.Fatal(err)
	}
	task := res.Trace.Tasks["task-1"]
	if task.CueID != "old-cue" {
		t.Fatalf("cue id = %q", task.CueID)
	}
	if task.CueInvocationID != "" {
		t.Fatalf("invented invocation = %q", task.CueInvocationID)
	}
}
