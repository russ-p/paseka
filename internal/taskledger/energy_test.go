package taskledger_test

import (
	"testing"

	"github.com/russ-p/paseka/internal/protocol"
	"github.com/russ-p/paseka/internal/taskledger"
)

func TestEnsureEnergySeeded(t *testing.T) {
	trace := taskledger.TraceSnapshot{TraceID: "trace-1"}
	updated, changed := taskledger.EnsureEnergySeeded(trace, 12)
	if !changed {
		t.Fatal("expected changed")
	}
	if updated.EnergyBudget != 12 || updated.EnergyRemaining != 12 {
		t.Fatalf("energy = %+v", updated)
	}

	again, changed := taskledger.EnsureEnergySeeded(updated, 20)
	if changed {
		t.Fatal("expected no change when already seeded")
	}
	if again.EnergyRemaining != 12 {
		t.Fatalf("remaining = %d", again.EnergyRemaining)
	}
}

func TestApplyEventEnergyAdd(t *testing.T) {
	trace := taskledger.TraceSnapshot{
		TraceID:         "trace-1",
		EnergyBudget:    12,
		EnergyRemaining: 2,
		Tasks:           map[string]taskledger.TaskSnapshot{},
	}
	ev, err := protocol.NewEvent("trace-1", "cli", 1, protocol.EventSignal, protocol.EnergyAddPayload{
		Kind:   protocol.SignalEnergyAdd,
		Amount: 5,
	})
	if err != nil {
		t.Fatal(err)
	}
	res, err := taskledger.ApplyEvent(trace, ev)
	if err != nil {
		t.Fatal(err)
	}
	if res.Trace.EnergyRemaining != 7 {
		t.Fatalf("remaining = %d", res.Trace.EnergyRemaining)
	}
	if res.Trace.EnergyBudget != 12 {
		t.Fatalf("budget changed = %d", res.Trace.EnergyBudget)
	}
	if res.Trace.EnergyAdded != 5 {
		t.Fatalf("added = %d, want 5", res.Trace.EnergyAdded)
	}
	if got := taskledger.Allocated(res.Trace.EnergyBudget, res.Trace.EnergyAdded); got != 17 {
		t.Fatalf("allocated = %d, want 17", got)
	}
}

func TestApplyEventEnergyAddDoesNotLockDefaultBudget(t *testing.T) {
	trace := taskledger.TraceSnapshot{
		TraceID: "trace-1",
		Tasks:   map[string]taskledger.TaskSnapshot{},
	}
	ev, err := protocol.NewEvent("trace-1", "cli", 1, protocol.EventSignal, protocol.EnergyAddPayload{
		Kind:   protocol.SignalEnergyAdd,
		Amount: 5,
	})
	if err != nil {
		t.Fatal(err)
	}
	res, err := taskledger.ApplyEvent(trace, ev)
	if err != nil {
		t.Fatal(err)
	}
	if res.Trace.EnergyRemaining != 5 {
		t.Fatalf("remaining = %d, want 5", res.Trace.EnergyRemaining)
	}
	if res.Trace.EnergyBudget != 0 {
		t.Fatalf("budget = %d, want 0 so SeedEnergy can apply colony defaults", res.Trace.EnergyBudget)
	}
	if res.Trace.EnergyAdded != 0 {
		t.Fatalf("added = %d, want 0 before seed", res.Trace.EnergyAdded)
	}

	seeded, changed := taskledger.EnsureEnergySeeded(res.Trace, 20)
	if !changed {
		t.Fatal("expected SeedEnergy to set custom budget")
	}
	if seeded.EnergyBudget != 20 {
		t.Fatalf("budget = %d, want 20", seeded.EnergyBudget)
	}
	if seeded.EnergyRemaining != 5 {
		t.Fatalf("remaining = %d, want 5 (prior add preserved)", seeded.EnergyRemaining)
	}
	if seeded.EnergyAdded != 0 {
		t.Fatalf("added = %d, want 0 after first seed", seeded.EnergyAdded)
	}
}

