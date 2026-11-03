package taskledger

import (
	"fmt"
	"sort"
	"sync"

	"github.com/russ-p/paseka/internal/protocol"
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

// TraceIDs lists the trails the ledger holds, so a reactor restart can find
// work deferred while no reactor was running.
func (l *MemoryLedger) TraceIDs() ([]string, error) {
	l.mu.Lock()
	defer l.mu.Unlock()
	ids := make([]string, 0, len(l.traces))
	for traceID := range l.traces {
		ids = append(ids, traceID)
	}
	sort.Strings(ids)
	return ids, nil
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

// DeferPendingDispatch records deferred work in the trace snapshot.
func (l *MemoryLedger) DeferPendingDispatch(traceID string, dispatch PendingDispatch) (bool, error) {
	if traceID == "" {
		return false, fmt.Errorf("taskledger: traceId is required")
	}
	l.mu.Lock()
	defer l.mu.Unlock()
	trace, ok := l.traces[traceID]
	if !ok {
		trace = TraceSnapshot{TraceID: traceID, Tasks: map[string]TaskSnapshot{}}
	}
	updated, changed := DeferPendingDispatch(trace, dispatch)
	if !changed {
		return false, nil
	}
	l.traces[traceID] = updated
	return true, nil
}

// ResolvePendingDispatch closes one deferred item in the trace snapshot.
func (l *MemoryLedger) ResolvePendingDispatch(traceID, dispatchID string, status PendingDispatchStatus) error {
	if traceID == "" {
		return fmt.Errorf("taskledger: traceId is required")
	}
	l.mu.Lock()
	defer l.mu.Unlock()
	trace, ok := l.traces[traceID]
	if !ok {
		return nil
	}
	updated, changed := ResolvePendingDispatch(trace, dispatchID, status)
	if changed {
		l.traces[traceID] = updated
	}
	return nil
}

// MarkPendingDispatchUnresolved keeps deferred work visible when its source
// event is gone.
func (l *MemoryLedger) MarkPendingDispatchUnresolved(traceID, dispatchID, diagnostic string) error {
	if traceID == "" {
		return fmt.Errorf("taskledger: traceId is required")
	}
	l.mu.Lock()
	defer l.mu.Unlock()
	trace, ok := l.traces[traceID]
	if !ok {
		return nil
	}
	updated, changed := MarkPendingDispatchUnresolved(trace, dispatchID, diagnostic)
	if changed {
		l.traces[traceID] = updated
	}
	return nil
}

// SeedEnergy initializes the honey reserve when not yet seeded.
func (l *MemoryLedger) SeedEnergy(traceID string, budget int) error {
	_, _, err := l.EnsureEnergySeed(traceID, budget)
	return err
}

// EnsureEnergySeed performs the authoritative seed transition and returns the
// winning allocation. In-memory locking makes concurrent callers observe one
// budget and one seed identity.
func (l *MemoryLedger) EnsureEnergySeed(traceID string, budget int) (EnergySeedSnapshot, bool, error) {
	if traceID == "" {
		return EnergySeedSnapshot{}, false, fmt.Errorf("taskledger: traceId is required")
	}
	l.mu.Lock()
	defer l.mu.Unlock()
	trace, ok := l.traces[traceID]
	if !ok {
		trace = TraceSnapshot{TraceID: traceID, Tasks: map[string]TaskSnapshot{}}
	}
	updated, seed, created := EnsureEnergySeed(trace, budget)
	if created {
		l.traces[traceID] = updated
	}
	return seed, created, nil
}

// MarkEnergySeedPublished records the seed observation send on the winning
// record. It is idempotent and refuses a mismatched seed identity.
func (l *MemoryLedger) MarkEnergySeedPublished(traceID, seedID string) error {
	if traceID == "" {
		return fmt.Errorf("taskledger: traceId is required")
	}
	l.mu.Lock()
	defer l.mu.Unlock()
	updated, changed, err := MarkEnergySeedPublished(l.traces[traceID], seedID)
	if err != nil {
		return err
	}
	if changed {
		l.traces[traceID] = updated
	}
	return nil
}

// ClaimEnergyPausePublished marks the open pause episode published, so only one
// caller sends the paused observation.
func (l *MemoryLedger) ClaimEnergyPausePublished(traceID string) (EnergyPauseSnapshot, bool, error) {
	if traceID == "" {
		return EnergyPauseSnapshot{}, false, fmt.Errorf("taskledger: traceId is required")
	}
	l.mu.Lock()
	defer l.mu.Unlock()
	trace := l.traces[traceID]
	updated, pause, ok := ClaimEnergyPausePublished(trace)
	if !ok {
		return EnergyPauseSnapshot{}, false, nil
	}
	l.traces[traceID] = updated
	return pause, true, nil
}

// ClearEnergyPause closes the open pause episode, so only one resumed
// observation is sent for it.
func (l *MemoryLedger) ClearEnergyPause(traceID string) (EnergyPauseSnapshot, bool, error) {
	if traceID == "" {
		return EnergyPauseSnapshot{}, false, fmt.Errorf("taskledger: traceId is required")
	}
	l.mu.Lock()
	defer l.mu.Unlock()
	trace := l.traces[traceID]
	updated, pause, ok := CloseEnergyPause(trace)
	if !ok {
		return EnergyPauseSnapshot{}, false, nil
	}
	l.traces[traceID] = updated
	return pause, true, nil
}
