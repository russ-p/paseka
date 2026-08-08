package energy

import (
	"context"
	"errors"
	"fmt"

	"github.com/paseka/paseka/internal/protocol"
	"github.com/paseka/paseka/internal/taskledger"
)

// ErrHoneyReserveExhausted is returned when a trace has insufficient honey reserve.
var ErrHoneyReserveExhausted = errors.New("honey reserve exhausted")

// Publisher publishes colony events.
type Publisher interface {
	PublishEvent(ctx context.Context, ev protocol.Event) error
}

// AddInput describes a honey reserve top-up for one trace.
type AddInput struct {
	TraceID string
	Amount  int
	AgentID string
}

// ConsumeInput describes a honey reserve charge for one trace.
type ConsumeInput struct {
	TraceID string
	Amount  int
	Reason  string
	TaskID  string
	AgentID string
}

// Add publishes SIGNAL/energy.add. When the hive reactor is not running,
// the event is also applied to the ledger so CLI callers see immediate state.
func Add(ctx context.Context, slug string, ledger taskledger.Ledger, pub Publisher, in AddInput) (taskledger.TraceSnapshot, error) {
	if ledger == nil {
		return taskledger.TraceSnapshot{}, fmt.Errorf("task ledger is required")
	}
	if pub == nil {
		return taskledger.TraceSnapshot{}, fmt.Errorf("nats client is required")
	}
	if in.TraceID == "" {
		return taskledger.TraceSnapshot{}, fmt.Errorf("trace id is required")
	}
	if err := ValidateAddAmount(in.Amount); err != nil {
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

	reactorRunning, err := ReactorAlive(slug)
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

	return waitIncrease(ledger, in.TraceID, before.EnergyRemaining, in.Amount)
}

// Consume publishes SIGNAL/energy.consume. When the hive reactor is not running,
// the event is also applied to the ledger so CLI callers see immediate state.
func Consume(ctx context.Context, slug, colonyRoot string, ledger taskledger.Ledger, pub Publisher, in ConsumeInput) (taskledger.TraceSnapshot, error) {
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
	if err := EnsureSeeded(ledger, colonyRoot, in.TraceID); err != nil {
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

	reactorRunning, err := ReactorAlive(slug)
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

	return waitDecrease(ledger, in.TraceID, before.EnergyRemaining, amount)
}
