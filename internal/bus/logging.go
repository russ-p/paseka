package bus

import (
	"github.com/paseka/paseka/internal/logging"
	"github.com/paseka/paseka/internal/protocol"
)

func logDomainEvent(direction, subject string, ev protocol.Event) {
	kind := protocol.PayloadKind(ev.Payload)
	if kind == "" {
		kind = "-"
	}
	agent := ev.AgentID
	if agent == "" {
		agent = "-"
	}
	logging.Component("bus").Info("domain event",
		logging.F("direction", direction),
		logging.F("subject", subject),
		logging.F("trace", ev.TraceID),
		logging.F("type", string(ev.Type)),
		logging.F("kind", kind),
		logging.F("agent", agent),
	)
}
