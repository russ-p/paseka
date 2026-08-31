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

func TestParseStreamJSONUsage(t *testing.T) {
	stdout := `{"type":"result","subtype":"success","result":"done","duration_ms":2453,"duration_api_ms":2400,"usage":{"inputTokens":8848,"outputTokens":56,"cacheReadTokens":5472,"cacheWriteTokens":0}}`

	got := parseStreamJSON(stdout, "t", "a")
	if got.Summary != "done" {
		t.Fatalf("summary = %q", got.Summary)
	}
	if got.Usage == nil {
		t.Fatal("expected usage")
	}
	if got.Usage.InputTokens != 8848 || got.Usage.OutputTokens != 56 {
		t.Fatalf("usage tokens = %+v", got.Usage)
	}
	if got.Usage.CacheReadTokens != 5472 || got.Usage.CacheWriteTokens != 0 {
		t.Fatalf("usage cache = %+v", got.Usage)
	}
	if got.Usage.DurationMs != 2453 {
		t.Fatalf("durationMs = %d, want 2453", got.Usage.DurationMs)
	}
	if got.Usage.Source != protocol.UsageSourceCursorStreamJSON {
		t.Fatalf("source = %q", got.Usage.Source)
	}
}

func TestParseStreamJSONUsageAbsent(t *testing.T) {
	stdout := `{"type":"result","subtype":"success","result":"done"}`
	got := parseStreamJSON(stdout, "t", "a")
	if got.Usage != nil {
		t.Fatalf("expected nil usage, got %+v", got.Usage)
	}
}

func TestParseStreamJSONSessionIDFromInit(t *testing.T) {
	stdout := `{"type":"system","subtype":"init","session_id":"init-uuid"}
{"type":"assistant","timestamp_ms":1,"message":{"content":[{"text":"hi"}]},"session_id":"later-uuid"}
{"type":"result","subtype":"success","result":"done","session_id":"later-uuid"}`

	got := parseStreamJSON(stdout, "t", "a")
	if got.SessionID != "init-uuid" {
		t.Fatalf("session id = %q, want init-uuid", got.SessionID)
	}
}

func TestParseStreamJSONSessionIDLaterLineFallback(t *testing.T) {
	stdout := `{"type":"assistant","timestamp_ms":1,"message":{"content":[{"text":"hi"}]},"session_id":"from-assistant"}
{"type":"result","subtype":"success","result":"done","session_id":"from-result"}`

	got := parseStreamJSON(stdout, "t", "a")
	if got.SessionID != "from-assistant" {
		t.Fatalf("session id = %q, want from-assistant", got.SessionID)
	}
}

func TestParseStreamJSONSessionIDInitWinsOverEarlierLine(t *testing.T) {
	stdout := `{"type":"user","session_id":"user-uuid"}
{"type":"system","subtype":"init","session_id":"init-uuid"}`

	got := parseStreamJSON(stdout, "t", "a")
	if got.SessionID != "init-uuid" {
		t.Fatalf("session id = %q, want init-uuid", got.SessionID)
	}
}

func TestParseStreamJSONSessionIDCompactJSON(t *testing.T) {
	stdout := `{"type":"result","subtype":"success","result":"done","session_id":"compact-uuid"}`
	got := parseStreamJSON(stdout, "t", "a")
	if got.SessionID != "compact-uuid" {
		t.Fatalf("session id = %q, want compact-uuid", got.SessionID)
	}
}

func TestParseStreamJSONSessionIDAbsentOnText(t *testing.T) {
	got := parseStreamJSON("plain agent output", "t", "a")
	if got.SessionID != "" {
		t.Fatalf("session id = %q, want empty", got.SessionID)
	}
}

func TestParseStreamJSONSessionIDPrettyPrinted(t *testing.T) {
	stdout := `{
  "type": "result",
  "subtype": "success",
  "result": "done",
  "session_id": "pretty-uuid"
}`
	got := parseStreamJSON(stdout, "t", "a")
	if got.SessionID != "pretty-uuid" {
		t.Fatalf("session id = %q, want pretty-uuid", got.SessionID)
	}
}