func TestApplyEventEnergyConsume(t *testing.T) {
	trace := taskledger.TraceSnapshot{
		TraceID:         "trace-1",
		EnergyBudget:    12,
		EnergyRemaining: 3,
		EnergyAdded:     4,
		Tasks:           map[string]taskledger.TaskSnapshot{},
	}
	ev, err := protocol.NewEvent("trace-1", "runtime", 1, protocol.EventSignal, protocol.EnergyConsumePayload{
		Kind:   protocol.SignalEnergyConsume,
		Amount: 1,
		Reason: "task.dispatch",
		TaskID: "task-1",
	})
	if err != nil {
		t.Fatal(err)
	}
	res, err := taskledger.ApplyEvent(trace, ev)
	if err != nil {
		t.Fatal(err)
	}
	if res.Trace.EnergyRemaining != 2 {
		t.Fatalf("remaining = %d", res.Trace.EnergyRemaining)
	}
	if res.Trace.EnergyAdded != 4 {
		t.Fatalf("added = %d, want unchanged 4", res.Trace.EnergyAdded)
	}
}

func TestApplyEventEnergyStipendSetsRemaining(t *testing.T) {
	trace := taskledger.TraceSnapshot{
		TraceID:         "trace-1",
		EnergyBudget:    10,
		EnergyRemaining: 1,
		EnergyAdded:     6,
		Tasks: map[string]taskledger.TaskSnapshot{
			"task-1": {
				TaskID:  "task-1",
				Status:  protocol.TaskStatusBlocked,
				Summary: protocol.HoneyReserveExhaustedSummary,
			},
		},
	}
	ev, err := protocol.NewEvent("trace-1", "cli", 1, protocol.EventSignal, protocol.EnergyStipendPayload{
		Kind:   protocol.SignalEnergyStipend,
		Amount: 4,
	})
	if err != nil {
		t.Fatal(err)
	}
	res, err := taskledger.ApplyEvent(trace, ev)
	if err != nil {
		t.Fatal(err)
	}
	if res.Trace.EnergyRemaining != 4 {
		t.Fatalf("remaining = %d, want 4", res.Trace.EnergyRemaining)
	}
	if res.Trace.EnergyBudget != 10 {
		t.Fatalf("budget = %d, want unchanged 10", res.Trace.EnergyBudget)
	}
	if res.Trace.EnergyAdded != 6 {
		t.Fatalf("added = %d, want unchanged 6", res.Trace.EnergyAdded)
	}
	if res.Trace.Tasks["task-1"].Status != protocol.TaskStatusBlocked {
		t.Fatalf("status = %q, want blocked", res.Trace.Tasks["task-1"].Status)
	}
}

func TestApplyEventEnergyStipendRequiresPositiveAmount(t *testing.T) {
	trace := taskledger.TraceSnapshot{
		TraceID:         "trace-1",
		EnergyBudget:    4,
		EnergyRemaining: 4,
		Tasks:           map[string]taskledger.TaskSnapshot{},
	}
	ev, err := protocol.NewEvent("trace-1", "cli", 1, protocol.EventSignal, protocol.EnergyStipendPayload{
		Kind:   protocol.SignalEnergyStipend,
		Amount: 0,
	})
	if err != nil {
		t.Fatal(err)
	}
	_, err = taskledger.ApplyEvent(trace, ev)
	if err == nil {
		t.Fatal("expected positive amount error")
	}
}

