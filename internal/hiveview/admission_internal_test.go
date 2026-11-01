package hiveview

import (
	"strings"
	"testing"

	"github.com/russ-p/paseka/internal/colony"
	"github.com/russ-p/paseka/internal/protocol"
	"github.com/russ-p/paseka/internal/taskledger"
	"github.com/russ-p/paseka/internal/trailview"
)

// The Console row must read the same admission projection as `paseka trail` and
// `paseka energy show`; it is a trail axis, not a task status.
func TestApplyTrailViewCarriesEnergyAdmission(t *testing.T) {
	view := TraceSummaryView{TraceID: "trace-1"}
	applyTrailView(&view, trailview.Trail{
		TraceID:      "trace-1",
		Paused:       true,
		PauseReason:  protocol.HoneyReserveExhaustedSummary,
		PendingCount: 2,
	})
	if !view.Paused || view.PauseReason != protocol.HoneyReserveExhaustedSummary || view.PendingCount != 2 {
		t.Fatalf("view admission = %+v", view)
	}
}

// The status attention block lists paused trails from the energy window, with
// the reason and the work waiting, instead of inferring a pause from a task.
func TestBuildSnapshotAttentionPausedTraces(t *testing.T) {
	energy := SnapshotEnergy{
		Available: true,
		Traces: []SnapshotEnergyTrace{
			{TraceID: "t1", Remaining: 0, Budget: 10, Paused: true, PauseReason: protocol.HoneyReserveExhaustedSummary, PendingCount: 2},
			{TraceID: "t2", Remaining: 5, Budget: 10},
			{TraceID: "t3", Remaining: 0, Budget: 10},
		},
	}
	att := buildSnapshotAttention(
		SnapshotRuntime{Status: "running", Alive: true},
		SnapshotNATS{},
		TaskBoardView{},
		nil,
		energy,
	)
	if len(att.PausedTraces) != 1 {
		t.Fatalf("pausedTraces = %+v", att.PausedTraces)
	}
	paused := att.PausedTraces[0]
	if paused.TraceID != "t1" || paused.Reason != protocol.HoneyReserveExhaustedSummary || paused.PendingCount != 2 {
		t.Fatalf("paused row = %+v", paused)
	}

	snap := ColonySnapshot{
		Slug:      "test",
		Attention: att,
	}
	text := FormatColonySnapshot(snap)
	if !strings.Contains(text, "paused trails: 1") || !strings.Contains(text, "t1 (2 pending)") {
		t.Fatalf("paused attention missing:\n%s", text)
	}
}

// The board counts must keep skipped and legacy blocked as separate statuses,
// and the pending queue as a trail-level count beside them.
func TestBuildTaskBoardDistinguishesSkippedBlockedAndPending(t *testing.T) {
	items := []TaskListItem{
		{TraceID: "trace-1", TaskID: "ready", Status: string(protocol.TaskStatusReady)},
		{TraceID: "trace-1", TaskID: "skipped", Status: string(protocol.TaskStatusSkipped)},
		{TraceID: "trace-1", TaskID: "legacy", Status: string(protocol.TaskStatusBlocked)},
	}
	board := buildTaskBoard(items)
	board.PendingDispatches = 1

	if board.TaskCounts[string(protocol.TaskStatusSkipped)] != 1 {
		t.Fatalf("skipped count = %d", board.TaskCounts[string(protocol.TaskStatusSkipped)])
	}
	if board.TaskCounts[string(protocol.TaskStatusBlocked)] != 1 {
		t.Fatalf("blocked count = %d", board.TaskCounts[string(protocol.TaskStatusBlocked)])
	}
	if board.PendingDispatches != 1 {
		t.Fatalf("pending = %d", board.PendingDispatches)
	}
	order := map[string]int{}
	for i, group := range board.Groups {
		order[group.Status] = i
	}
	if _, ok := order[string(protocol.TaskStatusSkipped)]; !ok {
		t.Fatalf("skipped group missing: %+v", board.Groups)
	}
	if order[string(protocol.TaskStatusBlocked)] > order[string(protocol.TaskStatusSkipped)] {
		t.Fatalf("legacy blocked should sort before skipped: %+v", order)
	}
}

// A ready task held in the pending queue keeps its true status on the board row
// and carries the admission fact beside it.
func TestTaskItemMarksPendingDispatchWithoutChangingStatus(t *testing.T) {
	ctx := colony.Context{ColonyRoot: t.TempDir(), Slug: "test"}
	task := taskledger.TaskSnapshot{TaskID: "task-1", Title: "Held work", Status: protocol.TaskStatusReady}
	snap := taskledger.TraceSnapshot{
		TraceID: "trace-1",
		Tasks:   map[string]taskledger.TaskSnapshot{"task-1": task},
		PendingDispatches: []taskledger.PendingDispatch{
			{ID: "task:task-1:builder", Kind: taskledger.PendingDispatchTask, TaskID: "task-1", Status: taskledger.PendingDispatchPending},
		},
	}

	item := TaskItemFromSnapshot(ctx, "trace-1", snap, task)
	if item.Status != string(protocol.TaskStatusReady) {
		t.Fatalf("status = %q, want the lifecycle status unchanged", item.Status)
	}
	if !item.PendingDispatch {
		t.Fatal("expected pendingDispatch on the held task")
	}
}

// A bulk projection derives the pending set once per snapshot and passes it in;
// the row must match the single-row wrapper, and a nil set must read as "no
// pending task" rather than panic.
func TestTaskItemFromSnapshotWithPendingSet(t *testing.T) {
	ctx := colony.Context{ColonyRoot: t.TempDir(), Slug: "test"}
	task := taskledger.TaskSnapshot{TaskID: "task-1", Title: "Held work", Status: protocol.TaskStatusReady}
	snap := taskledger.TraceSnapshot{
		TraceID: "trace-1",
		Tasks:   map[string]taskledger.TaskSnapshot{"task-1": task},
		PendingDispatches: []taskledger.PendingDispatch{
			{ID: "task:task-1:builder", Kind: taskledger.PendingDispatchTask, TaskID: "task-1", Status: taskledger.PendingDispatchPending},
		},
	}

	hoisted := TaskItemFromSnapshotWithPending(ctx, "trace-1", snap, task, taskledger.PendingTaskIDs(snap))
	if !hoisted.PendingDispatch {
		t.Fatal("expected pendingDispatch from the precomputed set")
	}
	if single := TaskItemFromSnapshot(ctx, "trace-1", snap, task); single.PendingDispatch != hoisted.PendingDispatch {
		t.Fatalf("hoisted row pendingDispatch = %t, single-row = %t", hoisted.PendingDispatch, single.PendingDispatch)
	}

	empty := TaskItemFromSnapshotWithPending(ctx, "trace-1", snap, task, nil)
	if empty.PendingDispatch {
		t.Fatal("a nil pending set must read as no pending task")
	}
	if empty.Status != string(protocol.TaskStatusReady) {
		t.Fatalf("status = %q, nil set must not alter the row", empty.Status)
	}
}
