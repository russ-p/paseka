package protocol

import "fmt"

// EnergyEventKind identifies honey-reserve payloads inside SIGNAL events.
type EnergyEventKind string

const (
	SignalEnergyAdd     EnergyEventKind = "energy.add"
	SignalEnergyConsume EnergyEventKind = "energy.consume"
	SignalEnergyStipend EnergyEventKind = "energy.stipend"

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
}

// EnergyStipendPayload is emitted as SIGNAL with payload.kind=energy.stipend.
// The ledger sets energyRemaining to Amount; it does not change budget or added.
type EnergyStipendPayload struct {
	Kind   EnergyEventKind `json:"kind"`
	Amount int             `json:"amount"`
}
