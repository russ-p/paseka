package runs

import (
	"encoding/json"
	"strings"

	"github.com/russ-p/paseka/internal/protocol"
)

// ResolvePRBody returns the latest INSIGHT/pr.body markdown for a flight trail.
// Last-write-wins by createdAt, then seq. Returns empty when none is present.
func ResolvePRBody(colonyRoot, traceID string) (string, error) {
	if colonyRoot == "" || traceID == "" {
		return "", nil
	}
	events, err := ReadTraceEvents(colonyRoot, traceID)
	if err != nil {
		return "", err
	}

	for i := len(events) - 1; i >= 0; i-- {
		ev := events[i]
		if ev.Type != protocol.EventInsight {
			continue
		}
		if protocol.PayloadKind(ev.Payload) != string(protocol.InsightPRBody) {
			continue
		}
		var p protocol.PRBodyPayload
		if err := json.Unmarshal(ev.Payload, &p); err != nil {
			continue
		}
		if body := strings.TrimSpace(p.Body); body != "" {
			return body, nil
		}
	}
	return "", nil
}
