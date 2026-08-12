package tasks

import (
	"context"
	"fmt"

	"github.com/paseka/paseka/internal/energy"
	"github.com/paseka/paseka/internal/taskledger"
)

// ErrHoneyReserveExhausted is returned when a trace has insufficient honey reserve.
var ErrHoneyReserveExhausted = energy.ErrHoneyReserveExhausted

type eventPublisher = energy.Publisher

// AddEnergyInput describes a honey reserve top-up for one trace.
type AddEnergyInput = energy.AddInput

// ConsumeEnergyInput describes a honey reserve charge for one trace.
type ConsumeEnergyInput = energy.ConsumeInput

// AddEnergy publishes SIGNAL/energy.add. When the hive reactor is not running,
// the event is also applied to the ledger so CLI callers see immediate state.
func AddEnergy(ctx context.Context, session *LedgerSession, in AddEnergyInput) (taskledger.TraceSnapshot, error) {
	if session == nil || session.Publisher == nil || session.Ledger == nil {
		return taskledger.TraceSnapshot{}, fmt.Errorf("nats url not configured")
	}
	return energy.Add(ctx, session.Colony.Slug, session.Ledger, session.Publisher, in)
}

// ConsumeEnergy publishes SIGNAL/energy.consume. When the hive reactor is not running,
// the event is also applied to the ledger so CLI callers see immediate state.
func ConsumeEnergy(ctx context.Context, session *LedgerSession, in ConsumeEnergyInput) (taskledger.TraceSnapshot, error) {
	if session == nil || session.Publisher == nil || session.Ledger == nil {
		return taskledger.TraceSnapshot{}, fmt.Errorf("nats url not configured")
	}
	return energy.Consume(ctx, session.Colony.Slug, session.Colony.ColonyRoot, session.Ledger, session.Publisher, in)
}

// EnsureEnergySeeded seeds the trace honey reserve from colony defaults when not yet seeded.
func EnsureEnergySeeded(ledger taskledger.Ledger, colonyRoot, traceID string) error {
	return energy.EnsureSeeded(ledger, colonyRoot, traceID)
}
