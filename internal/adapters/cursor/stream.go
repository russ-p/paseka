package cursor

import (
	"encoding/json"
	"strings"

	"github.com/paseka/paseka/internal/protocol"
)

type cliStreamLine struct {
	Type        string `json:"type"`
	Subtype     string `json:"subtype"`
	Result      string `json:"result"`
	TimestampMS *int64 `json:"timestamp_ms"`
	ModelCallID string `json:"model_call_id"`
	Message     *struct {
		Content []struct {
			Text string `json:"text"`
		} `json:"content"`
	} `json:"message"`
	ToolCall *struct {
		ReadToolCall *struct {
			Args struct {
				Path string `json:"path"`
			}
		} `json:"readToolCall"`
		WriteToolCall *struct {
			Args struct {
				Path string `json:"path"`
			}
		} `json:"writeToolCall"`
		Function *struct {
			Name string          `json:"name"`
			Args json.RawMessage `json:"arguments"`
		} `json:"function"`
	} `json:"toolCall"`
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
		var raw cliStreamLine
		if err := json.Unmarshal([]byte(line), &raw); err != nil {
			continue
		}
		switch raw.Type {
		case "result":
			if raw.Subtype == "success" && raw.Result != "" {
				out.Summary = raw.Result
			}
		case "assistant":
			text := assistantText(raw)
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
			for _, bus := range extractBusEvents(text, traceID, agentID, &seq) {
				out.Events = append(out.Events, bus)
			}
		case "tool_call":
			if raw.Subtype != "started" {
				continue
			}
			payload := toolCallPayload(raw)
			if payload == nil {
				continue
			}
			ev, err := protocol.NewEvent(traceID, agentID, seq, protocol.EventToolCall, payload)
			if err == nil {
				out.Events = append(out.Events, ev)
				seq++
			}
		}
	}
	return out
}

func assistantText(line cliStreamLine) string {
	if line.TimestampMS == nil || line.ModelCallID != "" {
		return ""
	}
	if line.Message == nil {
		return ""
	}
	var b strings.Builder
	for _, part := range line.Message.Content {
		b.WriteString(part.Text)
	}
	return b.String()
}

func toolCallPayload(line cliStreamLine) map[string]any {
	if line.ToolCall == nil {
		return nil
	}
	if line.ToolCall.ReadToolCall != nil {
		return map[string]any{
			"name": "Read",
			"path": line.ToolCall.ReadToolCall.Args.Path,
		}
	}
	if line.ToolCall.WriteToolCall != nil {
		return map[string]any{
			"name": "Write",
			"path": line.ToolCall.WriteToolCall.Args.Path,
		}
	}
	if line.ToolCall.Function != nil {
		return map[string]any{
			"name": line.ToolCall.Function.Name,
			"args": line.ToolCall.Function.Args,
		}
	}
	return nil
}

func extractBusEvents(text, traceID, agentID string, seq *int) []protocol.Event {
	var events []protocol.Event
	for _, line := range strings.Split(text, "\n") {
		line = strings.TrimSpace(line)
		if line == "" || line[0] != '{' {
			continue
		}
		var bus protocol.BusEvent
		if err := json.Unmarshal([]byte(line), &bus); err != nil {
			continue
		}
		if bus.Type == "" || !isBusEventType(bus.Type) {
			continue
		}
		if bus.TraceID != "" && bus.TraceID != traceID {
			continue
		}
		ev := protocol.Event{
			ProtocolVersion: protocol.Version,
			TraceID:         traceID,
			AgentID:         agentID,
			Seq:             *seq,
			Type:            bus.Type,
			Payload:         bus.Payload,
		}
		events = append(events, ev)
		*seq++
	}
	return events
}

func isBusEventType(t protocol.EventType) bool {
	switch t {
	case protocol.EventSignal, protocol.EventInsight, protocol.EventMutation, protocol.EventVerification:
		return true
	default:
		return false
	}
}
