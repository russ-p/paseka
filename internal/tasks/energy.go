package tasks

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/paseka/paseka/internal/colony"
	"github.com/paseka/paseka/internal/protocol"
	"github.com/paseka/paseka/internal/runtime"
	"github.com/paseka/paseka/internal/taskledger"
)

// ErrHoneyReserveExhausted is returned when a trace has insufficient honey reserve.
var ErrHoneyReserveExhausted = errors.New("honey reserve exhausted")

type eventPublisher interface {
	PublishEvent(ctx context.Context, ev protocol.Event) error
}

// AddEnergyInput describes a honey reserve top-up for one trace.
type AddEnergyInput struct {
	TraceID string
	Amount  int
	AgentID string
}

// ConsumeEnergyInput describes a honey reserve charge for one trace.
type ConsumeEnergyInput struct {
	TraceID string
	Amount  int
	Reason  string
	TaskID  string
	AgentID string
}

// AddEnergy publishes SIGNAL/energy.add. When the hive reactor is not running,
// the event is also applied to the ledger so CLI callers see immediate state.
func AddEnergy(ctx context.Context, session *LedgerSession, in AddEnergyInput) (taskledger.TraceSnapshot, error) {
	if session == nil || session.Client == nil || session.Ledger == nil {
		return taskledger.TraceSnapshot{}, fmt.Errorf("nats url not configured")
	}
	return addEnergy(ctx, session.Colony.Slug, session.Ledger, session.Client, in)
}

func addEnergy(ctx context.Context, slug string, ledger taskledger.Ledger, pub eventPublisher, in AddEnergyInput) (taskledger.TraceSnapshot, error) {
	if ledger == nil {
		return taskledger.TraceSnapshot{}, fmt.Errorf("task ledger is required")
	}
	if pub == nil {
		return taskledger.TraceSnapshot{}, fmt.Errorf("nats client is required")
	}
	if in.TraceID == "" {
		return taskledger.TraceSnapshot{}, fmt.Errorf("trace id is required")
	}
	if err := runtime.ValidateEnergyAddAmount(in.Amount); err != nil {
		return taskledger.TraceSnapshot{}, err
	}

	before, err := ledger.Snapshot(in.TraceID)
	if err != nil {
		return taskledger.TraceSnapshot{}, err
	}

	agentID := in.AgentID
	if agentID == "" {
		agentID = "cli"
	}
	ev, err := protocol.NewEvent(in.TraceID, agentID, 0, protocol.EventSignal, protocol.EnergyAddPayload{
		Kind:   protocol.SignalEnergyAdd,
		Amount: in.Amount,
	})
	if err != nil {
		return taskledger.TraceSnapshot{}, err
	}

	reactorRunning, err := isReactorRunning(slug)
	if err != nil {
		return taskledger.TraceSnapshot{}, err
	}

	if err := pub.PublishEvent(ctx, ev); err != nil {
		return taskledger.TraceSnapshot{}, err
	}
	if !reactorRunning {
		if _, err := ledger.Apply(ev); err != nil {
			return taskledger.TraceSnapshot{}, err
		}
		return ledger.Snapshot(in.TraceID)
	}

	return waitForEnergyIncrease(ledger, in.TraceID, before.EnergyRemaining, in.Amount)
}

// ConsumeEnergy publishes SIGNAL/energy.consume. When the hive reactor is not running,
// the event is also applied to the ledger so CLI callers see immediate state.
func ConsumeEnergy(ctx context.Context, session *LedgerSession, in ConsumeEnergyInput) (taskledger.TraceSnapshot, error) {
	if session == nil || session.Client == nil || session.Ledger == nil {
		return taskledger.TraceSnapshot{}, fmt.Errorf("nats url not configured")
	}
	return consumeEnergy(ctx, session.Colony.Slug, session.Colony.ColonyRoot, session.Ledger, session.Client, in)
}

func consumeEnergy(ctx context.Context, slug, colonyRoot string, ledger taskledger.Ledger, pub eventPublisher, in ConsumeEnergyInput) (taskledger.TraceSnapshot, error) {
	if ledger == nil {
		return taskledger.TraceSnapshot{}, fmt.Errorf("task ledger is required")
	}
	if pub == nil {
		return taskledger.TraceSnapshot{}, fmt.Errorf("nats client is required")
	}
	if in.TraceID == "" {
		return taskledger.TraceSnapshot{}, fmt.Errorf("trace id is required")
	}
	amount := in.Amount
	if amount <= 0 {
		amount = 1
	}
	if err := ensureEnergySeeded(ledger, colonyRoot, in.TraceID); err != nil {
		return taskledger.TraceSnapshot{}, err
	}
	before, err := ledger.Snapshot(in.TraceID)
	if err != nil {
		return taskledger.TraceSnapshot{}, err
	}
	if !taskledger.HasEnergy(before, amount) {
		return before, ErrHoneyReserveExhausted
	}

	agentID := in.AgentID
	if agentID == "" {
		agentID = "cli"
	}
	ev, err := protocol.NewEvent(in.TraceID, agentID, 0, protocol.EventSignal, protocol.EnergyConsumePayload{
		Kind:   protocol.SignalEnergyConsume,
		Amount: amount,
		Reason: in.Reason,
		TaskID: in.TaskID,
	})
	if err != nil {
		return taskledger.TraceSnapshot{}, err
	}

	reactorRunning, err := isReactorRunning(slug)
	if err != nil {
		return taskledger.TraceSnapshot{}, err
	}

	if err := pub.PublishEvent(ctx, ev); err != nil {
		return taskledger.TraceSnapshot{}, err
	}
	if !reactorRunning {
		if _, err := ledger.Apply(ev); err != nil {
			return taskledger.TraceSnapshot{}, err
		}
		return ledger.Snapshot(in.TraceID)
	}

	return waitForEnergyDecrease(ledger, in.TraceID, before.EnergyRemaining, amount)
}

func ensureEnergySeeded(ledger taskledger.Ledger, colonyRoot, traceID string) error {
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

func isReactorRunning(slug string) (bool, error) {
	st, err := runtime.ResolveStatus(slug)
	if err != nil {
		return false, err
	}
	return st.Alive && st.Status == runtime.RuntimeStatusRunning, nil
}

func waitForEnergyIncrease(ledger taskledger.Ledger, traceID string, beforeRemaining, amount int) (taskledger.TraceSnapshot, error) {
	target := beforeRemaining + amount
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		snap, err := ledger.Snapshot(traceID)
		if err != nil {
			return taskledger.TraceSnapshot{}, err
		}
		if snap.EnergyRemaining >= target {
			return snap, nil
		}
		time.Sleep(50 * time.Millisecond)
	}
	return ledger.Snapshot(traceID)
}

func waitForEnergyDecrease(ledger taskledger.Ledger, traceID string, beforeRemaining, amount int) (taskledger.TraceSnapshot, error) {
	target := beforeRemaining - amount
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		snap, err := ledger.Snapshot(traceID)
		if err != nil {
			return taskledger.TraceSnapshot{}, err
		}
		if snap.EnergyRemaining <= target {
			return snap, nil
		}
		time.Sleep(50 * time.Millisecond)
	}
	return ledger.Snapshot(traceID)
}
