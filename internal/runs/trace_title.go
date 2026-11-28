package runs

import (
	"encoding/json"
	"strings"

	"github.com/russ-p/paseka/internal/protocol"
	"github.com/russ-p/paseka/internal/taskledger"
)

const featureRequestedKind = "feature.requested"

// ResolveTraceTitle returns the human-readable title for a flight trail.
// Priority: latest INSIGHT/trace.title, latest SIGNAL/feature.requested title,
// first non-empty work task title from the filesystem projection, then empty
// string. Closing rounds and the merge gate are skipped: their titles are
// rendered from {{.TraceTitle}}, so using one as the fallback would rename the
// trail to the gate that closes it.
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
		if isTrailClosingTask(taskID, fm) {
			continue
		}
		if title := strings.TrimSpace(fm.Title); title != "" {
			return title, nil
		}
	}
	return "", nil
}

// isTrailClosingTask reports whether a projected task is a runtime closing
// round or the merge gate rather than trail work. Their ids sort ahead of
// generated work ids (closing-1, _review before task-…), so without this check
// the fallback would flip to the gate title as soon as the gate is planned.
func isTrailClosingTask(taskID string, fm TaskFrontmatter) bool {
	if taskID == taskledger.FinalReviewTaskID {
		return true
	}
	if _, ok := taskledger.ClosingRoundNumber(taskID); ok {
		return true
	}
	switch protocol.DeriveTaskPhase(fm.Phase, fm.Review) {
	case protocol.TaskPhaseClosing, protocol.TaskPhaseMerge:
		return true
	}
	return false
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
