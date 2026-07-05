package protocol_test

import (
	"encoding/json"
	"testing"
	"time"

	"github.com/paseka/paseka/internal/protocol"
)

func TestNewEventDefaults(t *testing.T) {
	ev, err := protocol.NewEvent("t1", "a1", 1, protocol.EventInsight, map[string]string{"k": "v"})
	if err != nil {
		t.Fatal(err)
	}
	if ev.ProtocolVersion != protocol.Version {
		t.Fatalf("version = %q", ev.ProtocolVersion)
	}
	if ev.TraceID != "t1" || ev.AgentID != "a1" || ev.Seq != 1 {
		t.Fatalf("ids mismatch: %+v", ev)
	}
}

func TestTaskPlanPayloadJSON(t *testing.T) {
	payload := protocol.TaskPlanPayload{
		Kind: protocol.TaskEventPlan,
		Tasks: []protocol.TaskSpec{
			{TaskID: "task-1", Title: "Add endpoint", Bee: "builder"},
			{TaskID: "task-2", Title: "Add UI", Bee: "builder", DependsOn: []string{"task-1"}},
		},
	}
	data, err := json.Marshal(payload)
	if err != nil {
		t.Fatal(err)
	}
	var decoded protocol.TaskPlanPayload
	if err := json.Unmarshal(data, &decoded); err != nil {
		t.Fatal(err)
	}
	if decoded.Kind != protocol.TaskEventPlan {
		t.Fatalf("kind = %q", decoded.Kind)
	}
	if len(decoded.Tasks) != 2 || decoded.Tasks[1].DependsOn[0] != "task-1" {
		t.Fatalf("tasks = %+v", decoded.Tasks)
	}
}

func TestTaskReadyPayloadJSON(t *testing.T) {
	payload := protocol.TaskReadyPayload{
		Kind:   protocol.TaskEventReady,
		TaskID: "task-1",
		Title:  "Add endpoint",
		Bee:    "builder",
	}
	data, err := json.Marshal(payload)
	if err != nil {
		t.Fatal(err)
	}
	var decoded protocol.TaskReadyPayload
	if err := json.Unmarshal(data, &decoded); err != nil {
		t.Fatal(err)
	}
	if decoded.Kind != protocol.TaskEventReady || decoded.TaskID != "task-1" {
		t.Fatalf("decoded = %+v", decoded)
	}
}

func TestTaskCompletedPayloadJSON(t *testing.T) {
	when := time.Date(2026, 7, 5, 8, 30, 0, 0, time.UTC)
	payload := protocol.TaskCompletedPayload{
		Kind:        protocol.TaskEventCompleted,
		TaskID:      "task-1",
		Status:      protocol.TaskStatusCompleted,
		Summary:     "done",
		Commit:      "abc123",
		CompletedAt: when,
	}
	data, err := json.Marshal(payload)
	if err != nil {
		t.Fatal(err)
	}
	var decoded protocol.TaskCompletedPayload
	if err := json.Unmarshal(data, &decoded); err != nil {
		t.Fatal(err)
	}
	if decoded.Kind != protocol.TaskEventCompleted || decoded.Commit != "abc123" {
		t.Fatalf("decoded = %+v", decoded)
	}
}

func TestRequestTaskIDJSON(t *testing.T) {
	req := protocol.Request{
		ProtocolVersion: protocol.Version,
		TraceID:         "trace-1",
		AgentID:         "agent-1",
		TaskID:          "task-1",
		Task:            "implement auth",
	}
	data, err := json.Marshal(req)
	if err != nil {
		t.Fatal(err)
	}
	var decoded protocol.Request
	if err := json.Unmarshal(data, &decoded); err != nil {
		t.Fatal(err)
	}
	if decoded.TaskID != "task-1" {
		t.Fatalf("taskId = %q", decoded.TaskID)
	}
}

func TestBusEventTaskIDJSON(t *testing.T) {
	bus := protocol.BusEvent{
		TraceID: "trace-1",
		TaskID:  "task-1",
		Type:    protocol.EventSignal,
	}
	data, err := json.Marshal(bus)
	if err != nil {
		t.Fatal(err)
	}
	if !json.Valid(data) {
		t.Fatal("invalid json")
	}
	var decoded protocol.BusEvent
	if err := json.Unmarshal(data, &decoded); err != nil {
		t.Fatal(err)
	}
	if decoded.TaskID != "task-1" {
		t.Fatalf("taskId = %q", decoded.TaskID)
	}
}
