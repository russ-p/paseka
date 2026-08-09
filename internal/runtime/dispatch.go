package runtime

import (
	"context"

	"github.com/paseka/paseka/internal/adapters"
	"github.com/paseka/paseka/internal/adapters/claude"
	"github.com/paseka/paseka/internal/adapters/cursor"
	"github.com/paseka/paseka/internal/adapters/pi"
	"github.com/paseka/paseka/internal/adapters/script"
	"github.com/paseka/paseka/internal/bus"
)

// DispatchRequest is input for spawning one bee/agent run.
type DispatchRequest struct {
	ColonyRoot     string
	Bee            string
	TraceID        string
	AgentID        string
	Task           string
	TaskID         string
	Sector         string
	SectorPath     string
	Intent         string
	Insights       []string
	InlinePrompt   string
	Workspace      string
	AdapterExtra   adapters.RunParams
	IsLastWorkTask bool // set by AFK ledger task dispatch only
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
			"claude": claude.New(),
			"script": script.New(),
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

// Dispatch loads bee config, renders prompt, runs the adapter, and finalizes the run.
func (d *Dispatcher) Dispatch(ctx context.Context, req DispatchRequest) (*adapters.RunResult, error) {
	prepared, err := d.prepareDispatch(ctx, req)
	if err != nil {
		return nil, err
	}
	result, err := d.runPrepared(ctx, prepared)
	if err != nil {
		return nil, err
	}
	return d.finalizeDispatch(ctx, prepared, result)
}
