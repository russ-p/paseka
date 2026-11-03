package runtime

import (
	"context"
	"fmt"
	"path/filepath"
	"strings"
	"time"

	"github.com/russ-p/paseka/internal/adapters"
	"github.com/russ-p/paseka/internal/artifacts"
	"github.com/russ-p/paseka/internal/bus"
	"github.com/russ-p/paseka/internal/colony"
	"github.com/russ-p/paseka/internal/launch"
	"github.com/russ-p/paseka/internal/logging"
	"github.com/russ-p/paseka/internal/prompts"
	"github.com/russ-p/paseka/internal/protocol"
	"github.com/russ-p/paseka/internal/runs"
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
	plan           launch.LaunchPlan
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
	configRoot := req.ConfigRoot
	if configRoot == "" {
		configRoot = colonyRoot
	} else {
		configRoot, err = filepath.Abs(configRoot)
		if err != nil {
			return nil, err
		}
	}
	if req.Bee == "" {
		return nil, fmt.Errorf("runtime: bee role is required")
	}
	if req.TraceID == "" {
		return nil, fmt.Errorf("runtime: traceId is required")
	}
	if req.Registry == nil {
		req.Registry = d.registry
	}
	// A direct caller of prepareDispatch bypasses DispatchColonyBee, so admission
	// attribution is resolved here too. An empty purpose on a new dispatch is
	// ordinary work; it is not the same as a legacy record that has no purpose.
	purpose, analysisOf, err := protocol.ResolveAdmissionAttribution(req.Purpose, req.AnalysisOf)
	if err != nil {
		return nil, fmt.Errorf("runtime: %w", err)
	}
	req.Purpose = purpose
	req.AnalysisOf = analysisOf

	agentID := req.AgentID
	if agentID == "" {
		id, err := colony.NewAgentID()
		if err != nil {
			return nil, err
		}
		agentID = id
	}

	manifest, err := colony.LoadColony(configRoot)
	if err != nil {
		return nil, err
	}
	bee, overlay, err := colony.LoadBee(configRoot, req.Bee)
	if err != nil {
		return nil, err
	}
	if req.ProfileApply != nil {
		bee, err = req.ProfileApply(bee)
		if err != nil {
			return nil, err
		}
	}
	// A runtime-owned policy may pin run params for its own task — the closing
	// round's `task.params`. It overlays the committed bee, so the closing Bee
	// can stay cheap by default while the wrap-up names a stronger model. The
	// params were validated with the colony at load and pinned with the trail.
	if len(req.PolicyParams) > 0 {
		bee = colony.WithParams(bee, req.PolicyParams)
	}
	if err := bee.ValidateAdapterRequirements(); err != nil {
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

	loader, err := prompts.NewLoader(configRoot)
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

	knownIntents, defaultIntent, err := prompts.DiscoverIntents(configRoot, bee)
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
		SuppressPRBody: req.SuppressPRBody,
		TrailInventory: req.TrailInventory,
		Adapter:        adapterName,
	}, knownIntents, defaultIntent)

	// An analysis studies captured evidence, not a live worktree. Name the
	// studied milestone and the retained files so the run still has its input
	// after delivery removed the source checkout.
	if req.Purpose == protocol.PurposeAnalysis && req.AnalysisOf != "" {
		promptCtx.AnalysisOf = req.AnalysisOf
		promptCtx.RetainedEvidence = runs.CompletionEvidencePaths(colonyRoot, req.TraceID, req.AnalysisOf)
	}

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

	aliases := req.ModelAliases
	if len(aliases) == 0 {
		aliases = manifest.ModelAliases
	}

	plan, err := launch.ResolveLaunch(launch.Request{
		Mode:         launch.AFK,
		Adapter:      adapterName,
		Bee:          bee,
		Extra:        req.AdapterExtra,
		Aliases:      aliases,
		Workspace:    workspace,
		TaskID:       req.TaskID,
		Prompt:       rendered,
		SystemPrompt: renderedSystem,
		RunDir:       runDir,
		Raw:          req.Raw,
	})
	if err != nil {
		return nil, err
	}
	for _, warning := range plan.Warnings {
		runtimeLog.Warn("launch warning",
			logging.F("bee", bee.Role),
			logging.F("trace", req.TraceID),
			logging.F("agent", agentID),
			logging.F("warning", warning),
		)
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
		ProtocolVersion:      protocol.Version,
		TraceID:              req.TraceID,
		AgentID:              agentID,
		Bee:                  bee.Role,
		Adapter:              adapterName,
		Workspace:            workspace,
		ColonyRoot:           colonyRoot,
		TaskID:               req.TaskID,
		Task:                 req.Task,
		Intent:               req.Intent,
		Insights:             insights,
		Model:                plan.ModelID,
		ModelAlias:           plan.ModelAlias,
		Profile:              req.Profile,
		AttemptID:            req.AttemptID,
		ConfigRef:            req.ConfigRef,
		ConfigRevision:       req.ConfigRevision,
		InitialCodeBaseSHA:   req.InitialCodeBaseSHA,
		InitialCodeBaseDirty: req.InitialCodeBaseDirty,
		LaunchForm:           string(plan.Form),
		Purpose:              req.Purpose,
		AnalysisOf:           req.AnalysisOf,
		ResultPath:           resultFile,
		EventLogPath:         runDir.EventsPath(),
		CreatedAt:            createdAt,
	}); err != nil {
		return nil, fmt.Errorf("runtime: write request: %w", err)
	}
	if err := runDir.WriteStatusSnapshot(protocol.StatusSnapshot{
		ProtocolVersion: protocol.Version,
		State:           protocol.StatusQueued,
		StartedAt:       createdAt,
		Provenance: protocol.Provenance{
			AttemptID:            req.AttemptID,
			ConfigRef:            req.ConfigRef,
			ConfigRevision:       req.ConfigRevision,
			InitialCodeBaseSHA:   req.InitialCodeBaseSHA,
			InitialCodeBaseDirty: req.InitialCodeBaseDirty,
		},
	}); err != nil {
		return nil, fmt.Errorf("runtime: write status: %w", err)
	}

	if req.Raw {
		if degraded := adapters.RawOutputDegradedFeatures(adapterName); len(degraded) > 0 {
			note := "raw output: degraded " + strings.Join(degraded, ", ")
			runtimeLog.Warn("raw output: structured parsing disabled for this run",
				logging.F("adapter", adapterName),
				logging.F("bee", bee.Role),
				logging.F("trace", req.TraceID),
				logging.F("agent", agentID),
				logging.F("degraded", strings.Join(degraded, ", ")),
			)
			_ = runDir.AppendStatusNote(note)
		}
	}

	runtimeLog.Info("adapter run",
		logging.F("adapter", adapterName),
		logging.F("bee", bee.Role),
		logging.F("trace", req.TraceID),
		logging.F("agent", agentID),
		logging.F("workspace", workspace),
		logging.F("run_dir", RelRunDir(colonyRoot, runDir.Root())),
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
		plan:           plan,
		runDir:         runDir,
		baseline:       baseline,
		insights:       insights,
		req:            filled,
	}, nil
}

