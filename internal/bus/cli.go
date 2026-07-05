package bus

import (
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/paseka/paseka/internal/protocol"
)

// PayloadKind extracts payload.kind from raw JSON.
func PayloadKind(payload json.RawMessage) string {
	return payloadKind(payload)
}

// NewEventFromCLI builds a protocol event from CLI flags.
func NewEventFromCLI(traceID, agentID, typ, payloadJSON string) (protocol.Event, error) {
	eventType := protocol.EventType(strings.ToUpper(strings.TrimSpace(typ)))
	if !protocol.IsDomainEvent(eventType) {
		return protocol.Event{}, fmt.Errorf("bus: invalid event type %q", typ)
	}
	if !json.Valid([]byte(payloadJSON)) {
		return protocol.Event{}, fmt.Errorf("bus: payload must be valid JSON")
	}
	return protocol.Event{
		ProtocolVersion: protocol.Version,
		TraceID:         traceID,
		AgentID:         agentID,
		Type:            eventType,
		CreatedAt:       time.Now().UTC(),
		Payload:         json.RawMessage(payloadJSON),
	}, nil
}
