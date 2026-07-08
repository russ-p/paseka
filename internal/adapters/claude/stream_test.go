package claude

import (
	"testing"

	"github.com/paseka/paseka/internal/protocol"
)

func TestParseStreamJSONResultAndText(t *testing.T) {
	stdout := `{"type":"system","subtype":"init","model":"claude-opus-4-8"}
{"type":"assistant","message":{"content":[{"type":"text","text":"working on it"}]}}
{"type":"result","subtype":"success","is_error":false,"result":"done"}`

	got := parseStreamJSON(stdout, "trace-1", "agent-1")
	if got.Summary != "done" {
		t.Fatalf("summary = %q, want done", got.Summary)
	}
	if len(got.Events) != 1 {
		t.Fatalf("events = %d, want 1", len(got.Events))
	}
	if got.Events[0].Type != protocol.EventAssistantText {
		t.Fatalf("event type = %q, want ASSISTANT_TEXT", got.Events[0].Type)
	}
}

func TestParseStreamJSONToolUse(t *testing.T) {
	stdout := `{"type":"assistant","message":{"content":[{"type":"tool_use","name":"Edit","input":{"file_path":"main.go"}}]}}
{"type":"result","subtype":"success","result":"ok"}`

	got := parseStreamJSON(stdout, "t", "a")
	if len(got.Events) != 1 {
		t.Fatalf("events = %d, want 1", len(got.Events))
	}
	if got.Events[0].Type != protocol.EventToolCall {
		t.Fatalf("type = %q, want TOOL_CALL", got.Events[0].Type)
	}
}

func TestParseStreamJSONMixedContentAndError(t *testing.T) {
	// One assistant message carrying both text and a tool_use, plus an error
	// result that must NOT overwrite the summary.
	stdout := `{"type":"assistant","message":{"content":[{"type":"text","text":"planning"},{"type":"tool_use","name":"Read","input":{"file_path":"go.mod"}}]}}
{"type":"result","subtype":"error_max_turns","is_error":true,"result":"stopped"}`

	got := parseStreamJSON(stdout, "t", "a")
	if got.Summary != "" {
		t.Fatalf("summary = %q, want empty on error result", got.Summary)
	}
	if len(got.Events) != 2 {
		t.Fatalf("events = %d, want 2", len(got.Events))
	}
	if got.Events[0].Type != protocol.EventAssistantText {
		t.Fatalf("event[0] type = %q", got.Events[0].Type)
	}
	if got.Events[1].Type != protocol.EventToolCall {
		t.Fatalf("event[1] type = %q", got.Events[1].Type)
	}
}
