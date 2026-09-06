package runtime

import (
	"github.com/russ-p/paseka/internal/invites"
	"github.com/russ-p/paseka/internal/protocol"
)

func (r *Reactor) handleInviteProjection(ev protocol.Event) error {
	svc := &invites.Service{Colony: r.colony}
	return svc.ProjectEvent(ev)
}
