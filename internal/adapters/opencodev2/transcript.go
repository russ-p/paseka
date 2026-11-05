package opencodev2

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"os"
	"os/exec"
	"strings"
	"sync"
	"time"

	"github.com/russ-p/paseka/internal/adapters"
)

// exportWaitDelay bounds how long a killed export command may keep its pipes
// open. A wedged child is killed by the context; a grandchild holding stdout
// must not outlive it.
const exportWaitDelay = 2 * time.Second

// ownedSessionClients is the registry of servers Paseka currently owns for live
// HITL sessions. A finished session has no entry, which is why reading it never
// starts a server: the provider's own export subcommand is used instead.
var ownedSessionClients = struct {
	sync.RWMutex
	bySession map[string]*apiClient
}{bySession: make(map[string]*apiClient)}

func registerOwnedSession(providerSessionID string, client *apiClient) {
	id := strings.TrimSpace(providerSessionID)
	if id == "" || client == nil {
		return
	}
	ownedSessionClients.Lock()
	ownedSessionClients.bySession[id] = client
	ownedSessionClients.Unlock()
}

func unregisterOwnedSession(providerSessionID string) {
	id := strings.TrimSpace(providerSessionID)
	if id == "" {
		return
	}
	ownedSessionClients.Lock()
	delete(ownedSessionClients.bySession, id)
	ownedSessionClients.Unlock()
}

func ownedSessionClient(providerSessionID string) (*apiClient, bool) {
	ownedSessionClients.RLock()
	client, ok := ownedSessionClients.bySession[strings.TrimSpace(providerSessionID)]
	ownedSessionClients.RUnlock()
	return client, ok
}

// ResolveTranscript reads a finished OpenCode 2 session without touching the
// SQLite store. When Paseka already owns a server for the session (a live HITL
// session), the provider's HTTP API answers; otherwise the provider's own JSON
// export subcommand does. Both routes are read-only, and neither starts a
// server just to read a finished session.
func (a *Adapter) ResolveTranscript(ctx context.Context, req adapters.TranscriptRequest) (adapters.Transcript, error) {
	if ctx == nil {
		ctx = context.Background()
	}
	if err := ctx.Err(); err != nil {
		return adapters.Transcript{Omitted: adapters.SessionLogOmittedTimeout}, nil
	}

	id := strings.TrimSpace(req.ProviderSessionID)
	if !safeSessionID(id) {
		return adapters.Transcript{Omitted: adapters.SessionLogOmittedStoreNotFound}, nil
	}

	if client, ok := ownedSessionClient(id); ok {
		return resolveFromServer(ctx, client, id, req), nil
	}
	return resolveFromExport(ctx, id, req), nil
}

// resolveFromServer reads the projected session through the HTTP API of a
// server Paseka already owns. It never falls back to starting a server.
func resolveFromServer(ctx context.Context, client *apiClient, id string, req adapters.TranscriptRequest) adapters.Transcript {
	raw, err := client.ExportSession(ctx, id)
	if err != nil {
		return adapters.Transcript{Omitted: exportOmission(ctx, err)}
	}
	turns, err := decodeExport(raw)
	if err != nil {
		return adapters.Transcript{Omitted: adapters.SessionLogOmittedParseError}
	}
	return normalizeWholeExport(turns, req)
}

// resolveFromExport asks the provider for its own JSON export. The command
// connects to the provider's existing service when it has one; it deliberately
// never passes --standalone, which would start a private server for a read.
func resolveFromExport(ctx context.Context, id string, req adapters.TranscriptRequest) adapters.Transcript {
	path, err := exec.LookPath(defaultBinary)
	if err != nil {
		return adapters.Transcript{Omitted: adapters.SessionLogOmittedStoreNotFound}
	}
	raw, err := runSessionExport(ctx, path, exportWorkspace(req), id)
	if err != nil {
		return adapters.Transcript{Omitted: exportOmission(ctx, err)}
	}
	turns, err := decodeExport(raw)
	if err != nil {
		return adapters.Transcript{Omitted: adapters.SessionLogOmittedParseError}
	}
	return normalizeWholeExport(turns, req)
}