func TestApplyEventEnergyStipendAbandonsEnergyDeferredDispatches(t *testing.T) {
	trace := taskledger.TraceSnapshot{
		TraceID:         "trace-1",
		EnergyBudget:    10,
		EnergyRemaining: 0,
		Tasks: map[string]taskledger.TaskSnapshot{
			"task-1": {TaskID: "task-1", Status: protocol.TaskStatusWaitingReview},
		},
		PendingDispatches: []taskledger.PendingDispatch{
			{ID: "active", Kind: taskledger.PendingDispatchDirect, TaskID: "task-1", Status: taskledger.PendingDispatchPending},
			{ID: "done", Kind: taskledger.PendingDispatchDirect, TaskID: "task-1", Status: taskledger.PendingDispatchConsumed},
			{ID: "gone", Kind: taskledger.PendingDispatchDirect, TaskID: "task-1", Status: taskledger.PendingDispatchUnresolved, Diagnostic: "source event missing"},
			{ID: "held", Kind: taskledger.PendingDispatchDirect, TaskID: "task-1", Reason: protocol.ConfigBarrierHoldReason, Status: taskledger.PendingDispatchPending},
		},
	}
	ev, err := protocol.NewEvent("trace-1", "cli", 1, protocol.EventSignal, protocol.EnergyStipendPayload{
		Kind:   protocol.SignalEnergyStipend,
		Amount: 4,
	})
	if err != nil {
		t.Fatal(err)
	}
	res, err := taskledger.ApplyEvent(trace, ev)
	if err != nil {
		t.Fatal(err)
	}
	if !res.Changed {
		t.Fatal("stipend must report a change so the ledger persists the abandonment")
	}
	if res.Trace.EnergyRemaining != 4 {
		t.Fatalf("remaining = %d, want 4", res.Trace.EnergyRemaining)
	}
	if got := res.Trace.Tasks["task-1"].Status; got != protocol.TaskStatusWaitingReview {
		t.Fatalf("task status = %q, want waiting_review", got)
	}
	statuses := map[string]taskledger.PendingDispatchStatus{}
	for _, item := range res.Trace.PendingDispatches {
		statuses[item.ID] = item.Status
	}
	if got := statuses["active"]; got != taskledger.PendingDispatchAbandoned {
		t.Fatalf("energy-waited entry status = %q, want abandoned", got)
	}
	if got := statuses["done"]; got != taskledger.PendingDispatchConsumed {
		t.Fatalf("consumed entry status = %q, want consumed", got)
	}
	if got := statuses["gone"]; got != taskledger.PendingDispatchUnresolved {
		t.Fatalf("unresolved entry status = %q, want unresolved", got)
	}
	// A stipend is a new honey allocation, not a configuration boundary: the
	// held reaction still belongs to the pending change and must stay runnable.
	if got := statuses["held"]; got != taskledger.PendingDispatchPending {
		t.Fatalf("configuration-held entry status = %q, want pending", got)
	}
	active := taskledger.ActivePendingDispatches(res.Trace)
	if len(active) != 1 || active[0].ID != "held" {
		t.Fatalf("active pending = %+v, want only the configuration-held entry", active)
	}
	if len(res.Trace.PendingDispatches) != 4 {
		t.Fatalf("pending entries = %d, want 4 kept as evidence", len(res.Trace.PendingDispatches))
	}
}

