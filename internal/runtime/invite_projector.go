package runtime

import (
	"github.com/paseka/paseka/internal/invites"
	"github.com/paseka/paseka/internal/protocol"
)

func (r *Reactor) handleInviteProjection(ev protocol.Event) error {
	svc := &invites.Service{Colony: r.colony}
	return svc.ProjectEvent(ev)
}
