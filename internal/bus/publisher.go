package bus

import (
	"context"

	"github.com/paseka/paseka/internal/protocol"
)

// Publisher publishes protocol events to the bus.
type Publisher interface {
	PublishEvent(ctx context.Context, event protocol.Event) error
}

// NopPublisher discards events (file-only mode).
type NopPublisher struct{}

func (NopPublisher) PublishEvent(context.Context, protocol.Event) error { return nil }