// A stipend starts a new allocation. Task entries that waited on the previous
// reserve are abandoned together with their tasks, in the same mutation: the
// task becomes skipped with the summary naming the replacement, so the old tick
// neither survives as a resumable entry nor looks like completed work.
func TestApplyEventEnergyStipendSupersedesEnergyDeferredTaskWork(t *testing.T) {
	trace := taskledger.TraceSnapshot{
		TraceID:         "trace-1",
		EnergyBudget:    10,
		EnergyRemaining: 0,
		Tasks: map[string]taskledger.TaskSnapshot{
			"task-1":  {TaskID: "task-1", Status: protocol.TaskStatusReady},
			"task-2":  {TaskID: "task-2", Status: protocol.TaskStatusReady},
			"held":    {TaskID: "held", Status: protocol.TaskStatusReady},
			"running": {TaskID: "running", Status: protocol.TaskStatusRunning},
			"legacy":  {TaskID: "legacy", Status: protocol.TaskStatusBlocked, Summary: protocol.HoneyReserveExhaustedSummary},
			"done":    {TaskID: "done", Status: protocol.TaskStatusCompleted},
		},
		PendingDispatches: []taskledger.PendingDispatch{
			{ID: "task:task-1:builder", Kind: taskledger.PendingDispatchTask, TaskID: "task-1", Reason: protocol.HoneyReserveExhaustedSummary, Status: taskledger.PendingDispatchPending},
			{ID: "task:task-2:builder", Kind: taskledger.PendingDispatchTask, TaskID: "task-2", Reason: protocol.HoneyReserveExhaustedSummary, Status: taskledger.PendingDispatchPending},
			{ID: "task:held:builder", Kind: taskledger.PendingDispatchTask, TaskID: "held", Reason: protocol.ConfigBarrierHoldReason, Status: taskledger.PendingDispatchPending},
			{ID: "task:running:builder", Kind: taskledger.PendingDispatchTask, TaskID: "running", Reason: protocol.HoneyReserveExhaustedSummary, Status: taskledger.PendingDispatchPending},
			{ID: "direct", Kind: taskledger.PendingDispatchDirect, TaskID: "task-1", Reason: protocol.HoneyReserveExhaustedSummary, Status: taskledger.PendingDispatchPending},
			{ID: "direct-held", Kind: taskledger.PendingDispatchDirect, TaskID: "task-1", Reason: protocol.ConfigBarrierHoldReason, Status: taskledger.PendingDispatchPending},
			{ID: "done", Kind: taskledger.PendingDispatchDirect, TaskID: "task-1", Status: taskledger.PendingDispatchConsumed},
		},
	}
	ev, err := protocol.NewEvent("trace-1", "cli", 1, protocol.EventSignal, protocol.EnergyStipendPayload{
		Kind:   protocol.SignalEnergyStipend,
		Amount: 4,
	})
	if err != nil {
		t.Fatal(err)
	}
	res, err := taskledger.ApplyEvent(trace, ev)
	if err != nil {
		t.Fatal(err)
	}
	if !res.Changed {
		t.Fatal("stipend must persist its supersession")
	}
	wantSummary := protocol.SupersededByStipendSummary(4)
	for _, taskID := range []string{"task-1", "task-2"} {
		task := res.Trace.Tasks[taskID]
		if task.Status != protocol.TaskStatusSkipped {
			t.Fatalf("%s status = %q, want skipped", taskID, task.Status)
		}
		if task.Summary != wantSummary {
			t.Fatalf("%s summary = %q, want %q", taskID, task.Summary, wantSummary)
		}
	}
	// A configuration-held task entry is not energy waiting; a running task and
	// a legacy honey-blocked task keep their status, and a completed task is
	// never rewritten.
	if got := res.Trace.Tasks["held"].Status; got != protocol.TaskStatusReady {
		t.Fatalf("held status = %q, want ready", got)
	}
	if got := res.Trace.Tasks["running"].Status; got != protocol.TaskStatusRunning {
		t.Fatalf("running status = %q, want running", got)
	}
	if got := res.Trace.Tasks["legacy"]; got.Status != protocol.TaskStatusBlocked || got.Summary != protocol.HoneyReserveExhaustedSummary {
		t.Fatalf("legacy task = %+v, want untouched blocked", got)
	}
	if got := res.Trace.Tasks["done"].Status; got != protocol.TaskStatusCompleted {
		t.Fatalf("completed status = %q, want completed", got)
	}
	statuses := map[string]taskledger.PendingDispatchStatus{}
	for _, item := range res.Trace.PendingDispatches {
		statuses[item.ID] = item.Status
	}
	for _, id := range []string{"task:task-1:builder", "task:task-2:builder", "task:running:builder", "direct"} {
		if got := statuses[id]; got != taskledger.PendingDispatchAbandoned {
			t.Fatalf("%s status = %q, want abandoned", id, got)
		}
	}
	if got := statuses["task:held:builder"]; got != taskledger.PendingDispatchPending {
		t.Fatalf("held task entry = %q, want pending", got)
	}
	if got := statuses["direct-held"]; got != taskledger.PendingDispatchPending {
		t.Fatalf("held direct entry = %q, want pending", got)
	}
	if got := statuses["done"]; got != taskledger.PendingDispatchConsumed {
		t.Fatalf("consumed entry = %q, want consumed", got)
	}
}

