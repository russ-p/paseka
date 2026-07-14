package runtime

import (
	"context"

	"github.com/paseka/paseka/internal/invites"
	"github.com/paseka/paseka/internal/protocol"
)

const colonyKindSpecReady = "spec.ready"

func (r *Reactor) handleInviteCompletion(ctx context.Context, ev protocol.Event) error {
	if ev.Type != protocol.EventSignal || protocol.PayloadKind(ev.Payload) != colonyKindSpecReady {
		return nil
	}
	svc := &invites.Service{Colony: r.colony, Bus: r.bus}
	if r.invitePublisher != nil {
		svc.Publisher = r.invitePublisher
	}
	published, ok, err := svc.CompleteFromSpecReady(ctx, ev)
	if err != nil {
		return err
	}
	if ok && published.TraceID != "" {
		r.rememberLocalEvent(published)
	}
	return nil
}
