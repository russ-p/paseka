package review

import (
	"context"
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/russ-p/paseka/internal/bus"
	"github.com/russ-p/paseka/internal/colony"
	"github.com/russ-p/paseka/internal/protocol"
	"github.com/russ-p/paseka/internal/runs"
	"github.com/russ-p/paseka/internal/taskledger"
	"github.com/russ-p/paseka/internal/trailconfig"
	"github.com/russ-p/paseka/internal/worktree"
)

const defaultClosingTaskTitle = "Review the trail and prepare the pull request"

// GatePolicies are the closing and merge policies pinned into a trail's
// configuration snapshot at trail start. The runtime passes them through so
// that editing colony.yaml mid-trail cannot change when a running trail closes.
type GatePolicies struct {
	Closing colony.ClosingPolicy
	Merge   colony.MergePolicy
}

// GateResult reports what the gate boundary changed. ClosingSeeded is set when
// this call planned and readied a closing round; the runtime dispatches it with
// the normal AFK scheduler. The merge gate is never dispatched.
type GateResult struct {
	ClosingSeeded *taskledger.TaskSnapshot
}

// ActivateFinalReviewGate is the existing gate boundary. It resolves the
// closing and merge policies pinned into the trail's configuration snapshot and
// then ensures the closing round before the merge gate. Callers that already
// hold the pinned policies use EnsureTrailPhases so a running trail never reads
// mutated live configuration.
func ActivateFinalReviewGate(ctx context.Context, pub bus.Publisher, ledger taskledger.Ledger, colonyCtx colony.Context, traceID string, opts WriteOptions) error {
	policies, err := pinnedGatePolicies(colonyCtx, traceID)
	if err != nil {
		return err
	}
	_, err = EnsureTrailPhases(ctx, pub, ledger, colonyCtx, traceID, policies, opts)
	return err
}

// pinnedGatePolicies reads the policies frozen at trail start. A trail with no
// binding (or no authored policy) runs with the platform defaults, exactly as
// before the closing phase existed.
func pinnedGatePolicies(colonyCtx colony.Context, traceID string) (GatePolicies, error) {
	if colonyCtx.ColonyRoot == "" || traceID == "" {
		return GatePolicies{}, nil
	}
	manifest, ok, err := trailconfig.LoadManifestIfExists(colonyCtx.ColonyRoot, traceID)
	if err != nil {
		return GatePolicies{}, err
	}
	if !ok {
		return GatePolicies{}, nil
	}
	return GatePolicies{
		Closing: manifest.ClosingPolicy(),
		Merge:   manifest.MergePolicy(),
	}, nil
}