// normalizeWholeExport answers a page request with the whole transcript. Both
// 2.x routes hand over every message in one document, so this reader cannot
// window cheaply; it says so by returning no further cursor instead of slicing
// or answering the window with an omission. Paging degrades rather than fails,
// and the export output stays identical to a full read.
func normalizeWholeExport(turns []adapters.Turn, req adapters.TranscriptRequest) adapters.Transcript {
	return adapters.NormalizeTranscript(adapters.Transcript{Turns: turns}, req.Include, req.Limits, adapters.WholeTranscriptWindow())
}

func exportStderr(output string) string {
	output = strings.TrimSpace(output)
	if output == "" {
		return ""
	}
	return " (" + output + ")"
}

func exportWorkspace(req adapters.TranscriptRequest) string {
	if workspace := strings.TrimSpace(req.Workspace); workspace != "" {
		return workspace
	}
	return strings.TrimSpace(req.ColonyRoot)
}

func runSessionExport(ctx context.Context, binary, workspace, id string) ([]byte, error) {
	cmd := exec.CommandContext(ctx, binary, "session", "export", id)
	// A recorded workspace that no longer exists (or is not a directory) must not
	// turn a readable session into a read failure; the export command resolves
	// its location from the session id.
	if workspace != "" {
		if info, err := os.Stat(workspace); err == nil && info.IsDir() {
			cmd.Dir = workspace
		}
	}
	cmd.WaitDelay = exportWaitDelay
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	err := cmd.Run()
	if ctx.Err() != nil {
		return nil, ctx.Err()
	}
	if err != nil {
		return nil, fmt.Errorf("opencodev2: session export: %w%s", err, exportStderr(stderr.String()))
	}
	return stdout.Bytes(), nil
}

// exportOmission maps a failed read to the stable omission set. A provider that
// has no session to export reads as "store not found"; a wedged call is a
// timeout, never a hang.
func exportOmission(ctx context.Context, err error) string {
	if ctx != nil && ctx.Err() != nil {
		return adapters.SessionLogOmittedTimeout
	}
	var apiErr *apiError
	if errors.As(err, &apiErr) {
		if apiErr.Status == 404 {
			return adapters.SessionLogOmittedStoreNotFound
		}
		return adapters.SessionLogOmittedResolveError
	}
	var netErr net.Error
	if errors.As(err, &netErr) {
		if netErr.Timeout() {
			return adapters.SessionLogOmittedTimeout
		}
		return adapters.SessionLogOmittedResolveError
	}
	var exitErr *exec.ExitError
	if errors.As(err, &exitErr) {
		return adapters.SessionLogOmittedStoreNotFound
	}
	if errors.Is(err, exec.ErrNotFound) || errors.Is(err, os.ErrNotExist) {
		return adapters.SessionLogOmittedStoreNotFound
	}
	return adapters.SessionLogOmittedResolveError
}

// safeSessionID keeps a provider id from being read as a command flag or a path
// segment. OpenCode ids are plain tokens (ses_*).
func safeSessionID(id string) bool {
	if id == "" || id == "." || id == ".." || strings.HasPrefix(id, "-") {
		return false
	}
	if strings.ContainsAny(id, `/\*?[`) {
		return false
	}
	return strings.TrimSpace(id) == id
}

// exportDocument is the provider's `{info, messages}` export. The HTTP client
// already unwraps its `{data: ...}` envelope; decodeExportDocument also accepts
// a wrapped body so a direct CLI/API shape difference is not a parse failure.
type exportDocument struct {
	Info     json.RawMessage   `json:"info"`
	Messages []json.RawMessage `json:"messages"`
}

type exportEnvelope struct {
	Data json.RawMessage `json:"data"`
}

// exportMessage accepts both export generations: 1.x wraps a message in
// `{info, parts}`, while 2.x puts the message fields and `content` parts at the
// top level. Unknown fields and non-conversation message types are ignored.
type exportMessage struct {
	Info    *exportMessageInfo `json:"info"`
	Parts   []exportPart       `json:"parts"`
	ID      string             `json:"id"`
	Role    string             `json:"role"`
	Type    string             `json:"type"`
	Text    string             `json:"text"`
	Content []exportPart       `json:"content"`
	Time    exportTime         `json:"time"`
}

type exportMessageInfo struct {
	ID   string     `json:"id"`
	Role string     `json:"role"`
	Type string     `json:"type"`
	Time exportTime `json:"time"`
}

