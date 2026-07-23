package runs

import (
	"encoding/json"
	"strings"

	"github.com/paseka/paseka/internal/protocol"
)

// ResolveTraceSummary returns the latest INSIGHT/trace.summary prose for a flight trail.
// Last-write-wins by createdAt, then seq. Returns empty when none is present.
func ResolveTraceSummary(colonyRoot, traceID string) (string, error) {
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
		if protocol.PayloadKind(ev.Payload) != string(protocol.InsightTraceSummary) {
			continue
		}
		var p protocol.TraceSummaryPayload
		if err := json.Unmarshal(ev.Payload, &p); err != nil {
			continue
		}
		if summary := strings.TrimSpace(p.Summary); summary != "" {
			return summary, nil
		}
	}
	return "", nil
}
