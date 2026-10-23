package proposal

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/russ-p/paseka/internal/adapters"
	"github.com/russ-p/paseka/internal/bus"
	"github.com/russ-p/paseka/internal/colony"
	"github.com/russ-p/paseka/internal/patches"
	"github.com/russ-p/paseka/internal/protocol"
)

// ArchiveRequest describes one proposal-finalization attempt.
//
// Everything the shared AFK and HITL path needs is here, so an interactive teardown runs
// the same gate, the same collector, and the same writer instead of calling through the
// AFK dispatcher.
type ArchiveRequest struct {
	protocol.Provenance
	ColonyRoot string
	TraceID    string
	AgentID    string
	Bee        colony.Bee
	TaskID     string
	Sector     string
	// Workspace is the repository workspace the run actually used. An empty value falls
	// back to ColonyRoot, matching how the dispatcher resolves it.
	Workspace string
	// Mode is patches.ModeAFK or patches.ModeHITL.
	Mode string
	// Baseline is the workspace baseline captured before the run started.
	Baseline adapters.WorkspaceBaseline
	Summary  string
	// AutoPublish reports whether the effective bee configuration permits this run to
	// auto-publish a proposal at all. The dispatcher resolves registry-backed policy; an
	// interactive session passes the bee's own declaration.
	AutoPublish bool
	// Completed reports that execution finished successfully after the applicable
	// completion checks. A failed, cancelled, timed-out, or completion-contract-rejected
	// run reached the end of a process without proposing anything.
	Completed bool
	// Explicit reports that the run already emitted or queued a code proposal of its own.
	// Such a run is passed through unchanged and gets no automatic patch conversion, so a
	// Guard is never dispatched twice for one proposal.
	Explicit bool
}

// ArchiveResult reports what proposal finalization did.
type ArchiveResult struct {
	// Event is the synthesized MUTATION, nil when nothing was prepared.
	Event *protocol.Event
	// Record is the installed archive record, nil when nothing was installed.
	Record *patches.Record
	// Skipped explains an eligible-looking run that produced no record. It is diagnostic:
	// a failed run, a non-mutating bee, an explicit proposal, and an empty diff are all
	// ordinary, not faults.
	Skipped string
	// Misconfigured is set when the bee declares auto-publish but its worktree and publish
	// kinds disagree, which is a configuration problem worth surfacing rather than a normal
	// skip.
	Misconfigured string
}

// Archive runs the proposal gate, collects the diff once, installs the archive record, and
// publishes the synthesized MUTATION against that ref.
//
// The archived bytes are exactly what the existing collector returned, with its limits
// intact: earlier edits inside a changed file can be included, untracked files and changes
// committed during the run can be omitted, and deletions can make collection fail. An empty
// result means only that the collector produced no diff — not that the workspace was
// unchanged — and no empty, partial, or summary-only proposal is substituted for it.
//
// Collection and storage errors are returned, not swallowed: a broken or partial record
// must never be published as a ref, even when bus connectivity is optional.
func Archive(ctx context.Context, pub bus.Publisher, req ArchiveRequest) (ArchiveResult, error) {
	var out ArchiveResult
	if !req.Completed {
		out.Skipped = "run did not complete successfully"
		return out, nil
	}
	if !req.AutoPublish {
		out.Skipped = "bee does not auto-publish proposals"
		return out, nil
	}

	plan := PlanAuto(req.Bee)
	if !plan.OK {
		out.Skipped = plan.Warn
		out.Misconfigured = plan.Warn
		return out, nil
	}
	if req.Explicit {
		out.Skipped = "run emitted an explicit code proposal"
		return out, nil
	}

	store := patches.NewStore(req.ColonyRoot, req.TraceID)

	// A complete record is the run's history, not a draft to recompute. Reusing it
	// before collecting again means a retry never depends on the workspace still
	// producing the same diff, and a confirmed record is not published twice.
	if existing, ok, err := store.Read(req.AgentID); err != nil {
		return out, fmt.Errorf("proposal: read existing patch: %w", err)
	} else if ok && existing.Event != nil {
		out.Record = &existing
		out.Event = existing.Event
		if existing.Publication == patches.PublicationPublished {
			return out, nil
		}
		if err := Publish(pub, store, existing.AgentID, *existing.Event); err != nil {
			return out, err
		}
		return out, nil
	}

	workspace := req.Workspace
	if workspace == "" {
		workspace = req.ColonyRoot
	}

	diff, err := adapters.AttributableDiff(ctx, workspace, req.Baseline)
	if err != nil {
		return out, fmt.Errorf("proposal: attributable diff: %w", err)
	}
	if strings.TrimSpace(diff) == "" {
		out.Skipped = "collector produced no diff"
		return out, nil
	}

	payload := protocol.MutationPayload{
		Kind:      plan.Kind,
		Ref:       store.Ref(req.AgentID),
		Summary:   strings.TrimSpace(req.Summary),
		TaskID:    req.TaskID,
		Workspace: plan.Workspace,
		BaseSha:   req.Baseline.BaseSHA,
		Sector:    req.Sector,
	}
	if req.Bee.Worktree {
		payload.WorktreePath = WorktreeRelPath(req.ColonyRoot, req.TraceID)
	}

	ev, err := protocol.NewEvent(req.TraceID, req.AgentID, 0, protocol.EventMutation, payload)
	if err != nil {
		return out, err
	}

	ev.Provenance = req.Provenance
	installed, err := store.Write(patches.Record{
		Provenance:   req.Provenance,
		TraceID:      req.TraceID,
		AgentID:      req.AgentID,
		Bee:          req.Bee.Role,
		Mode:         req.Mode,
		TaskID:       req.TaskID,
		Kind:         string(protocol.NormalizeCodeProposalKind(payload.Kind)),
		Workspace:    string(payload.Workspace),
		WorktreePath: payload.WorktreePath,
		BaseSha:      payload.BaseSha,
		Summary:      payload.Summary,
		Sector:       payload.Sector,
		FileCount:    patches.FileCountFromDiff(diff),
		CapturedAt:   time.Now().UTC(),
		Publication:  patches.PublicationUnpublished,
		Event:        &ev,
	}, []byte(diff))
	if err != nil {
		return out, fmt.Errorf("proposal: archive patch: %w", err)
	}
	// Write returns the record it found if another path installed it first. Its event
	// envelope is the one that belongs to the record, so publish that rather than the
	// fresh copy, and never republish a confirmed record.
	event := ev
	if installed.Event != nil {
		event = *installed.Event
	}
	out.Record = &installed
	out.Event = &event
	if installed.Publication == patches.PublicationPublished {
		return out, nil
	}

	if err := Publish(pub, store, installed.AgentID, event); err != nil {
		return out, err
	}
	return out, nil
}

