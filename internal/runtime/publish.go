package runtime

import (
	"context"

	"github.com/russ-p/paseka/internal/adapters"
	"github.com/russ-p/paseka/internal/colony"
	"github.com/russ-p/paseka/internal/logging"
	"github.com/russ-p/paseka/internal/proposal"
	"github.com/russ-p/paseka/internal/protocol"
)

// publishRunOutcome publishes what a finished run decided, in three passes: the events the
// adapter reported, the events the runtime synthesized, and finally the code proposal.
//
// The proposal pass is not conditional on a publisher being configured. A colony with no
// broker still archives the proposal diff on disk and reports the record as unpublished,
// because losing the code record to bus availability is exactly the failure the archive
// exists to prevent.
func (d *Dispatcher) publishRunOutcome(
	ctx context.Context,
	req DispatchRequest,
	bee colony.Bee,
	baseline adapters.WorkspaceBaseline,
	result *adapters.RunResult,
	synthesized []protocol.Event,
	explicitProposal bool,
) error {
	if result == nil {
		return nil
	}
	if d.publisher != nil {
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
	}

	res, err := proposal.Archive(ctx, d.publisher, d.proposalArchiveRequestFromRun(req, bee, baseline, result, explicitProposal))
	if res.Misconfigured != "" {
		logAutoProposalSkip(bee, res.Misconfigured)
		result.Warnings = append(result.Warnings, res.Misconfigured)
	}
	if res.Event != nil {
		d.advisePublish(req.Bee, *res.Event, result)
	}
	return err
}

func (d *Dispatcher) shouldAutoPublishMutation(bee colony.Bee) bool {
	if len(bee.Publishes) == 0 {
		return false
	}
	if d.registry != nil {
		return d.registry.ShouldAutoPublishMutation(bee.Role)
	}
	return proposal.ShouldAutoPublish(bee)
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
