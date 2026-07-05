package bus

import (
	"encoding/json"
	"fmt"
	"strings"

	"github.com/paseka/paseka/internal/protocol"
)

// EventSubject maps a protocol event to a colony-scoped NATS subject.
// Shape: <prefix>.events.<eventType>[.<payloadKind>]
func EventSubject(prefix string, event protocol.Event) string {
	base := fmt.Sprintf("%s.events.%s", prefix, event.Type)
	if kind := payloadKind(event.Payload); kind != "" {
		return base + "." + kind
	}
	return base
}

// EventsWildcard returns the subscribe pattern for all colony events.
func EventsWildcard(prefix string) string {
	return prefix + ".events.>"
}

func payloadKind(payload json.RawMessage) string {
	if len(payload) == 0 {
		return ""
	}
	var meta struct {
		Kind string `json:"kind"`
	}
	if err := json.Unmarshal(payload, &meta); err != nil {
		return ""
	}
	return strings.TrimSpace(meta.Kind)
}
