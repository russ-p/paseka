package sessions

import (
	"context"
	"fmt"
	"io"
	"os"
	"os/signal"
	"path/filepath"
	"strings"
	"sync"
	"syscall"
	"time"

	"github.com/creack/pty"
	"github.com/paseka/paseka/internal/adapters"
	"github.com/paseka/paseka/internal/adapters/cursor"
	"github.com/paseka/paseka/internal/adapters/pi"
	"github.com/paseka/paseka/internal/colony"
	"github.com/paseka/paseka/internal/prompts"
	"github.com/paseka/paseka/internal/protocol"
	"github.com/paseka/paseka/internal/runs"
	"github.com/paseka/paseka/internal/worktree"
	"golang.org/x/term"
)

// RunRequest is input for an interactive session.
type RunRequest struct {
	StartDir     string
	Bee          string
	TraceID      string
	Task         string
	TaskID       string
	Intent       string
	Insights     []string
	InlinePrompt string
}

// RunResult summarizes a completed interactive session.
type RunResult struct {
	SessionID string
	TraceID   string
	AgentID   string
	Workspace string
	RunDir    string
	State     adapters.SessionState
}

// Entry is a registry entry for an active session in this process.
type Entry struct {
	Handle     adapters.SessionHandle
	ColonyRoot string
	Slug       string
	RunDir     runs.Dir
}

// Manager owns interactive agent sessions.
type Manager struct {
	mu       sync.RWMutex
	sessions map[string]*activeSession
	adapters map[string]adapters.SessionAdapter
}

type activeSession struct {
	entry   Entry
	process *ptyProcess
	cancel  context.CancelFunc
	done    chan struct{}
}

// NewManager creates a session manager with default adapters.
func NewManager() *Manager {
	return &Manager{
		sessions: map[string]*activeSession{},
		adapters: map[string]adapters.SessionAdapter{
			"cursor": cursor.NewSession(),
			"pi":     pi.NewSession(),
		},
	}
}

// RegisterSessionAdapter adds or replaces a session adapter (for tests).
func (m *Manager) RegisterSessionAdapter(name string, a adapters.SessionAdapter) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.adapters[name] = a
}

// RunInteractive starts a session, attaches the current terminal, and blocks until exit.
func (m *Manager) RunInteractive(ctx context.Context, req RunRequest) (*RunResult, error) {
	active, err := m.launch(ctx, req)
	if err != nil {
		return nil, err
	}

	if err := attachPTY(active.process); err != nil {
		_ = m.stopSession(active.entry.Handle.SessionID)
		return nil, err
	}

	<-active.done

	return m.runResult(active), nil
}

// StartDetached launches an interactive session, captures PTY output in the background,
// and returns immediately without attaching the current terminal.
func (m *Manager) StartDetached(ctx context.Context, req RunRequest) (*RunResult, error) {
	active, err := m.launch(ctx, req)
	if err != nil {
		return nil, err
	}
	go m.capturePTYOutput(active)
	return m.runResult(active), nil
}

func (m *Manager) runResult(active *activeSession) *RunResult {
	return &RunResult{
		SessionID: active.entry.Handle.SessionID,
		TraceID:   active.entry.Handle.TraceID,
		AgentID:   active.entry.Handle.AgentID,
		Workspace: active.entry.Handle.Workspace,
		RunDir:    active.entry.RunDir.Root(),
		State:     active.entry.Handle.State,
	}
}

func (m *Manager) capturePTYOutput(active *activeSession) {
	buf := make([]byte, 4096)
	for {
		n, err := active.process.ReadWriteCloser().Read(buf)
		if n > 0 {
			text := runs.NormalizePTYOutput(buf[:n])
			if text != "" {
				_ = active.entry.RunDir.AppendTranscript(runs.TranscriptEntry{
					At:      time.Now().UTC(),
					Role:    "agent",
					Content: text,
				})
			}
		}
		if err != nil {
			if err != io.EOF {
				_ = active.entry.RunDir.AppendTranscript(runs.TranscriptEntry{
					At:      time.Now().UTC(),
					Role:    "system",
					Content: fmt.Sprintf("pty read error: %v", err),
				})
			}
			return
		}
	}
}

