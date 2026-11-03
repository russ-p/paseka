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
	CueInvocationID   string                     `json:"cueInvocationId,omitempty"` // invocation that created the task
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
	// PendingDispatchAbandoned marks work a later allocation deliberately gave
	// up: a stipend or a kill closed it. It is never resumed.
	PendingDispatchAbandoned PendingDispatchStatus = "abandoned"
	// PendingDispatchUnresolved marks work whose source event could no longer
	// be recovered. It stays visible as evidence and is never treated as
	// runnable.
	PendingDispatchUnresolved PendingDispatchStatus = "unresolved"
)

// Active reports whether deferred work still awaits admission.
func (s PendingDispatchStatus) Active() bool {
	return s == PendingDispatchPending
}

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
	Diagnostic    string                `json:"diagnostic,omitempty"`
	CreatedAt     time.Time             `json:"createdAt,omitempty"`
	UpdatedAt     time.Time             `json:"updatedAt,omitempty"`
}

// TraceSnapshot is the aggregated task ledger for one trace.
type TraceSnapshot struct {
	TraceID           string                  `json:"traceId"`
	EnergyBudget      int                     `json:"energyBudget,omitempty"`
	EnergyRemaining   int                     `json:"energyRemaining,omitempty"`
	EnergyAdded       int                     `json:"energyAdded,omitempty"`
	EnergySeed        *EnergySeedSnapshot     `json:"energySeed,omitempty"`
	EnergyPause       *EnergyPauseSnapshot    `json:"energyPause,omitempty"`
	Killed            bool                    `json:"killed,omitempty"`
	PendingReady      *PendingReadySnapshot   `json:"pendingReady,omitempty"`
	PendingDispatches []PendingDispatch       `json:"pendingDispatches,omitempty"`
	Tasks             map[string]TaskSnapshot `json:"tasks"`
}

// EnergySeedSnapshot is the durable provenance of the winning initial honey
// allocation. It records the seed identity, effective budget, and actual seed
// time before the first charged dispatch. Published marks that its
// SIGNAL/energy.seeded observation reached the publisher; an unpublished record
// is retried with the same identity and never reseeds.
//
// A ledger written before seed provenance existed has no record even though
// EnergyBudget may be positive. That history stays unknown: readers must not
// invent a budget, a timestamp, or a configuration for it.
type EnergySeedSnapshot struct {
	SeedID    string    `json:"seedId"`
	Budget    int       `json:"budget"`
	SeededAt  time.Time `json:"seededAt"`
	Published bool      `json:"published,omitempty"`
}

// EnergyPauseSnapshot is the durable identity of one pause episode: the interval
// in which the trail's shared reserve could not admit a dispatch. It exists only
// to make the pause and resume observations exactly-once; admission itself is
// always derived from EnergyRemaining.
type EnergyPauseSnapshot struct {
	EpisodeID string    `json:"episodeId"`
	Reason    string    `json:"reason,omitempty"`
	StartedAt time.Time `json:"startedAt,omitempty"`
	Remaining int       `json:"remaining"`
	Budget    int       `json:"budget,omitempty"`
	Pending   int       `json:"pending"`
	// Published records that the paused observation was claimed for sending.
	// It is marked before the send, so a crash loses at most one notification
	// and a restart never duplicates one.
	Published bool `json:"published,omitempty"`
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
	// CueInvocationID carries the invocation through a parked task.ready.
	CueInvocationID string `json:"cueInvocationId,omitempty"`
}

// ApplyResult is the outcome of applying one bus event to a trace ledger.
type ApplyResult struct {
	Trace   TraceSnapshot
	Ready   []TaskSnapshot // tasks that became ready after this event
	Changed bool

	// PauseClose is set when this reduction closed a pause episode. The field is
	// already gone from Trace, so the runtime publishes the resumed observation
	// exactly once for the episode reported here.
	PauseClose *EnergyPauseClose
}

// EnergyPauseClose records the episode a reduction closed and the counts its
// resumed observation must carry. A stipend closes the episode while it
// supersedes work, so the observation reports that through Source and Abandoned
// instead of pretending the previous allocation resumed.
type EnergyPauseClose struct {
	Episode       EnergyPauseSnapshot
	Source        protocol.EnergyResumeSource
	Remaining     int
	Budget        int
	PendingBefore int
	PendingAfter  int
	Dispatched    int
	Abandoned     int
}

// TraceLister is implemented by ledgers that can enumerate the trails they
// hold. Reactor startup uses it to find deferred work that a top-up approved
// while no reactor was running.
type TraceLister interface {
	// TraceIDs lists the trail ids present in the ledger.
	TraceIDs() ([]string, error)
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

	// EnsureEnergySeed is the authoritative seed transition: it allocates the
	// reserve when the trace has no budget and records the winning seed
	// identity, effective budget, and actual seed time. It returns the winning
	// record and whether this call created it. A second request observes the
	// existing winner; a legacy budget without provenance returns an empty
	// record and is never relabelled.
	EnsureEnergySeed(traceID string, budget int) (EnergySeedSnapshot, bool, error)

	// MarkEnergySeedPublished records that the seed observation for seedID
	// reached the publisher. It is idempotent and refuses a different identity,
	// so a retry reuses the recorded seed rather than selecting another.
	MarkEnergySeedPublished(traceID, seedID string) error

	// DeferPendingDispatch records deferred work unless an active entry with the
	// same id already exists. It reports whether a new entry was added.
	DeferPendingDispatch(traceID string, dispatch PendingDispatch) (bool, error)

	// ResolvePendingDispatch closes deferred work with the given status.
	ResolvePendingDispatch(traceID, dispatchID string, status PendingDispatchStatus) error

	// MarkPendingDispatchUnresolved keeps deferred work visible when its source
	// event can no longer be recovered. The entry is never drained again.
	MarkPendingDispatchUnresolved(traceID, dispatchID, diagnostic string) error

	// ClaimEnergyPausePublished marks the open pause episode published and
	// returns it exactly once. The runtime sends the paused observation after
	// this claim, so replay and restart cannot duplicate it.
	ClaimEnergyPausePublished(traceID string) (EnergyPauseSnapshot, bool, error)

	// ClearEnergyPause closes the open pause episode and returns the episode it
	// cleared, so the runtime sends at most one resumed observation after the
	// field is gone.
	ClearEnergyPause(traceID string) (EnergyPauseSnapshot, bool, error)
}
