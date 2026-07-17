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
