package runtime

import (
	"context"
	"fmt"
	"path/filepath"

	"github.com/russ-p/paseka/internal/colony"
	"github.com/russ-p/paseka/internal/logging"
	"github.com/russ-p/paseka/internal/protocol"
	"github.com/russ-p/paseka/internal/runs"
	"github.com/russ-p/paseka/internal/trailconfig"
	"github.com/russ-p/paseka/internal/trailfork"
	"github.com/russ-p/paseka/internal/trailview"
	"github.com/russ-p/paseka/internal/worktree"
)

// DispatchMode labels reactor dispatch paths in logs.
type DispatchMode string

const (
	DispatchModeTask   DispatchMode = "task"
	DispatchModeDirect DispatchMode = "direct"
	DispatchModeCLI    DispatchMode = "cli"
)

// ColonyDispatchRequest dispatches one bee using a resolved colony context.
type ColonyDispatchRequest struct {
	Bee            string
	TraceID        string
	Task           string
	TaskID         string
	Sector         string
	Intent         string
	Insights       []string
	InlinePrompt   string
	IsLastWorkTask bool // AFK ledger task dispatch only; ignored for CLI/direct
	// SuppressPRBody is honoured on AFK ledger task dispatch only.
	SuppressPRBody bool
	// TrailInventory is the structured task list of the trail, set for a
	// closing-phase dispatch.
	TrailInventory colony.TrailInventory
	// PolicyParams are the run params of a runtime-owned policy, set for a
	// closing-phase dispatch. They overlay the bee's committed params.
	PolicyParams map[string]any
	// ProposalKind, when set, resolves adapter cwd from the proposal MUTATION kind
	// (isolated/alias → trace worktree; root → colony root) instead of bee.Worktree.
	ProposalKind string
	// Provenance is the owning attempt and configuration snapshot, when the
	// caller already admitted one.
	Provenance protocol.Provenance
	// Raw requests plain-text adapter output for this run only (CLI --raw).
	Raw bool
}

