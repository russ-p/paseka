package runs

import (
	"bufio"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"time"
	"unicode"

	"github.com/paseka/paseka/internal/protocol"
)

var ansiEscapeRE = regexp.MustCompile(`\x1b\[[0-9;?]*[ -/]*[@-~]|\x1b\][^\x07]*\x07|\x1b[PX^_][^\x1b]*\x1b\\`)

const (
	ResultFileName     = "result.txt"
	PromptFileName     = "prompt.txt"
	SystemFileName     = "system.txt"
	MetaFileName       = "meta.json"
	StatusFileName     = "status.json"
	RequestFileName    = "request.json"
	EventsFileName     = "events.ndjson"
	ResultJSONFileName = "result.json"
	SessionFileName    = "session.json"
	TranscriptFileName = "transcript.ndjson"
)

// Dir holds paths for one spawned agent run.
type Dir struct {
	ColonyRoot string
	TraceID    string
	AgentID    string
}

// Root returns .paseka/runs/<traceId>/<agentId>/ under colony root.
func (d Dir) Root() string {
	return filepath.Join(d.ColonyRoot, ".paseka", "runs", d.TraceID, d.AgentID)
}

func (d Dir) ResultPath() string     { return filepath.Join(d.Root(), ResultFileName) }
func (d Dir) PromptPath() string     { return filepath.Join(d.Root(), PromptFileName) }
func (d Dir) SystemPath() string     { return filepath.Join(d.Root(), SystemFileName) }
func (d Dir) MetaPath() string       { return filepath.Join(d.Root(), MetaFileName) }
func (d Dir) StatusPath() string     { return filepath.Join(d.Root(), StatusFileName) }
func (d Dir) RequestPath() string    { return filepath.Join(d.Root(), RequestFileName) }
func (d Dir) EventsPath() string     { return filepath.Join(d.Root(), EventsFileName) }
func (d Dir) ResultJSONPath() string { return filepath.Join(d.Root(), ResultJSONFileName) }
func (d Dir) SessionPath() string    { return filepath.Join(d.Root(), SessionFileName) }
func (d Dir) TranscriptPath() string { return filepath.Join(d.Root(), TranscriptFileName) }

// Meta is written by the runtime before launching an agent (legacy observers).
type Meta struct {
	TraceID   string    `json:"traceId"`
	AgentID   string    `json:"agentId"`
	Bee       string    `json:"bee"`
	Adapter   string    `json:"adapter"`
	Workspace string    `json:"workspace"`
	StartedAt time.Time `json:"startedAt"`
}

// Prepare creates the run directory and removes stale outputs from a previous attempt.
func (d Dir) Prepare() error {
	if d.ColonyRoot == "" || d.TraceID == "" || d.AgentID == "" {
		return fmt.Errorf("runs: colony root, traceId, and agentId are required")
	}
	root := d.Root()
	if err := os.MkdirAll(root, 0o755); err != nil {
		return fmt.Errorf("runs: mkdir %s: %w", root, err)
	}
	for _, path := range []string{d.ResultPath(), d.ResultJSONPath(), d.EventsPath()} {
		_ = os.Remove(path)
	}
	return nil
}

func (d Dir) WritePrompt(prompt string) error {
	return os.WriteFile(d.PromptPath(), []byte(prompt), 0o644)
}

func (d Dir) WriteSystem(system string) error {
	return os.WriteFile(d.SystemPath(), []byte(system), 0o644)
}

