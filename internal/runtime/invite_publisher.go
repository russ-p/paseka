package runtime

import (
	"context"

	"github.com/russ-p/paseka/internal/bus"
	"github.com/russ-p/paseka/internal/invites"
	"github.com/russ-p/paseka/internal/protocol"
)

func (r *Reactor) handleAutoInvite(ctx context.Context, ev protocol.Event, execCtx *trailExecutionContext) error {
	rules := r.autoInvites
	if execCtx != nil {
		rules = execCtx.Manifest.AutoInvites
	}
	if len(rules) == 0 {
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
	published, ok, err := svc.AutoInviteFromEvent(ctx, ev, rules, traceEvents)
	if err != nil {
		return err
	}
	if ok {
		r.rememberLocalEvent(published)
	}
	return nil
}
