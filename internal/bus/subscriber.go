package bus

import (
	"encoding/json"
	"fmt"
	"strconv"
	"sync"
	"time"

	"github.com/nats-io/nats.go"
	"github.com/russ-p/paseka/internal/logging"
	"github.com/russ-p/paseka/internal/protocol"
	"github.com/russ-p/paseka/internal/runs"
)

// EventHandler processes one domain event from the bus.
type EventHandler func(event protocol.Event) error

// Subscription is a live bus event subscription.
type Subscription interface {
	Unsubscribe() error
}

// EventSubscriber subscribes to colony domain events.
type EventSubscriber interface {
	SubscribeEvents(durable string, handler EventHandler) (Subscription, error)
}

var _ EventSubscriber = (*Client)(nil)

const (
	// subscribeMaxDeliveries bounds how many times one message may be handed to
	// a handler that fails deterministically. The consumer is not created with
	// MaxDeliver: an existing durable may have been created without it, and
	// requesting a different value on subscribe would make the server refuse
	// the binding. The client terminates the message itself after this many
	// attempts.
	subscribeMaxDeliveries = 5
	// subscribeMaxBackoff is the ceiling for the delay between attempts.
	subscribeMaxBackoff = 30 * time.Second
)

// SubscribeEvents creates a durable JetStream subscription for colony events.
//
// A handler error used to be NAKed immediately, without a delay or a log line:
// one message that could not be applied was redelivered forever, burning CPU and
// filling the log with receive lines while the cause stayed invisible. Every
// failure is now logged with the event identity and delivery count, retried with
// exponential backoff, and terminated with a durable rejection record after the
// attempt limit so one poisoned event cannot spin the runtime.
func (c *Client) SubscribeEvents(durable string, handler EventHandler) (Subscription, error) {
	if durable == "" {
		durable = "paseka-reactor-" + sanitizeName(c.cfg.Slug)
	}
	subject := EventsWildcard(c.cfg.SubjectPrefix)
	tracker := newDeliveryTracker()
	sub, err := c.js.Subscribe(subject, func(msg *nats.Msg) {
		var ev protocol.Event
		if err := json.Unmarshal(msg.Data, &ev); err != nil {
			c.failedDelivery(msg, protocol.Event{}, "decode", fmt.Errorf("bus: decode event: %w", err), tracker)
			return
		}
		if !protocol.IsDomainEvent(ev.Type) {
			_ = msg.Ack()
			return
		}
		logDomainEvent("receive", msg.Subject, ev)
		if err := handler(ev); err != nil {
			c.failedDelivery(msg, ev, "handle", err, tracker)
			return
		}
		tracker.clear(deliveryKey(msg, ev))
		_ = msg.Ack()
	},
		nats.Durable(durable),
		nats.ManualAck(),
		nats.DeliverNew(),
	)
	if err != nil {
		return nil, fmt.Errorf("bus: subscribe %s: %w", subject, err)
	}
	return sub, nil
}

// failedDelivery logs a failed attempt, retries it with backoff, or terminates
// it after this process's delivery limit. Termination stops the redelivery loop
// but does not delete the message from the stream, so the event remains
// inspectable.
func (c *Client) failedDelivery(msg *nats.Msg, ev protocol.Event, stage string, cause error, tracker *deliveryTracker) {
	key := deliveryKey(msg, ev)
	attempt := tracker.next(key)
	serverDeliveries := serverDeliveryCount(msg)
	fields := []logging.Field{
		logging.F("subject", msg.Subject),
		logging.F("stage", stage),
		logging.F("attempt", strconv.FormatUint(attempt, 10)),
		logging.F("server_deliveries", strconv.FormatUint(serverDeliveries, 10)),
		logging.F("error", cause.Error()),
	}
	if ev.ID != "" {
		fields = append(fields, logging.F("event", ev.ID))
	}
	if ev.TraceID != "" {
		fields = append(fields, logging.F("trace", ev.TraceID))
	}
	if kind := protocol.PayloadKind(ev.Payload); kind != "" {
		fields = append(fields, logging.F("kind", kind))
	}
	if ev.AgentID != "" {
		fields = append(fields, logging.F("agent", ev.AgentID))
	}

	if attempt >= subscribeMaxDeliveries {
		tracker.clear(key)
		if err := msg.Term(); err != nil {
			logging.Component("bus").Warn("terminate rejected event failed", append(fields, logging.F("terminate_error", err.Error()))...)
		}
		logging.Component("bus").Error("bus event rejected after delivery limit", fields...)
		c.recordRejection(msg, ev, attempt, serverDeliveries, cause)
		return
	}

	delay := redeliveryBackoff(attempt)
	logging.Component("bus").Warn("bus event failed; retrying", append(fields, logging.F("retry_in", delay.String()))...)
	if err := msg.NakWithDelay(delay); err != nil {
		_ = msg.Nak()
	}
}

