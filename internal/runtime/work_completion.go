package runtime

import (
	"context"
	"fmt"
	"os"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/russ-p/paseka/internal/bus"
	"github.com/russ-p/paseka/internal/cues"
	"github.com/russ-p/paseka/internal/logging"
	"github.com/russ-p/paseka/internal/protocol"
	"github.com/russ-p/paseka/internal/runs"
	"github.com/russ-p/paseka/internal/taskledger"
)

// workCompletionBarrier serializes work-completion evaluation with new work
// admission for one trail. A completion is certified against a ledger snapshot
// that no other admission can change while the certificate describes it.
type workCompletionBarrier struct {
	mu sync.Mutex
}

// completionBarrier returns the per-trail completion/admission lock. It is
// created lazily and never removed: a trail identity is cheap, and a released
// lock that another goroutine still references must stay the same mutex.
func (r *Reactor) completionBarrier(traceID string) *workCompletionBarrier {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.completionBarriers == nil {
		r.completionBarriers = make(map[string]*workCompletionBarrier)
	}
	barrier := r.completionBarriers[traceID]
	if barrier == nil {
		barrier = &workCompletionBarrier{}
		r.completionBarriers[traceID] = barrier
	}
	return barrier
}

// applyLedgerEvent applies one event under the trail's completion barrier, so
// a new work admission cannot land between a completion's snapshot read and the
// evidence capture it certifies.
func (r *Reactor) applyLedgerEvent(ev protocol.Event) (taskledger.ApplyResult, error) {
	barrier := r.completionBarrier(ev.TraceID)
	barrier.mu.Lock()
	defer barrier.mu.Unlock()
	return r.ledger.Apply(ev)
}

// workInvocationInFlight reports whether this process still runs a working
// invocation for the trail. Task dispatches (work and closing alike) and
// ordinary direct work hold the milestone; an analysis reaction is studying an
// already-recorded result and must not hold it.
func (r *Reactor) workInvocationInFlight(traceID string) bool {
	taskPrefix := traceID + ":"
	directPrefix := traceID + "|"
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.spawning[traceID] > 0 {
		return true
	}
	for key := range r.inflight {
		if strings.HasPrefix(key, taskPrefix) {
			return true
		}
	}
	for key := range r.directInflight {
		if !strings.HasPrefix(key, directPrefix) {
			continue
		}
		if r.directPurpose[key] == protocol.PurposeAnalysis {
			continue
		}
		return true
	}
	return false
}

// noteDirectPurpose records why a direct invocation is running, so the
// completion barrier can exclude analysis without guessing from the Bee name.
func (r *Reactor) noteDirectPurpose(key string, purpose protocol.ExecutionPurpose) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.directPurpose == nil {
		r.directPurpose = make(map[string]protocol.ExecutionPurpose)
	}
	r.directPurpose[key] = purpose
}

func (r *Reactor) clearDirectPurpose(key string) {
	r.mu.Lock()
	defer r.mu.Unlock()
	delete(r.directPurpose, key)
}

func (r *Reactor) noteSpawningDispatch(traceID string) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.spawning == nil {
		r.spawning = make(map[string]int)
	}
	r.spawning[traceID]++
}

func (r *Reactor) clearSpawningDispatch(traceID string) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.spawning[traceID] <= 1 {
		delete(r.spawning, traceID)
		return
	}
	r.spawning[traceID]--
}

// standingTrail reports whether the trace identity belongs to a standing cue.
// A standing tick recurs under one identity: its completed work is a tick
// boundary, never a finished trail, so it emits no success milestone.
func (r *Reactor) standingTrail(traceID string) (bool, error) {
	if r.colony.ColonyRoot == "" || traceID == "" {
		return false, nil
	}
	ids, err := cues.StandingTraceIDs(r.colony.ColonyRoot)
	if err != nil {
		return false, err
	}
	_, ok := ids[traceID]
	return ok, nil
}