// A duplicate stipend finds nothing left to abandon: the abandoned entries and
// skipped tasks stay exactly as the first stipend recorded them.
func TestApplyEventEnergyStipendSupersessionIsIdempotent(t *testing.T) {
	trace := taskledger.TraceSnapshot{
		TraceID:         "trace-1",
		EnergyBudget:    10,
		EnergyRemaining: 0,
		Tasks: map[string]taskledger.TaskSnapshot{
			"task-1": {TaskID: "task-1", Status: protocol.TaskStatusReady},
		},
		PendingDispatches: []taskledger.PendingDispatch{
			{ID: "task:task-1:builder", Kind: taskledger.PendingDispatchTask, TaskID: "task-1", Reason: protocol.HoneyReserveExhaustedSummary, Status: taskledger.PendingDispatchPending},
			{ID: "direct", Kind: taskledger.PendingDispatchDirect, TaskID: "task-1", Reason: protocol.HoneyReserveExhaustedSummary, Status: taskledger.PendingDispatchPending},
		},
	}
	stipend, err := protocol.NewEvent("trace-1", "cli", 1, protocol.EventSignal, protocol.EnergyStipendPayload{
		Kind:   protocol.SignalEnergyStipend,
		Amount: 4,
	})
	if err != nil {
		t.Fatal(err)
	}
	first, err := taskledger.ApplyEvent(trace, stipend)
	if err != nil {
		t.Fatal(err)
	}
	second, err := taskledger.ApplyEvent(first.Trace, stipend)
	if err != nil {
		t.Fatal(err)
	}
	if active := taskledger.ActivePendingDispatches(second.Trace); len(active) != 0 {
		t.Fatalf("active pending = %+v, want nothing left to abandon", active)
	}
	if got := second.Trace.Tasks["task-1"].Status; got != protocol.TaskStatusSkipped {
		t.Fatalf("status = %q, want skipped", got)
	}
	if got := second.Trace.Tasks["task-1"].Summary; got != protocol.SupersededByStipendSummary(4) {
		t.Fatalf("summary = %q, want the first supersession summary", got)
	}
	for _, item := range second.Trace.PendingDispatches {
		if item.Status != taskledger.PendingDispatchAbandoned {
			t.Fatalf("%s status = %q, want abandoned", item.ID, item.Status)
		}
	}
}

// A top-up after a stipend cannot revive superseded task work: the entry is
// abandoned and the task is skipped, so the drain has nothing to resume.
func TestEnergyAddDoesNotReviveSupersededTaskWork(t *testing.T) {
	trace := taskledger.TraceSnapshot{
		TraceID:         "trace-1",
		EnergyBudget:    10,
		EnergyRemaining: 0,
		Tasks: map[string]taskledger.TaskSnapshot{
			"task-1": {TaskID: "task-1", Status: protocol.TaskStatusReady},
		},
		PendingDispatches: []taskledger.PendingDispatch{
			{ID: "task:task-1:builder", Kind: taskledger.PendingDispatchTask, TaskID: "task-1", Reason: protocol.HoneyReserveExhaustedSummary, Status: taskledger.PendingDispatchPending},
		},
	}
	stipend, err := protocol.NewEvent("trace-1", "cli", 1, protocol.EventSignal, protocol.EnergyStipendPayload{
		Kind:   protocol.SignalEnergyStipend,
		Amount: 4,
	})
	if err != nil {
		t.Fatal(err)
	}
	res, err := taskledger.ApplyEvent(trace, stipend)
	if err != nil {
		t.Fatal(err)
	}
	add, err := protocol.NewEvent("trace-1", "cli", 2, protocol.EventSignal, protocol.EnergyAddPayload{
		Kind:   protocol.SignalEnergyAdd,
		Amount: 5,
	})
	if err != nil {
		t.Fatal(err)
	}
	res, err = taskledger.ApplyEvent(res.Trace, add)
	if err != nil {
		t.Fatal(err)
	}
	if got := res.Trace.Tasks["task-1"].Status; got != protocol.TaskStatusSkipped {
		t.Fatalf("status after top-up = %q, want skipped", got)
	}
	if active := taskledger.ActivePendingDispatches(res.Trace); len(active) != 0 {
		t.Fatalf("active pending = %+v, want none revived", active)
	}
}

