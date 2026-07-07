package runtime

import (
	"context"
	"fmt"
	"path/filepath"
	"strings"
	"time"

	"github.com/paseka/paseka/internal/adapters"
	"github.com/paseka/paseka/internal/adapters/cursor"
	"github.com/paseka/paseka/internal/adapters/pi"
	"github.com/paseka/paseka/internal/bus"
	"github.com/paseka/paseka/internal/colony"
	"github.com/paseka/paseka/internal/logging"
	"github.com/paseka/paseka/internal/prompts"
	"github.com/paseka/paseka/internal/protocol"
	"github.com/paseka/paseka/internal/runs"
)

// DispatchRequest is input for spawning one bee/agent run.
type DispatchRequest struct {
	ColonyRoot   string
	Bee          string
	TraceID      string
	AgentID      string
	Task         string
	TaskID       string
	Sector       string
	SectorPath   string
	Intent       string
	Insights     []string
	InlinePrompt string
	Workspace    string
	AdapterExtra adapters.RunParams
}

// Dispatcher renders prompts and runs adapters.
type Dispatcher struct {
	adapters    map[string]adapters.Adapter
	publisher   bus.Publisher
	busRequired bool
	registry    *BeeRegistry
}

// NewDispatcher creates a dispatcher with default adapters.
func NewDispatcher() *Dispatcher {
	return &Dispatcher{
		adapters: map[string]adapters.Adapter{
			"cursor": cursor.New(),
			"pi":     pi.New(),
		},
	}
}

// RegisterAdapter adds or replaces an adapter by name (for tests).
func (d *Dispatcher) RegisterAdapter(name string, a adapters.Adapter) {
	d.adapters[name] = a
}

// SetPublisher configures optional NATS event publishing after adapter runs.
func (d *Dispatcher) SetPublisher(pub bus.Publisher, required bool) {
	d.publisher = pub
	d.busRequired = required
}

// SetBeeRegistry configures advisory publish validation against bee YAML contracts.
func (d *Dispatcher) SetBeeRegistry(reg *BeeRegistry) {
	d.registry = reg
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

	projectedInsights, err := GatherPromptInsights(colonyRoot, req.TraceID, req.TaskID)
	if err != nil {
		return nil, fmt.Errorf("runtime: gather insights: %w", err)
	}
	insights := MergeInsights(req.Insights, projectedInsights)

	rendered, err := loader.RenderResolved(prompts.ResolveInput{
		InlinePrompt:     req.InlinePrompt,
		BeeLocalTemplate: beeLocalTemplate,
		BeeTemplate:      bee.PromptTemplate,
		DefaultTemplate:  manifest.Defaults.PromptTemplate,
	}, prompts.PromptContext(prompts.Context{
		Bee:        bee.Role,
		TraceID:    req.TraceID,
		AgentID:    agentID,
		TaskID:     req.TaskID,
		ColonyRoot: colonyRoot,
		Workspace:  workspace,
		Sector:     req.Sector,
		SectorPath: req.SectorPath,
		Task:       req.Task,
		IntentRaw:  req.Intent,
		Insights:   insights,
		ResultFile: resultFile,
	}))
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

	params := colony.MergeRunParams(colony.RunParamsFromBee(bee), req.AdapterExtra)

	if err := runDir.Prepare(); err != nil {
		return nil, fmt.Errorf("runtime: prepare run dir: %w", err)
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

	runDirPath := runDir.Root()
	runtimeLog.Info("adapter run",
		logging.F("adapter", adapterName),
		logging.F("bee", bee.Role),
		logging.F("trace", req.TraceID),
		logging.F("agent", agentID),
		logging.F("workspace", workspace),
		logging.F("run_dir", RelRunDir(colonyRoot, runDirPath)),
	)

	result, err := adapter.Run(ctx, adapters.RunRequest{
		Bee:        bee.Role,
		Prompt:     rendered,
		ColonyRoot: colonyRoot,
		Workspace:  workspace,
		Sector:     req.Sector,
		SectorPath: req.SectorPath,
		Params:     params,
		TraceID:    req.TraceID,
		AgentID:    agentID,
		TaskID:     req.TaskID,
		Task:       req.Task,
		Intent:     req.Intent,
		Insights:   insights,
	})
	if err != nil {
		return nil, err
	}

	if summary := strings.TrimSpace(result.Summary); summary != "" {
		_ = runDir.WriteResultText(summary)
	}

	runEvents, readErr := runDir.ReadEvents()
	if readErr != nil {
		runEvents = nil
	}

	var synthesized []protocol.Event
	updatedEvents, synthesizedEvent, synthErr := d.synthesizeRunSummary(
		runDir, bee, req.TraceID, agentID, req.TaskID, result, runEvents,
	)
	if synthErr != nil {
		return result, synthErr
	}
	runEvents = updatedEvents
	if synthesizedEvent != nil {
		synthesized = append(synthesized, *synthesizedEvent)
	}

	d.enforceRunSummaryRequired(bee, agentID, result, runEvents)
	if readErr == nil {
		d.enforceCompletionContract(bee, runEvents, result)
	}

	if pubErr := d.publishRunOutcome(ctx, DispatchRequest{
		ColonyRoot: colonyRoot,
		Bee:        req.Bee,
		TraceID:    req.TraceID,
		AgentID:    agentID,
		TaskID:     req.TaskID,
	}, result, synthesized); pubErr != nil {
		if d.busRequired {
			return result, pubErr
		}
	}
	return result, nil
}