func (m *Manager) launch(ctx context.Context, req RunRequest) (*activeSession, error) {
	if req.Bee == "" {
		return nil, fmt.Errorf("sessions: bee role is required")
	}
	if req.Task == "" && req.InlinePrompt == "" {
		return nil, fmt.Errorf("sessions: task or inline prompt is required")
	}

	ctxColony, err := colony.ResolveContext(req.StartDir)
	if err != nil {
		return nil, err
	}

	traceID := req.TraceID
	if traceID == "" {
		id, err := colony.NewTraceID()
		if err != nil {
			return nil, err
		}
		traceID = id
	}

	manifest, err := colony.LoadColony(ctxColony.ColonyRoot)
	if err != nil {
		return nil, err
	}
	bee, beeLocalTemplate, err := colony.LoadBee(ctxColony.ColonyRoot, req.Bee)
	if err != nil {
		return nil, err
	}

	workspace := ctxColony.ColonyRoot
	if bee.Worktree {
		entry, err := worktree.Ensure(worktree.EnsureOptions{
			ColonyRoot: ctxColony.ColonyRoot,
			TraceID:    traceID,
			Slug:       ctxColony.Slug,
		})
		if err != nil {
			return nil, fmt.Errorf("sessions: worktree: %w", err)
		}
		workspace = entry.Path
	}

	agentID, err := colony.NewAgentID()
	if err != nil {
		return nil, err
	}
	sessionID := agentID

	runDir := runs.Dir{
		ColonyRoot: ctxColony.ColonyRoot,
		TraceID:    traceID,
		AgentID:    agentID,
	}
	if err := runDir.Prepare(); err != nil {
		return nil, err
	}

	resultFile, err := filepath.Abs(runDir.ResultPath())
	if err != nil {
		return nil, err
	}

	loader, err := prompts.NewLoader(ctxColony.ColonyRoot)
	if err != nil {
		return nil, err
	}
	rendered, err := loader.RenderResolved(prompts.ResolveInput{
		InlinePrompt:     req.InlinePrompt,
		BeeLocalTemplate: beeLocalTemplate,
		BeeTemplate:      bee.PromptTemplate,
		DefaultTemplate:  manifest.Defaults.PromptTemplate,
	}, prompts.PromptContext(prompts.Context{
		Bee:        bee.Role,
		TraceID:    traceID,
		AgentID:    agentID,
		TaskID:     req.TaskID,
		ColonyRoot: ctxColony.ColonyRoot,
		Workspace:  workspace,
		Task:       req.Task,
		IntentRaw:  req.Intent,
		Insights:   req.Insights,
		ResultFile: resultFile,
	}))
	if err != nil {
		return nil, fmt.Errorf("sessions: render prompt: %w", err)
	}

	adapterName, err := bee.ResolveAdapter()
	if err != nil {
		return nil, err
	}
	sessAdapter, ok := m.adapters[adapterName]
	if !ok {
		return nil, fmt.Errorf("sessions: adapter %q does not support interactive sessions", adapterName)
	}

	params := colony.MergeRunParams(colony.RunParamsFromBee(bee), colony.AdapterExtra(ctxColony, adapterName))

	startedAt := time.Now().UTC()
	if err := runDir.WritePrompt(rendered); err != nil {
		return nil, err
	}
	if err := runDir.WriteMeta(runs.Meta{
		TraceID:   traceID,
		AgentID:   agentID,
		Bee:       bee.Role,
		Adapter:   adapterName,
		Workspace: workspace,
		StartedAt: startedAt,
	}); err != nil {
		return nil, err
	}
	if err := runDir.WriteRequest(protocol.Request{
		ProtocolVersion: protocol.Version,
		TraceID:         traceID,
		AgentID:         agentID,
		Bee:             bee.Role,
		Adapter:         adapterName,
		Workspace:       workspace,
		ColonyRoot:      ctxColony.ColonyRoot,
		TaskID:          req.TaskID,
		Task:            req.Task,
		Intent:          req.Intent,
		Insights:        req.Insights,
		ResultPath:      resultFile,
		EventLogPath:    runDir.EventsPath(),
		CreatedAt:       startedAt,
	}); err != nil {
		return nil, err
	}
	if err := runDir.WriteStatusSnapshot(protocol.StatusSnapshot{
		ProtocolVersion: protocol.Version,
		State:           protocol.StatusRunning,
		StartedAt:       startedAt,
	}); err != nil {
		return nil, err
	}

	sessReq := adapters.SessionRequest{
		Bee:           bee.Role,
		InitialPrompt: rendered,
		ColonyRoot:    ctxColony.ColonyRoot,
		Workspace:     workspace,
		Params:        params,
		TraceID:       traceID,
		AgentID:       agentID,
		TaskID:        req.TaskID,
		Task:          req.Task,
		Intent:        req.Intent,
		Insights:      req.Insights,
	}

	cmd, err := sessAdapter.SessionCommand(sessReq)
	if err != nil {
		return nil, err
	}

	proc, err := startPTY(cmd)
	if err != nil {
		return nil, err
	}

	handle := adapters.SessionHandle{
		SessionID:  sessionID,
		TraceID:    traceID,
		AgentID:    agentID,
		RunDir:     runDir.Root(),
		Workspace:  workspace,
		ColonyRoot: ctxColony.ColonyRoot,
		Bee:        bee.Role,
		Adapter:    adapterName,
		PID:        proc.PID(),
		State:      adapters.SessionActive,
		StartedAt:  startedAt,
	}

	if err := runDir.WriteSession(runs.SessionMeta{
		SessionID:  sessionID,
		TraceID:    traceID,
		AgentID:    agentID,
		Bee:        bee.Role,
		Adapter:    adapterName,
		Workspace:  workspace,
		ColonyRoot: ctxColony.ColonyRoot,
		PID:        proc.PID(),
		State:      string(adapters.SessionActive),
		StartedAt:  startedAt,
	}); err != nil {
		_ = proc.Kill()
		return nil, err
	}
	_ = runDir.AppendTranscript(runs.TranscriptEntry{
		At:      startedAt,
		Role:    "system",
		Content: "session started",
	})

	sessCtx, cancel := context.WithCancel(ctx)
	active := &activeSession{
		entry: Entry{
			Handle:     handle,
			ColonyRoot: ctxColony.ColonyRoot,
			Slug:       ctxColony.Slug,
			RunDir:     runDir,
		},
		process: proc,
		cancel:  cancel,
		done:    make(chan struct{}),
	}

	m.mu.Lock()
	m.sessions[sessionID] = active
	m.mu.Unlock()

	if err := colony.RegisterSession(ctxColony.Slug, colony.SessionEntry{
		SessionID: sessionID,
		TraceID:   traceID,
		AgentID:   agentID,
		RunDir:    runDir.Root(),
		Bee:       bee.Role,
		PID:       proc.PID(),
		StartedAt: startedAt,
	}); err != nil {
		_ = m.stopSession(sessionID)
		return nil, err
	}

	go func() {
		m.waitSession(sessCtx, sessionID)
	}()

	return active, nil
}

