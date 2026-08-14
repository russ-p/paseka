package runtime

import (
	"context"

	"github.com/paseka/paseka/internal/bus"
	"github.com/paseka/paseka/internal/invites"
	"github.com/paseka/paseka/internal/protocol"
)

func (r *Reactor) handleAutoInvite(ctx context.Context, ev protocol.Event) error {
	if len(r.autoInvites) == 0 {
		return nil
	}

	var traceEvents []protocol.Event
	if r.replayer != nil {
		var err error
		traceEvents, err = r.replayer.ReplayTrace(ev.TraceID)
		if err != nil {
			return err
		}
	}

	svc := &invites.Service{Colony: r.colony, Publisher: r.publisher}
	if bus.PublisherAvailable(r.invitePublisher) {
		svc.Publisher = r.invitePublisher
	}
	published, ok, err := svc.AutoInviteFromEvent(ctx, ev, r.autoInvites, traceEvents)
	if err != nil {
		return err
	}
	if ok {
		r.rememberLocalEvent(published)
	}
	return nil
}