type exportTime struct {
	Created json.RawMessage `json:"created"`
}

// exportPart covers the payload-bearing part types. Step markers, patches,
// snapshots, and other records are not conversation parts and are ignored.
type exportPart struct {
	Type      string          `json:"type"`
	Text      string          `json:"text"`
	Reasoning string          `json:"reasoning"`
	CallID    string          `json:"callID"`
	ID        string          `json:"id"`
	Tool      string          `json:"tool"`
	Name      string          `json:"name"`
	State     exportToolState `json:"state"`
}

type exportToolState struct {
	Status  string          `json:"status"`
	Input   json.RawMessage `json:"input"`
	Output  json.RawMessage `json:"output"`
	Content json.RawMessage `json:"content"`
	Error   json.RawMessage `json:"error"`
}

// decodeExport turns the provider's export into provider-ordered turns with
// typed parts. A malformed message is skipped rather than failing the whole
// read; a malformed document is a parse error.
func decodeExport(raw []byte) ([]adapters.Turn, error) {
	doc, err := decodeExportDocument(raw)
	if err != nil {
		return nil, err
	}
	turns := make([]adapters.Turn, 0, len(doc.Messages))
	for _, rawMessage := range doc.Messages {
		turn, ok := decodeExportMessage(rawMessage)
		if !ok {
			continue
		}
		turns = append(turns, turn)
	}
	return turns, nil
}

func decodeExportDocument(raw []byte) (exportDocument, error) {
	trimmed := bytes.TrimSpace(raw)
	if len(trimmed) == 0 {
		return exportDocument{}, errors.New("opencodev2: empty session export")
	}
	var doc exportDocument
	if err := json.Unmarshal(trimmed, &doc); err != nil {
		return exportDocument{}, err
	}
	if len(doc.Messages) > 0 {
		return doc, nil
	}
	var envelope exportEnvelope
	if err := json.Unmarshal(trimmed, &envelope); err != nil || len(envelope.Data) == 0 {
		return doc, nil
	}
	var inner exportDocument
	if err := json.Unmarshal(envelope.Data, &inner); err != nil {
		return exportDocument{}, err
	}
	return inner, nil
}

func decodeExportMessage(raw json.RawMessage) (adapters.Turn, bool) {
	var message exportMessage
	if err := json.Unmarshal(raw, &message); err != nil {
		return adapters.Turn{}, false
	}
	role := exportMessageRole(message)
	switch role {
	case adapters.TurnRoleUser, adapters.TurnRoleAssistant, adapters.TurnRoleSystem:
	default:
		return adapters.Turn{}, false
	}

	turn := adapters.Turn{Role: role, Timestamp: exportMessageTimestamp(message)}
	for _, part := range exportMessageParts(message) {
		switch strings.ToLower(strings.TrimSpace(part.Type)) {
		case "text":
			if part.Text == "" {
				continue
			}
			turn.Parts = append(turn.Parts, adapters.Part{Kind: adapters.PartText, Text: part.Text})
		case "reasoning":
			text := part.Text
			if text == "" {
				text = part.Reasoning
			}
			if text == "" {
				continue
			}
			turn.Parts = append(turn.Parts, adapters.Part{Kind: adapters.PartReasoning, Reasoning: text})
		case "tool":
			name := firstNonEmpty(part.Name, part.Tool)
			if name == "" {
				continue
			}
			turn.Parts = append(turn.Parts, adapters.Part{Kind: adapters.PartTool, Tool: exportToolPart(part, name)})
		}
	}
	if len(turn.Parts) == 0 {
		return adapters.Turn{}, false
	}
	return turn, true
}

func exportMessageRole(message exportMessage) string {
	roles := []string{message.Role, message.Type}
	if message.Info != nil {
		roles = []string{message.Info.Role, message.Info.Type, message.Role, message.Type}
	}
	for _, role := range roles {
		if role = strings.ToLower(strings.TrimSpace(role)); role != "" {
			return role
		}
	}
	return ""
}

func exportMessageTimestamp(message exportMessage) time.Time {
	if message.Info != nil {
		if ts := exportTimestamp(message.Info.Time.Created); !ts.IsZero() {
			return ts
		}
	}
	return exportTimestamp(message.Time.Created)
}

