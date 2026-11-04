package protocol

import (
	"fmt"
	"strings"
	"time"
)

// EnergyEventKind identifies honey-reserve payloads inside SIGNAL events.
type EnergyEventKind string

const (
	SignalEnergyAdd     EnergyEventKind = "energy.add"
	SignalEnergyConsume EnergyEventKind = "energy.consume"
	SignalEnergyStipend EnergyEventKind = "energy.stipend"
	// SignalEnergySeeded is the platform observation that reports the winning
	// initial honey allocation. It observes an allocation the ledger already
	// accepted; replaying it never seeds, adds, or consumes again.
	SignalEnergySeeded EnergyEventKind = "energy.seeded"
	// SignalEnergyPaused is the platform observation that opens one pause
	// episode: the trail stopped admitting dispatches because its reserve ran
	// out. It is emitted once per episode and never routed as work.
	SignalEnergyPaused EnergyEventKind = "energy.paused"
	// SignalEnergyResumed is the platform observation that closes an episode
	// once an increase lets the trail admit work again.
	SignalEnergyResumed EnergyEventKind = "energy.resumed"

	// DefaultEnergyBudget is the per-trace honey reserve when colony.yaml omits energy_budget.
	DefaultEnergyBudget = 12

	// DefaultBee is the task role when colony.yaml omits defaults.default_bee and task.bee is empty.
	DefaultBee = "builder"

	// HoneyReserveExhaustedSummary marks legacy task.status=blocked rows written
	// before energy deferral moved to the pending queue. New runtime code never
	// writes it into a task status; it stays readable for old ledgers.
	HoneyReserveExhaustedSummary = "Honey reserve exhausted"

	// ConfigBarrierHoldReason is recorded on a pending dispatch that a pending
	// configuration transition held instead of dropping. It is persisted in the
	// ledger snapshot, and the ledger reads it to keep that work out of the
	// energy paths: neither a top-up nor a stipend owns it.
	ConfigBarrierHoldReason = "configuration change pending"
)

// SupersededByStipendSummary names the new allocation that replaced deferred
// task work. A skipped task records why it never ran instead of hiding the
// previous tick behind a bare terminal status.
func SupersededByStipendSummary(amount int) string {
	return fmt.Sprintf("Superseded by a new standing allocation of %d honey", amount)
}

// EnergyAddPayload is emitted as SIGNAL with payload.kind=energy.add.
type EnergyAddPayload struct {
	Kind   EnergyEventKind `json:"kind"`
	Amount int             `json:"amount"`
}

// EnergyConsumePayload is emitted as SIGNAL with payload.kind=energy.consume.
type EnergyConsumePayload struct {
	Kind   EnergyEventKind `json:"kind"`
	Amount int             `json:"amount"`
	Reason string          `json:"reason,omitempty"`
	TaskID string          `json:"taskId,omitempty"`
	// Purpose attributes the charge to the execution purpose that spent it.
	// Admission writes it so a later reader does not have to guess from the Bee
	// name or the charge reason. Old charges without it stay unknown unless a
	// task join proves their purpose.
	Purpose    ExecutionPurpose `json:"purpose,omitempty"`
	AnalysisOf string           `json:"analysisOf,omitempty"`
}

// EnergyStipendPayload is emitted as SIGNAL with payload.kind=energy.stipend.
// The ledger sets energyRemaining to Amount; it does not change budget or added.
type EnergyStipendPayload struct {
	Kind   EnergyEventKind `json:"kind"`
	Amount int             `json:"amount"`
}

// EnergySeededPayload is emitted as SIGNAL with payload.kind=energy.seeded.
// It records the winning initial allocation: its durable seed identity, the
// effective budget, and the actual seed time. The observation never allocates;
// it only reports an allocation the ledger already accepted.
type EnergySeededPayload struct {
	Kind     EnergyEventKind `json:"kind"`
	SeedID   string          `json:"seedId"`
	Budget   int             `json:"budget"`
	SeededAt time.Time       `json:"seededAt"`
}

// EnergyResumeSource names the increase that ended a pause episode, so an
// operator can tell a live top-up from the new allocation a standing tick opens.
type EnergyResumeSource string

const (
	EnergyResumeSourceAdd     EnergyResumeSource = "energy.add"
	EnergyResumeSourceStipend EnergyResumeSource = "energy.stipend"
)

// Known reports whether source is one the resume observation may carry.
func (s EnergyResumeSource) Known() bool {
	switch s {
	case EnergyResumeSourceAdd, EnergyResumeSourceStipend:
		return true
	default:
		return false
	}
}

// EnergyPausedPayload is emitted as SIGNAL with payload.kind=energy.paused.
// It carries the durable episode identity and the admission facts at the pause;
// it never seeds, adds, consumes, or resumes anything.
type EnergyPausedPayload struct {
	Kind         EnergyEventKind `json:"kind"`
	EpisodeID    string          `json:"episodeId"`
	Reason       string          `json:"reason,omitempty"`
	Remaining    int             `json:"remaining"`
	Budget       int             `json:"budget,omitempty"`
	PendingCount int             `json:"pendingCount"`
}

// EnergyResumedPayload is emitted as SIGNAL with payload.kind=energy.resumed
// once an active episode closes after an energy increase. Source distinguishes
// a live top-up from a stipend; a stipend carries the abandoned count because it
// supersedes the previous allocation instead of resuming it.
type EnergyResumedPayload struct {
	Kind          EnergyEventKind    `json:"kind"`
	EpisodeID     string             `json:"episodeId"`
	Source        EnergyResumeSource `json:"source"`
	Remaining     int                `json:"remaining"`
	Budget        int                `json:"budget,omitempty"`
	PendingBefore int                `json:"pendingBefore"`
	PendingAfter  int                `json:"pendingAfter"`
	Dispatched    int                `json:"dispatched"`
	Abandoned     int                `json:"abandoned,omitempty"`
}

// IsEnergyObservation reports whether kind is a seed, pause, or resume
// observation. Observations describe admission or allocation; they are live-only
// platform signals that are excluded from direct dispatch, deferred buffers, and
// auto-invite routing.
func IsEnergyObservation(kind string) bool {
	switch EnergyEventKind(strings.TrimSpace(kind)) {
	case SignalEnergySeeded, SignalEnergyPaused, SignalEnergyResumed:
		return true
	default:
		return false
	}
}