func (req DispatchRequest) provenance() protocol.Provenance {
	return protocol.Provenance{
		AttemptID:            req.AttemptID,
		ConfigRef:            req.ConfigRef,
		ConfigRevision:       req.ConfigRevision,
		InitialCodeBaseSHA:   req.InitialCodeBaseSHA,
		InitialCodeBaseDirty: req.InitialCodeBaseDirty,
	}
}

func (d *Dispatcher) runPrepared(ctx context.Context, p *preparedDispatch) (*adapters.RunResult, error) {
	// params.timeout is measured from launch: bound the run context so the
	// adapter process is brought down (SIGTERM, then SIGKILL after the grace)
	// and reports cancelled/timeout instead of holding a worker slot forever.
	if p.plan.Timeout > 0 {
		var cancel context.CancelFunc
		ctx, cancel = context.WithTimeout(ctx, p.plan.Timeout)
		defer cancel()
	}
	return p.adapter.Run(ctx, adapters.RunRequest{
		Bee:          p.bee.Role,
		Prompt:       p.rendered,
		SystemPrompt: p.renderedSystem,
		ColonyRoot:   p.colonyRoot,
		Workspace:    p.workspace,
		Sector:       p.req.Sector,
		SectorPath:   p.req.SectorPath,
		Params:       p.plan.Params,
		Command:      p.plan.Command,
		Binary:       p.plan.Binary,
		Env:          p.plan.Env,
		Dir:          p.plan.Dir,
		AdapterName:  p.adapterName,
		Raw:          p.plan.Raw,
		RedactedEnv:  p.plan.RedactedEnv,
		TraceID:      p.req.TraceID,
		AgentID:      p.agentID,
		TaskID:       p.req.TaskID,
		Task:         p.req.Task,
		Intent:       p.req.Intent,
		Insights:     p.insights,
		Profile:      p.req.Profile,
		Provenance:   p.req.provenance(),
	})
}