// retainWorkRevision records the accepted work revision so an offline metrics
// read can report it without the task ledger. It never declares completion.
func (r *Reactor) retainWorkRevision(trace taskledger.TraceSnapshot) {
	if r.colony.ColonyRoot == "" || trace.TraceID == "" || trace.WorkRevision < 1 {
		return
	}
	if err := runs.WriteTrailWorkRecord(r.colony.ColonyRoot, trace.TraceID, runs.TrailWorkRecord{
		Revision: trace.WorkRevision,
	}); err != nil {
		runtimeLog.Warn("retain work revision failed",
			logging.F("trace", trace.TraceID),
			logging.F("error", err.Error()),
		)
	}
}

// CheckWorkCompletion evaluates the work-completion boundary for one trail. It
// is the single entry point used after working task transitions, mandatory
// verification, closing-round transitions, direct-dispatch completion,
// registered session completion, run finalization, and reactor startup.
//
// It never seeds a closing round and never declares final acceptance. A missing
// or unreadable measurement is a warning on the retained metrics, not a reason
// to announce a completion the evidence does not support.
func (r *Reactor) CheckWorkCompletion(ctx context.Context, traceID string) error {
	return r.maybeCompleteWork(ctx, traceID)
}

func (r *Reactor) maybeCompleteWork(ctx context.Context, traceID string) error {
	if r == nil || r.ledger == nil || r.colony.ColonyRoot == "" || traceID == "" {
		return nil
	}
	barrier := r.completionBarrier(traceID)
	barrier.mu.Lock()
	defer barrier.mu.Unlock()

	snap, err := r.ledger.Snapshot(traceID)
	if err != nil {
		return err
	}
	if snap.Killed {
		// A killed trail is not a completion candidate and writes no work
		// evidence: an async dispatch may still land after its caller returned.
		return nil
	}
	r.retainWorkRevision(snap)

	// A recorded completion is the durable identity for its revision. Retrying
	// its publication must reuse it, and an already-published one must not be
	// announced again by a later unrelated event.
	if snap.WorkCompletion != nil && !snap.WorkCompletion.Published {
		// Retry the recorded episode first, even when the trail has since
		// accepted new work: its identity is historical and must not be
		// rewritten or silently dropped.
		if err := r.publishCompletion(ctx, snap, *snap.WorkCompletion); err != nil {
			return err
		}
		snap, err = r.ledger.Snapshot(traceID)
		if err != nil {
			return err
		}
		if snap.WorkCompletion != nil && !snap.WorkCompletion.Published {
			// The retry failed; the next boundary tries the same identity.
			return nil
		}
	}
	if snap.WorkCompletion != nil && snap.WorkCompletion.WorkRevision == snap.WorkRevision {
		return nil
	}

	verdict := taskledger.EvaluateWorkCompletion(snap)
	if !verdict.Complete {
		return nil
	}
	standing, err := r.standingTrail(traceID)
	if err != nil {
		runtimeLog.Warn("work completion check could not read standing cues",
			logging.F("trace", traceID),
			logging.F("error", err.Error()),
		)
		return nil
	}
	if standing {
		return nil
	}
	// A working invocation that has not left its in-flight set — including the
	// one that just persisted the last result — holds the milestone. Leaving it
	// calls this check again.
	if r.workInvocationInFlight(traceID) {
		return nil
	}
	if !bus.PublisherAvailable(r.publisher) {
		// No announcement channel means no milestone to certify; the next
		// boundary retries once a publisher exists.
		return nil
	}

	completedAt := time.Now().UTC()
	completionID := taskledger.CompletionIDForRevision(traceID, snap.WorkRevision)
	metrics, err := runs.BuildTrailMetrics(r.colony.ColonyRoot, traceID, completedAt)
	if err != nil {
		return fmt.Errorf("runtime: build completion metrics: %w", err)
	}
	result := r.workCompletionResult(traceID, snap, metrics)
	metrics.Work.Revision = &snap.WorkRevision
	metrics.Work.Completion = &runs.MetricsCompletion{
		CompletionID: completionID,
		WorkRevision: snap.WorkRevision,
		CompletedAt:  completedAt,
		ResultRef:    runs.WorkCompletionResultRef(traceID, completionID),
		MetricsRef:   runs.WorkCompletionMetricsRef(traceID, completionID),
	}
	checkpoint, err := runs.WriteWorkCompletion(r.colony.ColonyRoot, traceID, runs.WorkCompletionCheckpoint{
		CompletionID: completionID,
		WorkRevision: snap.WorkRevision,
		CompletedAt:  completedAt,
		TaskIDs:      completionTaskIDs(snap),
	}, result, metrics)
	if err != nil {
		// Required evidence that cannot persist is a visible finalization
		// error, never a successfully announced completion.
		return fmt.Errorf("runtime: capture work completion: %w", err)
	}
	if err := runs.WriteTrailMetrics(r.colony.ColonyRoot, traceID, metrics); err != nil {
		return fmt.Errorf("runtime: write metrics projection: %w", err)
	}

	recorded, _, err := r.ledger.RecordWorkCompletion(traceID, taskledger.WorkCompletionSnapshot{
		CompletionID: checkpoint.CompletionID,
		WorkRevision: checkpoint.WorkRevision,
		CompletedAt:  checkpoint.CompletedAt,
		ResultRef:    checkpoint.ResultRef,
		MetricsRef:   checkpoint.MetricsRef,
	})
	if err != nil {
		return err
	}
	r.retainWorkCompletion(traceID, snap.WorkRevision, recorded)
	return r.publishCompletion(ctx, snap, recorded)
}

