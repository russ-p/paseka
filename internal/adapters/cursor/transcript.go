package cursor

import (
	"bufio"
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"time"

	"github.com/russ-p/paseka/internal/adapters"
)

var sessionUUIDRe = regexp.MustCompile(`(?i)^[0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{12}$`)

type transcriptLine struct {
	Role      string `json:"role"`
	Timestamp string `json:"timestamp"`
	Message   *struct {
		Content []transcriptPart `json:"content"`
	} `json:"message"`
}

type transcriptPart struct {
	Type      string          `json:"type"`
	Text      string          `json:"text"`
	Name      string          `json:"name"`
	ID        string          `json:"id"`
	Input     json.RawMessage `json:"input"`
	Output    json.RawMessage `json:"output"`
	ToolUseID string          `json:"tool_use_id"`
	Content   json.RawMessage `json:"content"`
	IsError   bool            `json:"is_error"`
}

// ResolveTranscript reads Cursor agent-transcripts jsonl in place for the run's
// providerSessionId and normalizes it into an ordered conversation. Cursor
// records no reasoning parts, so a reasoning request answers with an empty set
// rather than an omission. The read is read-only and writes nothing.
func (a *Adapter) ResolveTranscript(ctx context.Context, req adapters.TranscriptRequest) (adapters.Transcript, error) {
	id := strings.TrimSpace(req.ProviderSessionID)
	if !sessionUUIDRe.MatchString(id) {
		return adapters.Transcript{Omitted: adapters.SessionLogOmittedStoreNotFound}, nil
	}
	path := findTranscriptPath(req.Workspace, id)
	if path == "" {
		return adapters.Transcript{Omitted: adapters.SessionLogOmittedStoreNotFound}, nil
	}
	transcript, err := parseTranscript(ctx, path)
	if err != nil {
		if ctx.Err() != nil {
			return adapters.Transcript{Omitted: adapters.SessionLogOmittedTimeout}, nil
		}
		return adapters.Transcript{Omitted: adapters.SessionLogOmittedParseError}, nil
	}
	return adapters.NormalizeTranscript(transcript, req.Include, req.Limits, req.Window), nil
}

func cursorDir() string {
	home, err := os.UserHomeDir()
	if err != nil || home == "" {
		return ""
	}
	return filepath.Join(home, ".cursor")
}

func projectSlug(workspace string) string {
	s := filepath.ToSlash(strings.TrimSpace(workspace))
	s = strings.ReplaceAll(s, "/", "-")
	return strings.Trim(s, "-")
}

func transcriptRel(id string) string {
	return filepath.Join("agent-transcripts", id, id+".jsonl")
}

func findTranscriptPath(workspace, id string) string {
	root := cursorDir()
	if root == "" {
		return ""
	}
	if slug := projectSlug(workspace); slug != "" && !strings.Contains(slug, "..") {
		preferred := filepath.Join(root, "projects", slug, transcriptRel(id))
		if st, err := os.Stat(preferred); err == nil && !st.IsDir() {
			return preferred
		}
	}
	matches, err := filepath.Glob(filepath.Join(root, "projects", "*", transcriptRel(id)))
	if err != nil || len(matches) == 0 {
		return ""
	}
	for _, p := range matches {
		if st, err := os.Stat(p); err == nil && !st.IsDir() {
			return p
		}
	}
	return ""
}

// parseTranscript turns the JSONL into ordered turns with typed parts. A tool
// result stored as its own record is folded into the call it answers by call id,
// so the provider's shape never reaches a consumer.
func parseTranscript(ctx context.Context, path string) (adapters.Transcript, error) {
	f, err := os.Open(path)
	if err != nil {
		return adapters.Transcript{}, err
	}
	defer f.Close()

	var turns []adapters.Turn
	var results []adapters.ToolResult
	sc := bufio.NewScanner(f)
	sc.Buffer(make([]byte, 0, 64*1024), 8*1024*1024)
	for sc.Scan() {
		if err := ctx.Err(); err != nil {
			return adapters.Transcript{}, err
		}
		line := strings.TrimSpace(sc.Text())
		if line == "" {
			continue
		}
		var raw transcriptLine
		if err := json.Unmarshal([]byte(line), &raw); err != nil {
			return adapters.Transcript{}, err
		}
		if raw.Message == nil {
			continue
		}
		role := strings.ToLower(strings.TrimSpace(raw.Role))
		switch role {
		case adapters.TurnRoleUser, adapters.TurnRoleAssistant, adapters.TurnRoleSystem:
		default:
			continue
		}
		turn := adapters.Turn{Role: role, Timestamp: parseTimestamp(raw.Timestamp)}
		for _, part := range raw.Message.Content {
			switch part.Type {
			case "text":
				if strings.TrimSpace(part.Text) == "" {
					continue
				}
				turn.Parts = append(turn.Parts, adapters.Part{Kind: adapters.PartText, Text: part.Text})
			case "tool_use":
				name := strings.TrimSpace(part.Name)
				if name == "" {
					continue
				}
				tool := &adapters.ToolPart{
					Name:   name,
					CallID: strings.TrimSpace(part.ID),
					Status: adapters.ToolStatusUnknown,
					Input:  rawToolInput(part.Input),
				}
				// A result nested on the call itself is attached here; a result
				// stored as its own record is folded in after the scan.
				if output := toolResultText(part.Output); output != "" {
					tool.Output = output
					tool.Status = toolResultStatus(part.IsError)
				}
				turn.Parts = append(turn.Parts, adapters.Part{Kind: adapters.PartTool, Tool: tool})
			case "tool_result":
				results = append(results, adapters.ToolResult{
					CallID: strings.TrimSpace(part.ToolUseID),
					Output: toolResultText(part.Content),
					Status: toolResultStatus(part.IsError),
				})
			}
		}
		if len(turn.Parts) > 0 {
			turns = append(turns, turn)
		}
	}
	if err := sc.Err(); err != nil {
		return adapters.Transcript{}, err
	}
	adapters.AttachToolResults(turns, results)
	if turns == nil {
		turns = []adapters.Turn{}
	}
	return adapters.Transcript{Turns: turns}, nil
}

func parseTimestamp(raw string) time.Time {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return time.Time{}
	}
	for _, layout := range []string{time.RFC3339Nano, time.RFC3339, "2006-01-02T15:04:05.000Z"} {
		if ts, err := time.Parse(layout, raw); err == nil {
			return ts
		}
	}
	return time.Time{}
}

func rawToolInput(raw json.RawMessage) string {
	trimmed := strings.TrimSpace(string(raw))
	if trimmed == "" || trimmed == "null" {
		return ""
	}
	return trimmed
}

func toolResultText(raw json.RawMessage) string {
	trimmed := strings.TrimSpace(string(raw))
	if trimmed == "" || trimmed == "null" {
		return ""
	}
	var text string
	if err := json.Unmarshal([]byte(trimmed), &text); err == nil {
		return text
	}
	var parts []transcriptPart
	if err := json.Unmarshal([]byte(trimmed), &parts); err == nil {
		var b strings.Builder
		for _, part := range parts {
			if part.Type != "text" || part.Text == "" {
				continue
			}
			if b.Len() > 0 {
				b.WriteString("\n")
			}
			b.WriteString(part.Text)
		}
		return b.String()
	}
	return trimmed
}

func toolResultStatus(isError bool) string {
	if isError {
		return adapters.ToolStatusError
	}
	return adapters.ToolStatusOK
}