// EnsureTrailPhases ensures the current closing round first, then opens the
// trace-level merge gate when every non-merge task — work and closing alike —
// has reached completed and there is something to merge (an isolated
// code.proposal and/or a non-empty worktree merge diff). When AFK work is done
// but there is nothing to merge, any planned/ready/waiting final-review task is
// auto-completed instead of leaving a hollow HITL gate.
func EnsureTrailPhases(ctx context.Context, pub bus.Publisher, ledger taskledger.Ledger, colonyCtx colony.Context, traceID string, policies GatePolicies, opts WriteOptions) (GateResult, error) {
	var result GateResult
	if ledger == nil || traceID == "" {
		return result, nil
	}
	snap, err := ledger.Snapshot(traceID)
	if err != nil {
		return result, err
	}

	seeded, ok, skipNote, err := ensureClosingRound(ctx, pub, ledger, traceID, snap, policies.Closing, opts)
	if err != nil {
		return result, err
	}
	if ok {
		result.ClosingSeeded = &seeded
		// The seeded closing round is not complete, so the gate must stay
		// closed. Return before merge synthesis: the merge gate is ensured on
		// the boundary where the last closing round completes.
		return result, nil
	}

	if !taskledger.AllPreMergeCompleted(snap) {
		return result, nil
	}

	// A skipped round is reported once, together with the gate it makes way
	// for. Every later boundary finds the gate already open and returns
	// earlier, so the operator reads the reason once instead of on each
	// approval or replayed completion.
	noteSkip := func() error {
		return recordClosingSkipNote(ctx, pub, ledger, traceID, skipNote, opts)
	}

	needsMerge := needsIsolatedMerge(snap, colonyCtx, traceID)
	if final, ok := taskledger.FindFinalReviewTask(snap); ok {
		if !needsMerge {
			switch final.Status {
			case protocol.TaskStatusCompleted, protocol.TaskStatusFailed, protocol.TaskStatusBlocked:
				return result, nil
			}
			if err := noteSkip(); err != nil {
				return result, err
			}
			return result, completeEmptyFinalGate(ctx, pub, ledger, traceID, final, opts)
		}
		if taskledger.HasWaitingReview(snap) && final.Status != protocol.TaskStatusWaitingReview {
			return result, nil
		}
		switch final.Status {
		case protocol.TaskStatusPlanned, protocol.TaskStatusReady:
			if err := noteSkip(); err != nil {
				return result, err
			}
			ev, err := statusEvent(traceID, final.TaskID, protocol.TaskStatusWaitingReview, "All tasks completed — awaiting human review and merge")
			if err != nil {
				return result, err
			}
			return result, WriteEvent(ctx, pub, ledger, ev, opts)
		}
		return result, nil
	}

	if !needsMerge || taskledger.HasWaitingReview(snap) {
		return result, nil
	}

	inventory := BuildTrailInventory(colonyCtx, traceID, snap)
	title, err := policies.Merge.ResolvedTitle(inventory)
	if err != nil {
		return result, err
	}
	body, err := policies.Merge.ResolvedBody(inventory)
	if err != nil {
		return result, err
	}
	if err := noteSkip(); err != nil {
		return result, err
	}
	planEv, err := protocol.NewEvent(traceID, protocol.RuntimeAgentID, 0, protocol.EventInsight, protocol.TaskPlanPayload{
		Kind: protocol.TaskEventPlan,
		Tasks: []protocol.TaskSpec{{
			TaskID: taskledger.FinalReviewTaskID,
			Title:  title,
			Body:   body,
			Review: protocol.TaskReviewFinal,
			Phase:  protocol.TaskPhaseMerge,
		}},
	})
	if err != nil {
		return result, err
	}
	if err := WriteEvent(ctx, pub, ledger, planEv, opts); err != nil {
		return result, err
	}
	ev, err := statusEvent(traceID, taskledger.FinalReviewTaskID, protocol.TaskStatusWaitingReview, "All tasks completed — awaiting human review and merge")
	if err != nil {
		return result, err
	}
	return result, WriteEvent(ctx, pub, ledger, ev, opts)
}

