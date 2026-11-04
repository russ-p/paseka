package pi

import (
	"bufio"
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/russ-p/paseka/internal/adapters"
)

// piEntry is one line of a Pi session JSONL. Only the conversation records are
// decoded; every other entry type (model_change, usage, compaction, label, ...)
// is ignored because it is not part of the conversation.
type piEntry struct {
	Type      string          `json:"type"`
	Timestamp json.RawMessage `json:"timestamp"`
	Message   *piMessage      `json:"message"`
}

type piMessage struct {
	Role       string          `json:"role"`
	Content    json.RawMessage `json:"content"`
	ToolCallID string          `json:"toolCallId"`
	ToolName   string          `json:"toolName"`
	IsError    bool            `json:"isError"`
	Timestamp  int64           `json:"timestamp"`
}

// piContentPart covers the three conversation parts Pi records. A thinking part
// carries its text under "thinking"; a tool call carries its arguments as a JSON
// object.
type piContentPart struct {
	Type      string          `json:"type"`
	Text      string          `json:"text"`
	Thinking  string          `json:"thinking"`
	ID        string          `json:"id"`
	Name      string          `json:"name"`
	Arguments json.RawMessage `json:"arguments"`
}

// ResolveTranscript reads the run-local Pi session JSONL. Pi stores sessions in
// <runDir>/pi-sessions, so resolution never leaves the run's own directory and
// never consults a global store. The file whose session header id matches the
// run's providerSessionId wins; otherwise the newest file in the directory is
// read alone, so several session files can never mix turns. The read is
// read-only and writes nothing.
func (a *Adapter) ResolveTranscript(ctx context.Context, req adapters.TranscriptRequest) (adapters.Transcript, error) {
	if ctx == nil {
		ctx = context.Background()
	}
	if err := ctx.Err(); err != nil {
		return adapters.Transcript{Omitted: adapters.SessionLogOmittedTimeout}, nil
	}

	dir := strings.TrimSpace(req.RunDir)
	if dir == "" {
		return adapters.Transcript{Omitted: adapters.SessionLogOmittedStoreNotFound}, nil
	}
	path := findPiSessionFile(filepath.Join(dir, piSessionsDir), req.ProviderSessionID)
	if path == "" {
		return adapters.Transcript{Omitted: adapters.SessionLogOmittedStoreNotFound}, nil
	}

	transcript, err := parsePiSession(ctx, path)
	if err != nil {
		if ctx.Err() != nil {
			return adapters.Transcript{Omitted: adapters.SessionLogOmittedTimeout}, nil
		}
		return adapters.Transcript{Omitted: adapters.SessionLogOmittedParseError}, nil
	}
	return adapters.NormalizeTranscript(transcript, req.Include, req.Limits, req.Window), nil
}

// findPiSessionFile lists only the run-local directory: no glob and no
// cross-run lookup. A header match is authoritative even when another file is
// newer; without a match the newest .jsonl file is used.
func findPiSessionFile(dir, providerSessionID string) string {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return ""
	}
	wanted := strings.TrimSpace(providerSessionID)
	newest := ""
	var newestMod time.Time
	for _, entry := range entries {
		if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".jsonl") {
			continue
		}
		path := filepath.Join(dir, entry.Name())
		if wanted != "" && sessionHeaderID(path) == wanted {
			return path
		}
		info, err := entry.Info()
		if err != nil {
			continue
		}
		if newest == "" || info.ModTime().After(newestMod) ||
			(info.ModTime().Equal(newestMod) && entry.Name() > filepath.Base(newest)) {
			newest = path
			newestMod = info.ModTime()
		}
	}
	return newest
}

// sessionHeaderID reads only the first record of a session file. Pi always
// writes the session header first, so matching a candidate never scans its
// whole conversation.
func sessionHeaderID(path string) string {
	f, err := os.Open(path)
	if err != nil {
		return ""
	}
	defer f.Close()

	sc := bufio.NewScanner(f)
	sc.Buffer(make([]byte, 0, 64*1024), 8*1024*1024)
	for sc.Scan() {
		line := strings.TrimSpace(sc.Text())
		if line == "" {
			continue
		}
		var header struct {
			Type string `json:"type"`
			ID   string `json:"id"`
		}
		if err := json.Unmarshal([]byte(line), &header); err != nil || header.Type != "session" {
			return ""
		}
		return strings.TrimSpace(header.ID)
	}
	return ""
}