func (d *Dispatcher) finalizeDispatch(ctx context.Context, p *preparedDispatch, result *adapters.RunResult) (*adapters.RunResult, error) {
	if summary := strings.TrimSpace(result.Summary); summary != "" {
		_ = p.runDir.WriteResultText(summary)
	}

	// Finalization owns the persisted outcome. An adapter normally writes
	// result.json before it returns, but an adapter that did not leaves the
	// runtime with nowhere to record its own verdict; materialize the record
	// first so the gate and contract corrections below have both files to
	// reconcile.
	if err := p.runDir.EnsureResultRecord(p.resultRecord(result)); err != nil {
		return result, fmt.Errorf("runtime: write result record: %w", err)
	}

	// The mechanical gate decides before anything leaves the run: it runs after
	// the adapter exits and before the deferred-event flush, so a queued proposal
	// cannot reach the bus ahead of a rejection. It also runs before run-summary
	// synthesis and completion-contract evaluation, so both see the gate verdict
	// rather than a stale completed status.
	gateRejection, gateErr := d.runCheckGate(ctx, p, result)
	if gateErr != nil {
		return result, gateErr
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
	// An unreadable event record cannot prove whether a run.summary already
	// exists, and appending to it would be a write into unknown state. Skip
	// synthesis and let the required-summary check and the completion contract
	// report what is known: a missing summary is its own runtime check and an
	// unevaluable contract is unavailable.
	if readErr == nil {
		updatedEvents, synthesizedEvent, synthErr := d.synthesizeRunSummary(
			p.runDir, p.bee, p.req.Registry, p.req.TraceID, p.agentID, p.req.TaskID, result, runEvents,
		)
		if synthErr != nil {
			return result, synthErr
		}
		runEvents = updatedEvents
		if synthesizedEvent != nil {
			synthesized = append(synthesized, *synthesizedEvent)
		}
	}
	// The rejection rides the same synthesized-event list as the run summary, so
	// the existing publication pass publishes it, records it in the run's event
	// log, and the trail log attributes it to the runtime.
	if gateRejection != nil {
		synthesized = append(synthesized, *gateRejection)
	}

	if summaryNote, missing := d.enforceRunSummaryRequired(p.bee, p.agentID, result, runEvents); missing {
		// A required-summary miss is persisted like any other runtime-owned
		// verdict, but under its own reason: it must never read as a
		// completion-contract failure.
		if err := p.runDir.ReconcileRunRecords(runs.RunRecordReconciliation{
			State:  protocol.StatusFailed,
			Reason: protocol.ReasonRunSummaryMissing,
			Note:   summaryNote,
		}); err != nil {
			return result, fmt.Errorf("runtime: reconcile run summary record: %w", err)
		}
	}
	// The contract is evaluated against a strict read. The lenient read above
	// still serves summary synthesis and proposal detection, where one damaged
	// line should not hide the rest of the run; a contract is different,
	// because the lost line may be the very event it requires.
	contractEvents, contractErr := runEvents, readErr
	if readErr == nil {
		if strict, err := p.runDir.ReadEventsStrict(); err != nil {
			contractEvents, contractErr = nil, err
		} else {
			contractEvents = strict
		}
	}
	if err := d.enforceCompletionContract(p.runDir, p.bee, contractEvents, contractErr, result); err != nil {
		return result, err
	}

	d.runPostExec(ctx, p.bee, p.rendered, p.workspace, p.runDir, p.req.TaskID, result, p.plan.Env)

	// The proposal gate reads the final status, not just a nil adapter error: a run the
	// completion contract downgraded, a cancelled run, and a timed-out run all reach here
	// having executed partly, and none of them is a completed proposal attempt.
	explicitProposal := runOpenedCodeProposalEvent(runEvents) || runOpenedCodeProposalEvent(result.Events)
	if !explicitProposal {
		// A deferred proposal is still the run's own proposal even when the flush failed
		// and left it in the queue: synthesizing a second one would dispatch a Guard twice.
		if pending, err := p.runDir.ReadPending(); err == nil {
			explicitProposal = runOpenedCodeProposalEvent(pending)
		}
	}

	if pubErr := d.publishRunOutcome(ctx, DispatchRequest{
		ColonyRoot:           p.colonyRoot,
		ConfigRoot:           p.req.ConfigRoot,
		ConfigRef:            p.req.ConfigRef,
		ConfigRevision:       p.req.ConfigRevision,
		AttemptID:            p.req.AttemptID,
		InitialCodeBaseSHA:   p.req.InitialCodeBaseSHA,
		InitialCodeBaseDirty: p.req.InitialCodeBaseDirty,
		Bee:                  p.req.Bee,
		TraceID:              p.req.TraceID,
		AgentID:              p.agentID,
		TaskID:               p.req.TaskID,
		Sector:               p.req.Sector,
		Workspace:            p.workspace,
		Registry:             p.req.Registry,
	}, p.bee, p.baseline, result, synthesized, explicitProposal); pubErr != nil {
		msg := "runtime: proposal finalization: " + pubErr.Error()
		if d.busRequired {
			return result, pubErr
		}
		result.Warnings = append(result.Warnings, msg)
		_ = p.runDir.AppendStatusNote(msg)
	}
	return result, nil
}
