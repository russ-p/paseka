package bus

import (
	"encoding/json"
	"fmt"

	"github.com/nats-io/nats.go"
	"github.com/paseka/paseka/internal/protocol"
)

// EventHandler processes one domain event from the bus.
type EventHandler func(event protocol.Event) error

// SubscribeEvents creates a durable JetStream subscription for colony events.
func (c *Client) SubscribeEvents(durable string, handler EventHandler) (*nats.Subscription, error) {
	if durable == "" {
		durable = "paseka-reactor-" + sanitizeName(c.cfg.Slug)
	}
	subject := EventsWildcard(c.cfg.SubjectPrefix)
	sub, err := c.js.Subscribe(subject, func(msg *nats.Msg) {
		var ev protocol.Event
		if err := json.Unmarshal(msg.Data, &ev); err != nil {
			_ = msg.Nak()
			return
		}
		if !protocol.IsDomainEvent(ev.Type) {
			_ = msg.Ack()
			return
		}
		if err := handler(ev); err != nil {
			_ = msg.Nak()
			return
		}
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
