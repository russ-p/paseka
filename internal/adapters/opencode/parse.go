package opencode

import (
	"encoding/json"
	"strings"

	"github.com/paseka/paseka/internal/protocol"
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
			if u := usageFromPart(part); u != nil {
				usage = addUsage(usage, u)
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

func addUsage(sum, next *protocol.Usage) *protocol.Usage {
	if next == nil {
		return sum
	}
	if sum == nil {
		cp := *next
		return &cp
	}
	sum.InputTokens += next.InputTokens
	sum.OutputTokens += next.OutputTokens
	sum.CacheReadTokens += next.CacheReadTokens
	sum.CacheWriteTokens += next.CacheWriteTokens
	if sum.Source == "" {
		sum.Source = next.Source
	}
	return sum
}

func usageFromPart(part map[string]json.RawMessage) *protocol.Usage {
	tokens := jsonObject(part["tokens"])
	if len(tokens) == 0 {
		return nil
	}
	cache := jsonObject(tokens["cache"])
	u := &protocol.Usage{
		InputTokens:      jsonInt(tokens["input"]),
		OutputTokens:     jsonInt(tokens["output"]),
		CacheReadTokens:  jsonInt(cache["read"]),
		CacheWriteTokens: jsonInt(cache["write"]),
		Source:           protocol.UsageSourceOpenCodeRunJSON,
	}
	if u.InputTokens == 0 && u.OutputTokens == 0 && u.CacheReadTokens == 0 && u.CacheWriteTokens == 0 {
		return nil
	}
	return u
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