// Publish records what is known about a synthesized proposal event and makes the broker
// attempt.
//
// The sequence is: the record is already installed as unpublished, flip it to unconfirmed
// immediately before a real attempt, then flip to published only after a positive
// acknowledgement. An error or a process interruption after the attempt leaves the record
// unconfirmed, because a published event may already reference those bytes — an uncertain
// outcome must not be presented as a definite rejection, and it must not delete bytes a
// published event may point at.
//
// A nil publisher cannot receive the event, so the record stays unpublished. A file-only
// publisher receives it and records the trail row, but cannot confirm a broker, so the
// record also stays unpublished. Neither claims a bus event that never left the machine.
// There is no cross-filesystem/bus transaction and no exactly-once guarantee here.
func Publish(pub bus.Publisher, store patches.Store, agentID string, ev protocol.Event) error {
	return publish(pub, store, agentID, ev, store.MarkPublication)
}

// markFunc writes a publication state to the sidecar. It is a parameter so a test can make
// the confirmation step fail after the broker has already accepted the event, which is the
// one outcome that cannot be provoked through the filesystem alone.
type markFunc func(agentID, state string, confirmedAt time.Time, publishErr string) error

func publish(pub bus.Publisher, store patches.Store, agentID string, ev protocol.Event, mark markFunc) error {
	if !bus.PublisherAvailable(pub) {
		return nil
	}
	// A file-only publisher still has to receive the event: its PublishEvent writes the
	// trail record that makes the patch and the timeline row appear together. It simply
	// cannot confirm a broker, so the record stays unpublished.
	canConfirm := bus.ConfirmsPublication(pub)
	if canConfirm {
		if err := mark(agentID, patches.PublicationUnconfirmed, time.Time{}, ""); err != nil {
			return fmt.Errorf("proposal: record attempt: %w", err)
		}
	}
	if err := pub.PublishEvent(context.Background(), ev); err != nil {
		if canConfirm {
			_ = mark(agentID, patches.PublicationUnconfirmed, time.Time{}, err.Error())
		}
		return fmt.Errorf("proposal: publish: %w", err)
	}
	if !canConfirm {
		return nil
	}
	if err := mark(agentID, patches.PublicationPublished, time.Now().UTC(), ""); err != nil {
		// The event is on the bus and the record is on disk; only the confirmation note
		// failed. Keep the record and report the write failure rather than letting the
		// operator assume the publish failed too. On disk it stays conservatively
		// unconfirmed.
		return fmt.Errorf("proposal: record publication: %w", err)
	}
	return nil
}