// ensureClosingRound seeds exactly one closing round when the pinned policy
// says the trail needs one. The id is derived from the highest existing round,
// so a replay, a restart, or an echoed task.plan is a no-op. A round is seeded
// only when every work task is complete, no closing round is in flight or
// unfinished, no merge task is completed, the predicates hold, and the latest
// closing round does not already cover the latest completed work.
//
// skipNote explains the two decisions an operator cannot read off the ledger —
// the predicates did not match, or the round cap is spent — and is empty when
// the policy simply had nothing to do yet. The caller records it once, at the
// boundary that opens the merge gate.
func ensureClosingRound(ctx context.Context, pub bus.Publisher, ledger taskledger.Ledger, traceID string, snap taskledger.TraceSnapshot, policy colony.ClosingPolicy, opts WriteOptions) (taskledger.TaskSnapshot, bool, string, error) {
	if !policy.Enabled {
		return taskledger.TaskSnapshot{}, false, "", nil
	}
	if taskledger.MergeTaskCompleted(snap) {
		return taskledger.TaskSnapshot{}, false, "", nil
	}
	if !taskledger.AllWorkCompleted(snap) {
		return taskledger.TaskSnapshot{}, false, "", nil
	}
	if !policy.Matches(taskledger.WorkTaskCount(snap), taskledger.HasIsolatedProposal(snap)) {
		return taskledger.TaskSnapshot{}, false, predicateSkipNote(policy, snap), nil
	}
	if _, ok := taskledger.ClosingInFlight(snap); ok {
		return taskledger.TaskSnapshot{}, false, "", nil
	}
	if taskledger.HasUnfinishedClosingTask(snap) {
		return taskledger.TaskSnapshot{}, false, "", nil
	}
	if taskledger.ClosingCoversLatestWork(snap) {
		return taskledger.TaskSnapshot{}, false, "", nil
	}

	round := taskledger.HighestClosingRound(snap) + 1
	maxRounds := policy.ResolvedMaxRounds()
	if round > maxRounds {
		return taskledger.TaskSnapshot{}, false, capSkipNote(snap, maxRounds), nil
	}

	// The id is the round number; it skips ids the ledger already uses, so the
	// body must name the round the id actually carries.
	taskID := taskledger.NextClosingTaskID(snap)
	if idRound, ok := taskledger.ClosingRoundNumber(taskID); ok {
		round = idRound
	}
	title := strings.TrimSpace(policy.Task.Title)
	if title == "" {
		title = defaultClosingTaskTitle
	}
	body := fmt.Sprintf("Review the accumulated changes of this trail before the merge gate (closing round %d of %d).", round, maxRounds)
	spec := protocol.TaskSpec{
		TaskID: taskID,
		Title:  title,
		Body:   body,
		Bee:    colony.EffectiveTaskBee(policy.Task.Bee, colony.Defaults{}),
		Intent: strings.TrimSpace(policy.Task.Intent),
		Review: protocol.NormalizeTaskReviewPolicy(policy.Task.Review),
		Phase:  protocol.TaskPhaseClosing,
	}

	planEv, err := protocol.NewEvent(traceID, protocol.RuntimeAgentID, 0, protocol.EventInsight, protocol.TaskPlanPayload{
		Kind:  protocol.TaskEventPlan,
		Tasks: []protocol.TaskSpec{spec},
	})
	if err != nil {
		return taskledger.TaskSnapshot{}, false, "", err
	}
	if err := WriteEvent(ctx, pub, ledger, planEv, opts); err != nil {
		return taskledger.TaskSnapshot{}, false, "", err
	}

	// Apply the standard task.ready kick so the existing scheduler dispatches
	// the closing round like any other AFK task. The runtime owns this task and
	// therefore publishes the kick under the reserved runtime agent id.
	readyEv, err := protocol.NewEvent(traceID, protocol.RuntimeAgentID, 0, protocol.EventSignal, protocol.TaskReadyPayload{
		Kind:   protocol.TaskEventReady,
		TaskID: spec.TaskID,
		Title:  spec.Title,
		Body:   spec.Body,
		Bee:    spec.Bee,
		Intent: spec.Intent,
	})
	if err != nil {
		return taskledger.TaskSnapshot{}, false, "", err
	}
	if err := WriteEvent(ctx, pub, ledger, readyEv, opts); err != nil {
		return taskledger.TaskSnapshot{}, false, "", err
	}

	updated, err := ledger.Snapshot(traceID)
	if err != nil {
		return taskledger.TaskSnapshot{}, false, "", err
	}
	task, ok := updated.Tasks[taskID]
	if !ok {
		return taskledger.TaskSnapshot{}, false, "", fmt.Errorf("review: closing task %q was not recorded", taskID)
	}
	return task, true, "", nil
}

// ClosingRoundGuaranteed reports whether the pinned policy will seed a closing
// round once the trail's remaining work completes. It mirrors the seed
// conditions in ensureClosingRound without mutating the ledger, so the last
// work task can hand pr.body to the closing round instead of writing it itself.
// A failed or blocked closing round is not replaced, so it does not count as a
// guarantee.
func ClosingRoundGuaranteed(policy colony.ClosingPolicy, trace taskledger.TraceSnapshot) bool {
	if !policy.Enabled {
		return false
	}
	if taskledger.MergeTaskCompleted(trace) {
		return false
	}
	if !policy.Matches(taskledger.WorkTaskCount(trace), taskledger.HasIsolatedProposal(trace)) {
		return false
	}
	if taskledger.HasUnfinishedClosingTask(trace) {
		return false
	}
	return taskledger.HighestClosingRound(trace)+1 <= policy.ResolvedMaxRounds()
}

// predicateSkipNote explains a trail that finished work the pinned predicates
// exclude, so the operator sees why no closing round was seeded instead of a
// trail that jumps from work straight to the merge gate.
func predicateSkipNote(policy colony.ClosingPolicy, snap taskledger.TraceSnapshot) string {
	work := taskledger.WorkTaskCount(snap)
	if policy.When.MinWorkTasks > 0 && work < policy.When.MinWorkTasks {
		return fmt.Sprintf("Closing round skipped: the pinned policy requires at least %d work task(s) and this trail has %d. No closing round will be seeded.", policy.When.MinWorkTasks, work)
	}
	if policy.When.HasIsolatedProposal && !taskledger.HasIsolatedProposal(snap) {
		return "Closing round skipped: the pinned policy requires an isolated code change and this trail has none. No closing round will be seeded."
	}
	return ""
}

