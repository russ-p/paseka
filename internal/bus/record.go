package bus

import (
	"context"
	"strings"

	"github.com/russ-p/paseka/internal/logging"
	"github.com/russ-p/paseka/internal/protocol"
	"github.com/russ-p/paseka/internal/runs"
)

var recordLog = logging.Component("bus")

// recordTrailEvent writes one domain event into its trail's record.
//
// This is the single writer behind every trail's history. It is called from the
// publisher seam rather than from each publishing site, so a producer that can
// publish can record, and a new producer cannot forget to. The same test that
// gates the bus gates the record: run-lifecycle records — tool calls, assistant
// text — are stream diagnostics, not what happened on the trail.
//
// A failure here is a warning, never an error. A colony that cannot write its
// record must still be able to run, and losing one trail's events is strictly
// better than refusing to publish the event that is telling anyone it happened.
func recordTrailEvent(colonyRoot string, event protocol.Event) error {
	traceID := strings.TrimSpace(event.TraceID)
	if colonyRoot == "" || traceID == "" {
		return nil
	}
	if !protocol.IsDomainEvent(event.Type) {
		return nil
	}
	trail := runs.Trail{ColonyRoot: colonyRoot, TraceID: traceID}
	if err := trail.AppendEvent(event); err != nil {
		recordLog.Warn("trail event log append failed",
			logging.F("traceId", traceID),
			logging.F("type", string(event.Type)),
			logging.F("agentId", event.AgentID),
			logging.F("path", trail.EventsPath()),
			logging.F("error", err.Error()))
		return err
	}
	return nil
}

// LocalPublisher is the colony's event sink with no bus: it records every
// domain event in its trail and publishes nothing. A colony configured without
// NATS is not a second-class record — what it decided to do is on disk either
// way.
type LocalPublisher struct {
	ColonyRoot string
}

// PublishEvent records the event in the trail log.
func (p LocalPublisher) PublishEvent(_ context.Context, event protocol.Event) error {
	return recordTrailEvent(p.ColonyRoot, event)
}

// ConfirmsPublication reports false: a file-only colony has no broker to acknowledge to.
func (LocalPublisher) ConfirmsPublication() bool { return false }

var _ Publisher = LocalPublisher{}