func (m *Manager) waitSession(ctx context.Context, sessionID string) {
	m.mu.RLock()
	active, ok := m.sessions[sessionID]
	m.mu.RUnlock()
	if !ok {
		return
	}

	waitErr := active.process.Wait()
	state := adapters.SessionCompleted
	if ctx.Err() != nil {
		state = adapters.SessionCancelled
	} else if waitErr != nil {
		state = adapters.SessionFailed
	}

	m.finishSession(sessionID, state, waitErr)
}

func (m *Manager) finishSession(sessionID string, state adapters.SessionState, waitErr error) {
	m.mu.Lock()
	active, ok := m.sessions[sessionID]
	if ok {
		active.entry.Handle.State = state
		delete(m.sessions, sessionID)
	}
	m.mu.Unlock()
	if !ok {
		return
	}

	finishedAt := time.Now().UTC()

	_ = active.entry.RunDir.WriteSession(runs.SessionMeta{
		SessionID:  sessionID,
		TraceID:    active.entry.Handle.TraceID,
		AgentID:    active.entry.Handle.AgentID,
		Bee:        active.entry.Handle.Bee,
		Adapter:    active.entry.Handle.Adapter,
		Workspace:  active.entry.Handle.Workspace,
		ColonyRoot: active.entry.Handle.ColonyRoot,
		State:      string(state),
		StartedAt:  active.entry.Handle.StartedAt,
		FinishedAt: finishedAt,
	})

	exitCode := 0
	errMsg := ""
	if waitErr != nil {
		errMsg = waitErr.Error()
		exitCode = 1
	}

	protoState := protocol.StatusCompleted
	if state == adapters.SessionFailed {
		protoState = protocol.StatusFailed
	} else if state == adapters.SessionCancelled {
		protoState = protocol.StatusCancelled
	}

	_ = active.entry.RunDir.WriteStatus(protoState, exitCode, active.entry.Handle.StartedAt, finishedAt, errMsg)
	_ = active.entry.RunDir.AppendTranscript(runs.TranscriptEntry{
		At:      finishedAt,
		Role:    "system",
		Content: fmt.Sprintf("session %s", state),
	})

	_ = active.entry.RunDir.WriteResultText(buildSessionResultText(active.entry.RunDir))

	_ = colony.UnregisterSession(active.entry.Slug, sessionID)
	close(active.done)
}

