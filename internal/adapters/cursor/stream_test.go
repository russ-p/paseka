package cursor

import (
	"testing"

	"github.com/paseka/paseka/internal/protocol"
)

func TestParseStreamJSONResultEvent(t *testing.T) {
	stdout := `{"type":"assistant","timestamp_ms":1,"message":{"content":[{"text":"working"}]}}
{"type":"result","subtype":"success","result":"done"}`

	got := parseStreamJSON(stdout, "trace-1", "agent-1")
	if got.Summary != "done" {
		t.Fatalf("summary = %q, want done", got.Summary)
	}
	if len(got.Events) != 1 {
		t.Fatalf("events = %d, want 1", len(got.Events))
	}
	if got.Events[0].Type != protocol.EventAssistantText {
		t.Fatalf("event type = %q", got.Events[0].Type)
	}
}

func TestParseStreamJSONSkipsBufferedAssistant(t *testing.T) {
	stdout := `{"type":"assistant","model_call_id":"x","message":{"content":[{"text":"skip"}]}}
{"type":"result","subtype":"success","result":"final"}`

	got := parseStreamJSON(stdout, "t", "a")
	if got.Summary != "final" {
		t.Fatalf("summary = %q", got.Summary)
	}
	if len(got.Events) != 0 {
		t.Fatalf("expected no assistant events, got %d", len(got.Events))
	}
}

func TestParseStreamJSONToolCall(t *testing.T) {
	stdout := `{"type":"tool_call","subtype":"started","toolCall":{"readToolCall":{"args":{"path":"main.go"}}}}
{"type":"result","subtype":"success","result":"ok"}`

	got := parseStreamJSON(stdout, "t", "a")
	if len(got.Events) != 1 {
		t.Fatalf("events = %d, want 1", len(got.Events))
	}
	if got.Events[0].Type != protocol.EventToolCall {
		t.Fatalf("type = %q", got.Events[0].Type)
	}
}
