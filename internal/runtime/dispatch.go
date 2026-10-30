package runtime

import (
	"context"

	"github.com/russ-p/paseka/internal/adapters"
	"github.com/russ-p/paseka/internal/adapters/claude"
	"github.com/russ-p/paseka/internal/adapters/cursor"
	"github.com/russ-p/paseka/internal/adapters/opencode"
	"github.com/russ-p/paseka/internal/adapters/opencodev1"
	"github.com/russ-p/paseka/internal/adapters/opencodev2"
	"github.com/russ-p/paseka/internal/adapters/pi"
	"github.com/russ-p/paseka/internal/adapters/script"
	"github.com/russ-p/paseka/internal/bus"
	"github.com/russ-p/paseka/internal/colony"
	"github.com/russ-p/paseka/internal/taskledger"
)

// DispatchRequest is input for spawning one bee/agent run.
type DispatchRequest struct {
	ColonyRoot           string
	ConfigRoot           string
	ConfigRef            string
	ConfigRevision       int
	AttemptID            string
	InitialCodeBaseSHA   string
	InitialCodeBaseDirty bool
	Bee                  string
	TraceID              string
	AgentID              string
	Task                 string
	TaskID               string
	Sector               string
	SectorPath           string
	Intent               string
	Insights             []string
	InlinePrompt         string
	Workspace            string
	AdapterExtra         adapters.RunParamsLayer
	ModelAliases         map[string]string
	IsLastWorkTask       bool // set by AFK ledger task dispatch only
	// SuppressPRBody drops the pr.body must-emit guidance when a closing round
	// is guaranteed to own the pull-request description.
	SuppressPRBody bool
	// TrailInventory is the structured task list of the trail, set for a
	// closing-phase dispatch.
	TrailInventory colony.TrailInventory
	Profile        string
	ProfileApply   func(colony.Bee) (colony.Bee, error)
	// Registry is the trail-scoped bee registry, when the caller has one.
	Registry *BeeRegistry
	// Raw requests plain-text adapter output for this run only (CLI --raw).
	Raw bool
}

// Dispatcher renders prompts and runs adapters.
type Dispatcher struct {
	adapters    map[string]adapters.Adapter
	publisher   bus.Publisher
	busRequired bool
	registry    *BeeRegistry
	ledger      taskledger.Ledger
}

// NewDispatcher creates a dispatcher with default adapters.
func NewDispatcher() *Dispatcher {
	return &Dispatcher{
		adapters: map[string]adapters.Adapter{
			"cursor":     cursor.New(),
			"pi":         pi.New(),
			"claude":     claude.New(),
			"opencode":   opencode.New(),
			"opencodev1": opencodev1.New(),
			"opencodev2": opencodev2.New(),
			"script":     script.New(),
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

// SetLedger configures the durable task ledger used to observe trace kill state.
func (d *Dispatcher) SetLedger(ledger taskledger.Ledger) {
	d.ledger = ledger
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
