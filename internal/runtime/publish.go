package runtime

import (
	"context"
	"fmt"
	"strings"

	"github.com/paseka/paseka/internal/adapters"
	"github.com/paseka/paseka/internal/bus"
	"github.com/paseka/paseka/internal/colony"
	"github.com/paseka/paseka/internal/logging"
	"github.com/paseka/paseka/internal/protocol"
)

func (d *Dispatcher) publishRunOutcome(
	ctx context.Context,
	req DispatchRequest,
	bee colony.Bee,
	baseline adapters.WorkspaceBaseline,
	result *adapters.RunResult,
	synthesized []protocol.Event,
) error {
	if d.publisher == nil || result == nil {
		return nil
	}
	for _, ev := range result.Events {
		if !protocol.IsDomainEvent(ev.Type) {
			continue
		}
		d.advisePublish(req.Bee, ev, result)
		if err := d.publisher.PublishEvent(ctx, ev); err != nil {
			return err
		}
	}
	for _, ev := range synthesized {
		d.advisePublish(req.Bee, ev, result)
		if err := d.publisher.PublishEvent(ctx, ev); err != nil {
			return err
		}
	}
	mutation, err := d.mutationFromRun(ctx, req, bee, baseline, result)
	if err != nil {
		return err
	}
	if mutation != nil {
		d.advisePublish(req.Bee, *mutation, result)
		if err := d.publisher.PublishEvent(ctx, *mutation); err != nil {
			return err
		}
	}
	return nil
}

func (d *Dispatcher) shouldAutoPublishMutation(bee colony.Bee) bool {
	if len(bee.Publishes) == 0 {
		return false
	}
	if d.registry != nil {
		return d.registry.ShouldAutoPublishMutation(bee.Role)
	}
	return shouldAutoPublishProposal(bee)
}

func (d *Dispatcher) advisePublish(beeRole string, ev protocol.Event, result *adapters.RunResult) {
	if d.registry == nil {
		return
	}
	warning, ok := d.registry.ValidatePublish(beeRole, ev)
	if ok {
		return
	}
	runtimeLog.Warn("advisory publish", logging.F("warning", warning))
	if result != nil {
		result.Warnings = append(result.Warnings, warning)
	}
}

func (d *Dispatcher) mutationFromRun(
	ctx context.Context,
	req DispatchRequest,
	bee colony.Bee,
	baseline adapters.WorkspaceBaseline,
	result *adapters.RunResult,
) (*protocol.Event, error) {
	if !d.shouldAutoPublishMutation(bee) {
		return nil, nil
	}

	plan := planAutoProposal(bee)
	if !plan.ok {
		appendAutoProposalSkipWarning(bee, plan, result)
		return nil, nil
	}

	workspace := req.Workspace
	if workspace == "" {
		workspace = req.ColonyRoot
	}

	diff, err := adapters.AttributableDiff(ctx, workspace, baseline)
	if err != nil {
		return nil, fmt.Errorf("runtime: attributable diff: %w", err)
	}
	if strings.TrimSpace(diff) == "" {
		return nil, nil
	}

	payload := protocol.MutationPayload{
		Kind:      plan.kind,
		Diff:      diff,
		Summary:   strings.TrimSpace(result.Summary),
		TaskID:    req.TaskID,
		Workspace: plan.workspace,
		BaseSha:   baseline.BaseSHA,
		Sector:    req.Sector,
	}
	if bee.Worktree {
		payload.WorktreePath = worktreeRelPath(req.ColonyRoot, req.TraceID)
	}

	if pub, ok := d.publisher.(*bus.Client); ok && len(diff) > 64*1024 {
		ref, storeErr := pub.StoreArtifact(fmt.Sprintf("%s-%s.diff", req.TraceID, req.AgentID), []byte(diff))
		if storeErr == nil {
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
