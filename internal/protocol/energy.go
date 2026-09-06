package protocol

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

	// HoneyReserveExhaustedSummary is published on task.status when dispatch is blocked.
	HoneyReserveExhaustedSummary = "Honey reserve exhausted"
)

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