// publishCompletion publishes one recorded completion with its durable
// identity. A publication failure leaves the record unpublished so the next
// boundary or reactor restart retries the same identity; the work itself is
// not failed by an undeliverable announcement.
func (r *Reactor) publishCompletion(ctx context.Context, snap taskledger.TraceSnapshot, completion taskledger.WorkCompletionSnapshot) error {
	if completion.ResultRef == "" || completion.MetricsRef == "" {
		return fmt.Errorf("runtime: completion %q has no evidence references", completion.CompletionID)
	}
	for _, ref := range []string{completion.ResultRef, completion.MetricsRef} {
		path, err := runs.ResolveTrailRef(r.colony.ColonyRoot, ref)
		if err != nil {
			return err
		}
		if _, err := os.Stat(path); err != nil {
			return fmt.Errorf("runtime: completion %q evidence %q is not readable: %w", completion.CompletionID, ref, err)
		}
	}
	ev, err := protocol.NewEvent(snap.TraceID, protocol.RuntimeAgentID, 0, protocol.EventSignal, protocol.TrailWorkCompletedPayload{
		Kind:         protocol.SignalTrailWorkCompleted,
		CompletionID: completion.CompletionID,
		WorkRevision: completion.WorkRevision,
		CompletedAt:  completion.CompletedAt,
		ResultRef:    completion.ResultRef,
		MetricsRef:   completion.MetricsRef,
	})
	if err != nil {
		return err
	}
	// Remember before publish so the JetStream echo cannot treat the milestone
	// as new work admission.
	r.rememberLocalEvent(ev)
	if err := r.publisher.PublishEvent(ctx, ev); err != nil {
		runtimeLog.Warn("work completion publication failed",
			logging.F("trace", snap.TraceID),
			logging.F("completion", completion.CompletionID),
			logging.F("error", err.Error()),
		)
		return nil
	}
	if err := r.ledger.MarkWorkCompletionPublished(snap.TraceID, completion.CompletionID); err != nil {
		runtimeLog.Warn("mark work completion published failed",
			logging.F("trace", snap.TraceID),
			logging.F("completion", completion.CompletionID),
			logging.F("error", err.Error()),
		)
	}
	r.retainWorkCompletion(snap.TraceID, snap.WorkRevision, completion)
	return nil
}