// parsePiSession folds Pi's message records into ordered turns. Tool results are
// kept out of the turn list and attached to the call they answer, so consumers
// see the normalized shape regardless of how Pi stored the result.
func parsePiSession(ctx context.Context, path string) (adapters.Transcript, error) {
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
		var entry piEntry
		if err := json.Unmarshal([]byte(line), &entry); err != nil {
			return adapters.Transcript{}, err
		}
		if entry.Type != "message" || entry.Message == nil {
			continue
		}
		turn, result, ok := piConversation(entry)
		if result != nil {
			results = append(results, *result)
		}
		if ok {
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

// piConversation maps one Pi message to a turn or a pending tool result. The
// bool reports whether the message produced a turn of its own.
func piConversation(entry piEntry) (adapters.Turn, *adapters.ToolResult, bool) {
	message := entry.Message
	role := strings.ToLower(strings.TrimSpace(message.Role))
	timestamp := piTimestamp(entry.Timestamp, message.Timestamp)

	switch role {
	case adapters.TurnRoleSystem, adapters.TurnRoleUser:
		turn := adapters.Turn{Role: role, Timestamp: timestamp}
		for _, part := range decodePiContent(message.Content) {
			if part.Type != "text" || part.Text == "" {
				continue
			}
			turn.Parts = append(turn.Parts, adapters.Part{Kind: adapters.PartText, Text: part.Text})
		}
		return turn, nil, len(turn.Parts) > 0

	case adapters.TurnRoleAssistant:
		turn := adapters.Turn{Role: role, Timestamp: timestamp}
		for _, part := range decodePiContent(message.Content) {
			switch part.Type {
			case "text":
				if part.Text == "" {
					continue
				}
				turn.Parts = append(turn.Parts, adapters.Part{Kind: adapters.PartText, Text: part.Text})
			case "thinking":
				if part.Thinking == "" {
					continue
				}
				turn.Parts = append(turn.Parts, adapters.Part{Kind: adapters.PartReasoning, Reasoning: part.Thinking})
			case "toolCall":
				name := strings.TrimSpace(part.Name)
				if name == "" {
					continue
				}
				turn.Parts = append(turn.Parts, adapters.Part{
					Kind: adapters.PartTool,
					Tool: &adapters.ToolPart{
						Name:   name,
						CallID: strings.TrimSpace(part.ID),
						Status: adapters.ToolStatusUnknown,
						Input:  piToolInput(part.Arguments),
					},
				})
			}
		}
		return turn, nil, len(turn.Parts) > 0

	case "toolresult":
		var output strings.Builder
		for _, part := range decodePiContent(message.Content) {
			if part.Type != "text" || part.Text == "" {
				continue
			}
			if output.Len() > 0 {
				output.WriteString("\n")
			}
			output.WriteString(part.Text)
		}
		status := adapters.ToolStatusOK
		if message.IsError {
			status = adapters.ToolStatusError
		}
		return adapters.Turn{}, &adapters.ToolResult{
			CallID: strings.TrimSpace(message.ToolCallID),
			Output: output.String(),
			Status: status,
		}, false
	}

	return adapters.Turn{}, nil, false
}

// decodePiContent accepts Pi's two content shapes: a plain string (user,
// system) and an array of typed parts (assistant, toolResult). A string becomes
// one text part; images and unknown part kinds are dropped.
func decodePiContent(raw json.RawMessage) []piContentPart {
	trimmed := strings.TrimSpace(string(raw))
	if trimmed == "" || trimmed == "null" {
		return nil
	}
	if strings.HasPrefix(trimmed, `"`) {
		var text string
		if err := json.Unmarshal([]byte(trimmed), &text); err != nil || text == "" {
			return nil
		}
		return []piContentPart{{Type: "text", Text: text}}
	}
	var parts []piContentPart
	if err := json.Unmarshal([]byte(trimmed), &parts); err != nil {
		return nil
	}
	return parts
}

func piToolInput(raw json.RawMessage) string {
	trimmed := strings.TrimSpace(string(raw))
	if trimmed == "" || trimmed == "null" {
		return ""
	}
	return trimmed
}

// piTimestamp prefers the session entry's ISO timestamp and falls back to the
// message's Unix millisecond timestamp. A record with neither stays zero rather
// than inventing a time.
func piTimestamp(entryTimestamp json.RawMessage, messageTimestamp int64) time.Time {
	if ts := parsePiTimestamp(entryTimestamp); !ts.IsZero() {
		return ts
	}
	if messageTimestamp > 0 {
		return time.UnixMilli(messageTimestamp).UTC()
	}
	return time.Time{}
}

func parsePiTimestamp(raw json.RawMessage) time.Time {
	trimmed := strings.TrimSpace(string(raw))
	if trimmed == "" || trimmed == "null" {
		return time.Time{}
	}
	var text string
	if err := json.Unmarshal(raw, &text); err == nil {
		for _, layout := range []string{time.RFC3339Nano, time.RFC3339, "2006-01-02T15:04:05.000Z"} {
			if ts, err := time.Parse(layout, strings.TrimSpace(text)); err == nil {
				return ts
			}
		}
		return time.Time{}
	}
	var millis int64
	if err := json.Unmarshal(raw, &millis); err == nil && millis > 0 {
		return time.UnixMilli(millis).UTC()
	}
	return time.Time{}
}
