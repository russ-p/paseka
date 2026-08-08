package review

import (
	"context"
	"reflect"

	"github.com/paseka/paseka/internal/bus"
	"github.com/paseka/paseka/internal/protocol"
	"github.com/paseka/paseka/internal/taskledger"
)

// WriteOptions configures ledger event writes.
type WriteOptions struct {
	// AfterApply runs after successful ledger.Apply and before Publish.
	// Reactor uses this for rememberLocalEvent.
	AfterApply func(protocol.Event)
}

// WriteEvent applies locally first, then optionally publishes to the bus.
// Publish-before-apply would leave a bad stream event when CAS/apply fails,
// and the reactor's own subscription would double-apply non-idempotent events.
func WriteEvent(ctx context.Context, pub bus.Publisher, ledger taskledger.Ledger, ev protocol.Event, opts WriteOptions) error {
	if ledger == nil {
		return nil
	}
	if _, err := ledger.Apply(ev); err != nil {
		return err
	}
	if opts.AfterApply != nil {
		opts.AfterApply(ev)
	}
	if publisherAvailable(pub) {
		if err := pub.PublishEvent(ctx, ev); err != nil {
			return err
		}
	}
	return nil
}

func publisherAvailable(pub bus.Publisher) bool {
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
