package runtime

import (
	"context"

	"github.com/paseka/paseka/internal/invites"
	"github.com/paseka/paseka/internal/protocol"
)

func (r *Reactor) handleInviteCompletion(ctx context.Context, ev protocol.Event) error {
	if len(r.inviteCompletion) == 0 {
		return nil
	}
	svc := &invites.Service{Colony: r.colony, Bus: r.bus}
	if r.invitePublisher != nil {
		svc.Publisher = r.invitePublisher
	}
	published, ok, err := svc.CompleteFromEvent(ctx, ev, r.inviteCompletion)
	if err != nil {
		return err
	}
	if ok && published.TraceID != "" {
		r.rememberLocalEvent(published)
	}
	return nil
}
