package tasks

import (
	"context"
	"fmt"
	"time"

	"github.com/paseka/paseka/internal/protocol"
	"github.com/paseka/paseka/internal/runtime"
	"github.com/paseka/paseka/internal/taskledger"
)

type eventPublisher interface {
	PublishEvent(ctx context.Context, ev protocol.Event) error
}

// AddEnergyInput describes a honey reserve top-up for one trace.
type AddEnergyInput struct {
	TraceID string
	Amount  int
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
