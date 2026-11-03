package cues

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/russ-p/paseka/internal/bus"
	"github.com/russ-p/paseka/internal/ids"
	"github.com/russ-p/paseka/internal/protocol"
	"github.com/russ-p/paseka/internal/runs"
)

// newCueInvocationID mints the durable identity of one cue invocation.
func newCueInvocationID() (string, error) {
	return ids.Prefixed("cueinv-")
}

// buildInvocation assembles the durable record for one validated cue
// invocation. events are the invocation's normal ingress events in publish
// order; taskIDs are the tasks it creates, if any.
//
// invokedAt is the invocation time. It is also the cue.ingress observation's
// creation time, which keeps a sorted trail reading the observation before the
// plan or signal it announces even though the observation is built after them.
func buildInvocation(invocationID string, cue Cue, traceID, source, agentID string, mode protocol.CueIngressMode, invokedAt time.Time, taskIDs []string, events []protocol.Event) (runs.CueInvocation, error) {
	eventIDs := make([]string, 0, len(events))
	for _, ev := range events {
		if ev.ID != "" {
			eventIDs = append(eventIDs, ev.ID)
		}
	}
	ingress, err := protocol.NewEvent(traceID, agentID, 0, protocol.EventSignal, protocol.CueIngressPayload{
		Kind:         protocol.SignalCueIngress,
		InvocationID: invocationID,
		CueID:        cue.ID,
		Source:       source,
		Mode:         mode,
		InvokedAt:    invokedAt,
		EventIDs:     eventIDs,
		TaskIDs:      append([]string(nil), taskIDs...),
	})
	if err != nil {
		return runs.CueInvocation{}, fmt.Errorf("cue %q: build cue.ingress: %w", cue.ID, err)
	}
	ingress.CreatedAt = invokedAt

	recorded := make([]runs.CueInvocationEvent, 0, len(events))
	for _, ev := range events {
		recorded = append(recorded, runs.CueInvocationEvent{Event: ev})
	}
	return runs.CueInvocation{
		InvocationID: invocationID,
		TraceID:      traceID,
		AgentID:      agentID,
		CueID:        cue.ID,
		Source:       source,
		Mode:         string(mode),
		InvokedAt:    invokedAt,
		TaskIDs:      append([]string(nil), taskIDs...),
		Ingress:      ingress,
		Events:       recorded,
	}, nil
}

// publishInvocation records the invocation before publishing anything, then
// publishes the cue.ingress observation and the normal ingress events in order.
//
// Identity is durable, not in-process: each successful publish is recorded back
// into the same record, so a retry replays only what is still unconfirmed and
// never mints a second invocation or a second event identity.
func publishInvocation(ctx context.Context, publisher bus.Publisher, colonyRoot string, inv runs.CueInvocation) error {
	if err := runs.WriteCueInvocation(colonyRoot, inv.TraceID, inv); err != nil {
		return fmt.Errorf("record invocation: %w", err)
	}
	if !inv.IngressPublished {
		if err := publisher.PublishEvent(ctx, inv.Ingress); err != nil {
			return fmt.Errorf("publish cue.ingress: %w", err)
		}
		inv.IngressPublished = true
		if err := runs.WriteCueInvocation(colonyRoot, inv.TraceID, inv); err != nil {
			return fmt.Errorf("record cue.ingress publication: %w", err)
		}
	}
	for i := range inv.Events {
		if inv.Events[i].Published {
			continue
		}
		ev := inv.Events[i].Event
		if err := publisher.PublishEvent(ctx, ev); err != nil {
			return fmt.Errorf("publish %s: %w", protocol.PayloadKind(ev.Payload), err)
		}
		inv.Events[i].Published = true
		if err := runs.WriteCueInvocation(colonyRoot, inv.TraceID, inv); err != nil {
			return fmt.Errorf("record %s publication: %w", protocol.PayloadKind(ev.Payload), err)
		}
	}
	return nil
}

// RepublishInvocation retries the publication of one recorded invocation with
// the identities it already holds. It never creates a new invocation, never
// mints a new event identity, and never re-runs a cue definition, so a retry
// after a refused publish is the same invocation rather than a second one.
func RepublishInvocation(ctx context.Context, publisher bus.Publisher, colonyRoot, traceID, invocationID string) (RunResult, error) {
	if !bus.PublisherAvailable(publisher) {
		return RunResult{}, fmt.Errorf("nats url not configured (cue run requires NATS)")
	}
	invocationID = strings.TrimSpace(invocationID)
	inv, ok, err := runs.ReadCueInvocation(colonyRoot, traceID, invocationID)
	if err != nil {
		return RunResult{}, fmt.Errorf("cue invocation %q: %w", invocationID, err)
	}
	if !ok {
		return RunResult{}, fmt.Errorf("cue invocation %q: not found on trace %q", invocationID, traceID)
	}
	if err := publishInvocation(ctx, publisher, colonyRoot, inv); err != nil {
		return RunResult{}, fmt.Errorf("cue %q: invocation %s: %w", inv.CueID, inv.InvocationID, err)
	}
	return resultForInvocation(inv), nil
}

// resultForInvocation derives the CLI/console result from the recorded
// invocation. The first normal event is the invocation's primary event, so a
// task cue reports task.plan even when autorun added task.ready.
func resultForInvocation(inv runs.CueInvocation) RunResult {
	res := RunResult{TraceID: inv.TraceID, InvocationID: inv.InvocationID}
	if len(inv.TaskIDs) > 0 {
		res.TaskID = inv.TaskIDs[0]
	}
	if len(inv.Events) > 0 {
		first := inv.Events[0].Event
		res.EventType = string(first.Type)
		res.Kind = protocol.PayloadKind(first.Payload)
	}
	return res
}