// retainWorkCompletion writes the current completion into the retained work
// record for offline metrics reads. The task ledger remains the authority for
// retry identity.
func (r *Reactor) retainWorkCompletion(traceID string, revision int, completion taskledger.WorkCompletionSnapshot) {
	if r.colony.ColonyRoot == "" || traceID == "" {
		return
	}
	record := runs.TrailWorkRecord{Revision: revision}
	if completion.CompletionID != "" {
		record.Revision = completion.WorkRevision
		record.Completion = &runs.MetricsCompletion{
			CompletionID: completion.CompletionID,
			WorkRevision: completion.WorkRevision,
			CompletedAt:  completion.CompletedAt,
			ResultRef:    completion.ResultRef,
			MetricsRef:   completion.MetricsRef,
		}
	}
	if err := runs.WriteTrailWorkRecord(r.colony.ColonyRoot, traceID, record); err != nil {
		runtimeLog.Warn("retain work completion failed",
			logging.F("trace", traceID),
			logging.F("error", err.Error()),
		)
	}
}

// RecoverPendingWorkCompletions retries recorded completions whose milestone
// never reached the publisher. Startup republishes them with the same identity
// before new work is admitted, so a crash cannot silently lose a completion.
func (r *Reactor) RecoverPendingWorkCompletions(ctx context.Context) error {
	if r.ledger == nil {
		return nil
	}
	lister, ok := r.ledger.(taskledger.TraceLister)
	if !ok {
		return nil
	}
	traceIDs, err := lister.TraceIDs()
	if err != nil {
		return err
	}
	sort.Strings(traceIDs)
	for _, traceID := range traceIDs {
		snap, err := r.ledger.Snapshot(traceID)
		if err != nil {
			runtimeLog.Warn("recover work completion failed",
				logging.F("trace", traceID),
				logging.F("error", err.Error()),
			)
			continue
		}
		if snap.WorkCompletion == nil || snap.WorkCompletion.Published {
			continue
		}
		if err := r.maybeCompleteWork(ctx, traceID); err != nil {
			runtimeLog.Warn("recover work completion failed",
				logging.F("trace", traceID),
				logging.F("error", err.Error()),
			)
		}
	}
	return nil
}

func (r *Reactor) workCompletionResult(traceID string, snap taskledger.TraceSnapshot, metrics runs.TrailMetrics) runs.WorkCompletionResult {
	var ids []string
	for taskID := range snap.Tasks {
		ids = append(ids, taskID)
	}
	sort.Strings(ids)
	result := runs.WorkCompletionResult{
		TraceID:      traceID,
		WorkRevision: snap.WorkRevision,
		Tasks:        make([]runs.WorkCompletionTask, 0, len(ids)),
	}
	for _, taskID := range ids {
		task := snap.Tasks[taskID]
		if taskledger.IsMergeTask(task) {
			// Final acceptance and delivery are later observations.
			continue
		}
		taskResult := runs.WorkCompletionTask{
			TaskID:       task.TaskID,
			Phase:        string(taskledger.TaskPhaseOf(task)),
			Bee:          task.Bee,
			Intent:       task.Intent,
			Status:       string(task.Status),
			Commit:       task.Commit,
			Summary:      task.Summary,
			ProposalType: string(task.ProposalWorkspace),
			Skipped:      task.Status == protocol.TaskStatusSkipped,
		}
		if taskDir, err := runs.NewTaskDir(r.colony.ColonyRoot, traceID, taskID); err == nil {
			if entries, err := taskDir.ReadTaskRuns(); err == nil {
				for _, entry := range entries {
					if entry.AgentID != "" {
						taskResult.Runs = append(taskResult.Runs, entry.AgentID)
					}
				}
			}
		}
		result.Tasks = append(result.Tasks, taskResult)
	}
	result.Checks = runs.WorkCompletionChecks{
		Runs:               metrics.Runs.Retained,
		ChecksPassed:       metrics.Runs.ChecksPassed,
		ChecksFailed:       metrics.Runs.ChecksFailed,
		VerificationFailed: metrics.VerificationFailed,
		GateRejections:     metrics.GateRejections,
	}
	return result
}

func completionTaskIDs(snap taskledger.TraceSnapshot) []string {
	ids := make([]string, 0, len(snap.Tasks))
	for taskID, task := range snap.Tasks {
		if taskledger.IsMergeTask(task) {
			continue
		}
		ids = append(ids, taskID)
	}
	sort.Strings(ids)
	return ids
}
