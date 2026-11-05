package opencodev1

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/russ-p/paseka/internal/adapters"
)

// ResolveTranscript reads the frozen OpenCode 1 storage JSON tree in place. The
// session record is found by provider session id under the project-hash
// directories; below it messages are keyed by session id and parts by message
// id, so no further project search is needed. The read is read-only and writes
// nothing.
func (a *Adapter) ResolveTranscript(ctx context.Context, req adapters.TranscriptRequest) (adapters.Transcript, error) {
	if ctx == nil {
		ctx = context.Background()
	}
	if err := ctx.Err(); err != nil {
		return adapters.Transcript{Omitted: adapters.SessionLogOmittedTimeout}, nil
	}

	id := strings.TrimSpace(req.ProviderSessionID)
	if !safeProviderID(id) {
		return adapters.Transcript{Omitted: adapters.SessionLogOmittedStoreNotFound}, nil
	}
	root := opencodeStorageRoot()
	if root == "" {
		return adapters.Transcript{Omitted: adapters.SessionLogOmittedStoreNotFound}, nil
	}

	path := findSessionRecord(root, id, req.Workspace)
	if path == "" {
		return adapters.Transcript{Omitted: adapters.SessionLogOmittedStoreNotFound}, nil
	}
	record, err := readSessionRecord(path)
	if err != nil {
		return adapters.Transcript{Omitted: readOmission(ctx, err)}, nil
	}
	if record.ID != "" && record.ID != id {
		return adapters.Transcript{Omitted: adapters.SessionLogOmittedParseError}, nil
	}

	turns, err := readSessionTurns(ctx, root, id)
	if err != nil {
		return adapters.Transcript{Omitted: readOmission(ctx, err)}, nil
	}
	return adapters.NormalizeTranscript(adapters.Transcript{Turns: turns}, req.Include, req.Limits, req.Window), nil
}

// opencodeStorageRoot is the provider's data directory. XDG_DATA_HOME is
// honoured first because that is where the provider writes on Linux.
func opencodeStorageRoot() string {
	if xdg := strings.TrimSpace(os.Getenv("XDG_DATA_HOME")); xdg != "" {
		return filepath.Join(xdg, "opencode", "storage")
	}
	home, err := os.UserHomeDir()
	if err != nil || strings.TrimSpace(home) == "" {
		return ""
	}
	return filepath.Join(home, ".local", "share", "opencode", "storage")
}

// safeProviderID keeps a provider id out of path traversal. OpenCode ids are
// plain tokens (ses_*, msg_*, prt_*); a separator, a glob character, or a dot
// segment is not a session id and must not be joined into a path.
func safeProviderID(id string) bool {
	if id == "" || id == "." || id == ".." {
		return false
	}
	if strings.ContainsAny(id, `/\*?[`) {
		return false
	}
	return filepath.Base(id) == id
}

// findSessionRecord searches only the session lookup. Message and part
// directories are keyed by the provider ids themselves, so the project hash is
// irrelevant below this point. When the same id appears in several projects the
// record whose directory matches the run's workspace wins; otherwise the
// lexicographically first candidate is chosen so resolution is deterministic.
func findSessionRecord(root, id, workspace string) string {
	sessionsDir := filepath.Join(root, "session")
	direct := filepath.Join(sessionsDir, id+".json")
	if isRegularFile(direct) {
		return direct
	}

	entries, err := os.ReadDir(sessionsDir)
	if err != nil {
		return ""
	}
	candidates := make([]string, 0, len(entries))
	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}
		candidate := filepath.Join(sessionsDir, entry.Name(), id+".json")
		if isRegularFile(candidate) {
			candidates = append(candidates, candidate)
		}
	}
	if len(candidates) == 0 {
		return ""
	}
	sort.Strings(candidates)
	if len(candidates) == 1 {
		return candidates[0]
	}

	wanted := workspaceKey(workspace)
	if wanted != "" {
		for _, candidate := range candidates {
			record, err := readSessionRecord(candidate)
			if err == nil && workspaceKey(record.Directory) == wanted {
				return candidate
			}
		}
	}
	return candidates[0]
}

func isRegularFile(path string) bool {
	info, err := os.Stat(path)
	return err == nil && !info.IsDir()
}

func workspaceKey(path string) string {
	path = strings.TrimSpace(path)
	if path == "" {
		return ""
	}
	if abs, err := filepath.Abs(path); err == nil {
		path = abs
	}
	return filepath.Clean(path)
}

// opencodeSession is the session record. Only the fields resolution needs are
// decoded; the conversation lives in the message and part trees.
type opencodeSession struct {
	ID        string `json:"id"`
	Directory string `json:"directory"`
}

type opencodeTime struct {
	Created int64 `json:"created"`
}

func readSessionRecord(path string) (opencodeSession, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return opencodeSession{}, err
	}
	var record opencodeSession
	if err := json.Unmarshal(data, &record); err != nil {
		return opencodeSession{}, err
	}
	if record.ID != "" && record.ID != strings.TrimSuffix(filepath.Base(path), ".json") {
		return opencodeSession{}, fmt.Errorf("opencode: session record id mismatch in %s", path)
	}
	return record, nil
}

type opencodeMessage struct {
	ID        string       `json:"id"`
	SessionID string       `json:"sessionID"`
	Role      string       `json:"role"`
	Time      opencodeTime `json:"time"`
}

