package runtime

import (
	"context"
	"fmt"
	"path/filepath"

	"github.com/paseka/paseka/internal/colony"
	"github.com/paseka/paseka/internal/logging"
	"github.com/paseka/paseka/internal/protocol"
	"github.com/paseka/paseka/internal/runs"
	"github.com/paseka/paseka/internal/worktree"
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
	Bee          string
	TraceID      string
	Task         string
	TaskID       string
	Sector       string
	Intent       string
	Insights     []string
	InlinePrompt string
	// ProposalKind, when set, resolves adapter cwd from the proposal MUTATION kind
	// (isolated/alias → trace worktree; root → colony root) instead of bee.Worktree.
	ProposalKind string
}

// DispatchColonyBee prepares workspace and runs one bee without re-resolving colony context.
func (d *Dispatcher) DispatchColonyBee(ctx context.Context, ctxColony colony.Context, req ColonyDispatchRequest, mode DispatchMode) (*BeeRunResult, error) {
	if req.Bee == "" {
		return nil, fmt.Errorf("runtime: bee role is required")
	}
	if req.TraceID == "" {
		return nil, fmt.Errorf("runtime: traceId is required")
	}

	bee, _, err := colony.LoadBee(ctxColony.ColonyRoot, req.Bee)
	if err != nil {
		return nil, err
	}
	if bee.RequiresPrompt() && req.Task == "" && req.InlinePrompt == "" {
		return nil, fmt.Errorf("runtime: task or inline prompt is required")
	}

	manifest, err := colony.LoadColony(ctxColony.ColonyRoot)
	if err != nil {
		return nil, err
	}

	effectiveSector := colony.EffectiveSector(req.Sector, bee.Sector)
	workspace, sectorRel, err := workspaceForDispatch(ctxColony, manifest, bee, req.TraceID, effectiveSector, req.ProposalKind)
	if err != nil {
		return nil, err
	}

	agentID, err := colony.NewAgentID()
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
	adapterExtra := colony.AdapterExtra(ctxColony, adapterName)

	result, err := d.Dispatch(ctx, DispatchRequest{
		ColonyRoot:   ctxColony.ColonyRoot,
		Bee:          req.Bee,
		TraceID:      req.TraceID,
		AgentID:      agentID,
		TaskID:       req.TaskID,
		Task:         req.Task,
		Sector:       effectiveSector,
		SectorPath:   sectorRel,
		Intent:       req.Intent,
		Insights:     req.Insights,
		InlinePrompt: req.InlinePrompt,
		Workspace:    workspace,
		AdapterExtra: adapterExtra,
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

func workspaceForDispatch(ctxColony colony.Context, manifest colony.Colony, bee colony.Bee, traceID, sectorName, proposalKind string) (workspace string, sectorRel string, err error) {
	base := ctxColony.ColonyRoot
	useWorktree := bee.Worktree
	if proposalKind != "" {
		if !protocol.IsCodeProposalKind(proposalKind) {
			return "", "", fmt.Errorf("runtime: unsupported proposal kind %q for workspace affinity", proposalKind)
		}
		useWorktree = protocol.NormalizeCodeProposalKind(protocol.MutationKind(proposalKind)) == protocol.MutationCodeProposalIsolated
	}
	if useWorktree {
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

// RelRunDir returns a path relative to colony root when possible.
func RelRunDir(colonyRoot, runDir string) string {
	if rel, err := filepath.Rel(colonyRoot, runDir); err == nil && rel != ".." && !filepath.IsAbs(rel) {
		return rel
	}
	return runDir
}
