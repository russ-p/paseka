package runtime

import (
	"context"
	"fmt"
	"path/filepath"
	"strings"
	"time"

	"github.com/paseka/paseka/internal/adapters"
	"github.com/paseka/paseka/internal/adapters/cursor"
	"github.com/paseka/paseka/internal/artifacts"
	"github.com/paseka/paseka/internal/bus"
	"github.com/paseka/paseka/internal/colony"
	"github.com/paseka/paseka/internal/logging"
	"github.com/paseka/paseka/internal/prompts"
	"github.com/paseka/paseka/internal/protocol"
	"github.com/paseka/paseka/internal/runs"
)

// preparedDispatch holds everything needed after Prepare through Finalize.
type preparedDispatch struct {
	dispatcher     *Dispatcher
	colonyRoot     string
	workspace      string
	agentID        string
	bee            colony.Bee
	adapter        adapters.Adapter
	adapterName    string
	rendered       string
	renderedSystem string
	params         adapters.RunParams
	command        []string
	runDir         runs.Dir
	baseline       adapters.WorkspaceBaseline
	insights       []string
	req            DispatchRequest
}

func (d *Dispatcher) prepareDispatch(ctx context.Context, req DispatchRequest) (*preparedDispatch, error) {
	if req.ColonyRoot == "" {
		return nil, fmt.Errorf("runtime: colony root is required")
	}
	colonyRoot, err := filepath.Abs(req.ColonyRoot)
	if err != nil {
		return nil, err
	}
	if req.Bee == "" {
		return nil, fmt.Errorf("runtime: bee role is required")
	}
	if req.TraceID == "" {
		return nil, fmt.Errorf("runtime: traceId is required")
	}

	agentID := req.AgentID
	if agentID == "" {
		id, err := colony.NewAgentID()
		if err != nil {
			return nil, err
		}
		agentID = id
	}

	manifest, err := colony.LoadColony(colonyRoot)
	if err != nil {
		return nil, err
	}
	bee, overlay, err := colony.LoadBee(colonyRoot, req.Bee)
	if err != nil {
		return nil, err
	}

	workspace := req.Workspace
	if workspace == "" {
		workspace = colonyRoot
	} else {
		workspace, err = filepath.Abs(workspace)
		if err != nil {
			return nil, err
		}
	}

	runDir := runs.Dir{
		ColonyRoot: colonyRoot,
		TraceID:    req.TraceID,
		AgentID:    agentID,
	}
	resultFile, err := filepath.Abs(runDir.ResultPath())
	if err != nil {
		return nil, err
	}

	loader, err := prompts.NewLoader(colonyRoot)
	if err != nil {
		return nil, err
	}

	projectedInsights, err := GatherPromptInsights(colonyRoot, req.TraceID, req.TaskID)
	if err != nil {
		return nil, fmt.Errorf("runtime: gather insights: %w", err)
	}
	insights := MergeInsights(req.Insights, projectedInsights)

	traceTitle, err := runs.ResolveTraceTitle(colonyRoot, req.TraceID)
	if err != nil {
		return nil, fmt.Errorf("runtime: resolve trace title: %w", err)
	}
	worktreeBranch, err := runs.ResolveWorktreeBranch(colonyRoot, req.TraceID)
	if err != nil {
		return nil, fmt.Errorf("runtime: resolve worktree branch: %w", err)
	}

	adapterName, err := bee.ResolveAdapter()
	if err != nil {
		return nil, err
	}

	knownIntents, defaultIntent, err := prompts.DiscoverIntents(colonyRoot, bee)
	if err != nil {
		return nil, fmt.Errorf("runtime: discover intents: %w", err)
	}

	promptCtx := prompts.PromptContext(prompts.Context{
		Bee:            bee.Role,
		TraceID:        req.TraceID,
		TraceTitle:     traceTitle,
		WorktreeBranch: worktreeBranch,
		AgentID:        agentID,
		TaskID:         req.TaskID,
		ColonyRoot:     colonyRoot,
		Workspace:      workspace,
		Sector:         req.Sector,
		SectorPath:     req.SectorPath,
		Task:           req.Task,
		IntentRaw:      req.Intent,
		Insights:       insights,
		ResultFile:     resultFile,
		ArtifactsDir:   artifacts.DirForPrompt(colonyRoot, req.TraceID),
		Interactive:    false,
		IsLastWorkTask: req.IsLastWorkTask,
		Adapter:        adapterName,
	}, knownIntents, defaultIntent)

	renderedSystem, err := loader.RenderSystemResolved(prompts.SystemResolveInput{
		BeeLocalTemplate: overlay.SystemTemplate,
		BeeTemplate:      bee.SystemTemplate,
		DefaultTemplate:  manifest.Defaults.SystemTemplate,
	}, promptCtx)
	if err != nil {
		return nil, fmt.Errorf("runtime: render system prompt: %w", err)
	}

	resolveInput := prompts.ResolveInput{
		InlinePrompt:     req.InlinePrompt,
		BeeLocalTemplate: overlay.PromptTemplate,
		BeeTemplate:      bee.PromptTemplate,
		DefaultTemplate:  manifest.Defaults.PromptTemplate,
	}
	if adapterName == "script" {
		resolveInput.SkipDefaults = true
		resolveInput.AllowEmpty = true
	}

	rendered, err := loader.RenderResolved(resolveInput, promptCtx)
	if err != nil {
		return nil, fmt.Errorf("runtime: render prompt: %w", err)
	}

	adapter, ok := d.adapters[adapterName]
	if !ok {
		return nil, fmt.Errorf("runtime: adapter %q not registered", adapterName)
	}

	params := colony.MergeRunParams(colony.RunParamsFromBee(bee), req.AdapterExtra)
	aliases := req.ModelAliases
	if len(aliases) == 0 {
		aliases = manifest.ModelAliases
	}
	colony.ApplyModelAliases(&params, aliases)

	runDirPath := runDir.Root()
	commandPrompt := rendered
	if adapterName == "cursor" {
		commandPrompt = cursor.JoinPrompt(renderedSystem, rendered)
	}
	cmdVars := colony.CommandVars{
		Prompt:       commandPrompt,
		SystemPrompt: renderedSystem,
		SystemFile:   runDir.SystemPath(),
		Workspace:    workspace,
		TraceID:      req.TraceID,
		AgentID:      agentID,
		TaskID:       req.TaskID,
		ColonyRoot:   colonyRoot,
		ResultFile:   resultFile,
		RunDir:       runDirPath,
	}

	var command []string
	if bee.Command.IsSet() {
		if bee.HasParams() {
			runtimeLog.Warn("bee command overrides params",
				logging.F("bee", bee.Role),
				logging.F("trace", req.TraceID),
				logging.F("agent", agentID),
			)
		}
		command, err = bee.Command.RenderCommand(cmdVars)
		if err != nil {
			return nil, fmt.Errorf("runtime: render command: %w", err)
		}
	} else if adapterName == "script" {
		return nil, fmt.Errorf("runtime: bee %q: adapter script requires command", bee.Role)
	}

	if err := runDir.Prepare(); err != nil {
		return nil, fmt.Errorf("runtime: prepare run dir: %w", err)
	}
	if renderedSystem != "" {
		if err := runDir.WriteSystem(renderedSystem); err != nil {
			return nil, fmt.Errorf("runtime: write system: %w", err)
		}
	}

	createdAt := time.Now().UTC()
	if err := runDir.WriteRequest(protocol.Request{
		ProtocolVersion: protocol.Version,
		TraceID:         req.TraceID,
		AgentID:         agentID,
		Bee:             bee.Role,
		Adapter:         adapterName,
		Workspace:       workspace,
		ColonyRoot:      colonyRoot,
		TaskID:          req.TaskID,
		Task:            req.Task,
		Intent:          req.Intent,
		Insights:        insights,
		ResultPath:      resultFile,
		EventLogPath:    runDir.EventsPath(),
		CreatedAt:       createdAt,
	}); err != nil {
		return nil, fmt.Errorf("runtime: write request: %w", err)
	}
	if err := runDir.WriteStatusSnapshot(protocol.StatusSnapshot{
		ProtocolVersion: protocol.Version,
		State:           protocol.StatusQueued,
		StartedAt:       createdAt,
	}); err != nil {
		return nil, fmt.Errorf("runtime: write status: %w", err)
	}

	runtimeLog.Info("adapter run",
		logging.F("adapter", adapterName),
		logging.F("bee", bee.Role),
		logging.F("trace", req.TraceID),
		logging.F("agent", agentID),
		logging.F("workspace", workspace),
		logging.F("run_dir", RelRunDir(colonyRoot, runDirPath)),
	)

	baseline, baselineErr := adapters.CaptureWorkspaceBaseline(ctx, workspace)
	if baselineErr != nil {
		runtimeLog.Warn("workspace baseline capture failed", logging.F("error", baselineErr.Error()))
	}
	captureArtifactsBaseline(colonyRoot, req.TraceID, agentID)

	filled := req
	filled.ColonyRoot = colonyRoot
	filled.AgentID = agentID
	filled.Workspace = workspace

	return &preparedDispatch{
		dispatcher:     d,
		colonyRoot:     colonyRoot,
		workspace:      workspace,
		agentID:        agentID,
		bee:            bee,
		adapter:        adapter,
		adapterName:    adapterName,
		rendered:       rendered,
		renderedSystem: renderedSystem,
		params:         params,
		command:        command,
		runDir:         runDir,
		baseline:       baseline,
		insights:       insights,
		req:            filled,
	}, nil
}