// exportMessageParts accepts both export shapes: 1.x parts under `parts`,
// 2.x content parts under `content`, and a bare `text` field.
func exportMessageParts(message exportMessage) []exportPart {
	if len(message.Parts) > 0 {
		return message.Parts
	}
	if len(message.Content) > 0 {
		return message.Content
	}
	if message.Text != "" {
		return []exportPart{{Type: "text", Text: message.Text}}
	}
	return nil
}

func exportToolPart(part exportPart, name string) *adapters.ToolPart {
	status := exportToolStatus(part.State.Status)
	output := exportToolOutput(part.State)
	if output == "" && status == adapters.ToolStatusError {
		output = exportErrorMessage(part.State.Error)
	}
	return &adapters.ToolPart{
		Name:   name,
		CallID: firstNonEmpty(part.CallID, part.ID),
		Status: status,
		Input:  exportJSONText(part.State.Input),
		Output: output,
	}
}

func exportToolStatus(status string) string {
	switch strings.ToLower(strings.TrimSpace(status)) {
	case "completed":
		return adapters.ToolStatusOK
	case "error":
		return adapters.ToolStatusError
	default:
		return adapters.ToolStatusUnknown
	}
}

func exportToolOutput(state exportToolState) string {
	if output := exportJSONText(state.Output); output != "" {
		return output
	}
	return exportContentText(state.Content)
}

// exportContentText extracts text from a 2.x tool `content` array, or reads a
// plain string/object answer from an older shape.
func exportContentText(raw json.RawMessage) string {
	trimmed := strings.TrimSpace(string(raw))
	if trimmed == "" || trimmed == "null" {
		return ""
	}
	if strings.HasPrefix(trimmed, `"`) {
		var text string
		if err := json.Unmarshal([]byte(trimmed), &text); err == nil {
			return text
		}
		return trimmed
	}
	if strings.HasPrefix(trimmed, "[") {
		var parts []exportPart
		if err := json.Unmarshal([]byte(trimmed), &parts); err != nil {
			return trimmed
		}
		var texts []string
		for _, part := range parts {
			if strings.EqualFold(strings.TrimSpace(part.Type), "text") && part.Text != "" {
				texts = append(texts, part.Text)
			}
		}
		return strings.Join(texts, "\n")
	}
	var object struct {
		Type string `json:"type"`
		Text string `json:"text"`
	}
	if err := json.Unmarshal([]byte(trimmed), &object); err == nil && strings.EqualFold(object.Type, "text") && object.Text != "" {
		return object.Text
	}
	return trimmed
}

func exportErrorMessage(raw json.RawMessage) string {
	trimmed := strings.TrimSpace(string(raw))
	if trimmed == "" || trimmed == "null" {
		return ""
	}
	var text string
	if err := json.Unmarshal([]byte(trimmed), &text); err == nil {
		return text
	}
	var object struct {
		Message string `json:"message"`
	}
	if err := json.Unmarshal([]byte(trimmed), &object); err == nil && strings.TrimSpace(object.Message) != "" {
		return object.Message
	}
	return trimmed
}

func exportJSONText(raw json.RawMessage) string {
	trimmed := strings.TrimSpace(string(raw))
	if trimmed == "" || trimmed == "null" {
		return ""
	}
	var text string
	if err := json.Unmarshal([]byte(trimmed), &text); err == nil {
		return text
	}
	return trimmed
}

// exportTimestamp accepts the provider's millisecond number and, defensively,
// an ISO timestamp from a future export shape. A record with neither stays zero
// rather than inventing a time.
func exportTimestamp(raw json.RawMessage) time.Time {
	trimmed := strings.TrimSpace(string(raw))
	if trimmed == "" || trimmed == "null" {
		return time.Time{}
	}
	var millis int64
	if err := json.Unmarshal([]byte(trimmed), &millis); err == nil && millis > 0 {
		return time.UnixMilli(millis).UTC()
	}
	var text string
	if err := json.Unmarshal([]byte(trimmed), &text); err == nil {
		for _, layout := range []string{time.RFC3339Nano, time.RFC3339} {
			if ts, err := time.Parse(layout, strings.TrimSpace(text)); err == nil {
				return ts
			}
		}
	}
	return time.Time{}
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if trimmed := strings.TrimSpace(value); trimmed != "" {
			return trimmed
		}
	}
	return ""
}