func TestApplyEventEnergyAddDoesNotReviveStipendAbandonedWork(t *testing.T) {
	trace := taskledger.TraceSnapshot{
		TraceID:         "trace-1",
		EnergyBudget:    10,
		EnergyRemaining: 0,
		Tasks:           map[string]taskledger.TaskSnapshot{},
		PendingDispatches: []taskledger.PendingDispatch{
			{ID: "active", Kind: taskledger.PendingDispatchDirect, Status: taskledger.PendingDispatchPending},
		},
	}
	stipend, err := protocol.NewEvent("trace-1", "cli", 1, protocol.EventSignal, protocol.EnergyStipendPayload{
		Kind:   protocol.SignalEnergyStipend,
		Amount: 2,
	})
	if err != nil {
		t.Fatal(err)
	}
	res, err := taskledger.ApplyEvent(trace, stipend)
	if err != nil {
		t.Fatal(err)
	}
	if got := res.Trace.PendingDispatches[0].Status; got != taskledger.PendingDispatchAbandoned {
		t.Fatalf("status after stipend = %q, want abandoned", got)
	}

	add, err := protocol.NewEvent("trace-1", "cli", 2, protocol.EventSignal, protocol.EnergyAddPayload{
		Kind:   protocol.SignalEnergyAdd,
		Amount: 5,
	})
	if err != nil {
		t.Fatal(err)
	}
	res, err = taskledger.ApplyEvent(res.Trace, add)
	if err != nil {
		t.Fatal(err)
	}
	if res.Trace.EnergyRemaining != 7 {
		t.Fatalf("remaining = %d, want 7", res.Trace.EnergyRemaining)
	}
	if got := res.Trace.PendingDispatches[0].Status; got != taskledger.PendingDispatchAbandoned {
		t.Fatalf("status after energy.add = %q, want abandoned", got)
	}
	if active := taskledger.ActivePendingDispatches(res.Trace); len(active) != 0 {
		t.Fatalf("active pending = %+v, want none revived by energy.add", active)
	}
}

func TestApplyEventEnergyConsumeInsufficient(t *testing.T) {
	trace := taskledger.TraceSnapshot{
		TraceID:         "trace-1",
		EnergyBudget:    12,
		EnergyRemaining: 0,
		Tasks:           map[string]taskledger.TaskSnapshot{},
	}
	ev, err := protocol.NewEvent("trace-1", "runtime", 1, protocol.EventSignal, protocol.EnergyConsumePayload{
		Kind:   protocol.SignalEnergyConsume,
		Amount: 1,
	})
	if err != nil {
		t.Fatal(err)
	}
	_, err = taskledger.ApplyEvent(trace, ev)
	if err == nil {
		t.Fatal("expected insufficient honey reserve error")
	}
}

