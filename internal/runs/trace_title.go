package runs

import (
	"encoding/json"
	"strings"

	"github.com/paseka/paseka/internal/protocol"
)

const featureRequestedKind = "feature.requested"

// ResolveTraceTitle returns the human-readable title for a flight trail.
// Priority: latest INSIGHT/trace.title, latest SIGNAL/feature.requested title,
// first non-empty task title from the filesystem projection, then empty string.
func ResolveTraceTitle(colonyRoot, traceID string) (string, error) {
	if colonyRoot == "" || traceID == "" {
		return "", nil
	}
	events, err := ReadTraceEvents(colonyRoot, traceID)
	if err != nil {
		return "", err
	}

	var fromTitle, fromFeature string
	for i := len(events) - 1; i >= 0; i-- {
		ev := events[i]
		kind := protocol.PayloadKind(ev.Payload)
		if fromTitle == "" && ev.Type == protocol.EventInsight && kind == string(protocol.InsightTraceTitle) {
			var p protocol.TraceTitlePayload
			if err := json.Unmarshal(ev.Payload, &p); err == nil {
				fromTitle = strings.TrimSpace(p.Title)
			}
		}
		if fromFeature == "" && ev.Type == protocol.EventSignal && kind == featureRequestedKind {
			var p struct {
				Title string `json:"title"`
			}
			if err := json.Unmarshal(ev.Payload, &p); err == nil {
				fromFeature = strings.TrimSpace(p.Title)
			}
		}
		if fromTitle != "" && fromFeature != "" {
			break
		}
	}
	if fromTitle != "" {
		return fromTitle, nil
	}
	if fromFeature != "" {
		return fromFeature, nil
	}

	ids, err := ListTraceTaskIDs(colonyRoot, traceID)
	if err != nil {
		return "", err
	}
	for _, taskID := range ids {
		d, err := NewTaskDir(colonyRoot, traceID, taskID)
		if err != nil {
			continue
		}
		fm, _, err := d.ReadTask()
		if err != nil {
			continue
		}
		if title := strings.TrimSpace(fm.Title); title != "" {
			return title, nil
		}
	}
	return "", nil
}
