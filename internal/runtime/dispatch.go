package runtime

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"path/filepath"

	"github.com/paseka/paseka/internal/adapters"
	"github.com/paseka/paseka/internal/adapters/cursor"
	"github.com/paseka/paseka/internal/colony"
	"github.com/paseka/paseka/internal/prompts"
	"github.com/paseka/paseka/internal/runs"
)

// DispatchRequest is input for spawning one bee/agent run.
type DispatchRequest struct {
	ColonyRoot   string
	Bee          string
	TraceID      string
	AgentID      string
	Task         string
	Insights     []string
	InlinePrompt string
	Workspace    string
}

// Dispatcher renders prompts and runs adapters.
type Dispatcher struct {
	adapters map[string]adapters.Adapter
}

// NewDispatcher creates a dispatcher with default adapters.
func NewDispatcher() *Dispatcher {
	return &Dispatcher{
		adapters: map[string]adapters.Adapter{
			"cursor": cursor.New(),
		},
	}
}

// RegisterAdapter adds or replaces an adapter by name (for tests).
func (d *Dispatcher) RegisterAdapter(name string, a adapters.Adapter) {
	d.adapters[name] = a
}

// Dispatch loads bee config, renders prompt, and runs the adapter.
func (d *Dispatcher) Dispatch(ctx context.Context, req DispatchRequest) (*adapters.RunResult, error) {
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
		id, err := newAgentID()
		if err != nil {
			return nil, err
		}
		agentID = id
	}

	manifest, err := colony.LoadColony(colonyRoot)
	if err != nil {
		return nil, err
	}
	bee, beeLocalTemplate, err := colony.LoadBee(colonyRoot, req.Bee)
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

	rendered, err := loader.RenderResolved(prompts.ResolveInput{
		InlinePrompt:     req.InlinePrompt,
		BeeLocalTemplate: beeLocalTemplate,
		BeeTemplate:      bee.PromptTemplate,
		DefaultTemplate:  manifest.Defaults.PromptTemplate,
	}, prompts.Context{
		Bee:        bee.Role,
		TraceID:    req.TraceID,
		AgentID:    agentID,
		ColonyRoot: colonyRoot,
		Workspace:  workspace,
		Task:       req.Task,
		Insights:   req.Insights,
		ResultFile: resultFile,
	})
	if err != nil {
		return nil, fmt.Errorf("runtime: render prompt: %w", err)
	}

	adapterName, err := bee.ResolveAdapter()
	if err != nil {
		return nil, err
	}
	adapter, ok := d.adapters[adapterName]
	if !ok {
		return nil, fmt.Errorf("runtime: adapter %q not registered", adapterName)
	}

	return adapter.Run(ctx, adapters.RunRequest{
		Bee:        bee.Role,
		Prompt:     rendered,
		ColonyRoot: colonyRoot,
		Workspace:  workspace,
		Params:     colony.RunParamsFromBee(bee),
		TraceID:    req.TraceID,
		AgentID:    agentID,
	})
}

func newAgentID() (string, error) {
	var b [8]byte
	if _, err := rand.Read(b[:]); err != nil {
		return "", fmt.Errorf("runtime: agent id: %w", err)
	}
	return hex.EncodeToString(b[:]), nil
}