func TestMemoryLedgerSeedEnergy(t *testing.T) {
	ledger := taskledger.NewMemoryLedger()
	if err := ledger.SeedEnergy("trace-1", 12); err != nil {
		t.Fatal(err)
	}
	snap, err := ledger.Snapshot("trace-1")
	if err != nil {
		t.Fatal(err)
	}
	if snap.EnergyBudget != 12 || snap.EnergyRemaining != 12 {
		t.Fatalf("snap = %+v", snap)
	}
	if err := ledger.SeedEnergy("trace-1", 20); err != nil {
		t.Fatal(err)
	}
	snap, err = ledger.Snapshot("trace-1")
	if err != nil {
		t.Fatal(err)
	}
	if snap.EnergyRemaining != 12 {
		t.Fatalf("seed should be idempotent, remaining = %d", snap.EnergyRemaining)
	}
}

func TestEnsureEnergySeededDoesNotClearPostSeedAdded(t *testing.T) {
	trace := taskledger.TraceSnapshot{
		TraceID:         "trace-1",
		EnergyBudget:    12,
		EnergyRemaining: 7,
		EnergyAdded:     5,
	}
	again, changed := taskledger.EnsureEnergySeeded(trace, 20)
	if changed {
		t.Fatal("expected no change when already seeded")
	}
	if again.EnergyAdded != 5 {
		t.Fatalf("added = %d, want 5", again.EnergyAdded)
	}
}

func TestFormatHoney(t *testing.T) {
	if got := taskledger.Allocated(12, 8); got != 20 {
		t.Fatalf("allocated = %d", got)
	}
	if got := taskledger.Allocated(0, 5); got != 0 {
		t.Fatalf("unseeded allocated = %d", got)
	}
	if got := taskledger.FormatHoneyPrimary(5, 12, 8); got != "5 / 20" {
		t.Fatalf("primary = %q", got)
	}
	if got := taskledger.FormatHoneyPrimary(5, 0, 0); got != "5" {
		t.Fatalf("unseeded primary = %q, want remaining only", got)
	}
	if got := taskledger.FormatHoneyCompact(5, 12, 8); got != "5/20" {
		t.Fatalf("compact = %q", got)
	}
	if got := taskledger.FormatHoneyCompact(5, 0, 0); got != "5" {
		t.Fatalf("unseeded compact = %q", got)
	}
	if got := taskledger.FormatHoneyRemaining(5, 12, 8); got != "5/20 remaining" {
		t.Fatalf("remaining primary = %q", got)
	}
	if got := taskledger.FormatHoneyRemaining(5, 12, 0); got != "5/12 remaining" {
		t.Fatalf("remaining primary without top-up = %q", got)
	}
	if got := taskledger.FormatHoneyRemaining(5, 0, 0); got != "5" {
		t.Fatalf("unseeded remaining primary = %q, want remaining only", got)
	}
	if got := taskledger.FormatHoneySecondary(12, 8); got != "seed 12 · topped 8" {
		t.Fatalf("secondary = %q", got)
	}
	if got := taskledger.FormatHoneySecondary(12, 0); got != "" {
		t.Fatalf("secondary = %q, want empty", got)
	}
	report := taskledger.FormatHoneyReport(5, 12, 8)
	wantReport := []string{"budget:    12", "remaining: 5", "added:     8", "5 / 20", "seed 12 · topped 8"}
	if len(report) != len(wantReport) {
		t.Fatalf("report = %#v", report)
	}
	for i := range wantReport {
		if report[i] != wantReport[i] {
			t.Fatalf("report[%d] = %q, want %q", i, report[i], wantReport[i])
		}
	}
	plain := taskledger.FormatHoneyReport(12, 12, 0)
	if len(plain) != 2 || plain[0] != "budget:    12" || plain[1] != "remaining: 12" {
		t.Fatalf("plain report = %#v", plain)
	}
}

func TestFormatPendingReport(t *testing.T) {
	if got := taskledger.FormatPendingReport(0); len(got) != 1 || got[0] != "pending:   0" {
		t.Fatalf("empty pending report = %#v", got)
	}
	if got := taskledger.FormatPendingReport(2); len(got) != 1 || got[0] != "pending:   2" {
		t.Fatalf("pending report = %#v", got)
	}
}
