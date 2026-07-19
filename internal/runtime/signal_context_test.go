package runtime

import (
	"encoding/json"
	"testing"

	"github.com/paseka/paseka/internal/protocol"
)

func TestEventDispatchContextSignalFeatureRequested(t *testing.T) {
	ev, err := protocol.NewEvent("trace-1", "telegram", 0, protocol.EventSignal, map[string]any{
		"kind":  "feature.requested",
		"title": "Live bees indicator",
		"body":  "Show active bees in the console header.",
	})
	if err != nil {
		t.Fatal(err)
	}

	taskID, taskBody, err := eventDispatchContext(ev)
	if err != nil {
		t.Fatal(err)
	}
	if taskID != "" {
		t.Fatalf("taskId = %q, want empty", taskID)
	}
	if taskBody != "Live bees indicator\n\nShow active bees in the console header." {
		t.Fatalf("taskBody = %q", taskBody)
	}
}

func TestEventDispatchContextSignalBodyOnly(t *testing.T) {
	ev, err := protocol.NewEvent("trace-1", "human", 0, protocol.EventSignal, map[string]any{
		"kind": "spec.ready",
		"body": "Break down docs/specs/004-live-bees-indicator.md",
	})
	if err != nil {
		t.Fatal(err)
	}

	_, taskBody, err := eventDispatchContext(ev)
	if err != nil {
		t.Fatal(err)
	}
	if taskBody != "Break down docs/specs/004-live-bees-indicator.md" {
		t.Fatalf("taskBody = %q", taskBody)
	}
}

func TestEventDispatchContextSignalDenylist(t *testing.T) {
	for _, kind := range []string{
		string(protocol.TaskEventReady),
		string(protocol.TaskEventStatus),
		string(protocol.SignalEnergyAdd),
		string(protocol.SignalEnergyConsume),
		string(protocol.SignalSessionInvite),
		string(protocol.SignalBeekeeperReady),
	} {
		t.Run(kind, func(t *testing.T) {
			ev, err := protocol.NewEvent("trace-1", "human", 0, protocol.EventSignal, map[string]any{
				"kind": kind,
				"body": "should not dispatch",
			})
			if err != nil {
				t.Fatal(err)
			}
			_, _, err = eventDispatchContext(ev)
			if err == nil {
				t.Fatal("expected error for denylisted SIGNAL kind")
			}
		})
	}
}

func TestEventDispatchContextSignalWithTaskID(t *testing.T) {
	raw := json.RawMessage(`{"kind":"feature.requested","taskId":"task-1","title":"Title only"}`)
	ev := protocol.Event{
		TraceID: "trace-1",
		AgentID: "human",
		Type:    protocol.EventSignal,
		Payload: raw,
	}
	taskID, taskBody, err := eventDispatchContext(ev)
	if err != nil {
		t.Fatal(err)
	}
	if taskID != "task-1" {
		t.Fatalf("taskId = %q", taskID)
	}
	if taskBody != "Title only" {
		t.Fatalf("taskBody = %q", taskBody)
	}
}
