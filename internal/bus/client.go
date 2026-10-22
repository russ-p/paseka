package bus

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"time"

	"github.com/nats-io/nats.go"
	"github.com/russ-p/paseka/internal/protocol"
	"github.com/russ-p/paseka/internal/trailconfig"
)

// Client is a NATS/JetStream connection for one colony.
type Client struct {
	cfg    Config
	nc     *nats.Conn
	js     nats.JetStreamContext
	closed bool
}

// Connect dials NATS, ensures JetStream resources, and returns a ready client.
func Connect(cfg Config) (*Client, error) {
	if !cfg.Enabled() {
		return nil, fmt.Errorf("bus: nats url is required")
	}
	nc, err := nats.Connect(cfg.URL,
		nats.Name("paseka-"+cfg.Slug),
		nats.Timeout(5*time.Second),
		nats.ReconnectWait(time.Second),
		nats.MaxReconnects(10),
	)
	if err != nil {
		return nil, fmt.Errorf("bus: connect %s: %w", cfg.URL, err)
	}
	js, err := nc.JetStream()
	if err != nil {
		nc.Close()
		return nil, fmt.Errorf("bus: jetstream: %w", err)
	}
	c := &Client{cfg: cfg, nc: nc, js: js}
	if err := ensureStream(js, cfg); err != nil {
		nc.Close()
		return nil, err
	}
	return c, nil
}

// ConnectFull dials NATS and ensures stream, KV, and object store resources.
func ConnectFull(cfg Config) (*Client, error) {
	c, err := Connect(cfg)
	if err != nil {
		return nil, err
	}
	if err := c.EnsureStorage(); err != nil {
		c.Close()
		return nil, err
	}
	return c, nil
}

// Config returns the client configuration.
func (c *Client) Config() Config { return c.cfg }

// JetStream returns the underlying JetStream context.
func (c *Client) JetStream() nats.JetStreamContext { return c.js }

// Conn returns the underlying NATS connection.
func (c *Client) Conn() *nats.Conn { return c.nc }

// EnsureStorage provisions KV and object store buckets.
func (c *Client) EnsureStorage() error {
	if err := ensureKVBuckets(c.js, c.cfg); err != nil {
		return err
	}
	return ensureObjectStore(c.js, c.cfg)
}

// Health checks connectivity and JetStream availability.
func (c *Client) Health() error {
	if c.nc == nil || !c.nc.IsConnected() {
		return fmt.Errorf("bus: not connected")
	}
	if _, err := c.js.AccountInfo(); err != nil {
		return fmt.Errorf("bus: jetstream unavailable: %w", err)
	}
	if _, err := c.js.StreamInfo(streamName(c.cfg.Slug)); err != nil {
		return fmt.Errorf("bus: stream missing: %w", err)
	}
	return nil
}

// PublishEvent publishes a domain event to JetStream.
//
// The trail record is written first, so the window between "published" and
// "recorded" — the window that loses events when a broker dies mid-run — is
// closed rather than widened. The record is the colony's account of what it
// decided to do: an event it holds may still have failed to reach the bus, and
// `paseka verify` is the way to tell.
func (c *Client) PublishEvent(ctx context.Context, event protocol.Event) error {
	if !protocol.IsDomainEvent(event.Type) {
		return nil
	}
	if c.cfg.ColonyRoot != "" {
		if _, err := trailconfig.EnsureForRoot(c.cfg.ColonyRoot, event.TraceID); err != nil && !errors.Is(err, trailconfig.ErrNotColony) {
			return fmt.Errorf("bus: trail config: %w", err)
		}
	}
	// The record is best-effort and never blocks the publish: see recordTrailEvent.
	_ = recordTrailEvent(c.cfg.ColonyRoot, event)
	if c.js == nil {
		return fmt.Errorf("bus: not connected")
	}
	subject := EventSubject(c.cfg.SubjectPrefix, event)
	data, err := json.Marshal(event)
	if err != nil {
		return fmt.Errorf("bus: marshal event: %w", err)
	}
	msg := &nats.Msg{
		Subject: subject,
		Data:    data,
		Header:  nats.Header{},
	}
	if event.TraceID != "" {
		msg.Header.Set("Trace-Id", event.TraceID)
	}
	if event.AgentID != "" {
		msg.Header.Set("Agent-Id", event.AgentID)
	}
	msg.Header.Set("Event-Type", string(event.Type))
	if _, err := c.js.PublishMsg(msg); err != nil {
		return fmt.Errorf("bus: publish %s: %w", subject, err)
	}
	logDomainEvent("publish", subject, event)
	return nil
}