// readSessionTurns loads every message of the session and folds each one into a
// turn. Messages are ordered by the provider's creation time, with the id as a
// stable tie-break, because the file names are an encoding detail.
func readSessionTurns(ctx context.Context, root, sessionID string) ([]adapters.Turn, error) {
	messageDir := filepath.Join(root, "message", sessionID)
	entries, err := os.ReadDir(messageDir)
	if err != nil {
		if os.IsNotExist(err) {
			return []adapters.Turn{}, nil
		}
		return nil, err
	}

	messages := make([]opencodeMessage, 0, len(entries))
	for _, entry := range entries {
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".json") {
			continue
		}
		data, err := os.ReadFile(filepath.Join(messageDir, entry.Name()))
		if err != nil {
			return nil, err
		}
		var message opencodeMessage
		if err := json.Unmarshal(data, &message); err != nil {
			return nil, err
		}
		if message.ID == "" {
			message.ID = strings.TrimSuffix(entry.Name(), ".json")
		}
		if message.SessionID != "" && message.SessionID != sessionID {
			continue
		}
		messages = append(messages, message)
	}
	sort.SliceStable(messages, func(i, j int) bool {
		if messages[i].Time.Created != messages[j].Time.Created {
			return messages[i].Time.Created < messages[j].Time.Created
		}
		return messages[i].ID < messages[j].ID
	})

	turns := make([]adapters.Turn, 0, len(messages))
	for _, message := range messages {
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		role := strings.ToLower(strings.TrimSpace(message.Role))
		switch role {
		case adapters.TurnRoleUser, adapters.TurnRoleAssistant, adapters.TurnRoleSystem:
		default:
			continue
		}
		parts, err := readMessageParts(ctx, filepath.Join(root, "part", message.ID))
		if err != nil {
			return nil, err
		}
		if len(parts) == 0 {
			continue
		}
		turn := adapters.Turn{Role: role, Parts: parts}
		if message.Time.Created > 0 {
			turn.Timestamp = time.UnixMilli(message.Time.Created).UTC()
		}
		turns = append(turns, turn)
	}
	return turns, nil
}

// opencodePart covers the payload-bearing part types. Step markers, patches,
// snapshots, and other storage records are not conversation parts and are
// ignored.
type opencodePart struct {
	Type   string        `json:"type"`
	Text   string        `json:"text"`
	CallID string        `json:"callID"`
	Tool   string        `json:"tool"`
	State  opencodeState `json:"state"`
}

type opencodeState struct {
	Status string          `json:"status"`
	Input  json.RawMessage `json:"input"`
	Output json.RawMessage `json:"output"`
	Error  string          `json:"error"`
}

// readMessageParts decodes one message's part directory in provider order. The
// file names carry the provider's part ordering and ReadDir already sorts them,
// so no second ordering heuristic is invented.
func readMessageParts(ctx context.Context, dir string) ([]adapters.Part, error) {
	entries, err := os.ReadDir(dir)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, err
	}

	parts := make([]adapters.Part, 0, len(entries))
	for _, entry := range entries {
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".json") {
			continue
		}
		data, err := os.ReadFile(filepath.Join(dir, entry.Name()))
		if err != nil {
			return nil, err
		}
		var raw opencodePart
		if err := json.Unmarshal(data, &raw); err != nil {
			return nil, err
		}
		switch strings.ToLower(strings.TrimSpace(raw.Type)) {
		case "text":
			if raw.Text == "" {
				continue
			}
			parts = append(parts, adapters.Part{Kind: adapters.PartText, Text: raw.Text})
		case "reasoning":
			if raw.Text == "" {
				continue
			}
			parts = append(parts, adapters.Part{Kind: adapters.PartReasoning, Reasoning: raw.Text})
		case "tool":
			name := strings.TrimSpace(raw.Tool)
			if name == "" {
				continue
			}
			status := opencodeToolStatus(raw.State.Status)
			output := rawTextValue(raw.State.Output)
			if output == "" && status == adapters.ToolStatusError {
				output = strings.TrimSpace(raw.State.Error)
			}
			parts = append(parts, adapters.Part{Kind: adapters.PartTool, Tool: &adapters.ToolPart{
				Name:   name,
				CallID: strings.TrimSpace(raw.CallID),
				Status: status,
				Input:  rawJSONText(raw.State.Input),
				Output: output,
			}})
		}
	}
	return parts, nil
}

func opencodeToolStatus(status string) string {
	switch strings.ToLower(strings.TrimSpace(status)) {
	case "completed":
		return adapters.ToolStatusOK
	case "error":
		return adapters.ToolStatusError
	default:
		return adapters.ToolStatusUnknown
	}
}

func rawJSONText(raw json.RawMessage) string {
	trimmed := strings.TrimSpace(string(raw))
	if trimmed == "" || trimmed == "null" {
		return ""
	}
	return trimmed
}

func rawTextValue(raw json.RawMessage) string {
	trimmed := strings.TrimSpace(string(raw))
	if trimmed == "" || trimmed == "null" {
		return ""
	}
	var text string
	if err := json.Unmarshal(raw, &text); err == nil {
		return text
	}
	return trimmed
}

// readOmission turns a read failure into the stable omit reason consumers share.
func readOmission(ctx context.Context, err error) string {
	if ctx != nil && ctx.Err() != nil {
		return adapters.SessionLogOmittedTimeout
	}
	if os.IsNotExist(err) {
		return adapters.SessionLogOmittedStoreNotFound
	}
	return adapters.SessionLogOmittedParseError
}
