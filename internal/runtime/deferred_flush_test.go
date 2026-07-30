package runtime_test

import (
	"context"
	"strings"
	"testing"

	"github.com/paseka/paseka/internal/adapters"
	"github.com/paseka/paseka/internal/protocol"
	"github.com/paseka/paseka/internal/runs"
	"github.com/paseka/paseka/internal/runtime"
)

type deferredSeedingAdapter struct {
	result  *adapters.RunResult
	pending []protocol.Event
}

func (a *deferredSeedingAdapter) Name() string { return "cursor" }

func (a *deferredSeedingAdapter) Run(_ context.Context, req adapters.RunRequest) (*adapters.RunResult, error) {
	runDir := runs.Dir{
		ColonyRoot: req.ColonyRoot,
		TraceID:    req.TraceID,
		AgentID:    req.AgentID,
	}
	for _, ev := range a.pending {
		if err := runDir.AppendPending(ev); err != nil {
			return nil, err
		}
	}
	if a.result != nil {
		return a.result, nil
	}
	return &adapters.RunResult{Status: string(protocol.StatusCompleted), Output: "ok"}, nil
}

func TestDispatchFlushesDeferredOnSuccessBeforeRunSummary(t *testing.T) {
	root := t.TempDir()
	writeColony(t, root)

	first, err := protocol.NewEvent("trace-def", "agent-def", 0, protocol.EventInsight, map[string]any{
		"kind":    "context.note",
		"summary": "first deferred",
	})
	if err != nil {
		t.Fatal(err)
	}
	second, err := protocol.NewEvent("trace-def", "agent-def", 0, protocol.EventInsight, map[string]any{
		"kind":    "context.note",
		"summary": "second deferred",
	})
	if err != nil {
		t.Fatal(err)
	}

	adapter := &deferredSeedingAdapter{
		result: &adapters.RunResult{
			Status:  string(protocol.StatusCompleted),
			Summary: "done",
		},
		pending: []protocol.Event{first, second},
	}
	pub := &recordingPublisher{}
	d := runtime.NewDispatcher()
	d.RegisterAdapter("cursor", adapter)
	d.SetPublisher(pub, false)

	res, err := d.Dispatch(context.Background(), runtime.DispatchRequest{
		ColonyRoot: root,
		Bee:        "builder",
		TraceID:    "trace-def",
		AgentID:    "agent-def",
		Task:       "ship deferred flush",
	})
	if err != nil {
		t.Fatal(err)
	}
	if res.Status != string(protocol.StatusCompleted) {
		t.Fatalf("status = %q", res.Status)
	}

	if len(pub.events) < 3 {
		t.Fatalf("published %d events, want at least 3 (2 deferred + run.summary)", len(pub.events))
	}
	if protocol.PayloadKind(pub.events[0].Payload) != "context.note" {
		t.Fatalf("first publish kind = %q, want context.note", protocol.PayloadKind(pub.events[0].Payload))
	}
	if protocol.PayloadKind(pub.events[1].Payload) != "context.note" {
		t.Fatalf("second publish kind = %q, want context.note", protocol.PayloadKind(pub.events[1].Payload))
	}
	summaryIdx := -1
	for i, ev := range pub.events {
		if ev.Type == protocol.EventInsight && protocol.PayloadKind(ev.Payload) == string(protocol.InsightRunSummary) {
			summaryIdx = i
			break
		}
	}
	if summaryIdx < 2 {
		t.Fatalf("run.summary index = %d, want after deferred events; events=%+v", summaryIdx, pub.events)
	}

	runDir := runs.Dir{ColonyRoot: root, TraceID: "trace-def", AgentID: "agent-def"}
	pending, err := runDir.ReadPending()
	if err != nil {
		t.Fatal(err)
	}
	if len(pending) != 0 {
		t.Fatalf("pending = %d, want 0 after success flush", len(pending))
	}
}

func TestDispatchLeavesPendingOnFailedRun(t *testing.T) {
	root := t.TempDir()
	writeColony(t, root)

	ev, err := protocol.NewEvent("trace-fail", "agent-fail", 0, protocol.EventInsight, map[string]any{
		"kind":    "context.note",
		"summary": "should stay queued",
	})
	if err != nil {
		t.Fatal(err)
	}

	adapter := &deferredSeedingAdapter{
		result: &adapters.RunResult{
			Status: string(protocol.StatusFailed),
			Output: "boom",
		},
		pending: []protocol.Event{ev},
	}
	pub := &recordingPublisher{}
	d := runtime.NewDispatcher()
	d.RegisterAdapter("cursor", adapter)
	d.SetPublisher(pub, false)

	res, err := d.Dispatch(context.Background(), runtime.DispatchRequest{
		ColonyRoot: root,
		Bee:        "builder",
		TraceID:    "trace-fail",
		AgentID:    "agent-fail",
		Task:       "fail with pending",
	})
	if err != nil {
		t.Fatal(err)
	}
	if res.Status != string(protocol.StatusFailed) {
		t.Fatalf("status = %q", res.Status)
	}
	if len(pub.events) != 0 {
		t.Fatalf("published %d events, want 0 on failed run", len(pub.events))
	}

	runDir := runs.Dir{ColonyRoot: root, TraceID: "trace-fail", AgentID: "agent-fail"}
	pending, err := runDir.ReadPending()
	if err != nil {
		t.Fatal(err)
	}
	if len(pending) != 1 {
		t.Fatalf("pending = %d, want 1", len(pending))
	}

	foundWarning := false
	for _, w := range res.Warnings {
		if strings.Contains(w, "deferred event") {
			foundWarning = true
			break
		}
	}
	if !foundWarning {
		t.Fatalf("expected pending warning in result, got %#v", res.Warnings)
	}

	status, err := runDir.ReadStatus()
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(status.Error, "deferred event") {
		t.Fatalf("status error = %q, want pending warning", status.Error)
	}
}
