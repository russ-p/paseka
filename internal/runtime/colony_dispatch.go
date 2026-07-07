package runtime

import (
	"context"
	"fmt"
	"log"
	"path/filepath"

	"github.com/paseka/paseka/internal/colony"
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
	Intent       string
	Insights     []string
	InlinePrompt string
}

// DispatchColonyBee prepares workspace and runs one bee without re-resolving colony context.
func (d *Dispatcher) DispatchColonyBee(ctx context.Context, ctxColony colony.Context, req ColonyDispatchRequest, mode DispatchMode) (*BeeRunResult, error) {
	if req.Bee == "" {
		return nil, fmt.Errorf("runtime: bee role is required")
	}
	if req.Task == "" && req.InlinePrompt == "" {
		return nil, fmt.Errorf("runtime: task or inline prompt is required")
	}
	if req.TraceID == "" {
		return nil, fmt.Errorf("runtime: traceId is required")
	}

	bee, _, err := colony.LoadBee(ctxColony.ColonyRoot, req.Bee)
	if err != nil {
		return nil, err
	}

	workspace, err := workspaceForBee(ctxColony, bee, req.TraceID)
	if err != nil {
		return nil, err
	}

	agentID, err := colony.NewAgentID()
	if err != nil {
		return nil, err
	}

	taskPart := ""
	if req.TaskID != "" {
		taskPart = fmt.Sprintf(" task=%s", req.TaskID)
	}
	log.Printf("runtime: dispatching %s bee=%s trace=%s%s agent=%s",
		mode, req.Bee, req.TraceID, taskPart, agentID)

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

func workspaceForBee(ctxColony colony.Context, bee colony.Bee, traceID string) (string, error) {
	workspace := ctxColony.ColonyRoot
	if !bee.Worktree {
		return workspace, nil
	}
	entry, err := worktree.Ensure(worktree.EnsureOptions{
		ColonyRoot: ctxColony.ColonyRoot,
		TraceID:    traceID,
		Slug:       ctxColony.Slug,
	})
	if err != nil {
		return "", fmt.Errorf("runtime: worktree: %w", err)
	}
	return entry.Path, nil
}

// RelRunDir returns a path relative to colony root when possible.
func RelRunDir(colonyRoot, runDir string) string {
	if rel, err := filepath.Rel(colonyRoot, runDir); err == nil && rel != ".." && !filepath.IsAbs(rel) {
		return rel
	}
	return runDir
}
