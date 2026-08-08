package tasks

import (
	"context"
	"fmt"
	"time"

	"github.com/paseka/paseka/internal/energy"
	"github.com/paseka/paseka/internal/protocol"
	"github.com/paseka/paseka/internal/taskledger"
)

// KillTraceInput describes a hard kill for one trace.
type KillTraceInput struct {
	TraceID string
	Reason  string
	AgentID string
}

// KillTrace publishes SIGNAL/system.kill. When the hive reactor is not running,
// the event is also applied to the ledger so CLI callers see immediate state.
func KillTrace(ctx context.Context, session *LedgerSession, in KillTraceInput) (taskledger.TraceSnapshot, error) {
	if session == nil || session.Client == nil || session.Ledger == nil {
		return taskledger.TraceSnapshot{}, fmt.Errorf("nats url not configured")
	}
	return killTrace(ctx, session.Colony.Slug, session.Ledger, session.Client, in)
}

func killTrace(ctx context.Context, slug string, ledger taskledger.Ledger, pub eventPublisher, in KillTraceInput) (taskledger.TraceSnapshot, error) {
	if ledger == nil {
		return taskledger.TraceSnapshot{}, fmt.Errorf("task ledger is required")
	}
	if pub == nil {
		return taskledger.TraceSnapshot{}, fmt.Errorf("nats client is required")
	}
	if in.TraceID == "" {
		return taskledger.TraceSnapshot{}, fmt.Errorf("trace id is required")
	}

	before, err := ledger.Snapshot(in.TraceID)
	if err != nil {
		return taskledger.TraceSnapshot{}, err
	}

	agentID := in.AgentID
	if agentID == "" {
		agentID = "cli"
	}
	ev, err := protocol.NewEvent(in.TraceID, agentID, 0, protocol.EventSignal, protocol.SystemKillPayload{
		Kind:   protocol.SignalSystemKill,
		Reason: in.Reason,
	})
	if err != nil {
		return taskledger.TraceSnapshot{}, err
	}

	reactorRunning, err := energy.ReactorAlive(slug)
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

	return waitForTraceKilled(ledger, in.TraceID, before.Killed)
}

func waitForTraceKilled(ledger taskledger.Ledger, traceID string, beforeKilled bool) (taskledger.TraceSnapshot, error) {
	if beforeKilled {
		return ledger.Snapshot(traceID)
	}
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		snap, err := ledger.Snapshot(traceID)
		if err != nil {
			return taskledger.TraceSnapshot{}, err
		}
		if snap.Killed {
			return snap, nil
		}
		time.Sleep(50 * time.Millisecond)
	}
	return ledger.Snapshot(traceID)
}
