package console

import (
	"context"
	"fmt"

	"github.com/paseka/paseka/internal/colony"
	"github.com/paseka/paseka/internal/energy"
	"github.com/paseka/paseka/internal/taskledger"
	"github.com/paseka/paseka/internal/tasks"
)

// EnergyAddRequest is the JSON body for POST /api/traces/:traceId/energy/add.
type EnergyAddRequest struct {
	Amount int `json:"amount"`
}

// EnergyAddResponse is returned after topping up a trace honey reserve.
type EnergyAddResponse struct {
	TraceID         string `json:"traceId"`
	Amount          int    `json:"amount"`
	EnergyBudget    int    `json:"energyBudget"`
	EnergyRemaining int    `json:"energyRemaining"`
	EnergyAdded     int    `json:"energyAdded"`
	EnergyAllocated int    `json:"energyAllocated"`
	LowEnergy       bool   `json:"lowEnergy"`
}

// AddTraceEnergy publishes SIGNAL/energy.add for one trace from Queen Console.
func AddTraceEnergy(ctx context.Context, colonyCtx colony.Context, traceID string, amount int) (EnergyAddResponse, error) {
	if traceID == "" {
		return EnergyAddResponse{}, fmt.Errorf("trace id is required")
	}
	if err := energy.ValidateAddAmount(amount); err != nil {
		return EnergyAddResponse{}, err
	}

	session, err := tasks.OpenLedger(colonyCtx)
	if err != nil {
		return EnergyAddResponse{}, err
	}
	defer session.Close()
	if session.Publisher == nil || session.Ledger == nil {
		return EnergyAddResponse{}, fmt.Errorf("nats url not configured")
	}

	snap, err := tasks.AddEnergy(ctx, session, tasks.AddEnergyInput{
		TraceID: traceID,
		Amount:  amount,
		AgentID: "console",
	})
	if err != nil {
		return EnergyAddResponse{}, err
	}

	budget := snap.EnergyBudget
	remaining := snap.EnergyRemaining
	added := snap.EnergyAdded
	allocated := taskledger.Allocated(budget, added)
	low := budget > 0 && remaining <= budget/4
	return EnergyAddResponse{
		TraceID:         traceID,
		Amount:          amount,
		EnergyBudget:    budget,
		EnergyRemaining: remaining,
		EnergyAdded:     added,
		EnergyAllocated: allocated,
		LowEnergy:       low,
	}, nil
}