// capSkipNote explains a cap skip instead of creating and cancelling a
// placeholder task: a cancelled task would block the gate exactly like a
// planned one, because every non-merge task must complete.
func capSkipNote(snap taskledger.TraceSnapshot, maxRounds int) string {
	last, ok := taskledger.LatestClosingTask(snap)
	lastID := "the previous closing round"
	if ok {
		lastID = fmt.Sprintf("%q", last.TaskID)
	}
	return fmt.Sprintf("Closing round skipped: the pinned policy allows %d round(s) and follow-up work completed after %s. No further review round will be seeded.", maxRounds, lastID)
}

// recordClosingSkipNote records the reason a closing round was skipped. An
// empty summary means the policy had nothing to report.
func recordClosingSkipNote(ctx context.Context, pub bus.Publisher, ledger taskledger.Ledger, traceID, summary string, opts WriteOptions) error {
	if strings.TrimSpace(summary) == "" {
		return nil
	}
	ev, err := protocol.NewEvent(traceID, protocol.RuntimeAgentID, 0, protocol.EventInsight, protocol.NarrativeInsightPayload{
		Kind:    protocol.InsightContextNote,
		Summary: summary,
	})
	if err != nil {
		return err
	}
	return WriteEvent(ctx, pub, ledger, ev, opts)
}

// BuildTrailInventory builds the structured task list shared by closing prompts
// and gate copy templates. It is not a transcript: one row per task with the
// fields a Bee needs to review the whole trail.
func BuildTrailInventory(colonyCtx colony.Context, traceID string, snap taskledger.TraceSnapshot) colony.TrailInventory {
	inventory := colony.TrailInventory{TraceID: traceID}
	if title, err := runs.ResolveTraceTitle(colonyCtx.ColonyRoot, traceID); err == nil {
		inventory.TraceTitle = title
	}
	ids := make([]string, 0, len(snap.Tasks))
	for taskID := range snap.Tasks {
		ids = append(ids, taskID)
	}
	sort.Strings(ids)
	inventory.Tasks = make([]colony.TrailTask, 0, len(ids))
	for _, taskID := range ids {
		task := snap.Tasks[taskID]
		inventory.Tasks = append(inventory.Tasks, colony.TrailTask{
			TaskID:    task.TaskID,
			Title:     task.Title,
			Phase:     taskledger.TaskPhaseOf(task),
			Intent:    task.Intent,
			Status:    task.Status,
			Commit:    task.Commit,
			Summary:   task.Summary,
			DependsOn: append([]string(nil), task.DependsOn...),
		})
	}
	return inventory
}

func needsIsolatedMerge(snap taskledger.TraceSnapshot, colonyCtx colony.Context, traceID string) bool {
	if taskledger.HasIsolatedProposal(snap) {
		return true
	}
	if colonyCtx.ColonyRoot == "" || traceID == "" {
		return false
	}
	diff, err := worktree.MergeDiff(worktree.MergeDiffOptions{
		ColonyRoot: colonyCtx.ColonyRoot,
		TraceID:    traceID,
		Slug:       colonyCtx.Slug,
	})
	if err != nil {
		return false
	}
	return !diff.Missing && !diff.Empty
}

func completeEmptyFinalGate(ctx context.Context, pub bus.Publisher, ledger taskledger.Ledger, traceID string, final taskledger.TaskSnapshot, opts WriteOptions) error {
	switch final.Status {
	case protocol.TaskStatusCompleted, protocol.TaskStatusFailed, protocol.TaskStatusBlocked:
		return nil
	}
	completed, err := protocol.NewEvent(traceID, protocol.RuntimeAgentID, 0, protocol.EventVerification, protocol.TaskCompletedPayload{
		Kind:        protocol.TaskEventCompleted,
		TaskID:      final.TaskID,
		Status:      protocol.TaskStatusCompleted,
		Summary:     "Nothing to merge — skipped final review gate",
		CompletedAt: time.Now().UTC(),
	})
	if err != nil {
		return err
	}
	return WriteEvent(ctx, pub, ledger, completed, opts)
}

func statusEvent(traceID, taskID string, status protocol.TaskStatus, summary string) (protocol.Event, error) {
	return protocol.NewEvent(traceID, protocol.RuntimeAgentID, 0, protocol.EventSignal, protocol.TaskStatusPayload{
		Kind:    protocol.TaskEventStatus,
		TaskID:  taskID,
		Status:  status,
		Summary: summary,
	})
}
