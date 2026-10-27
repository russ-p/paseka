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
	"github.com/russ-p/paseka/internal/adapters"
	"github.com/russ-p/paseka/internal/adapters/claude"
	"github.com/russ-p/paseka/internal/adapters/cursor"
	"github.com/russ-p/paseka/internal/adapters/opencode"
	"github.com/russ-p/paseka/internal/adapters/opencodev1"
	"github.com/russ-p/paseka/internal/adapters/opencodev2"
	"github.com/russ-p/paseka/internal/adapters/pi"
	"github.com/russ-p/paseka/internal/artifacts"
	"github.com/russ-p/paseka/internal/bus"
	"github.com/russ-p/paseka/internal/colony"
	"github.com/russ-p/paseka/internal/homestate"
	"github.com/russ-p/paseka/internal/launch"
	"github.com/russ-p/paseka/internal/logging"
	"github.com/russ-p/paseka/internal/prompts"
	"github.com/russ-p/paseka/internal/protocol"
	"github.com/russ-p/paseka/internal/runs"
	"github.com/russ-p/paseka/internal/tasks"
	"github.com/russ-p/paseka/internal/trailconfig"
	"github.com/russ-p/paseka/internal/trailfork"
	"github.com/russ-p/paseka/internal/trailview"
	"github.com/russ-p/paseka/internal/worktree"
	"golang.org/x/term"
)

// sessionUsageTimeout bounds the post-exit provider accounting lookup so a slow
// or unreachable provider cannot stall session teardown.
const sessionUsageTimeout = 30 * time.Second

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
	// ConfigRef and ConfigRevision are the configuration version the session
	// must run under. A continuation keeps the version captured when its work
	// was admitted; an empty ConfigRef resolves the trail's active version, as
	// legacy sessions do.
	ConfigRef      string
	ConfigRevision int

	resumeFrom       string
	resumeProviderID string
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
	hub     *ptyHub
	cancel  context.CancelFunc
	done    chan struct{}
	// cleanup releases resources the adapter started before the PTY, such as a
	// Paseka-managed OpenCode server. It runs once when the session finishes.
	cleanup func()
	// proposal carries what proposal finalization needs: the workspace baseline captured
	// before the PTY started and the effective bee configuration for this launch. A
	// resumed provider session still mints a fresh run id and a fresh baseline here, so
	// it never overwrites the earlier session's record.
	proposal sessionProposal
}

// sessionProposal is the proposal-finalization context captured at launch.
type sessionProposal struct {
	bee        colony.Bee
	baseline   adapters.WorkspaceBaseline
	taskID     string
	task       string
	provenance protocol.Provenance
}