// ConfirmsPublication reports true: a nil return from PublishEvent means JetStream
// accepted the message.
func (c *Client) ConfirmsPublication() bool { return true }

// StoreArtifact stores bytes in the colony object store and returns the object name.
//
// Proposals no longer come through here — see ArtifactStore — so this only serves
// artifacts written by earlier versions, which `paseka purge --bus` still deletes.
func (c *Client) StoreArtifact(name string, data []byte) (string, error) {
	if err := ensureObjectStore(c.js, c.cfg); err != nil {
		return "", err
	}
	os, err := c.js.ObjectStore(objectStoreName(c.cfg.Slug))
	if err != nil {
		return "", err
	}
	meta := &nats.ObjectMeta{Name: name}
	if _, err := os.Put(meta, bytesReader(data)); err != nil {
		return "", fmt.Errorf("bus: store artifact %q: %w", name, err)
	}
	return name, nil
}

const (
	// replayIdleTimeout paces `paseka replay`, which prints what it has read and
	// is a listing rather than a verdict.
	replayIdleTimeout = 200 * time.Millisecond
	// exhaustiveIdleTimeout paces a read whose result is compared against another
	// record, where stopping early would invent a disagreement.
	exhaustiveIdleTimeout = 2 * time.Second
	// exhaustiveReadBudget bounds that read outright, so a stream that keeps
	// trickling cannot hold the command open.
	exhaustiveReadBudget = 30 * time.Second
)

// ReplayTrace returns all domain events for a trace from JetStream.
//
// It stops after the first stretch of silence, which is enough for a listing and
// not enough for a verdict: a caller that compares this against another record
// must use ReplayTraceExhaustive, because an event that arrives later would read
// as one the broker never took.
func (c *Client) ReplayTrace(traceID string) ([]protocol.Event, error) {
	events, _, err := c.replayTrace(traceID, replayIdleTimeout, false)
	return events, err
}

// ReplayTraceExhaustive returns all domain events for a trace, waiting until the
// stream has actually been drained rather than until it goes quiet.
//
// complete is false when the budget ran out with the stream still delivering.
// That is not an empty answer — it is an unknown one, and a caller comparing
// against another record must report it as such instead of calling it a
// divergence.
func (c *Client) ReplayTraceExhaustive(traceID string) (events []protocol.Event, complete bool, err error) {
	return c.replayTrace(traceID, exhaustiveIdleTimeout, true)
}

func (c *Client) replayTrace(traceID string, idle time.Duration, exhaustive bool) ([]protocol.Event, bool, error) {
	if traceID == "" {
		return nil, false, fmt.Errorf("bus: traceId is required")
	}
	subject := EventsWildcard(c.cfg.SubjectPrefix)
	sub, err := c.js.SubscribeSync(subject, nats.DeliverAll(), nats.AckNone())
	if err != nil {
		return nil, false, fmt.Errorf("bus: replay subscribe: %w", err)
	}
	defer func() { _ = sub.Unsubscribe() }()

	started := time.Now()
	var events []protocol.Event
	for {
		msg, err := sub.NextMsg(idle)
		if err == nats.ErrTimeout {
			if !exhaustive {
				return events, true, nil
			}
			// Quiet is only proof of an empty stream when the consumer says
			// nothing is left to deliver. Otherwise keep waiting within budget.
			if !streamPending(sub) {
				return events, true, nil
			}
			if time.Since(started) >= exhaustiveReadBudget {
				return events, false, nil
			}
			continue
		}
		if err != nil {
			return nil, false, fmt.Errorf("bus: replay next: %w", err)
		}
		var ev protocol.Event
		if err := json.Unmarshal(msg.Data, &ev); err != nil {
			continue
		}
		if ev.TraceID == traceID && protocol.IsDomainEvent(ev.Type) {
			events = append(events, ev)
		}
	}
}

// streamPending reports whether the consumer still has messages to deliver. A
// consumer that cannot say is treated as pending, because guessing "nothing left"
// is what turns a slow stream into a false divergence.
func streamPending(sub *nats.Subscription) bool {
	info, err := sub.ConsumerInfo()
	if err != nil || info == nil {
		return true
	}
	return info.NumPending > 0
}

// Close shuts down the NATS connection.
func (c *Client) Close() {
	if c == nil || c.closed {
		return
	}
	c.closed = true
	if c.nc != nil {
		c.nc.Close()
	}
}

func bytesReader(data []byte) io.Reader {
	return bytes.NewReader(data)
}
