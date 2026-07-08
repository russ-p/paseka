package claude

import (
	"encoding/json"
	"strings"

	"github.com/paseka/paseka/internal/protocol"
)

// claudeStreamLine models a single NDJSON line from
// `claude -p --output-format stream-json --verbose`.
//
// Relevant shapes:
//
//	{"type":"system","subtype":"init",...}
//	{"type":"assistant","message":{"content":[
//	    {"type":"text","text":"..."},
//	    {"type":"tool_use","name":"Edit","input":{...}}]}}
//	{"type":"user","message":{"content":[{"type":"tool_result",...}]}}
//	{"type":"result","subtype":"success","result":"final answer","is_error":false}
type claudeStreamLine struct {
	Type    string `json:"type"`
	Subtype string `json:"subtype"`
	Result  string `json:"result"`
	IsError bool   `json:"is_error"`
	Message *struct {
		Content []struct {
			Type  string          `json:"type"`
			Text  string          `json:"text"`
			Name  string          `json:"name"`
			Input json.RawMessage `json:"input"`
		} `json:"content"`
	} `json:"message"`
}

type streamParseOutput struct {
	Summary string
	Events  []protocol.Event
}

func parseStreamJSON(stdout, traceID, agentID string) streamParseOutput {
	var out streamParseOutput
	seq := 1
	for _, line := range strings.Split(stdout, "\n") {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		var raw claudeStreamLine
		if err := json.Unmarshal([]byte(line), &raw); err != nil {
			continue
		}
		switch raw.Type {
		case "result":
			// Prefer the final success result; ignore error summaries so the
			// normalized summary stays clean (Diagnostics already carry errors).
			if !raw.IsError && raw.Result != "" {
				out.Summary = raw.Result
			}
		case "assistant":
			if raw.Message == nil {
				continue
			}
			for _, part := range raw.Message.Content {
				switch part.Type {
				case "text":
					text := strings.TrimSpace(part.Text)
					if text == "" {
						continue
					}
					ev, err := protocol.NewEvent(traceID, agentID, seq, protocol.EventAssistantText, map[string]string{
						"text": text,
					})
					if err == nil {
						out.Events = append(out.Events, ev)
						seq++
					}
				case "tool_use":
					if part.Name == "" {
						continue
					}
					ev, err := protocol.NewEvent(traceID, agentID, seq, protocol.EventToolCall, map[string]any{
						"name": part.Name,
						"args": part.Input,
					})
					if err == nil {
						out.Events = append(out.Events, ev)
						seq++
					}
				}
			}
		}
	}
	return out
}