// DispatchColonyBee prepares workspace and runs one bee without re-resolving colony context.
func (d *Dispatcher) DispatchColonyBee(ctx context.Context, ctxColony colony.Context, req ColonyDispatchRequest, mode DispatchMode) (*BeeRunResult, error) {
	if req.Bee == "" {
		return nil, fmt.Errorf("runtime: bee role is required")
	}
	if req.TraceID == "" {
		return nil, fmt.Errorf("runtime: traceId is required")
	}
	execColony := ctxColony
	scope, err := trailview.ResolveScope(ctxColony, req.TraceID)
	if err != nil {
		return nil, fmt.Errorf("runtime: resolve trail scope: %w", err)
	}
	if scope.AdmissionState == trailfork.StateDropped {
		return nil, fmt.Errorf("runtime: trail %s was dropped and cannot dispatch", req.TraceID)
	}
	// A fork whose creation is incomplete has no published initial task and is
	// not yet a runnable trail.
	if scope.AdmissionState == trailfork.StateCreating {
		return nil, fmt.Errorf("runtime: fork %s is still being created and cannot dispatch", req.TraceID)
	}
	if d.ledger != nil {
		if snap, snapErr := d.ledger.Snapshot(req.TraceID); snapErr == nil && snap.Killed {
			return nil, fmt.Errorf("runtime: trail %s was killed and cannot dispatch", req.TraceID)
		}
	}
	if !execColony.BoundConfig() {
		bound, _, boundNow, err := trailconfig.BindNewWork(ctxColony, req.TraceID)
		if err != nil {
			return nil, fmt.Errorf("runtime: trail config: %w", err)
		}
		execColony = bound
		if scope.Experimental && !boundNow {
			return nil, fmt.Errorf("runtime: experimental trail %s has no configuration binding", req.TraceID)
		}
	}
	if scope.Experimental {
		manifest, err := trailconfig.Load(ctxColony.ColonyRoot, req.TraceID)
		if err != nil {
			return nil, fmt.Errorf("runtime: experimental trail %s: %w", req.TraceID, err)
		}
		if manifest.Policy != trailconfig.PolicyPinned {
			return nil, fmt.Errorf("runtime: experimental trail %s has non-pinned policy %q", req.TraceID, manifest.Policy)
		}
		if execColony.ConfigRef != "" && scope.ConfigRef != "" && execColony.ConfigRef != scope.ConfigRef {
			return nil, fmt.Errorf("runtime: experimental trail %s resolved a configuration the lineage does not pin", req.TraceID)
		}
	}

	agentID, err := colony.NewAgentID()
	if err != nil {
		return nil, err
	}

	// A CLI invocation has no reactor admission step; give it durable ownership
	// here. Reactor task and direct paths admit before calling so they also get
	// retry, rework, and duplicate semantics from the same store.
	if mode == DispatchModeCLI && req.Provenance.AttemptID == "" {
		attempt, err := ensureCLIAttempt(ctxColony, execColony, req, agentID)
		if err != nil {
			return nil, err
		}
		req.Provenance = attemptProvenance(attempt)
		if attempt.ConfigRef != "" && attempt.ConfigRef != execColony.ConfigRef {
			bound, err := trailconfig.BindRef(execColony, req.TraceID, attempt.ConfigRef, attempt.ConfigRevision)
			if err != nil {
				return nil, fmt.Errorf("runtime: bind attempt config: %w", err)
			}
			execColony = bound
		}
	}

	bee, _, err := execColony.LoadBee(req.Bee)
	if err != nil {
		return nil, err
	}
	if bee.RequiresPrompt() && req.Task == "" && req.InlinePrompt == "" {
		return nil, fmt.Errorf("runtime: task or inline prompt is required")
	}

	manifest, err := colony.LoadColony(execColony.ConfigRootPath())
	if err != nil {
		return nil, err
	}

	effectiveSector := colony.EffectiveSector(req.Sector, bee.Sector)
	workspace, sectorRel, err := workspaceForDispatch(execColony, manifest, bee, req.TraceID, effectiveSector, req.ProposalKind)
	if err != nil {
		return nil, err
	}

	fields := []logging.Field{
		logging.F("mode", string(mode)),
		logging.F("bee", req.Bee),
		logging.F("trace", req.TraceID),
		logging.F("agent", agentID),
	}
	if req.TaskID != "" {
		fields = append(fields, logging.F("task", req.TaskID))
	}
	if effectiveSector != "" {
		fields = append(fields, logging.F("sector", effectiveSector))
	}
	runtimeLog.Info("dispatching", fields...)

	adapterName, err := bee.ResolveAdapter()
	if err != nil {
		return nil, err
	}
	adapterExtra := colony.AdapterExtraLayer(execColony, adapterName)
	aliases := execColony.ModelAliases
	if len(aliases) == 0 {
		aliases = manifest.ModelAliases
	}
	registry := d.registry
	if execColony.BoundConfig() {
		built, err := BuildBeeRegistryFromContext(execColony)
		if err != nil {
			return nil, fmt.Errorf("runtime: build trail registry: %w", err)
		}
		registry = built
	}

	isLastWorkTask := false
	suppressPRBody := false
	if mode == DispatchModeTask {
		isLastWorkTask = req.IsLastWorkTask
		suppressPRBody = req.SuppressPRBody
	}

	var profileApply func(colony.Bee) (colony.Bee, error)
	if !execColony.BoundConfig() {
		profileApply = func(bee colony.Bee) (colony.Bee, error) {
			applied, _, err := execColony.ApplyBeeProfile(bee)
			return applied, err
		}
	}

	result, err := d.Dispatch(ctx, DispatchRequest{
		ColonyRoot:           ctxColony.ColonyRoot,
		ConfigRoot:           execColony.ConfigRootPath(),
		ConfigRef:            execColony.ConfigRef,
		ConfigRevision:       execColony.ConfigRevision,
		AttemptID:            req.Provenance.AttemptID,
		InitialCodeBaseSHA:   req.Provenance.InitialCodeBaseSHA,
		InitialCodeBaseDirty: req.Provenance.InitialCodeBaseDirty,
		Bee:                  req.Bee,
		TraceID:              req.TraceID,
		AgentID:              agentID,
		TaskID:               req.TaskID,
		Task:                 req.Task,
		Sector:               effectiveSector,
		SectorPath:           sectorRel,
		Intent:               req.Intent,
		Insights:             req.Insights,
		InlinePrompt:         req.InlinePrompt,
		Workspace:            workspace,
		AdapterExtra:         adapterExtra,
		ModelAliases:         aliases,
		IsLastWorkTask:       isLastWorkTask,
		SuppressPRBody:       suppressPRBody,
		TrailInventory:       req.TrailInventory,
		PolicyParams:         req.PolicyParams,
		Profile:              execColony.Profile,
		Raw:                  req.Raw,
		ProfileApply:         profileApply,
		Registry:             registry,
	})
	if err != nil {
		return nil, err
	}

	runDir := runs.Dir{
		ColonyRoot: ctxColony.ColonyRoot,
		TraceID:    req.TraceID,
		AgentID:    agentID,
	}

	return &BeeRunResult{
		TraceID:   req.TraceID,
		AgentID:   agentID,
		Workspace: workspace,
		RunDir:    runDir.Root(),
		Result:    result,
	}, nil
}

