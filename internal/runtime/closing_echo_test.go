package runtime_test

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/russ-p/paseka/internal/colony"
	"github.com/russ-p/paseka/internal/protocol"
	"github.com/russ-p/paseka/internal/taskledger"
)

// A slow closing round blocks the JetStream consumer for minutes. With the
// default 30s AckWait the seed events are redelivered, so the reactor sees the
// same task.ready and task.status events again. Its own echo of the seed must
// stay suppressed for every copy: re-applying the status event moves a
// completed closing round back to running, and re-applying the ready event
// dispatches it a second time.
func TestClosingSeedEchoDoesNotRedispatchCompletedRound(t *testing.T) {
	const closingYAML = `defaults:
  prompt_template: default.md
closing:
  enabled: true
  when:
    min_work_tasks: 1
  task:
    bee: reviewer
    review: none
  max_rounds: 2
`
	r, pub, adapter := newClosingTestReactor(t, closingYAML, map[string]colony.Bee{
		"builder":  {Role: "builder"},
		"reviewer": {Role: "reviewer"},
	})
	traceID := "trace-closing-echo"

	plan := func(spec protocol.TaskSpec) {
		t.Helper()
		ev, err := protocol.NewEvent(traceID, "scout", 0, protocol.EventInsight, protocol.TaskPlanPayload{
			Kind:  protocol.TaskEventPlan,
			Tasks: []protocol.TaskSpec{spec},
		})
		if err != nil {
			t.Fatal(err)
		}
		if err := r.ProcessEvent(context.Background(), ev); err != nil {
			t.Fatal(err)
		}
	}
	complete := func(taskID string) {
		t.Helper()
		ev, err := protocol.NewEvent(traceID, "builder-agent", 0, protocol.EventVerification, protocol.TaskCompletedPayload{
			Kind:   protocol.TaskEventCompleted,
			TaskID: taskID,
			Status: protocol.TaskStatusCompleted,
		})
		if err != nil {
			t.Fatal(err)
		}
		if err := r.ProcessEvent(context.Background(), ev); err != nil {
			t.Fatal(err)
		}
	}

	plan(protocol.TaskSpec{TaskID: "task-1", Title: "Only slice", Bee: "builder"})
	complete("task-1")

	// The last work completion seeds closing-1 and dispatches it; the recording
	// adapter completes it, so the gate opens on a completed round.
	snap, err := r.Ledger().Snapshot(traceID)
	if err != nil {
		t.Fatal(err)
	}
	closing, ok := snap.Tasks["closing-1"]
	if !ok {
		t.Fatalf("closing-1 was not seeded: %+v", snap.Tasks)
	}
	if closing.Status != protocol.TaskStatusCompleted {
		t.Fatalf("closing-1 status = %q, want completed", closing.Status)
	}
	if adapter.calls != 1 {
		t.Fatalf("closing dispatched %d times, want 1", adapter.calls)
	}

	// Collect the events the seed itself published, then deliver each of the
	// redelivery-sensitive ones twice: that is what the bus does when the
	// handler outlives AckWait.
	var ready, running protocol.Event
	for _, ev := range pub.events {
		payload := ev.Payload
		switch {
		case ev.Type == protocol.EventSignal && protocol.PayloadKind(payload) == string(protocol.TaskEventReady) &&
			protocol.PayloadTaskID(payload) == "closing-1":
			ready = ev
		case ev.Type == protocol.EventSignal && protocol.PayloadKind(payload) == string(protocol.TaskEventStatus) &&
			protocol.PayloadTaskID(payload) == "closing-1" && taskStatusOf(t, payload) == protocol.TaskStatusRunning:
			running = ev
		}
	}
	if ready.ID == "" || running.ID == "" {
		t.Fatalf("seed events not found: ready=%q running=%q", ready.ID, running.ID)
	}

	for i := 0; i < 2; i++ {
		for _, ev := range []protocol.Event{ready, running} {
			if err := r.ProcessEvent(context.Background(), ev); err != nil {
				t.Fatalf("redelivery %d of %s: %v", i, ev.ID, err)
			}
		}
	}

	snap, err = r.Ledger().Snapshot(traceID)
	if err != nil {
		t.Fatal(err)
	}
	if got := snap.Tasks["closing-1"].Status; got != protocol.TaskStatusCompleted {
		t.Fatalf("closing-1 status after redelivery = %q, want completed", got)
	}
	if adapter.calls != 1 {
		t.Fatalf("closing dispatched %d times after redelivery, want 1", adapter.calls)
	}
}

func taskStatusOf(t *testing.T, payload json.RawMessage) protocol.TaskStatus {
	t.Helper()
	var p protocol.TaskStatusPayload
	if err := json.Unmarshal(payload, &p); err != nil {
		t.Fatal(err)
	}
	return p.Status
}

var _ = taskledger.TaskSnapshot{}
