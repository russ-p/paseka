package pi

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"

	"github.com/russ-p/paseka/internal/adapters"
	"github.com/russ-p/paseka/internal/protocol"
)

// usageFromStdout sums LLM tokens from Pi's `--mode json` event stream. Each
// assistant turn ends with exactly one `message_end` event that carries the
// authoritative message usage, while the preceding `message_update` deltas
// repeat the same running totals, so only `message_end` lines are summed. Tool
// results end the same way but carry tool-level usage, so the assistant role is
// required. Non-event lines (text mode prose, RPC frames) are ignored, which
// leaves the caller free to fall back to the session file.
func usageFromStdout(stdout string) *protocol.Usage {
	var sum *protocol.Usage
	for _, line := range strings.Split(stdout, "\n") {
		event := jsonObject([]byte(strings.TrimSpace(line)))
		if jsonText(event["type"]) != "message_end" {
			continue
		}
		message := jsonObject(event["message"])
		if jsonText(message["role"]) != "assistant" {
			continue
		}
		sum = adapters.AddUsage(sum, usageFromFields(jsonObject(message["usage"]), protocol.UsageSourcePiPrintJSON))
	}
	return sum
}

// usageFromSessionDir sums the session file(s) Pi appends under --session-dir.
// That directory is run-scoped, so its totals are this run's totals, and it is
// the only token source in text and rpc mode where stdout carries no usage.
// Besides assistant turns it counts standalone spend entries (cache warming)
// and compaction summaries, which are separate LLM calls whose assistant
// message is never persisted as a message entry.
func usageFromSessionDir(dir string) *protocol.Usage {
	if strings.TrimSpace(dir) == "" {
		return nil
	}
	entries, err := os.ReadDir(dir)
	if err != nil {
		return nil
	}
	var sum *protocol.Usage
	for _, entry := range entries {
		if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".jsonl") {
			continue
		}
		data, err := os.ReadFile(filepath.Join(dir, entry.Name()))
		if err != nil {
			continue
		}
		sum = adapters.AddUsage(sum, usageFromSessionJSONL(string(data)))
	}
	return sum
}

func usageFromSessionJSONL(data string) *protocol.Usage {
	var sum *protocol.Usage
	for _, line := range strings.Split(data, "\n") {
		entry := jsonObject([]byte(strings.TrimSpace(line)))
		switch jsonText(entry["type"]) {
		case "message":
			message := jsonObject(entry["message"])
			if jsonText(message["role"]) == "assistant" {
				sum = adapters.AddUsage(sum, usageFromFields(jsonObject(message["usage"]), protocol.UsageSourcePiSessionJSONL))
			}
		case "usage", "compaction", "branch_summary":
			sum = adapters.AddUsage(sum, usageFromFields(jsonObject(entry["usage"]), protocol.UsageSourcePiSessionJSONL))
		}
	}
	return sum
}

// usageFromFields maps Pi's usage object {input, output, cacheRead, cacheWrite}
// onto protocol.Usage. An all-zero usage is dropped so a placeholder event never
// turns into a zero-valued usage record.
func usageFromFields(fields map[string]json.RawMessage, source string) *protocol.Usage {
	if len(fields) == 0 {
		return nil
	}
	usage := &protocol.Usage{
		InputTokens:      jsonNumber(fields["input"]),
		OutputTokens:     jsonNumber(fields["output"]),
		CacheReadTokens:  jsonNumber(fields["cacheRead"]),
		CacheWriteTokens: jsonNumber(fields["cacheWrite"]),
		Source:           source,
	}
	if usage.InputTokens == 0 && usage.OutputTokens == 0 && usage.CacheReadTokens == 0 && usage.CacheWriteTokens == 0 {
		return nil
	}
	return usage
}

func jsonObject(data []byte) map[string]json.RawMessage {
	if len(data) == 0 || data[0] != '{' {
		return nil
	}
	var obj map[string]json.RawMessage
	if err := json.Unmarshal(data, &obj); err != nil {
		return nil
	}
	return obj
}

func jsonText(raw json.RawMessage) string {
	if len(raw) == 0 {
		return ""
	}
	var s string
	if err := json.Unmarshal(raw, &s); err != nil {
		return ""
	}
	return strings.TrimSpace(s)
}

func jsonNumber(raw json.RawMessage) int64 {
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