func ensureCLIAttempt(ctxColony, execColony colony.Context, req ColonyDispatchRequest, agentID string) (trailconfig.Attempt, error) {
	if _, ok, err := trailconfig.LoadManifestIfExists(ctxColony.ColonyRoot, req.TraceID); err != nil {
		return trailconfig.Attempt{}, err
	} else if !ok {
		return trailconfig.Attempt{}, nil
	}
	if req.TaskID == "" {
		attempt, _, blocked, err := trailconfig.AdmitChainAttemptGated(execColony, req.TraceID, "cli:"+req.TraceID+":"+agentID, "", "")
		if err != nil {
			return trailconfig.Attempt{}, err
		}
		if blocked {
			return trailconfig.Attempt{}, fmt.Errorf("runtime: trail %s has a pending configuration change; new work is held", req.TraceID)
		}
		return attempt, nil
	}
	input := trailconfig.NewTaskInputEvidence(req.TaskID, "", req.Task, req.Bee, req.Sector, req.Intent, "")
	attempt, blocked, err := trailconfig.EnsureTaskAttemptGated(execColony, req.TraceID, req.TaskID, "cli:"+agentID, input)
	if err != nil {
		return trailconfig.Attempt{}, err
	}
	if blocked {
		return trailconfig.Attempt{}, fmt.Errorf("runtime: trail %s has a pending configuration change; new work is held", req.TraceID)
	}
	if _, _, err := trailconfig.MarkAttemptDispatched(ctxColony.ColonyRoot, req.TraceID, attempt.AttemptID, "cli:"+agentID); err != nil {
		return trailconfig.Attempt{}, err
	}
	return attempt, nil
}

func workspaceForDispatch(ctxColony colony.Context, manifest colony.Colony, bee colony.Bee, traceID, sectorName, proposalKind string) (workspace string, sectorRel string, err error) {
	base := ctxColony.ColonyRoot
	fixedRoot := ""
	if ctxColony.WorkspaceRoot != "" {
		base = ctxColony.WorkspaceRoot
		if samePath(ctxColony.WorkspaceRoot, ctxColony.ColonyRoot) {
			base = ctxColony.ColonyRoot
		} else {
			fixedRoot = ctxColony.WorkspaceRoot
		}
	}
	useWorktree := bee.Worktree
	if proposalKind != "" {
		if !protocol.IsCodeProposalKind(proposalKind) {
			return "", "", fmt.Errorf("runtime: unsupported proposal kind %q for workspace affinity", proposalKind)
		}
		useWorktree = protocol.NormalizeCodeProposalKind(protocol.MutationKind(proposalKind)) == protocol.MutationCodeProposalIsolated
	}
	if useWorktree {
		// A fork owns its checkout. Worktree-enabled roles and isolated
		// proposals reuse that execution root instead of creating a per-role
		// worktree or falling back to the primary colony checkout.
		if fixedRoot == "" {
			entry, err := worktree.Ensure(worktree.EnsureOptions{
				ColonyRoot: ctxColony.ColonyRoot,
				TraceID:    traceID,
				Slug:       ctxColony.Slug,
			})
			if err != nil {
				return "", "", fmt.Errorf("runtime: worktree: %w", err)
			}
			base = entry.Path
		}
	}

	sectorRel, err = manifest.SectorRelPath(sectorName)
	if err != nil {
		return "", "", err
	}
	workspace, err = colony.JoinSectorPath(base, sectorRel)
	if err != nil {
		return "", "", fmt.Errorf("runtime: sector: %w", err)
	}
	if sectorRel != "" {
		if err := colony.EnsureSectorDirExists(workspace); err != nil {
			return "", "", fmt.Errorf("runtime: %w", err)
		}
	}
	return workspace, sectorRel, nil
}

func samePath(a, b string) bool {
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

// RelRunDir returns a path relative to colony root when possible.
func RelRunDir(colonyRoot, runDir string) string {
	if rel, err := filepath.Rel(colonyRoot, runDir); err == nil && rel != ".." && !filepath.IsAbs(rel) {
		return rel
	}
	return runDir
}
