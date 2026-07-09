package taskledger

import (
	"encoding/json"
	"errors"
	"fmt"

	"github.com/nats-io/nats.go"
	"github.com/paseka/paseka/internal/protocol"
)

const kvApplyMaxRetries = 16

// KVLedger persists trace snapshots in a JetStream KV bucket.
type KVLedger struct {
	kv nats.KeyValue
}

// NewKVLedger wraps a JetStream KV bucket as a task ledger.
func NewKVLedger(kv nats.KeyValue) *KVLedger {
	return &KVLedger{kv: kv}
}

// Snapshot returns the current task ledger for a trace.
func (l *KVLedger) Snapshot(traceID string) (TraceSnapshot, error) {
	if traceID == "" {
		return TraceSnapshot{}, fmt.Errorf("taskledger: traceId is required")
	}
	trace, _, err := l.loadTrace(traceID)
	return trace, err
}

// Apply processes one bus event and persists the updated snapshot.
func (l *KVLedger) Apply(event protocol.Event) (ApplyResult, error) {
	traceID := event.TraceID
	if traceID == "" {
		return ApplyResult{}, fmt.Errorf("taskledger: event missing traceId")
	}
	return l.mutateCAS(traceID, func(trace TraceSnapshot) (TraceSnapshot, ApplyResult, error) {
		res, err := ApplyEvent(trace, event)
		if err != nil {
			return TraceSnapshot{}, ApplyResult{}, err
		}
		return res.Trace, res, nil
	})
}

// SeedEnergy initializes the honey reserve when not yet seeded.
func (l *KVLedger) SeedEnergy(traceID string, budget int) error {
	if traceID == "" {
		return fmt.Errorf("taskledger: traceId is required")
	}
	_, err := l.mutateCAS(traceID, func(trace TraceSnapshot) (TraceSnapshot, ApplyResult, error) {
		updated, changed := EnsureEnergySeeded(trace, budget)
		return updated, ApplyResult{Trace: updated, Changed: changed}, nil
	})
	return err
}

func (l *KVLedger) loadTrace(traceID string) (TraceSnapshot, uint64, error) {
	entry, err := l.kv.Get(traceID)
	if err != nil {
		if errors.Is(err, nats.ErrKeyNotFound) {
			return TraceSnapshot{TraceID: traceID, Tasks: map[string]TaskSnapshot{}}, 0, nil
		}
		return TraceSnapshot{}, 0, fmt.Errorf("taskledger: kv get: %w", err)
	}
	var snap TraceSnapshot
	if err := json.Unmarshal(entry.Value(), &snap); err != nil {
		return TraceSnapshot{}, 0, fmt.Errorf("taskledger: decode snapshot: %w", err)
	}
	if snap.Tasks == nil {
		snap.Tasks = map[string]TaskSnapshot{}
	}
	if snap.TraceID == "" {
		snap.TraceID = traceID
	}
	return snap, entry.Revision(), nil
}

func (l *KVLedger) mutateCAS(traceID string, mutate func(TraceSnapshot) (TraceSnapshot, ApplyResult, error)) (ApplyResult, error) {
	for attempt := 0; attempt < kvApplyMaxRetries; attempt++ {
		trace, revision, err := l.loadTrace(traceID)
		if err != nil {
			return ApplyResult{}, err
		}

		updated, res, err := mutate(trace)
		if err != nil {
			return ApplyResult{}, err
		}
		if !res.Changed {
			return res, nil
		}

		data, err := json.Marshal(updated)
		if err != nil {
			return ApplyResult{}, err
		}

		var putErr error
		if revision == 0 {
			_, putErr = l.kv.Create(traceID, data)
		} else {
			_, putErr = l.kv.Update(traceID, data, revision)
		}
		if putErr == nil {
			res.Trace = updated
			return res, nil
		}
		if errors.Is(putErr, nats.ErrKeyExists) || errors.Is(putErr, nats.ErrKeyNotFound) {
			continue
		}
		return ApplyResult{}, fmt.Errorf("taskledger: kv put: %w", putErr)
	}
	return ApplyResult{}, fmt.Errorf("taskledger: kv apply conflict after %d retries", kvApplyMaxRetries)
}
