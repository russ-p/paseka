package runtime

import (
	"context"

	"github.com/paseka/paseka/internal/bus"
	"github.com/paseka/paseka/internal/invites"
	"github.com/paseka/paseka/internal/protocol"
)

func (r *Reactor) handleInviteCompletion(ctx context.Context, ev protocol.Event) error {
	svc := &invites.Service{Colony: r.colony, Publisher: r.publisher}
	if bus.PublisherAvailable(r.invitePublisher) {
		svc.Publisher = r.invitePublisher
	}
	published, ok, err := svc.CompleteFromEvent(ctx, ev)
	if err != nil {
		return err
	}
	if ok && published.TraceID != "" {
		r.rememberLocalEvent(published)
	}
	return nil
}