func (d *Dispatcher) runPrepared(ctx context.Context, p *preparedDispatch) (*adapters.RunResult, error) {
	return p.adapter.Run(ctx, adapters.RunRequest{
		Bee:          p.bee.Role,
		Prompt:       p.rendered,
		SystemPrompt: p.renderedSystem,
		ColonyRoot:   p.colonyRoot,
		Workspace:    p.workspace,
		Sector:       p.req.Sector,
		SectorPath:   p.req.SectorPath,
		Params:       p.params,
		Command:      p.command,
		TraceID:      p.req.TraceID,
		AgentID:      p.agentID,
		TaskID:       p.req.TaskID,
		Task:         p.req.Task,
		Intent:       p.req.Intent,
		Insights:     p.insights,
	})
}

func (d *Dispatcher) finalizeDispatch(ctx context.Context, p *preparedDispatch, result *adapters.RunResult) (*adapters.RunResult, error) {
	if summary := strings.TrimSpace(result.Summary); summary != "" {
		_ = p.runDir.WriteResultText(summary)
	}

	if result.Status == string(protocol.StatusCompleted) {
		if err := d.flushDeferredEvents(ctx, p.colonyRoot, p.req.TraceID, p.agentID); err != nil {
			msg := "runtime: flush deferred events: " + err.Error()
			if d.busRequired {
				return result, fmt.Errorf("%s", msg)
			}
			result.Warnings = append(result.Warnings, msg)
			_ = p.runDir.AppendStatusNote(msg)
		}
		if err := d.flushArtifactDelta(ctx, p.colonyRoot, p.req.TraceID, p.agentID); err != nil {
			msg := "runtime: flush artifacts: " + err.Error()
			if d.busRequired {
				return result, fmt.Errorf("%s", msg)
			}
			result.Warnings = append(result.Warnings, msg)
			_ = p.runDir.AppendStatusNote(msg)
		}
	} else if warn, ok := bus.PendingWarning(p.colonyRoot, p.req.TraceID, p.agentID); ok {
		result.Warnings = append(result.Warnings, warn)
		_ = p.runDir.AppendStatusNote(warn)
	}

	runEvents, readErr := p.runDir.ReadEvents()
	if readErr != nil {
		runEvents = nil
	}

	var synthesized []protocol.Event
	updatedEvents, synthesizedEvent, synthErr := d.synthesizeRunSummary(
		p.runDir, p.bee, p.req.TraceID, p.agentID, p.req.TaskID, result, runEvents,
	)
	if synthErr != nil {
		return result, synthErr
	}
	runEvents = updatedEvents
	if synthesizedEvent != nil {
		synthesized = append(synthesized, *synthesizedEvent)
	}

	d.enforceRunSummaryRequired(p.bee, p.agentID, result, runEvents)
	if readErr == nil {
		d.enforceCompletionContract(p.bee, runEvents, result)
	}

	d.runPostExec(ctx, p.bee, p.rendered, p.workspace, p.runDir, p.req.TaskID, result)

	if pubErr := d.publishRunOutcome(ctx, DispatchRequest{
		ColonyRoot: p.colonyRoot,
		Bee:        p.req.Bee,
		TraceID:    p.req.TraceID,
		AgentID:    p.agentID,
		TaskID:     p.req.TaskID,
		Sector:     p.req.Sector,
		Workspace:  p.workspace,
	}, p.bee, p.baseline, result, synthesized); pubErr != nil {
		if d.busRequired {
			return result, pubErr
		}
	}
	return result, nil
}
