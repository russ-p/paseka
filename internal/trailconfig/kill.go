package trailconfig

import (
	"fmt"
	"strings"

	"github.com/russ-p/paseka/internal/protocol"
	"github.com/russ-p/paseka/internal/runs"
)

// TraceKilled reports whether a trail's durable event record contains a
// system.kill signal. The kill is sticky, so the last such event is the state
// of the trail.
//
// The trail's own record is authoritative because internal/tasks imports this
// package and cannot be used here. A trail with no trail-events.ndjson at all
// (one created before the spec-040 event log existed) is not killed: absence of
// evidence keeps its legacy behaviour. An unreadable record is an error so a
// caller refuses to reconfigure a trail whose state it cannot prove.
func TraceKilled(colonyRoot, traceID string) (bool, error) {
	if strings.TrimSpace(colonyRoot) == "" {
		return false, fmt.Errorf("trailconfig: colony root is required")
	}
	if err := validateTraceID(traceID); err != nil {
		return false, err
	}
	events, err := runs.ReadTrailEvents(colonyRoot, traceID)
	if err != nil {
		return false, fmt.Errorf("trailconfig: read trail events for %s: %w", traceID, err)
	}
	killed := false
	for _, event := range events {
		if event.Type != protocol.EventSignal {
			continue
		}
		if protocol.PayloadKind(event.Payload) == string(protocol.SignalSystemKill) {
			killed = true
		}
	}
	return killed, nil
}