// NewManager creates a session manager with default adapters.
func NewManager() *Manager {
	return &Manager{
		sessions: map[string]*activeSession{},
		adapters: map[string]adapters.SessionAdapter{
			"cursor":     cursor.NewSession(),
			"pi":         pi.NewSession(),
			"claude":     claude.NewSession(),
			"opencode":   opencode.NewSession(),
			"opencodev1": opencodev1.NewSession(),
			"opencodev2": opencodev2.NewSession(),
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
	active, err := m.launch(ctx, req, false)
	if err != nil {
		return nil, err
	}

	if err := attachPTY(active.process); err != nil {
		_ = m.stopSession(active.entry.Handle.SessionID)
		return nil, err
	}

	<-active.done

	result := launchResult(active)
	if meta, err := active.entry.RunDir.ReadSession(); err == nil {
		result.State = adapters.SessionState(meta.State)
	}
	return result, nil
}

// StartDetached launches an interactive agent session (same TUI as bee chat),
// captures PTY I/O via a hub for later AttachPTY, and returns immediately
// without attaching the current terminal.
func (m *Manager) StartDetached(ctx context.Context, req RunRequest) (*RunResult, error) {
	active, err := m.launch(ctx, req, true)
	if err != nil {
		return nil, err
	}

	m.mu.Lock()
	stillActive := m.sessions[active.entry.Handle.SessionID] == active
	if stillActive {
		active.hub = newPTYHub(active.process, active.entry.RunDir)
	}
	m.mu.Unlock()
	if stillActive {
		active.hub.start()
	}

	return launchResult(active), nil
}

func launchResult(active *activeSession) *RunResult {
	handle := active.entry.Handle
	return &RunResult{
		SessionID: handle.SessionID,
		TraceID:   handle.TraceID,
		AgentID:   handle.AgentID,
		Workspace: handle.Workspace,
		RunDir:    active.entry.RunDir.Root(),
		State:     adapters.SessionActive,
	}
}

func (m *Manager) launch(ctx context.Context, req RunRequest, detached bool) (*activeSession, error) {
	if req.Bee == "" {
		return nil, fmt.Errorf("sessions: bee role is required")
	}

	ctxColony, err := colony.ResolveContext(req.StartDir)
	if err != nil {
		return nil, err
	}

	resuming := strings.TrimSpace(req.resumeProviderID) != ""
	traceID := req.TraceID
	if resuming && strings.TrimSpace(traceID) == "" {
		return nil, fmt.Errorf("sessions: resume requires traceId")
	}
	if traceID == "" {
		id, err := colony.NewTraceID()
		if err != nil {
			return nil, err
		}
		traceID = id
	}
	scope, err := trailview.ResolveScope(ctxColony, traceID)
	if err != nil {
		return nil, fmt.Errorf("sessions: resolve trail scope: %w", err)
	}
	if scope.AdmissionState == trailfork.StateDropped {
		return nil, fmt.Errorf("sessions: trail %s was dropped and cannot dispatch", traceID)
	}
	// A fork whose creation is incomplete has no published initial task and is
	// not yet a runnable trail.
	if scope.AdmissionState == trailfork.StateCreating {
		return nil, fmt.Errorf("sessions: fork %s is still being created and cannot dispatch", traceID)
	}
	killed, err := tasks.TraceKilled(ctxColony, traceID)
	if err != nil {
		return nil, fmt.Errorf("sessions: inspect trace kill state: %w", err)
	}
	if killed {
		return nil, fmt.Errorf("sessions: trail %s was killed and cannot dispatch", traceID)
	}
	execColony, trailManifest, boundConfig, err := trailconfig.BindNewWork(ctxColony, traceID)
	if err != nil {
		return nil, fmt.Errorf("sessions: trail config: %w", err)
	}
	// A continuation keeps the configuration captured when it was admitted.
	// Resolving the trail's later active revision here would run new prompts,
	// tools, and adapters inside an already-open session.
	if ref := strings.TrimSpace(req.ConfigRef); ref != "" && ref != execColony.ConfigRef {
		bound, err := trailconfig.BindRef(execColony, traceID, ref, req.ConfigRevision)
		if err != nil {
			return nil, fmt.Errorf("sessions: configuration %s revision %d is unavailable: %w", ref, req.ConfigRevision, err)
		}
		execColony = bound
	}
	if scope.Experimental {
		if !boundConfig {
			return nil, fmt.Errorf("sessions: experimental trail %s has no configuration binding", traceID)
		}
		if trailManifest.Policy != trailconfig.PolicyPinned {
			return nil, fmt.Errorf("sessions: experimental trail %s has non-pinned policy %q", traceID, trailManifest.Policy)
		}
		if execColony.ConfigRef != "" && scope.ConfigRef != "" && execColony.ConfigRef != scope.ConfigRef {
			return nil, fmt.Errorf("sessions: experimental trail %s resolved a configuration the lineage does not pin", traceID)
		}
	}

	var attempt trailconfig.Attempt
	if req.TaskID != "" && boundConfig {
		input := trailconfig.NewTaskInputEvidence(req.TaskID, "", req.Task, req.Bee, "", req.Intent, "")
		var blocked bool
		attempt, blocked, err = trailconfig.EnsureTaskAttemptGated(execColony, traceID, req.TaskID, "", input)
		if err != nil {
			return nil, fmt.Errorf("sessions: task attempt: %w", err)
		}
		if blocked {
			return nil, fmt.Errorf("sessions: trail %s has a pending configuration change; new work is held", traceID)
		}
		if attempt.ConfigRef != "" && attempt.ConfigRef != execColony.ConfigRef {
			execColony, err = trailconfig.BindRef(execColony, traceID, attempt.ConfigRef, attempt.ConfigRevision)
			if err != nil {
				return nil, fmt.Errorf("sessions: bind attempt config: %w", err)
			}
		}
	}

	manifest, err := colony.LoadColony(execColony.ConfigRootPath())
	if err != nil {
		return nil, err
	}
	bee, overlay, err := execColony.LoadBee(req.Bee)
	if err != nil {
		return nil, err
	}
	if !resuming && req.Task == "" && req.InlinePrompt == "" && !colony.HasSystemTemplate(bee, overlay, manifest.Defaults) {
		return nil, fmt.Errorf("sessions: task, inline prompt, or system_template is required")
	}

	workspace := ctxColony.ColonyRoot
	fixedRoot := ""
	if execColony.WorkspaceRoot != "" {
		workspace = execColony.WorkspaceRoot
		if !sameSessionPath(execColony.WorkspaceRoot, ctxColony.ColonyRoot) {
			fixedRoot = execColony.WorkspaceRoot
		}
	}
	if bee.Worktree && fixedRoot == "" {
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
	if attempt.AttemptID == "" && boundConfig {
		var blocked bool
		attempt, _, blocked, err = trailconfig.AdmitChainAttemptGated(execColony, traceID, "session:"+traceID+":"+agentID, "", "")
		if err != nil {
			return nil, fmt.Errorf("sessions: session attempt: %w", err)
		}
		if blocked {
			return nil, fmt.Errorf("sessions: trail %s has a pending configuration change; new work is held", traceID)
		}
	}
	provenance := attemptProvenanceForSession(attempt)

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

	adapterName, err := bee.ResolveAdapter()
	if err != nil {
		return nil, err
	}

	var rendered, renderedSystem string
	if resuming {
		rendered = strings.TrimSpace(req.Task)
	} else {
		loader, err := prompts.NewLoader(execColony.ConfigRootPath())
		if err != nil {
			return nil, err
		}
		knownIntents, defaultIntent, err := prompts.DiscoverIntents(execColony.ConfigRootPath(), bee)
		if err != nil {
			return nil, fmt.Errorf("sessions: discover intents: %w", err)
		}
		traceTitle, err := runs.ResolveTraceTitle(ctxColony.ColonyRoot, traceID)
		if err != nil {
			return nil, fmt.Errorf("sessions: resolve trace title: %w", err)
		}
		worktreeBranch, err := runs.ResolveWorktreeBranch(ctxColony.ColonyRoot, traceID)
		if err != nil {
			return nil, fmt.Errorf("sessions: resolve worktree branch: %w", err)
		}
		promptCtx := prompts.PromptContext(prompts.Context{
			Bee:            bee.Role,
			TraceID:        traceID,
			TraceTitle:     traceTitle,
			WorktreeBranch: worktreeBranch,
			AgentID:        agentID,
			TaskID:         req.TaskID,
			ColonyRoot:     ctxColony.ColonyRoot,
			Workspace:      workspace,
			Task:           req.Task,
			IntentRaw:      req.Intent,
			Insights:       req.Insights,
			ResultFile:     resultFile,
			ArtifactsDir:   artifacts.DirForPrompt(ctxColony.ColonyRoot, traceID),
			Interactive:    true,
			Adapter:        adapterName,
		}, knownIntents, defaultIntent)

		renderedSystem, err = loader.RenderSystemResolved(prompts.SystemResolveInput{
			BeeLocalTemplate: overlay.SystemTemplate,
			BeeTemplate:      bee.SystemTemplate,
			DefaultTemplate:  manifest.Defaults.SystemTemplate,
		}, promptCtx)
		if err != nil {
			return nil, fmt.Errorf("sessions: render system prompt: %w", err)
		}

		rendered, err = loader.RenderResolved(prompts.ResolveInput{
			InlinePrompt:     req.InlinePrompt,
			BeeLocalTemplate: overlay.PromptTemplate,
			BeeTemplate:      bee.PromptTemplate,
			DefaultTemplate:  manifest.Defaults.PromptTemplate,
			AllowEmpty:       colony.HasSystemTemplate(bee, overlay, manifest.Defaults),
		}, promptCtx)
		if err != nil {
			return nil, fmt.Errorf("sessions: render prompt: %w", err)
		}
	}

	sessAdapter, ok := m.adapters[adapterName]
	if !ok {
		return nil, fmt.Errorf("sessions: adapter %q does not support interactive sessions", adapterName)
	}

	plan, err := launch.ResolveLaunch(launch.Request{
		Mode:         launch.Interactive,
		Adapter:      adapterName,
		Bee:          bee,
		Extra:        colony.AdapterExtraLayer(execColony, adapterName),
		Aliases:      execColony.ModelAliases,
		Workspace:    workspace,
		TaskID:       req.TaskID,
		Prompt:       rendered,
		SystemPrompt: renderedSystem,
		Resume:       resuming,
		RunDir:       runDir,
	})
	if err != nil {
		return nil, err
	}
	for _, warning := range plan.Warnings {
		logging.Component("sessions").Warn("launch warning",
			logging.F("bee", bee.Role),
			logging.F("trace", traceID),
			logging.F("agent", agentID),
			logging.F("warning", warning),
		)
	}

	startedAt := time.Now().UTC()
	if rendered != "" {
		if err := runDir.WritePrompt(rendered); err != nil {
			return nil, err
		}
	}
	if renderedSystem != "" {
		if err := runDir.WriteSystem(renderedSystem); err != nil {
			return nil, fmt.Errorf("sessions: write system: %w", err)
		}
	}
	if err := runDir.WriteMeta(runs.Meta{
		TraceID:           traceID,
		AgentID:           agentID,
		Bee:               bee.Role,
		Adapter:           adapterName,
		Workspace:         workspace,
		ProviderSessionID: req.resumeProviderID,
		ResumedFrom:       req.resumeFrom,
		Profile:           execColony.Profile,
		StartedAt:         startedAt,
		Provenance:        provenance,
	}); err != nil {
		return nil, err
	}
	if err := runDir.WriteRequest(protocol.Request{
		ProtocolVersion:      protocol.Version,
		TraceID:              traceID,
		AgentID:              agentID,
		Bee:                  bee.Role,
		Adapter:              adapterName,
		Workspace:            workspace,
		ColonyRoot:           ctxColony.ColonyRoot,
		TaskID:               req.TaskID,
		Task:                 req.Task,
		Intent:               req.Intent,
		Insights:             req.Insights,
		Model:                plan.ModelID,
		ModelAlias:           plan.ModelAlias,
		Profile:              execColony.Profile,
		AttemptID:            provenance.AttemptID,
		ConfigRef:            execColony.ConfigRef,
		ConfigRevision:       execColony.ConfigRevision,
		InitialCodeBaseSHA:   provenance.InitialCodeBaseSHA,
		InitialCodeBaseDirty: provenance.InitialCodeBaseDirty,
		LaunchForm:           string(plan.Form),
		ResultPath:           resultFile,
		EventLogPath:         runDir.EventsPath(),
		CreatedAt:            startedAt,
	}); err != nil {
		return nil, err
	}
	if err := runDir.WriteStatusSnapshot(protocol.StatusSnapshot{
		ProtocolVersion: protocol.Version,
		State:           protocol.StatusRunning,
		StartedAt:       startedAt,
		Provenance:      provenance,
	}); err != nil {
		return nil, err
	}
	if err := artifacts.CaptureBaseline(ctxColony.ColonyRoot, traceID, agentID); err != nil {
		logging.Component("sessions").Warn("artifacts baseline capture failed",
			logging.F("trace", traceID),
			logging.F("agent", agentID),
			logging.F("error", err.Error()),
		)
	}
	// The workspace baseline is captured here, beside the comb baseline and before the PTY
	// starts, so the same collector rules and attribution limits apply to interactive work
	// as to AFK work. A capture failure leaves a zero baseline, which widens attribution
	// rather than blocking the session.
	baseline, baselineErr := adapters.CaptureWorkspaceBaseline(ctx, workspace)
	if baselineErr != nil {
		logging.Component("sessions").Warn("workspace baseline capture failed",
			logging.F("trace", traceID),
			logging.F("agent", agentID),
			logging.F("error", baselineErr.Error()),
		)
	}

	// Detached attach mode (PTY hub / no local terminal) must still launch the
	// interactive agent TUI. Headless -p belongs to Adapter.Run(), not sessions.
	sessReq := adapters.SessionRequest{
		Bee:             bee.Role,
		InitialPrompt:   rendered,
		SystemPrompt:    renderedSystem,
		ColonyRoot:      ctxColony.ColonyRoot,
		Workspace:       workspace,
		Params:          plan.Params,
		Command:         plan.Command,
		Binary:          plan.Binary,
		Env:             plan.Env,
		Dir:             plan.Dir,
		AdapterName:     adapterName,
		TraceID:         traceID,
		AgentID:         agentID,
		TaskID:          req.TaskID,
		Task:            req.Task,
		Intent:          req.Intent,
		Insights:        req.Insights,
		ResumeSessionID: req.resumeProviderID,
		Provenance:      provenance,
	}

	cmd, err := sessAdapter.SessionCommand(sessReq)
	if err != nil {
		return nil, err
	}
	// The adapter may have started resources before the PTY (for example a
	// Paseka-managed OpenCode server). Until the session is registered they are
	// this function's responsibility; after that the active session owns them.
	cleanup := cmd.Cleanup
	cleanupOwned := false
	defer func() {
		if !cleanupOwned && cleanup != nil {
			cleanup()
		}
	}()

	providerSessionID := strings.TrimSpace(cmd.ProviderSessionID)
	if providerSessionID == "" {
		providerSessionID = strings.TrimSpace(req.resumeProviderID)
	}
	if providerSessionID != "" {
		if err := runDir.WriteMeta(runs.Meta{
			TraceID:           traceID,
			AgentID:           agentID,
			Bee:               bee.Role,
			Adapter:           adapterName,
			Implementation:    cmd.Implementation,
			Workspace:         workspace,
			ProviderSessionID: providerSessionID,
			ResumedFrom:       req.resumeFrom,
			Profile:           execColony.Profile,
			StartedAt:         startedAt,
			Provenance:        provenance,
		}); err != nil {
			return nil, fmt.Errorf("sessions: write meta: %w", err)
		}
	}

	proc, err := startPTY(cmd)
	if err != nil {
		if attempt.Kind == trailconfig.AttemptKindDirect {
			_ = trailconfig.CloseAttempt(ctxColony.ColonyRoot, traceID, attempt.AttemptID, "session start failed")
		}
		return nil, err
	}
	if attempt.AttemptID != "" {
		if _, _, err := trailconfig.MarkAttemptDispatched(ctxColony.ColonyRoot, traceID, attempt.AttemptID, "session:"+agentID); err != nil {
			_ = proc.Kill()
			return nil, fmt.Errorf("sessions: mark attempt dispatch: %w", err)
		}
	}
	if err := trailconfig.RecordAttemptAgent(ctxColony.ColonyRoot, traceID, attempt.AttemptID, agentID); err != nil {
		logging.Component("sessions").Warn("attempt agent record failed", logging.F("error", err.Error()))
	}

	handle := adapters.SessionHandle{
		SessionID:         sessionID,
		TraceID:           traceID,
		AgentID:           agentID,
		RunDir:            runDir.Root(),
		Workspace:         workspace,
		ColonyRoot:        ctxColony.ColonyRoot,
		Bee:               bee.Role,
		Adapter:           adapterName,
		Implementation:    cmd.Implementation,
		Binary:            cmd.Binary,
		Env:               plan.Env,
		Model:             plan.ModelID,
		PID:               proc.PID(),
		State:             adapters.SessionActive,
		StartedAt:         startedAt,
		ProviderSessionID: providerSessionID,
		ResumedFrom:       req.resumeFrom,
		Profile:           execColony.Profile,
	}

	if err := runDir.WriteSession(runs.SessionMeta{
		SessionID:         sessionID,
		TraceID:           traceID,
		AgentID:           agentID,
		Bee:               bee.Role,
		Adapter:           adapterName,
		Implementation:    cmd.Implementation,
		Workspace:         workspace,
		ColonyRoot:        ctxColony.ColonyRoot,
		Model:             plan.ModelID,
		PID:               proc.PID(),
		State:             string(adapters.SessionActive),
		ProviderSessionID: providerSessionID,
		ResumedFrom:       req.resumeFrom,
		Profile:           execColony.Profile,
		StartedAt:         startedAt,
		Provenance:        provenance,
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
	if detached {
		sessCtx, cancel = context.WithCancel(context.Background())
	}

	if cmd.Prompt != nil {
		go deliverSessionPrompt(sessCtx, *cmd.Prompt)
	}

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
		cleanup: cleanup,
		proposal: sessionProposal{
			bee:        bee,
			baseline:   baseline,
			taskID:     req.TaskID,
			task:       req.Task,
			provenance: provenance,
		},
	}

	m.mu.Lock()
	m.sessions[sessionID] = active
	m.mu.Unlock()
	cleanupOwned = true

	if err := homestate.RegisterSession(ctxColony.Slug, homestate.SessionEntry{
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
	active.cancel()

	m.finishSession(sessionID, state, waitErr)
}

// resolveSessionUsage asks the adapter for what an interactive session spent.
// A TUI keeps no machine-readable stdout, so this is best-effort and must never
// delay or fail session teardown.
func (m *Manager) resolveSessionUsage(handle adapters.SessionHandle) *protocol.Usage {
	m.mu.RLock()
	adapter := m.adapters[handle.Adapter]
	m.mu.RUnlock()
	resolver, ok := adapter.(adapters.SessionUsageResolver)
	if !ok {
		return nil
	}
	ctx, cancel := context.WithTimeout(context.Background(), sessionUsageTimeout)
	defer cancel()
	return resolver.SessionUsage(ctx, handle)
}

func (m *Manager) finishSession(sessionID string, state adapters.SessionState, waitErr error) {
	m.mu.Lock()
	active, ok := m.sessions[sessionID]
	if !ok {
		m.mu.Unlock()
		return
	}
	entry := active.entry
	m.mu.Unlock()

	if active.cleanup != nil {
		active.cleanup()
		active.cleanup = nil
	}

	finishedAt := time.Now().UTC()

	providerSessionID := entry.Handle.ProviderSessionID
	resumedFrom := entry.Handle.ResumedFrom
	if existing, err := entry.RunDir.ReadSession(); err == nil {
		if strings.TrimSpace(existing.ProviderSessionID) != "" {
			providerSessionID = existing.ProviderSessionID
		}
		if strings.TrimSpace(existing.ResumedFrom) != "" {
			resumedFrom = existing.ResumedFrom
		}
	}

	sessionMeta := runs.SessionMeta{
		SessionID:         sessionID,
		TraceID:           entry.Handle.TraceID,
		AgentID:           entry.Handle.AgentID,
		Bee:               entry.Handle.Bee,
		Adapter:           entry.Handle.Adapter,
		Implementation:    entry.Handle.Implementation,
		Workspace:         entry.Handle.Workspace,
		ColonyRoot:        entry.Handle.ColonyRoot,
		Model:             entry.Handle.Model,
		State:             string(state),
		ProviderSessionID: providerSessionID,
		ResumedFrom:       resumedFrom,
		Profile:           entry.Handle.Profile,
		StartedAt:         entry.Handle.StartedAt,
		FinishedAt:        finishedAt,
		Provenance:        entry.handleProvenance(),
	}
	_ = entry.RunDir.WriteSession(sessionMeta)

	// Provider accounting can mean starting a short-lived provider server, so it
	// follows the state transition instead of delaying it.
	if usage := m.resolveSessionUsage(entry.Handle); usage != nil {
		sessionMeta.Usage = usage
		_ = entry.RunDir.WriteSession(sessionMeta)
	}

	exitCode := 0
	statusErr := ""
	if waitErr != nil {
		statusErr = waitErr.Error()
		exitCode = 1
	}

	protoState := protocol.StatusCompleted
	switch state {
	case adapters.SessionFailed:
		protoState = protocol.StatusFailed
	case adapters.SessionCancelled:
		protoState = protocol.StatusCancelled
	}

	resultText := buildSessionResultText(entry.RunDir)
	_ = entry.RunDir.WriteResultText(resultText)

	var proposalPub bus.Publisher
	if state == adapters.SessionCompleted {
		ctxColony := colony.EnrichFromHome(colony.Context{ColonyRoot: entry.Handle.ColonyRoot, Slug: entry.Slug})
		if err := bus.FlushPendingForRun(context.Background(), ctxColony, entry.Handle.TraceID, entry.Handle.AgentID); err != nil {
			msg := "runtime: flush deferred events: " + err.Error()
			logging.Component("sessions").Warn("flush deferred events failed",
				logging.F("bee", entry.Handle.Bee),
				logging.F("trace", entry.Handle.TraceID),
				logging.F("agent", entry.Handle.AgentID),
				logging.F("error", err.Error()),
			)
			statusErr = joinStatusNotes(statusErr, msg)
		}
		var pub bus.Publisher = bus.LocalPublisher{ColonyRoot: entry.Handle.ColonyRoot}
		if client, err := bus.ConnectColony(ctxColony, false); err == nil && client != nil {
			pub = client
			defer client.Close()
		}
		proposalPub = pub
		if err := flushSessionArtifacts(context.Background(), entry.Handle.ColonyRoot, entry.Handle.TraceID, entry.Handle.AgentID, pub); err != nil {
			msg := "runtime: flush artifacts: " + err.Error()
			logging.Component("sessions").Warn("flush artifacts failed",
				logging.F("trace", entry.Handle.TraceID),
				logging.F("agent", entry.Handle.AgentID),
				logging.F("error", err.Error()),
			)
			statusErr = joinStatusNotes(statusErr, msg)
		}
	} else if warn, ok := bus.PendingWarning(entry.Handle.ColonyRoot, entry.Handle.TraceID, entry.Handle.AgentID); ok {
		logging.Component("sessions").Warn(warn,
			logging.F("trace", entry.Handle.TraceID),
			logging.F("agent", entry.Handle.AgentID),
		)
		statusErr = joinStatusNotes(statusErr, warn)
	}

	_ = entry.RunDir.WriteStatusSnapshot(protocol.StatusSnapshot{
		ProtocolVersion: protocol.Version,
		State:           protoState,
		ExitCode:        exitCode,
		StartedAt:       entry.Handle.StartedAt,
		FinishedAt:      finishedAt,
		Error:           statusErr,
		Provenance:      entry.handleProvenance(),
	})
	_ = entry.RunDir.AppendTranscript(runs.TranscriptEntry{
		At:      finishedAt,
		Role:    "system",
		Content: fmt.Sprintf("session %s", state),
	})

	if bee, _, beeErr := colony.LoadBee(entry.Handle.ColonyRoot, entry.Handle.Bee); beeErr == nil {
		prompt, _ := os.ReadFile(entry.RunDir.PromptPath())
		req, _ := entry.RunDir.ReadRequest()
		vars := colony.CommandVars{
			Prompt:     string(prompt),
			Workspace:  entry.Handle.Workspace,
			TraceID:    entry.Handle.TraceID,
			AgentID:    entry.Handle.AgentID,
			TaskID:     req.TaskID,
			ColonyRoot: entry.Handle.ColonyRoot,
			Result:     strings.TrimSpace(resultText),
			ResultFile: entry.RunDir.ResultPath(),
			Meta:       entry.RunDir.MetaPath(),
			RunDir:     entry.RunDir.Root(),
			Env:        adapters.EnvMap(entry.Handle.Env),
		}
		if err := colony.RunPostExec(context.Background(), bee.PostExec, vars, entry.Handle.Workspace, entry.Handle.Env); err != nil {
			logging.Component("sessions").Warn("post_exec failed",
				logging.F("bee", entry.Handle.Bee),
				logging.F("trace", entry.Handle.TraceID),
				logging.F("agent", entry.Handle.AgentID),
				logging.F("error", err.Error()),
			)
		}
	}

	m.finalizeSessionProposal(active, state, proposalPub)

	_ = homestate.UnregisterSession(entry.Slug, sessionID)
	_ = homestate.MarkInviteIncompleteOnSessionEnd(entry.Slug, sessionID)

	m.mu.Lock()
	delete(m.sessions, sessionID)
	m.mu.Unlock()

	close(active.done)
}

func attemptProvenanceForSession(attempt trailconfig.Attempt) protocol.Provenance {
	return protocol.Provenance{
		AttemptID:            attempt.AttemptID,
		ConfigRef:            attempt.ConfigRef,
		ConfigRevision:       attempt.ConfigRevision,
		InitialCodeBaseSHA:   attempt.InitialCodeBaseSHA,
		InitialCodeBaseDirty: attempt.InitialCodeBaseDirty,
	}
}

// handleProvenance rereads the run request so a resumed or remote handle still
// carries the attempt recorded at launch.
func (e Entry) handleProvenance() protocol.Provenance {
	req, err := e.RunDir.ReadRequest()
	if err != nil {
		return protocol.Provenance{}
	}
	return protocol.Provenance{
		AttemptID:            req.AttemptID,
		ConfigRef:            req.ConfigRef,
		ConfigRevision:       req.ConfigRevision,
		InitialCodeBaseSHA:   req.InitialCodeBaseSHA,
		InitialCodeBaseDirty: req.InitialCodeBaseDirty,
	}
}

func joinStatusNotes(existing, note string) string {
	note = strings.TrimSpace(note)
	if note == "" {
		return existing
	}
	if strings.TrimSpace(existing) != "" {
		return existing + "; " + note
	}
	return note
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
	entry, err := homestate.FindSession(slug, sessionID)
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
	_ = homestate.UnregisterSession(slug, sessionID)
	_ = homestate.MarkInviteIncompleteOnSessionEnd(slug, sessionID)
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

// AttachPTY subscribes to raw PTY output for a detached session in this process.
func (m *Manager) AttachPTY(sessionID string) (*PTYStream, error) {
	m.mu.RLock()
	active, ok := m.sessions[sessionID]
	m.mu.RUnlock()
	if !ok {
		return nil, fmt.Errorf("sessions: session %q not active in this process", sessionID)
	}
	if active.hub == nil {
		return nil, fmt.Errorf("sessions: session %q has no pty hub (not detached)", sessionID)
	}
	return active.hub.subscribe(active.done), nil
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

// sameSessionPath reports whether two execution roots identify the same directory.
func sameSessionPath(a, b string) bool {
	if a == "" || b == "" {
		return false
	}
	absA, errA := filepath.Abs(a)
	absB, errB := filepath.Abs(b)
	if errA == nil && errB == nil {
		return absA == absB
	}
	return filepath.Clean(a) == filepath.Clean(b)
}

// DefaultManager is the process-wide session manager for CLI attach/stop.
var DefaultManager = NewManager()
