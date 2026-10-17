package bus

import (
	"encoding/json"
	"fmt"
	"strings"

	"github.com/russ-p/paseka/internal/protocol"
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
	// NewEvent is the single place identity is minted, so a hand-authored
	// event is recognizable in the trail log exactly like a runtime one.
	return protocol.NewEvent(traceID, agentID, 0, eventType, json.RawMessage(payloadJSON))
}
