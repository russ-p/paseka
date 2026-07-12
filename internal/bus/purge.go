package bus

import (
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/nats-io/nats.go"
	"github.com/paseka/paseka/internal/protocol"
)

// PurgeTracePlan describes bus artifacts that would be removed for one trace.
type PurgeTracePlan struct {
	TraceID         string
	TaskLedgerKey   bool
	EventCount      int
	ArtifactObjects []string
}

// Empty reports whether purge would affect nothing.
func (p PurgeTracePlan) Empty() bool {
	return !p.TaskLedgerKey && p.EventCount == 0 && len(p.ArtifactObjects) == 0
}

// PurgeTraceResult reports bus artifacts removed for one trace.
type PurgeTraceResult struct {
	KeysRemoved    []string `json:"keysRemoved,omitempty"`
	EventsRemoved  int      `json:"eventsRemoved"`
	ObjectsRemoved []string `json:"objectsRemoved,omitempty"`
}

// PlanPurgeTrace lists bus artifacts that would be removed for one trace.
func (c *Client) PlanPurgeTrace(traceID string) (PurgeTracePlan, error) {
	if traceID == "" {
		return PurgeTracePlan{}, fmt.Errorf("bus: traceId is required")
	}
	plan := PurgeTracePlan{TraceID: traceID}

	kv, err := c.js.KeyValue(kvBucketName(c.cfg.Slug))
	if err == nil {
		if _, err := kv.Get(traceID); err == nil {
			plan.TaskLedgerKey = true
		} else if !errors.Is(err, nats.ErrKeyNotFound) {
			return plan, fmt.Errorf("bus: task ledger kv: %w", err)
		}
	} else if !errors.Is(err, nats.ErrBucketNotFound) {
		return plan, fmt.Errorf("bus: task ledger kv: %w", err)
	}

	events, err := c.ReplayTrace(traceID)
	if err != nil {
		return plan, err
	}
	plan.EventCount = len(events)

	os, err := c.js.ObjectStore(objectStoreName(c.cfg.Slug))
	if err == nil {
		objs, err := os.List()
		if err != nil && !errors.Is(err, nats.ErrNoObjectsFound) {
			return plan, fmt.Errorf("bus: list artifacts: %w", err)
		}
		prefix := traceID + "-"
		for _, obj := range objs {
			name := obj.Name
			if strings.HasPrefix(name, prefix) && strings.HasSuffix(name, ".diff") {
				plan.ArtifactObjects = append(plan.ArtifactObjects, name)
			}
		}
	} else if !errors.Is(err, nats.ErrBucketNotFound) && !errors.Is(err, nats.ErrStreamNotFound) {
		return plan, fmt.Errorf("bus: artifacts store: %w", err)
	}

	return plan, nil
}

// PurgeTrace removes task-ledger KV, matching stream events, and trace artifacts.
func (c *Client) PurgeTrace(traceID string) (PurgeTraceResult, error) {
	if traceID == "" {
		return PurgeTraceResult{}, fmt.Errorf("bus: traceId is required")
	}
	var res PurgeTraceResult
	if err := c.purgeTaskLedgerKey(traceID, &res); err != nil {
		return res, err
	}
	if err := c.purgeTraceEvents(traceID, &res); err != nil {
		return res, err
	}
	if err := c.purgeTraceArtifacts(traceID, &res); err != nil {
		return res, err
	}
	return res, nil
}

func (c *Client) purgeTaskLedgerKey(traceID string, res *PurgeTraceResult) error {
	kv, err := c.js.KeyValue(kvBucketName(c.cfg.Slug))
	if err != nil {
		if errors.Is(err, nats.ErrBucketNotFound) {
			return nil
		}
		return fmt.Errorf("bus: task ledger kv: %w", err)
	}
	if err := kv.Delete(traceID); err != nil {
		if errors.Is(err, nats.ErrKeyNotFound) {
			return nil
		}
		return fmt.Errorf("bus: purge task ledger key %q: %w", traceID, err)
	}
	res.KeysRemoved = append(res.KeysRemoved, traceID)
	return nil
}

func (c *Client) purgeTraceEvents(traceID string, res *PurgeTraceResult) error {
	subject := EventsWildcard(c.cfg.SubjectPrefix)
	sub, err := c.js.SubscribeSync(subject, nats.DeliverAll(), nats.AckNone())
	if err != nil {
		return fmt.Errorf("bus: purge events subscribe: %w", err)
	}
	defer sub.Unsubscribe()

	stream := streamName(c.cfg.Slug)
	var seqs []uint64
	for {
		msg, err := sub.NextMsg(200 * time.Millisecond)
		if err == nats.ErrTimeout {
			break
		}
		if err != nil {
			return fmt.Errorf("bus: purge events next: %w", err)
		}
		var ev protocol.Event
		if err := json.Unmarshal(msg.Data, &ev); err != nil {
			continue
		}
		if ev.TraceID != traceID || !protocol.IsDomainEvent(ev.Type) {
			continue
		}
		meta, err := msg.Metadata()
		if err != nil {
			continue
		}
		seqs = append(seqs, meta.Sequence.Stream)
	}
	for _, seq := range seqs {
		if err := c.js.DeleteMsg(stream, seq); err != nil {
			return fmt.Errorf("bus: purge event seq %d: %w", seq, err)
		}
		res.EventsRemoved++
	}
	return nil
}

func (c *Client) purgeTraceArtifacts(traceID string, res *PurgeTraceResult) error {
	os, err := c.js.ObjectStore(objectStoreName(c.cfg.Slug))
	if err != nil {
		if errors.Is(err, nats.ErrBucketNotFound) || errors.Is(err, nats.ErrStreamNotFound) {
			return nil
		}
		return fmt.Errorf("bus: artifacts store: %w", err)
	}
	objs, err := os.List()
	if err != nil && !errors.Is(err, nats.ErrNoObjectsFound) {
		return fmt.Errorf("bus: list artifacts: %w", err)
	}
	prefix := traceID + "-"
	for _, obj := range objs {
		name := obj.Name
		if !strings.HasPrefix(name, prefix) || !strings.HasSuffix(name, ".diff") {
			continue
		}
		if err := os.Delete(name); err != nil {
			return fmt.Errorf("bus: delete artifact %q: %w", name, err)
		}
		res.ObjectsRemoved = append(res.ObjectsRemoved, name)
	}
	return nil
}
