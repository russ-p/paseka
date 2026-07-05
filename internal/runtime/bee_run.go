package runtime

import (
	"context"
	"fmt"

	"github.com/paseka/paseka/internal/adapters"
	"github.com/paseka/paseka/internal/bus"
	"github.com/paseka/paseka/internal/colony"
)

// BeeRunRequest is input for a one-shot bee dispatch from the CLI.
type BeeRunRequest struct {
	StartDir     string
	Bee          string
	TraceID      string
	Task         string
	TaskID       string
	Insights     []string
	InlinePrompt string
	NoBus        bool
	BusRequired  bool
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

	if !req.NoBus {
		busClient, busErr := bus.ConnectColony(ctxColony, false)
		if busErr != nil {
			if req.BusRequired {
				return nil, busErr
			}
		} else if busClient != nil {
			d.SetPublisher(busClient, req.BusRequired)
			defer busClient.Close()
		}
	}

	traceID := req.TraceID
	if traceID == "" {
		id, err := colony.NewTraceID()
		if err != nil {
			return nil, err
		}
		traceID = id
	}

	if registry, regErr := BuildBeeRegistry(ctxColony.ColonyRoot); regErr == nil {
		d.SetBeeRegistry(registry)
	}

	return d.DispatchColonyBee(ctx, ctxColony, ColonyDispatchRequest{
		Bee:          req.Bee,
		TraceID:      traceID,
		Task:         req.Task,
		TaskID:       req.TaskID,
		Insights:     req.Insights,
		InlinePrompt: req.InlinePrompt,
	}, DispatchModeCLI)
}
