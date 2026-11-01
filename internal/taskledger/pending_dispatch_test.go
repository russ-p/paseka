package taskledger_test

import (
	"testing"
	"time"

	"github.com/russ-p/paseka/internal/taskledger"
)

func TestActivePendingDispatchesUseCreationOrder(t *testing.T) {
	trace := taskledger.TraceSnapshot{
		PendingDispatches: []taskledger.PendingDispatch{
			{ID: "later", Status: taskledger.PendingDispatchPending, CreatedAt: time.Unix(2, 0)},
			{ID: "consumed", Status: taskledger.PendingDispatchConsumed, CreatedAt: time.Unix(0, 0)},
			{ID: "first", Status: taskledger.PendingDispatchPending, CreatedAt: time.Unix(1, 0)},
			{ID: "first-tie", Status: taskledger.PendingDispatchPending, CreatedAt: time.Unix(1, 0)},
		},
	}
	got := taskledger.ActivePendingDispatches(trace)
	want := []string{"first", "first-tie", "later"}
	if len(got) != len(want) {
		t.Fatalf("len = %d, want %d", len(got), len(want))
	}
	for i := range want {
		if got[i].ID != want[i] {
			t.Fatalf("pending[%d] = %q, want %q", i, got[i].ID, want[i])
		}
	}
}

func TestDeferPendingDispatchDedupesActiveEntry(t *testing.T) {
	trace := taskledger.TraceSnapshot{}
	trace, added := taskledger.DeferPendingDispatch(trace, taskledger.PendingDispatch{ID: "same"})
	if !added {
		t.Fatal("expected first deferral to add")
	}
	trace, added = taskledger.DeferPendingDispatch(trace, taskledger.PendingDispatch{ID: "same"})
	if added {
		t.Fatal("expected active duplicate to be ignored")
	}
	trace, changed := taskledger.ResolvePendingDispatch(trace, "same", taskledger.PendingDispatchConsumed)
	if !changed {
		t.Fatal("expected resolve to change status")
	}
	_, added = taskledger.DeferPendingDispatch(trace, taskledger.PendingDispatch{ID: "same"})
	if !added {
		t.Fatal("expected a new deferral after the prior entry was consumed")
	}
}

func TestPendingDispatchStatusVocabulary(t *testing.T) {
	if got := string(taskledger.PendingDispatchUnresolved); got != "unresolved" {
		t.Fatalf("unresolved status = %q, want unresolved", got)
	}
	if got := string(taskledger.PendingDispatchAbandoned); got != "abandoned" {
		t.Fatalf("abandoned status = %q, want abandoned", got)
	}
	if !taskledger.PendingDispatchPending.Active() {
		t.Fatal("pending status must be active")
	}
	for _, status := range []taskledger.PendingDispatchStatus{
		taskledger.PendingDispatchConsumed,
		taskledger.PendingDispatchAbandoned,
		taskledger.PendingDispatchUnresolved,
	} {
		if status.Active() {
			t.Fatalf("%q must not be active", status)
		}
	}
}

func TestMarkPendingDispatchUnresolvedStaysVisible(t *testing.T) {
	trace := taskledger.TraceSnapshot{}
	trace, added := taskledger.DeferPendingDispatch(trace, taskledger.PendingDispatch{
		ID:            "direct-key",
		Kind:          taskledger.PendingDispatchDirect,
		SourceEventID: "event-missing",
	})
	if !added {
		t.Fatal("expected deferral to add")
	}

	trace, changed := taskledger.MarkPendingDispatchUnresolved(trace, "direct-key", "source event missing")
	if !changed {
		t.Fatal("expected unresolved transition to change the snapshot")
	}
	if got := trace.PendingDispatches[0].Status; got != taskledger.PendingDispatchUnresolved {
		t.Fatalf("status = %q, want unresolved", got)
	}
	if trace.PendingDispatches[0].Diagnostic != "source event missing" {
		t.Fatalf("diagnostic = %q", trace.PendingDispatches[0].Diagnostic)
	}
	if got := taskledger.ActivePendingDispatches(trace); len(got) != 0 {
		t.Fatalf("unresolved entry must not be active, got %d", len(got))
	}
	if len(trace.PendingDispatches) != 1 {
		t.Fatalf("unresolved entry must stay in the ledger, got %d entries", len(trace.PendingDispatches))
	}
}
