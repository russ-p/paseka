package taskledger

import (
	"time"

	"github.com/russ-p/paseka/internal/protocol"
)

// TaskSnapshot is the current state of one task within a trace.
type TaskSnapshot struct {
	TaskID            string                     `json:"taskId"`
	Title             string                     `json:"title,omitempty"`
	Body              string                     `json:"body,omitempty"`
	Bee               string                     `json:"bee,omitempty"`
	Sector            string                     `json:"sector,omitempty"`
	Intent            string                     `json:"intent,omitempty"`
	CueID             string                     `json:"cueId,omitempty"`
	Review            protocol.TaskReviewPolicy  `json:"review,omitempty"`
	Phase             protocol.TaskPhase         `json:"phase,omitempty"`
	Status            protocol.TaskStatus        `json:"status"`
	DependsOn         []string                   `json:"dependsOn,omitempty"`
	Summary           string                     `json:"summary,omitempty"`
	Commit            string                     `json:"commit,omitempty"`
	ProposalWorkspace protocol.ProposalWorkspace `json:"proposalWorkspace,omitempty"`
	CreatedAt         time.Time                  `json:"createdAt,omitempty"`
	UpdatedAt         time.Time                  `json:"updatedAt,omitempty"`
}

// withCreatedAt stamps the task origin once, so a task first seen through
// task.status, task.completed, or code.proposal still carries a creation time.
// Never moves an existing stamp.
func withCreatedAt(task TaskSnapshot, now time.Time) TaskSnapshot {
	if task.CreatedAt.IsZero() {
		task.CreatedAt = now
	}
	return task
}

// PendingDispatchKind identifies the dispatch path a deferred item belongs to.
type PendingDispatchKind string

const (
	PendingDispatchTask   PendingDispatchKind = "task"
	PendingDispatchDirect PendingDispatchKind = "direct"
)

// PendingDispatchStatus tracks whether deferred work still awaits admission.
type PendingDispatchStatus string

const (
	PendingDispatchPending  PendingDispatchStatus = "pending"
	PendingDispatchConsumed PendingDispatchStatus = "consumed"
)

// PendingDispatch is work a trail admitted but could not start because the
// honey reserve was exhausted. The dispatch path owns the context needed to
// resume it; the task lifecycle status is deliberately not part of this record.
type PendingDispatch struct {
	ID            string                `json:"id"`
	Kind          PendingDispatchKind   `json:"kind"`
	TaskID        string                `json:"taskId,omitempty"`
	Bee           string                `json:"bee,omitempty"`
	SourceEventID string                `json:"sourceEventId,omitempty"`
	Reason        string                `json:"reason,omitempty"`
	Status        PendingDispatchStatus `json:"status"`
	CreatedAt     time.Time             `json:"createdAt,omitempty"`
	UpdatedAt     time.Time             `json:"updatedAt,omitempty"`
}

// TraceSnapshot is the aggregated task ledger for one trace.
type TraceSnapshot struct {
	TraceID           string                  `json:"traceId"`
	EnergyBudget      int                     `json:"energyBudget,omitempty"`
	EnergyRemaining   int                     `json:"energyRemaining,omitempty"`
	EnergyAdded       int                     `json:"energyAdded,omitempty"`
	Killed            bool                    `json:"killed,omitempty"`
	PendingReady      *PendingReadySnapshot   `json:"pendingReady,omitempty"`
	PendingDispatches []PendingDispatch       `json:"pendingDispatches,omitempty"`
	Tasks             map[string]TaskSnapshot `json:"tasks"`
}

// PendingReadySnapshot parks a task.ready kick until the matching task.plan registers the task.
type PendingReadySnapshot struct {
	TaskID string `json:"taskId"`
	Title  string `json:"title,omitempty"`
	Body   string `json:"body,omitempty"`
	Bee    string `json:"bee,omitempty"`
	Sector string `json:"sector,omitempty"`
	Intent string `json:"intent,omitempty"`
	CueID  string `json:"cueId,omitempty"`
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

	// DeferPendingDispatch records deferred work unless an active entry with the
	// same id already exists. It reports whether a new entry was added.
	DeferPendingDispatch(traceID string, dispatch PendingDispatch) (bool, error)

	// ResolvePendingDispatch closes deferred work with the given status.
	ResolvePendingDispatch(traceID, dispatchID string, status PendingDispatchStatus) error
}
