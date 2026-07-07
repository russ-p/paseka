package bus

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"time"

	"github.com/nats-io/nats.go"
	"github.com/paseka/paseka/internal/protocol"
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
func (c *Client) PublishEvent(_ context.Context, event protocol.Event) error {
	if !protocol.IsDomainEvent(event.Type) {
		return nil
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

// StoreArtifact stores bytes in the colony object store and returns the object name.
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

// ReplayTrace returns all domain events for a trace from JetStream.
func (c *Client) ReplayTrace(traceID string) ([]protocol.Event, error) {
	if traceID == "" {
		return nil, fmt.Errorf("bus: traceId is required")
	}
	subject := EventsWildcard(c.cfg.SubjectPrefix)
	sub, err := c.js.SubscribeSync(subject, nats.DeliverAll(), nats.AckNone())
	if err != nil {
		return nil, fmt.Errorf("bus: replay subscribe: %w", err)
	}
	defer sub.Unsubscribe()

	var events []protocol.Event
	for {
		msg, err := sub.NextMsg(200 * time.Millisecond)
		if err == nats.ErrTimeout {
			break
		}
		if err != nil {
			return nil, fmt.Errorf("bus: replay next: %w", err)
		}
		var ev protocol.Event
		if err := json.Unmarshal(msg.Data, &ev); err != nil {
			continue
		}
		if ev.TraceID == traceID && protocol.IsDomainEvent(ev.Type) {
			events = append(events, ev)
		}
	}
	return events, nil
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
