package energy

import (
	"time"

	"github.com/russ-p/paseka/internal/taskledger"
)

func waitIncrease(ledger taskledger.Ledger, traceID string, beforeRemaining, amount int) (taskledger.TraceSnapshot, error) {
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

func waitRemaining(ledger taskledger.Ledger, traceID string, amount int) (taskledger.TraceSnapshot, error) {
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		snap, err := ledger.Snapshot(traceID)
		if err != nil {
			return taskledger.TraceSnapshot{}, err
		}
		if snap.EnergyRemaining == amount {
			return snap, nil
		}
		time.Sleep(50 * time.Millisecond)
	}
	return ledger.Snapshot(traceID)
}

func waitDecrease(ledger taskledger.Ledger, traceID string, beforeRemaining, amount int) (taskledger.TraceSnapshot, error) {
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
