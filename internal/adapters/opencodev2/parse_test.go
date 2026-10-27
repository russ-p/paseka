package opencodev2

import (
	"encoding/json"
	"testing"

	"github.com/russ-p/paseka/internal/protocol"
)

func TestParseJSONLSingleStep(t *testing.T) {
	stdout := `{"type":"text","sessionID":"ses_abc","part":{"type":"text","text":"first"}}
{"type":"text","sessionID":"ses_abc","part":{"type":"text","text":"second"}}
{"type":"tool_use","sessionID":"ses_abc","part":{"type":"tool","tool":"bash"}}
`
	got := parseRunOutput(stdout, "json")
	if got.Summary != "first\nsecond" {
		t.Fatalf("summary = %q", got.Summary)
	}
	if got.SessionID != "ses_abc" {
		t.Fatalf("session = %q", got.SessionID)
	}
	if got.Usage != nil {
		t.Fatalf("usage = %+v, want nil without a token event", got.Usage)
	}
}

func TestParseJSONLStepFinishUsage(t *testing.T) {
	stdout := `{"type":"step_finish","sessionID":"ses_usage","part":{"tokens":{"input":10,"output":4,"reasoning":2,"cache":{"read":3,"write":1}},"cost":0.25}}
{"type":"text","sessionID":"ses_usage","part":{"text":"done"}}
`
	got := parseRunOutput(stdout, "json")
	if got.Usage == nil {
		t.Fatal("usage missing")
	}
	want := protocol.Usage{
		InputTokens:      10,
		OutputTokens:     4,
		ReasoningTokens:  2,
		CacheReadTokens:  3,
		CacheWriteTokens: 1,
		CostUSD:          0.25,
		Source:           protocol.UsageSourceOpenCodeV2RunStream,
	}
	if *got.Usage != want {
		t.Fatalf("usage = %+v, want %+v", *got.Usage, want)
	}
}

func TestParseJSONLStepFinishParamAlias(t *testing.T) {
	stdout := `{"type":"step_finish","sessionId":"ses_snake","part":{"type":"step-finish","tokens":{"input":1,"output":2}}}`
	got := parseRunOutput(stdout, "json")
	if got.SessionID != "ses_snake" {
		t.Fatalf("session = %q", got.SessionID)
	}
	if got.Usage == nil || got.Usage.InputTokens != 1 || got.Usage.OutputTokens != 2 {
		t.Fatalf("usage = %+v", got.Usage)
	}
}

func TestParseTextFormat(t *testing.T) {
	got := parseRunOutput("plain text output\n", "default")
	if got.Summary != "plain text output" || got.SessionID != "" || got.Usage != nil {
		t.Fatalf("parsed = %+v", got)
	}
}

func TestFailsOutcome(t *testing.T) {
	for _, outcome := range []string{"failed", " FAILED ", "interrupted", "cancelled"} {
		if !failsOutcome(outcome) {
			t.Fatalf("failsOutcome(%q) = false", outcome)
		}
	}
	for _, outcome := range []string{"", "completed", "success", "unknown"} {
		if failsOutcome(outcome) {
			t.Fatalf("failsOutcome(%q) = true", outcome)
		}
	}
}

func TestUsageFromRecordAndMerge(t *testing.T) {
	record, fallback := usageFromRecord(sessionRecord{
		Tokens: map[string]json.RawMessage{
			"input":     json.RawMessage(`7`),
			"output":    json.RawMessage(`3`),
			"reasoning": json.RawMessage(`2`),
			"cache":     json.RawMessage(`{"read":5}`),
		},
		Cost: 0.5,
	}), &protocol.Usage{
		InputTokens:     7,
		OutputTokens:    3,
		CacheReadTokens: 5,
		Source:          protocol.UsageSourceOpenCodeV2RunStream,
	}
	if record == nil || record.Source != protocol.UsageSourceOpenCodeV2Session {
		t.Fatalf("record usage = %+v", record)
	}
	merged := mergeUsageRecord(fallback, record)
	if merged.ReasoningTokens != 2 || merged.CostUSD != 0.5 {
		t.Fatalf("merged = %+v", merged)
	}
	if merged.InputTokens != 7 || merged.Source != protocol.UsageSourceOpenCodeV2RunStream {
		t.Fatalf("merged stream fields = %+v", merged)
	}
	if usageFromRecord(sessionRecord{}) != nil {
		t.Fatal("empty record should not report usage")
	}
}
