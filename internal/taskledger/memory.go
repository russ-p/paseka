package taskledger

import (
	"fmt"
	"sync"

	"github.com/paseka/paseka/internal/protocol"
)

// MemoryLedger is an in-process Ledger for tests and single-process reactors.
type MemoryLedger struct {
	mu     sync.Mutex
	traces map[string]TraceSnapshot
}

// NewMemoryLedger creates an empty in-memory ledger.
func NewMemoryLedger() *MemoryLedger {
	return &MemoryLedger{traces: make(map[string]TraceSnapshot)}
}

// Snapshot returns the current task ledger for a trace.
func (l *MemoryLedger) Snapshot(traceID string) (TraceSnapshot, error) {
	if traceID == "" {
		return TraceSnapshot{}, fmt.Errorf("taskledger: traceId is required")
	}
	l.mu.Lock()
	defer l.mu.Unlock()
	snap, ok := l.traces[traceID]
	if !ok {
		return TraceSnapshot{TraceID: traceID, Tasks: map[string]TaskSnapshot{}}, nil
	}
	if snap.Tasks == nil {
		snap.Tasks = map[string]TaskSnapshot{}
	}
	return snap, nil
}

// Apply processes one bus event and updates in-memory state.
func (l *MemoryLedger) Apply(event protocol.Event) (ApplyResult, error) {
	traceID := event.TraceID
	if traceID == "" {
		return ApplyResult{}, fmt.Errorf("taskledger: event missing traceId")
	}
	l.mu.Lock()
	defer l.mu.Unlock()
	trace, ok := l.traces[traceID]
	if !ok {
		trace = TraceSnapshot{TraceID: traceID, Tasks: map[string]TaskSnapshot{}}
	}
	res, err := ApplyEvent(trace, event)
	if err != nil {
		return ApplyResult{}, err
	}
	if res.Changed {
		l.traces[traceID] = res.Trace
	}
	return res, nil
}

// SeedEnergy initializes the honey reserve when not yet seeded.
func (l *MemoryLedger) SeedEnergy(traceID string, budget int) error {
	if traceID == "" {
		return fmt.Errorf("taskledger: traceId is required")
	}
	l.mu.Lock()
	defer l.mu.Unlock()
	trace, ok := l.traces[traceID]
	if !ok {
		trace = TraceSnapshot{TraceID: traceID, Tasks: map[string]TaskSnapshot{}}
	}
	updated, changed := EnsureEnergySeeded(trace, budget)
	if !changed {
		return nil
	}
	l.traces[traceID] = updated
	return nil
}