func (d Dir) WriteMeta(meta Meta) error {
	data, err := json.MarshalIndent(meta, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(d.MetaPath(), data, 0o644)
}

func (d Dir) WriteRequest(req protocol.Request) error {
	if req.ProtocolVersion == "" {
		req.ProtocolVersion = protocol.Version
	}
	data, err := json.MarshalIndent(req, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(d.RequestPath(), data, 0o644)
}

func (d Dir) ReadRequest() (protocol.Request, error) {
	data, err := os.ReadFile(d.RequestPath())
	if err != nil {
		return protocol.Request{}, err
	}
	var req protocol.Request
	if err := json.Unmarshal(data, &req); err != nil {
		return protocol.Request{}, err
	}
	return req, nil
}

func (d Dir) WriteStatusSnapshot(snap protocol.StatusSnapshot) error {
	if snap.ProtocolVersion == "" {
		snap.ProtocolVersion = protocol.Version
	}
	data, err := json.MarshalIndent(snap, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(d.StatusPath(), data, 0o644)
}

// WriteStatus is a convenience wrapper for the final completed/failed snapshot.
func (d Dir) WriteStatus(state protocol.RunStatus, exitCode int, startedAt, finishedAt time.Time, errMsg string) error {
	return d.WriteStatusSnapshot(protocol.StatusSnapshot{
		ProtocolVersion: protocol.Version,
		State:           state,
		ExitCode:        exitCode,
		StartedAt:       startedAt,
		FinishedAt:      finishedAt,
		Error:           errMsg,
	})
}

func (d Dir) ReadStatus() (protocol.StatusSnapshot, error) {
	data, err := os.ReadFile(d.StatusPath())
	if err != nil {
		return protocol.StatusSnapshot{}, err
	}
	var snap protocol.StatusSnapshot
	if err := json.Unmarshal(data, &snap); err != nil {
		return protocol.StatusSnapshot{}, err
	}
	return snap, nil
}

func (d Dir) AppendEvent(ev protocol.Event) error {
	if ev.ProtocolVersion == "" {
		ev.ProtocolVersion = protocol.Version
	}
	if ev.Seq == 0 {
		seq, err := d.nextEventSeq()
		if err != nil {
			return err
		}
		ev.Seq = seq
	}
	data, err := json.Marshal(ev)
	if err != nil {
		return err
	}
	f, err := os.OpenFile(d.EventsPath(), os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0o644)
	if err != nil {
		return err
	}
	defer f.Close()
	if _, err := f.Write(append(data, '\n')); err != nil {
		return err
	}
	return nil
}

func (d Dir) nextEventSeq() (int, error) {
	events, err := d.ReadEvents()
	if err != nil {
		return 0, err
	}
	return len(events) + 1, nil
}

func (d Dir) ReadEvents() ([]protocol.Event, error) {
	f, err := os.Open(d.EventsPath())
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, err
	}
	defer f.Close()

	var events []protocol.Event
	scanner := bufio.NewScanner(f)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" {
			continue
		}
		var ev protocol.Event
		if err := json.Unmarshal([]byte(line), &ev); err != nil {
			return nil, fmt.Errorf("runs: parse event: %w", err)
		}
		events = append(events, ev)
	}
	if err := scanner.Err(); err != nil {
		return nil, err
	}
	return events, nil
}

func (d Dir) WriteResult(res protocol.Result) error {
	if res.ProtocolVersion == "" {
		res.ProtocolVersion = protocol.Version
	}
	data, err := json.MarshalIndent(res, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(d.ResultJSONPath(), data, 0o644)
}

func (d Dir) ReadResultJSON() (protocol.Result, error) {
	data, err := os.ReadFile(d.ResultJSONPath())
	if err != nil {
		return protocol.Result{}, err
	}
	var res protocol.Result
	if err := json.Unmarshal(data, &res); err != nil {
		return protocol.Result{}, err
	}
	return res, nil
}

func (d Dir) ReadResult() (string, error) {
	data, err := os.ReadFile(d.ResultPath())
	if err != nil {
		return "", err
	}
	return string(data), nil
}

// WriteResultText persists a human-readable run summary log artifact.
func (d Dir) WriteResultText(summary string) error {
	summary = strings.TrimSpace(summary)
	if summary == "" {
		return nil
	}
	return os.WriteFile(d.ResultPath(), []byte(summary), 0o644)
}

// SessionMeta is written to session.json for interactive agent sessions.
type SessionMeta struct {
	ProtocolVersion string    `json:"protocolVersion"`
	SessionID       string    `json:"sessionId"`
	TraceID         string    `json:"traceId"`
	AgentID         string    `json:"agentId"`
	Bee             string    `json:"bee"`
	Adapter         string    `json:"adapter"`
	Workspace       string    `json:"workspace"`
	ColonyRoot      string    `json:"colonyRoot"`
	PID             int       `json:"pid,omitempty"`
	State           string    `json:"state"`
	StartedAt       time.Time `json:"startedAt"`
	FinishedAt      time.Time `json:"finishedAt,omitempty"`
}

// TranscriptEntry is one NDJSON line in transcript.ndjson.
type TranscriptEntry struct {
	At      time.Time `json:"at"`
	Role    string    `json:"role"` // user | agent | system
	Content string    `json:"content"`
}

func (d Dir) WriteSession(meta SessionMeta) error {
	if meta.ProtocolVersion == "" {
		meta.ProtocolVersion = protocol.Version
	}
	data, err := json.MarshalIndent(meta, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(d.SessionPath(), data, 0o644)
}

func (d Dir) ReadSession() (SessionMeta, error) {
	data, err := os.ReadFile(d.SessionPath())
	if err != nil {
		return SessionMeta{}, err
	}
	var meta SessionMeta
	if err := json.Unmarshal(data, &meta); err != nil {
		return SessionMeta{}, err
	}
	return meta, nil
}

func (d Dir) AppendTranscript(entry TranscriptEntry) error {
	if entry.At.IsZero() {
		entry.At = time.Now().UTC()
	}
	data, err := json.Marshal(entry)
	if err != nil {
		return err
	}
	f, err := os.OpenFile(d.TranscriptPath(), os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0o644)
	if err != nil {
		return err
	}
	defer f.Close()
	_, err = f.Write(append(data, '\n'))
	return err
}

func (d Dir) ReadTranscript() ([]TranscriptEntry, error) {
	return d.readTranscriptFrom(0)
}

// ReadTranscriptAfter returns transcript entries with index > after and the next cursor.
func (d Dir) ReadTranscriptAfter(after int) ([]TranscriptEntry, int, error) {
	entries, err := d.readTranscriptFrom(after)
	if err != nil {
		return nil, after, err
	}
	next := after + len(entries)
	return entries, next, nil
}

func (d Dir) readTranscriptFrom(skip int) ([]TranscriptEntry, error) {
	f, err := os.Open(d.TranscriptPath())
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, err
	}
	defer f.Close()

	var entries []TranscriptEntry
	scanner := bufio.NewScanner(f)
	idx := 0
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" {
			continue
		}
		if idx < skip {
			idx++
			continue
		}
		var entry TranscriptEntry
		if err := json.Unmarshal([]byte(line), &entry); err != nil {
			return nil, fmt.Errorf("runs: parse transcript: %w", err)
		}
		entries = append(entries, entry)
		idx++
	}
	if err := scanner.Err(); err != nil {
		return nil, err
	}
	return entries, nil
}

