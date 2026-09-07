package runs

import (
	"encoding/json"
	"strings"

	"github.com/russ-p/paseka/internal/protocol"
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

// HasInsightTraceTitle reports whether an INSIGHT/trace.title event exists on the trail.
// Empty or unreadable titles do not count. Fallbacks (feature.requested, task.md) are ignored.
func HasInsightTraceTitle(colonyRoot, traceID string) (bool, error) {
	if colonyRoot == "" || traceID == "" {
		return false, nil
	}
	events, err := ReadTraceEvents(colonyRoot, traceID)
	if err != nil {
		return false, err
	}
	for _, ev := range events {
		if ev.Type != protocol.EventInsight {
			continue
		}
		if protocol.PayloadKind(ev.Payload) != string(protocol.InsightTraceTitle) {
			continue
		}
		var p protocol.TraceTitlePayload
		if err := json.Unmarshal(ev.Payload, &p); err != nil {
			continue
		}
		if strings.TrimSpace(p.Title) != "" {
			return true, nil
		}
	}
	return false, nil
}
