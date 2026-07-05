package runtime

import (
	"context"
	"fmt"
	"strings"

	"github.com/paseka/paseka/internal/adapters"
	"github.com/paseka/paseka/internal/bus"
	"github.com/paseka/paseka/internal/protocol"
)

func (d *Dispatcher) publishRunOutcome(ctx context.Context, req DispatchRequest, result *adapters.RunResult) error {
	if d.publisher == nil || result == nil {
		return nil
	}
	for _, ev := range result.Events {
		if !protocol.IsDomainEvent(ev.Type) {
			continue
		}
		if err := d.publisher.PublishEvent(ctx, ev); err != nil {
			return err
		}
	}
	mutation, err := mutationFromResult(d.publisher, req, result)
	if err != nil {
		return err
	}
	if mutation != nil {
		if err := d.publisher.PublishEvent(ctx, *mutation); err != nil {
			return err
		}
	}
	return nil
}

func mutationFromResult(pub bus.Publisher, req DispatchRequest, result *adapters.RunResult) (*protocol.Event, error) {
	diff := ""
	for _, a := range result.Artifacts {
		if a.Kind == "diff" && strings.TrimSpace(a.Content) != "" {
			diff = a.Content
			break
		}
	}
	if diff == "" {
		return nil, nil
	}
	payload := protocol.MutationPayload{
		Kind:    protocol.MutationCodeProposal,
		Diff:    diff,
		Summary: strings.TrimSpace(result.Summary),
	}
	if d, ok := pub.(*bus.Client); ok && len(diff) > 64*1024 {
		ref, err := d.StoreArtifact(fmt.Sprintf("%s-%s.diff", req.TraceID, req.AgentID), []byte(diff))
		if err == nil {
			payload.Ref = ref
			payload.Diff = ""
		}
	}
	ev, err := protocol.NewEvent(req.TraceID, req.AgentID, 0, protocol.EventMutation, payload)
	if err != nil {
		return nil, err
	}
	return &ev, nil
}
