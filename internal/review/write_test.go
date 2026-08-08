package review_test

import (
	"context"
	"errors"
	"testing"

	"github.com/paseka/paseka/internal/protocol"
	"github.com/paseka/paseka/internal/review"
	"github.com/paseka/paseka/internal/taskledger"
)

type recordingPublisher struct {
	events []protocol.Event
}

func (p *recordingPublisher) PublishEvent(_ context.Context, ev protocol.Event) error {
	p.events = append(p.events, ev)
	return nil
}

type failingLedger struct {
	err error
}

func (f failingLedger) Snapshot(string) (taskledger.TraceSnapshot, error) {
	return taskledger.TraceSnapshot{}, f.err
}

func (f failingLedger) Apply(protocol.Event) (taskledger.ApplyResult, error) {
	return taskledger.ApplyResult{}, f.err
}

func (f failingLedger) SeedEnergy(string, int) error {
	return f.err
}

func TestWriteEventApplyBeforePublish(t *testing.T) {
	ledger := taskledger.NewMemoryLedger()
	pub := &recordingPublisher{}
	traceID := "trace-write-order"

	plan, err := protocol.NewEvent(traceID, "scout", 0, protocol.EventInsight, protocol.TaskPlanPayload{
		Kind: protocol.TaskEventPlan,
		Tasks: []protocol.TaskSpec{
			{TaskID: "task-1", Title: "Work"},
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := ledger.Apply(plan); err != nil {
		t.Fatal(err)
	}

	ev, err := protocol.NewEvent(traceID, "runtime", 0, protocol.EventVerification, protocol.TaskCompletedPayload{
		Kind:   protocol.TaskEventCompleted,
		TaskID: "task-1",
		Status: protocol.TaskStatusCompleted,
	})
	if err != nil {
		t.Fatal(err)
	}

	var afterApplyCalled bool
	if err := review.WriteEvent(context.Background(), pub, ledger, ev, review.WriteOptions{
		AfterApply: func(protocol.Event) { afterApplyCalled = true },
	}); err != nil {
		t.Fatal(err)
	}

	snap, err := ledger.Snapshot(traceID)
	if err != nil {
		t.Fatal(err)
	}
	if snap.Tasks["task-1"].Status != protocol.TaskStatusCompleted {
		t.Fatalf("task status = %q, want completed before publish assertion", snap.Tasks["task-1"].Status)
	}
	if !afterApplyCalled {
		t.Fatal("AfterApply hook was not called")
	}
	if len(pub.events) != 1 {
		t.Fatalf("published %d events, want 1", len(pub.events))
	}
}

func TestWriteEventDoesNotPublishWhenApplyFails(t *testing.T) {
	pub := &recordingPublisher{}
	failErr := errors.New("apply failed")
	ev, err := protocol.NewEvent("trace-1", "runtime", 0, protocol.EventVerification, protocol.TaskCompletedPayload{
		Kind:   protocol.TaskEventCompleted,
		TaskID: "task-1",
		Status: protocol.TaskStatusCompleted,
	})
	if err != nil {
		t.Fatal(err)
	}

	err = review.WriteEvent(context.Background(), pub, failingLedger{err: failErr}, ev, review.WriteOptions{})
	if !errors.Is(err, failErr) {
		t.Fatalf("WriteEvent err = %v, want %v", err, failErr)
	}
	if len(pub.events) != 0 {
		t.Fatalf("published %d events, want 0 when apply fails", len(pub.events))
	}
}