func buildSessionResultText(runDir runs.Dir) string {
	entries, err := runDir.ReadTranscript()
	if err != nil || len(entries) == 0 {
		return ""
	}
	var b strings.Builder
	for _, e := range entries {
		line := strings.TrimSpace(e.Content)
		if line == "" {
			continue
		}
		if b.Len() > 0 {
			b.WriteByte('\n')
		}
		fmt.Fprintf(&b, "[%s] %s", e.Role, line)
	}
	return b.String()
}

func (m *Manager) stopSession(sessionID string) error {
	m.mu.RLock()
	active, ok := m.sessions[sessionID]
	m.mu.RUnlock()
	if !ok {
		return fmt.Errorf("sessions: session %q not found in this process", sessionID)
	}
	active.cancel()
	if err := active.process.Kill(); err != nil {
		return err
	}
	<-active.done
	return nil
}

// Stop terminates an active session in this process.
func (m *Manager) Stop(sessionID string) error {
	return m.stopSession(sessionID)
}

// StopAll terminates every session owned by this manager.
func (m *Manager) StopAll() {
	m.mu.RLock()
	ids := make([]string, 0, len(m.sessions))
	for id := range m.sessions {
		ids = append(ids, id)
	}
	m.mu.RUnlock()
	for _, id := range ids {
		_ = m.Stop(id)
	}
}

// Get returns an active session entry when owned by this process.
func (m *Manager) Get(sessionID string) (Entry, bool) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	active, ok := m.sessions[sessionID]
	if !ok {
		return Entry{}, false
	}
	return active.entry, true
}

// StopRemote sends SIGTERM to a session PID from the colony registry.
func StopRemote(slug, sessionID string) error {
	entry, err := colony.FindSession(slug, sessionID)
	if err != nil {
		return err
	}
	if entry.PID <= 0 {
		return fmt.Errorf("sessions: session %q has no PID", sessionID)
	}
	proc, err := os.FindProcess(entry.PID)
	if err != nil {
		return err
	}
	if err := proc.Signal(syscall.SIGTERM); err != nil {
		return fmt.Errorf("sessions: signal pid %d: %w", entry.PID, err)
	}
	_ = colony.UnregisterSession(slug, sessionID)
	return nil
}

// ListActive returns active sessions in this process.
func (m *Manager) ListActive() []Entry {
	m.mu.RLock()
	defer m.mu.RUnlock()
	out := make([]Entry, 0, len(m.sessions))
	for _, s := range m.sessions {
		out = append(out, s.entry)
	}
	return out
}

// AttachInPlace connects the current terminal to a session PTY in this process.
func (m *Manager) AttachInPlace(sessionID string) error {
	m.mu.RLock()
	active, ok := m.sessions[sessionID]
	m.mu.RUnlock()
	if !ok {
		return fmt.Errorf("sessions: session %q not active in this process", sessionID)
	}
	return attachPTY(active.process)
}

func attachPTY(proc *ptyProcess) error {
	ptmx := proc.pty
	fd := int(os.Stdin.Fd())

	if term.IsTerminal(fd) {
		oldState, err := term.MakeRaw(fd)
		if err != nil {
			return fmt.Errorf("sessions: terminal raw mode: %w", err)
		}
		defer func() { _ = term.Restore(fd, oldState) }()
	}

	if err := pty.InheritSize(os.Stdin, ptmx); err != nil {
		_ = err
	}

	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, syscall.SIGWINCH)
	go func() {
		for range sigCh {
			_ = pty.InheritSize(os.Stdin, ptmx)
		}
	}()
	defer signal.Stop(sigCh)

	done := make(chan struct{}, 2)
	go func() {
		_, _ = io.Copy(ptmx, os.Stdin)
		done <- struct{}{}
	}()
	go func() {
		_, _ = io.Copy(os.Stdout, ptmx)
		done <- struct{}{}
	}()

	<-done
	return nil
}

// LaunchInGhostty opens a Ghostty window running session run for the given bee.
func LaunchInGhostty(cfg colony.TerminalConfig, runArgs []string) error {
	self, err := os.Executable()
	if err != nil {
		return err
	}
	attachCmd := append([]string{self, "session", "run"}, runArgs...)
	termCfg := TerminalConfig{
		Kind:          TerminalGhostty,
		GhosttyBinary: cfg.GhosttyBinary,
	}
	return LaunchAttach(termCfg, attachCmd)
}

// DefaultManager is the process-wide session manager for CLI attach/stop.
var DefaultManager = NewManager()
