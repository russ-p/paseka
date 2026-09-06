package energy

import (
	"strings"

	"github.com/russ-p/paseka/internal/colony"
	"github.com/russ-p/paseka/internal/protocol"
	"github.com/russ-p/paseka/internal/taskledger"
)

// EnsureSeeded seeds the trace honey reserve from colony defaults when not yet seeded.
func EnsureSeeded(ledger taskledger.Ledger, colonyRoot, traceID string) error {
	snap, err := ledger.Snapshot(traceID)
	if err != nil {
		return err
	}
	if snap.EnergyBudget > 0 {
		return nil
	}
	budget := protocol.DefaultEnergyBudget
	if strings.TrimSpace(colonyRoot) != "" {
		manifest, err := colony.LoadColony(colonyRoot)
		if err != nil {
			return err
		}
		budget = manifest.ResolvedEnergyBudget()
	}
	return ledger.SeedEnergy(traceID, budget)
}
