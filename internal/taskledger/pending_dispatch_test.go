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