// NormalizePTYOutput strips ANSI escape sequences and non-printable control bytes.
func NormalizePTYOutput(data []byte) string {
	s := ansiEscapeRE.ReplaceAllString(string(data), "")
	var b strings.Builder
	for _, r := range s {
		if r == '\n' || r == '\r' || r == '\t' || !unicode.IsControl(r) {
			b.WriteRune(r)
		}
	}
	return strings.TrimSpace(b.String())
}

// ScanRecentSessions walks .paseka/runs and returns up to limit session.json records,
// newest first by StartedAt.
func ScanRecentSessions(colonyRoot string, limit int) ([]SessionMeta, error) {
	if limit <= 0 {
		return nil, nil
	}
	runsRoot := filepath.Join(colonyRoot, ".paseka", "runs")
	traceDirs, err := os.ReadDir(runsRoot)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, err
	}

	var metas []SessionMeta
	for _, traceEntry := range traceDirs {
		if !traceEntry.IsDir() {
			continue
		}
		tracePath := filepath.Join(runsRoot, traceEntry.Name())
		agentDirs, err := os.ReadDir(tracePath)
		if err != nil {
			continue
		}
		for _, agentEntry := range agentDirs {
			if !agentEntry.IsDir() {
				continue
			}
			d := Dir{
				ColonyRoot: colonyRoot,
				TraceID:    traceEntry.Name(),
				AgentID:    agentEntry.Name(),
			}
			meta, err := d.ReadSession()
			if err != nil {
				continue
			}
			metas = append(metas, meta)
		}
	}

	sortSessionsByStartedAt(metas)
	if len(metas) > limit {
		metas = metas[:limit]
	}
	return metas, nil
}

// FindSessionMeta locates session.json by sessionId anywhere under .paseka/runs.
func FindSessionMeta(colonyRoot, sessionID string) (SessionMeta, bool, error) {
	if sessionID == "" {
		return SessionMeta{}, false, nil
	}
	runsRoot := filepath.Join(colonyRoot, ".paseka", "runs")
	traceDirs, err := os.ReadDir(runsRoot)
	if err != nil {
		if os.IsNotExist(err) {
			return SessionMeta{}, false, nil
		}
		return SessionMeta{}, false, err
	}
	for _, traceEntry := range traceDirs {
		if !traceEntry.IsDir() {
			continue
		}
		tracePath := filepath.Join(runsRoot, traceEntry.Name())
		agentDirs, err := os.ReadDir(tracePath)
		if err != nil {
			continue
		}
		for _, agentEntry := range agentDirs {
			if !agentEntry.IsDir() {
				continue
			}
			d := Dir{
				ColonyRoot: colonyRoot,
				TraceID:    traceEntry.Name(),
				AgentID:    agentEntry.Name(),
			}
			meta, err := d.ReadSession()
			if err != nil {
				continue
			}
			if meta.SessionID == sessionID {
				return meta, true, nil
			}
		}
	}
	return SessionMeta{}, false, nil
}

func sortSessionsByStartedAt(metas []SessionMeta) {
	for i := 0; i < len(metas); i++ {
		for j := i + 1; j < len(metas); j++ {
			if metas[j].StartedAt.After(metas[i].StartedAt) {
				metas[i], metas[j] = metas[j], metas[i]
			}
		}
	}
}
