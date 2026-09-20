package bus

import (
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/nats-io/nats.go"
)

// TraceActivity is the newest task-ledger update observed for one trace.
type TraceActivity struct {
	TraceID  string
	LastUsed time.Time
}

// ledgerActivityJSON is a minimal view of a persisted trace snapshot. It keeps
// the bus package decoupled from the task-ledger projection while still reading
// the one field age-based cleanup needs.
type ledgerActivityJSON struct {
	Tasks map[string]struct {
		UpdatedAt time.Time `json:"updatedAt"`
	} `json:"tasks"`
}

// ListTraceActivity returns the newest task update time for every trace in the
// task-ledger KV bucket. Traces with no task timestamps report a zero time and
// are treated as not correlatable for age-based cleanup.
func (c *Client) ListTraceActivity() ([]TraceActivity, error) {
	kv, err := c.js.KeyValue(kvBucketName(c.cfg.Slug))
	if err != nil {
		if errors.Is(err, nats.ErrBucketNotFound) {
			return nil, nil
		}
		return nil, fmt.Errorf("bus: task ledger kv: %w", err)
	}
	keys, err := kv.Keys()
	if err != nil {
		if errors.Is(err, nats.ErrNoKeysFound) {
			return nil, nil
		}
		return nil, fmt.Errorf("bus: list task ledger keys: %w", err)
	}
	out := make([]TraceActivity, 0, len(keys))
	for _, key := range keys {
		entry, err := kv.Get(key)
		if err != nil {
			if errors.Is(err, nats.ErrKeyNotFound) {
				continue
			}
			return nil, fmt.Errorf("bus: get task ledger key %q: %w", key, err)
		}
		out = append(out, TraceActivity{TraceID: key, LastUsed: lastTaskUpdatedAt(entry.Value())})
	}
	return out, nil
}

func lastTaskUpdatedAt(raw []byte) time.Time {
	var snap ledgerActivityJSON
	if err := json.Unmarshal(raw, &snap); err != nil {
		return time.Time{}
	}
	var last time.Time
	for _, task := range snap.Tasks {
		if task.UpdatedAt.After(last) {
			last = task.UpdatedAt
		}
	}
	return last
}
