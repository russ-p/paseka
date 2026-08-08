package energy_test

import (
	"testing"

	"github.com/paseka/paseka/internal/energy"
	"github.com/paseka/paseka/internal/protocol"
	"github.com/paseka/paseka/internal/taskledger"
)

func TestEnsureSeededUsesDefaultBudget(t *testing.T) {
	ledger := taskledger.NewMemoryLedger()
	if err := energy.EnsureSeeded(ledger, "", "trace-1"); err != nil {
		t.Fatal(err)
	}
	snap, err := ledger.Snapshot("trace-1")
	if err != nil {
		t.Fatal(err)
	}
	if snap.EnergyBudget != protocol.DefaultEnergyBudget {
		t.Fatalf("budget = %d", snap.EnergyBudget)
	}
}

func TestValidateAddAmount(t *testing.T) {
	if err := energy.ValidateAddAmount(1); err != nil {
		t.Fatalf("unexpected err: %v", err)
	}
	if err := energy.ValidateAddAmount(0); err == nil {
		t.Fatal("expected error for zero amount")
	}
	if err := energy.ValidateAddAmount(-1); err == nil {
		t.Fatal("expected error for negative amount")
	}
}
