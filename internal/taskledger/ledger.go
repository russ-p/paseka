package taskledger

import (
	"time"

	"github.com/paseka/paseka/internal/protocol"
)

// TaskSnapshot is the current state of one task within a trace.
type TaskSnapshot struct {
	TaskID            string                     `json:"taskId"`
	Title             string                     `json:"title,omitempty"`
	Body              string                     `json:"body,omitempty"`
	Bee               string                     `json:"bee,omitempty"`
	Sector            string                     `json:"sector,omitempty"`
	Intent            string                     `json:"intent,omitempty"`
	Review            protocol.TaskReviewPolicy  `json:"review,omitempty"`
	Status            protocol.TaskStatus        `json:"status"`
	DependsOn         []string                   `json:"dependsOn,omitempty"`
	Summary           string                     `json:"summary,omitempty"`
	Commit            string                     `json:"commit,omitempty"`
	ProposalWorkspace protocol.ProposalWorkspace `json:"proposalWorkspace,omitempty"`
	UpdatedAt         time.Time                  `json:"updatedAt,omitempty"`
}

// TraceSnapshot is the aggregated task ledger for one trace.
type TraceSnapshot struct {
	TraceID         string                  `json:"traceId"`
	EnergyBudget    int                     `json:"energyBudget,omitempty"`
	EnergyRemaining int                     `json:"energyRemaining,omitempty"`
	Tasks           map[string]TaskSnapshot `json:"tasks"`
}

// ApplyResult is the outcome of applying one bus event to a trace ledger.
type ApplyResult struct {
	Trace   TraceSnapshot
	Ready   []TaskSnapshot // tasks that became ready after this event
	Changed bool
}

// Ledger persists and projects task state for one or more traces.
// Implementations may use NATS KV, local files, or in-memory storage.
type Ledger interface {
	// Snapshot returns the current task ledger for a trace.
	Snapshot(traceID string) (TraceSnapshot, error)

	// Apply processes one bus event and returns the updated trace snapshot
	// plus any tasks that newly transitioned to ready.
	Apply(event protocol.Event) (ApplyResult, error)

	// SeedEnergy initializes the honey reserve when not yet seeded.
	SeedEnergy(traceID string, budget int) error
}
