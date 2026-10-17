package bus

import (
	"context"
	"reflect"

	"github.com/russ-p/paseka/internal/protocol"
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
//
// Proposals no longer use it: a synthesized code proposal is archived on disk under the
// trail's proposal store instead. It stays for objects written by earlier versions, which
// `paseka purge --bus` still deletes.
type ArtifactStore interface {
	StoreArtifact(name string, data []byte) (string, error)
}

// NopPublisher discards events (file-only mode).
type NopPublisher struct{}

func (NopPublisher) PublishEvent(context.Context, protocol.Event) error { return nil }

// ConfirmsPublication reports that PublishEvent's nil means a broker took the event.
func (NopPublisher) ConfirmsPublication() bool { return false }

// ConfirmsPublication reports whether pub's acknowledgement means a broker accepted the
// event.
//
// A publisher opts out by answering false: the file-only recorder and the discard no-op
// both return nil from PublishEvent with no broker having seen anything, so a durable
// record must not read their silence as confirmation. Anything else is treated as a
// broker, because PublishEvent returning nil is otherwise the only acknowledgement a
// caller has.
func ConfirmsPublication(pub Publisher) bool {
	if !PublisherAvailable(pub) {
		return false
	}
	confirmer, ok := pub.(interface{ ConfirmsPublication() bool })
	if !ok {
		return true
	}
	return confirmer.ConfirmsPublication()
}

// PublisherAvailable reports whether pub is non-nil and not a typed-nil pointer/interface.
func PublisherAvailable(pub Publisher) bool {
	return seamAvailable(pub)
}

// ReplayerAvailable reports whether r is usable, guarding the typed-nil
// pointer a nil *Client lands in an interface as.
func ReplayerAvailable(r TraceReplayer) bool {
	return seamAvailable(r)
}

// streamAvailable is ReplayerAvailable for the verification seam.
func streamAvailable(s TraceStream) bool {
	return seamAvailable(s)
}

// seamAvailable reports whether an interface value carries a real value, so a
// typed-nil pointer is never mistaken for a live collaborator.
func seamAvailable(v any) bool {
	if v == nil {
		return false
	}
	val := reflect.ValueOf(v)
	switch val.Kind() {
	case reflect.Ptr, reflect.Interface, reflect.Map, reflect.Chan, reflect.Func, reflect.Slice:
		return !val.IsNil()
	default:
		return true
	}
}
