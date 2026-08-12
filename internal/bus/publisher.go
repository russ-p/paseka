package bus

import (
	"context"
	"reflect"

	"github.com/paseka/paseka/internal/protocol"
)

// Publisher publishes protocol events to the bus.
type Publisher interface {
	PublishEvent(ctx context.Context, event protocol.Event) error
}

// TraceReplayer reads persisted domain events for one trace.
type TraceReplayer interface {
	ReplayTrace(traceID string) ([]protocol.Event, error)
}

// ArtifactStore stores large binary artifacts (e.g. diffs) in the colony object store.
type ArtifactStore interface {
	StoreArtifact(name string, data []byte) (string, error)
}

// NopPublisher discards events (file-only mode).
type NopPublisher struct{}

func (NopPublisher) PublishEvent(context.Context, protocol.Event) error { return nil }

// PublisherAvailable reports whether pub is non-nil and not a typed-nil pointer/interface.
func PublisherAvailable(pub Publisher) bool {
	if pub == nil {
		return false
	}
	val := reflect.ValueOf(pub)
	switch val.Kind() {
	case reflect.Ptr, reflect.Interface, reflect.Map, reflect.Chan, reflect.Func, reflect.Slice:
		return !val.IsNil()
	default:
		return true
	}
}
