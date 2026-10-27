package opencodev2

import (
	"encoding/json"
	"strings"

	"github.com/russ-p/paseka/internal/adapters"
	"github.com/russ-p/paseka/internal/protocol"
)

type parsedRun struct {
	Summary   string
	SessionID string
	Usage     *protocol.Usage
}

func parseRunOutput(stdout, format string) parsedRun {
	stdout = strings.TrimSpace(stdout)
	if stdout == "" {
		return parsedRun{}
	}
	if format == "default" {
		return parsedRun{Summary: stdout}
	}
	return parseJSONL(stdout)
}

// parseJSONL keeps the tolerant line reader: text parts build the summary, tool
// and error events are ignored for summary purposes, and the provider session id
// is read from the event envelope. A V2 single-step run may emit no step_finish,
// so a missing usage here is normal and the caller falls back to the API.
func parseJSONL(stdout string) parsedRun {
	var out parsedRun
	var texts []string
	var usage *protocol.Usage
	for _, line := range strings.Split(stdout, "\n") {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		var raw map[string]json.RawMessage
		if err := json.Unmarshal([]byte(line), &raw); err != nil {
			continue
		}
		if id := firstSessionID(raw); id != "" && out.SessionID == "" {
			out.SessionID = id
		}
		typ := jsonString(raw["type"])
		part := jsonObject(raw["part"])
		if typ == "text" || jsonString(part["type"]) == "text" {
			if text := jsonString(part["text"]); text != "" {
				texts = append(texts, text)
			} else if text := jsonString(raw["text"]); text != "" {
				texts = append(texts, text)
			}
		}
		if typ == "step_finish" || jsonString(part["type"]) == "step-finish" {
			u := usageFromPart(part)
			if u != nil {
				usage = adapters.AddUsage(usage, u)
			}
		}
	}
	out.Summary = strings.TrimSpace(strings.Join(texts, "\n"))
	out.Usage = usage
	return out
}

func firstSessionID(raw map[string]json.RawMessage) string {
	if id := jsonString(raw["sessionID"]); id != "" {
		return id
	}
	return jsonString(raw["sessionId"])
}

func usageFromPart(part map[string]json.RawMessage) *protocol.Usage {
	u := usageFromTokens(jsonObject(part["tokens"]), protocol.UsageSourceOpenCodeV2RunStream)
	if u == nil {
		return nil
	}
	u.CostUSD = jsonFloat(part["cost"])
	return u
}

// usageFromTokens maps OpenCode's token object {input, output, reasoning,
// cache:{read, write}} onto protocol.Usage. An all-zero total is dropped so a
// step that spent nothing never becomes a usage record.
func usageFromTokens(tokens map[string]json.RawMessage, source string) *protocol.Usage {
	if len(tokens) == 0 {
		return nil
	}
	cache := jsonObject(tokens["cache"])
	u := &protocol.Usage{
		InputTokens:      jsonInt(tokens["input"]),
		OutputTokens:     jsonInt(tokens["output"]),
		ReasoningTokens:  jsonInt(tokens["reasoning"]),
		CacheReadTokens:  jsonInt(cache["read"]),
		CacheWriteTokens: jsonInt(cache["write"]),
		Source:           source,
	}
	if u.InputTokens == 0 && u.OutputTokens == 0 && u.ReasoningTokens == 0 &&
		u.CacheReadTokens == 0 && u.CacheWriteTokens == 0 {
		return nil
	}
	return u
}

// mergeUsageRecord fills what a run-stream usage record did not carry. Token
// totals stay from the stream when it reported them; reasoning and cost are
// commonly only in the session record.
func mergeUsageRecord(stream, record *protocol.Usage) *protocol.Usage {
	if record == nil {
		return stream
	}
	if stream == nil {
		return record
	}
	if stream.ReasoningTokens == 0 {
		stream.ReasoningTokens = record.ReasoningTokens
	}
	if stream.CostUSD == 0 {
		stream.CostUSD = record.CostUSD
	}
	return stream
}

func jsonObject(raw json.RawMessage) map[string]json.RawMessage {
	if len(raw) == 0 {
		return nil
	}
	var obj map[string]json.RawMessage
	if err := json.Unmarshal(raw, &obj); err != nil {
		return nil
	}
	return obj
}

func jsonString(raw json.RawMessage) string {
	if len(raw) == 0 {
		return ""
	}
	var s string
	if err := json.Unmarshal(raw, &s); err == nil {
		return strings.TrimSpace(s)
	}
	return ""
}

func jsonInt(raw json.RawMessage) int64 {
	if len(raw) == 0 {
		return 0
	}
	var n int64
	if err := json.Unmarshal(raw, &n); err == nil {
		return n
	}
	var f float64
	if err := json.Unmarshal(raw, &f); err == nil {
		return int64(f)
	}
	return 0
}

func jsonFloat(raw json.RawMessage) float64 {
	if len(raw) == 0 {
		return 0
	}
	var f float64
	if err := json.Unmarshal(raw, &f); err == nil {
		return f
	}
	return 0
}
