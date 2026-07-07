package pi

import (
	"encoding/json"
	"strings"
)

func piMode(format string) string {
	switch format {
	case "text", "json", "rpc":
		return format
	default:
		return "json"
	}
}

func extractSummary(stdout, mode string) string {
	stdout = strings.TrimSpace(stdout)
	if stdout == "" {
		return ""
	}
	if mode == "text" {
		return stdout
	}
	return parseJSONSummary(stdout)
}

func parseJSONSummary(stdout string) string {
	stdout = strings.TrimSpace(stdout)
	if stdout == "" {
		return ""
	}
	if summary := summaryFromJSON(stdout); summary != "" {
		return summary
	}
	for _, line := range strings.Split(stdout, "\n") {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		if summary := summaryFromJSON(line); summary != "" {
			return summary
		}
	}
	return ""
}

func summaryFromJSON(data string) string {
	var raw map[string]json.RawMessage
	if err := json.Unmarshal([]byte(data), &raw); err != nil {
		return ""
	}
	for _, key := range []string{"summary", "output", "text", "content", "message", "result"} {
		value, ok := raw[key]
		if !ok {
			continue
		}
		if summary := stringFromJSONValue(value); summary != "" {
			return summary
		}
	}
	if nested, ok := raw["response"]; ok {
		if summary := summaryFromJSON(string(nested)); summary != "" {
			return summary
		}
	}
	if nested, ok := raw["data"]; ok {
		if summary := summaryFromJSON(string(nested)); summary != "" {
			return summary
		}
	}
	return ""
}

func stringFromJSONValue(raw json.RawMessage) string {
	var s string
	if err := json.Unmarshal(raw, &s); err == nil {
		return strings.TrimSpace(s)
	}
	var nested map[string]json.RawMessage
	if err := json.Unmarshal(raw, &nested); err != nil {
		return ""
	}
	for _, key := range []string{"summary", "output", "text", "content", "message", "result"} {
		value, ok := nested[key]
		if !ok {
			continue
		}
		if summary := stringFromJSONValue(value); summary != "" {
			return summary
		}
	}
	return ""
}