// deliveryTracker counts the attempts this process has made for one message.
// The server's NumDelivered is evidence, not the retry budget: a message that
// already spun in an older binary must get a fresh window, otherwise a deploy
// that fixes its handler would terminate the event before the fix could run.
type deliveryTracker struct {
	mu       sync.Mutex
	attempts map[string]uint64
}

func newDeliveryTracker() *deliveryTracker {
	return &deliveryTracker{attempts: make(map[string]uint64)}
}

func (t *deliveryTracker) next(key string) uint64 {
	t.mu.Lock()
	defer t.mu.Unlock()
	if t.attempts == nil {
		t.attempts = make(map[string]uint64)
	}
	t.attempts[key]++
	return t.attempts[key]
}

func (t *deliveryTracker) clear(key string) {
	t.mu.Lock()
	defer t.mu.Unlock()
	delete(t.attempts, key)
}

// deliveryKey identifies one stream message across redeliveries. The stream
// sequence is stable and available for every JetStream delivery; the event id
// covers a record written before sequence metadata, and the subject is the last
// resort so a key is always produced.
func deliveryKey(msg *nats.Msg, ev protocol.Event) string {
	if meta, err := msg.Metadata(); err == nil && meta != nil && meta.Sequence.Stream > 0 {
		return "seq:" + strconv.FormatUint(meta.Sequence.Stream, 10)
	}
	if ev.ID != "" {
		return "event:" + ev.ID
	}
	return "subject:" + msg.Subject
}

// serverDeliveryCount returns the broker's delivery count for logging. A
// metadata read that fails is reported as zero rather than guessed.
func serverDeliveryCount(msg *nats.Msg) uint64 {
	meta, err := msg.Metadata()
	if err != nil || meta == nil {
		return 0
	}
	return meta.NumDelivered
}

// redeliveryBackoff doubles from one second up to subscribeMaxBackoff. The
// attempt is one-based: the wait after the first failure is one second.
func redeliveryBackoff(attempt uint64) time.Duration {
	if attempt < 1 {
		attempt = 1
	}
	shift := attempt - 1
	if shift > 5 {
		shift = 5
	}
	delay := time.Second << shift
	if delay > subscribeMaxBackoff {
		return subscribeMaxBackoff
	}
	return delay
}

// recordRejection writes the durable evidence for a terminated delivery beside
// the trail it belonged to. The write is best-effort: the loop has already been
// stopped, and a failed record is a warning, not another retry.
func (c *Client) recordRejection(msg *nats.Msg, ev protocol.Event, attempts, serverDeliveries uint64, cause error) {
	if c.cfg.ColonyRoot == "" || ev.TraceID == "" {
		return
	}
	trail := runs.Trail{ColonyRoot: c.cfg.ColonyRoot, TraceID: ev.TraceID}
	err := trail.AppendRejection(runs.RejectedEvent{
		EventID:          ev.ID,
		TraceID:          ev.TraceID,
		Type:             string(ev.Type),
		Kind:             protocol.PayloadKind(ev.Payload),
		AgentID:          ev.AgentID,
		Subject:          msg.Subject,
		Attempts:         attempts,
		ServerDeliveries: serverDeliveries,
		Error:            cause.Error(),
		RejectedAt:       time.Now().UTC(),
	})
	if err != nil {
		logging.Component("bus").Warn("rejected event record failed",
			logging.F("trace", ev.TraceID),
			logging.F("path", trail.RejectionsPath()),
			logging.F("error", err.Error()))
	}
}
