package taskledger

import (
	"encoding/json"
	"errors"
	"fmt"

	"github.com/nats-io/nats.go"
	"github.com/paseka/paseka/internal/protocol"
)

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
	entry, err := l.kv.Get(traceID)
	if err != nil {
		if errors.Is(err, nats.ErrKeyNotFound) {
			return TraceSnapshot{TraceID: traceID, Tasks: map[string]TaskSnapshot{}}, nil
		}
		return TraceSnapshot{}, fmt.Errorf("taskledger: kv get: %w", err)
	}
	var snap TraceSnapshot
	if err := json.Unmarshal(entry.Value(), &snap); err != nil {
		return TraceSnapshot{}, fmt.Errorf("taskledger: decode snapshot: %w", err)
	}
	if snap.Tasks == nil {
		snap.Tasks = map[string]TaskSnapshot{}
	}
	if snap.TraceID == "" {
		snap.TraceID = traceID
	}
	return snap, nil
}

// Apply processes one bus event and persists the updated snapshot.
func (l *KVLedger) Apply(event protocol.Event) (ApplyResult, error) {
	traceID := event.TraceID
	if traceID == "" {
		return ApplyResult{}, fmt.Errorf("taskledger: event missing traceId")
	}
	trace, err := l.Snapshot(traceID)
	if err != nil {
		return ApplyResult{}, err
	}
	res, err := ApplyEvent(trace, event)
	if err != nil {
		return ApplyResult{}, err
	}
	if res.Changed {
		data, err := json.Marshal(res.Trace)
		if err != nil {
			return ApplyResult{}, err
		}
		if _, err := l.kv.Put(traceID, data); err != nil {
			return ApplyResult{}, fmt.Errorf("taskledger: kv put: %w", err)
		}
	}
	return res, nil
}
