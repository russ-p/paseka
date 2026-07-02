package runtime

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"path/filepath"

	"github.com/paseka/paseka/internal/adapters"
	"github.com/paseka/paseka/internal/colony"
	"github.com/paseka/paseka/internal/runs"
	"github.com/paseka/paseka/internal/worktree"
)

// BeeRunRequest is input for a one-shot bee dispatch from the CLI.
type BeeRunRequest struct {
	StartDir     string
	Bee          string
	TraceID      string
	Task         string
	Insights     []string
	InlinePrompt string
}

// BeeRunResult summarizes a completed bee run.
type BeeRunResult struct {
	TraceID   string
	AgentID   string
	Workspace string
	RunDir    string
	Result    *adapters.RunResult
}

// BeeRun resolves colony context, prepares workspace, and dispatches one bee.
func (d *Dispatcher) BeeRun(ctx context.Context, req BeeRunRequest) (*BeeRunResult, error) {
	if req.Bee == "" {
		return nil, fmt.Errorf("runtime: bee role is required")
	}
	if req.Task == "" && req.InlinePrompt == "" {
		return nil, fmt.Errorf("runtime: task or inline prompt is required")
	}

	ctxColony, err := colony.ResolveContext(req.StartDir)
	if err != nil {
		return nil, err
	}

	traceID := req.TraceID
	if traceID == "" {
		id, err := newTraceID()
		if err != nil {
			return nil, err
		}
		traceID = id
	}

	bee, _, err := colony.LoadBee(ctxColony.ColonyRoot, req.Bee)
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
			return nil, fmt.Errorf("runtime: worktree: %w", err)
		}
		workspace = entry.Path
	}

	agentID, err := newAgentID()
	if err != nil {
		return nil, err
	}

	adapterExtra := adapters.RunParams{
		Binary: ctxColony.Cursor.Binary,
		APIKey: ctxColony.Cursor.APIKey(),
	}

	result, err := d.Dispatch(ctx, DispatchRequest{
		ColonyRoot:   ctxColony.ColonyRoot,
		Bee:          req.Bee,
		TraceID:      traceID,
		AgentID:      agentID,
		Task:         req.Task,
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
		TraceID:    traceID,
		AgentID:    agentID,
	}

	return &BeeRunResult{
		TraceID:   traceID,
		AgentID:   agentID,
		Workspace: workspace,
		RunDir:    runDir.Root(),
		Result:    result,
	}, nil
}

func newTraceID() (string, error) {
	var b [8]byte
	if _, err := rand.Read(b[:]); err != nil {
		return "", fmt.Errorf("runtime: trace id: %w", err)
	}
	return "trace-" + hex.EncodeToString(b[:]), nil
}

// RelRunDir returns a path relative to colony root when possible.
func RelRunDir(colonyRoot, runDir string) string {
	if rel, err := filepath.Rel(colonyRoot, runDir); err == nil && rel != ".." && !filepath.IsAbs(rel) {
		return rel
	}
	return runDir
}
