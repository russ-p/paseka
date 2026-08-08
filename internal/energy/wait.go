package energy

import (
	"time"

	"github.com/paseka/paseka/internal/taskledger"
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
