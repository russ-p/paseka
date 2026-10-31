package taskledger

import (
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/russ-p/paseka/internal/protocol"
)

// ClosingTaskIDPrefix is the stable id prefix of a runtime-seeded closing round.
const ClosingTaskIDPrefix = "closing-"

// ClosingRoundNumber reads the round number out of a closing task id. The id is
// the source of truth: a round is identified by closing-N, never by a count of
// tasks or a random value, so a restart derives the same next id.
func ClosingRoundNumber(taskID string) (int, bool) {
	if !strings.HasPrefix(taskID, ClosingTaskIDPrefix) {
		return 0, false
	}
	round, err := strconv.Atoi(strings.TrimPrefix(taskID, ClosingTaskIDPrefix))
	if err != nil || round < 1 {
		return 0, false
	}
	return round, true
}

// HighestClosingRound returns the highest closing round recorded on the trail,
// or 0 when no closing round exists.
func HighestClosingRound(trace TraceSnapshot) int {
	highest := 0
	for _, task := range trace.Tasks {
		if !IsClosingTask(task) {
			continue
		}
		round, ok := ClosingRoundNumber(task.TaskID)
		if ok && round > highest {
			highest = round
		}
	}
	return highest
}

// NextClosingTaskID derives the next stable closing task id from the highest
// existing round, skipping ids the ledger already uses. It is safe to call
// repeatedly: the same ledger yields the same id until a new round is recorded.
// Skipping a taken id keeps a stray `closing-N` task (an operator's own, or a
// hand-written plan) from silently stalling the round, because seeding an
// existing id is a no-op the runtime would never notice.
func NextClosingTaskID(trace TraceSnapshot) string {
	round := HighestClosingRound(trace) + 1
	for {
		id := ClosingTaskIDPrefix + strconv.Itoa(round)
		if _, taken := trace.Tasks[id]; !taken {
			return id
		}
		round++
	}
}

// ClosingInFlight returns a closing task that is occupying the trail: planned,
// ready, running, awaiting review, or blocked. A failed or cancelled closing
// round is not in flight, but it still blocks the gate and is never replaced by
// seeding another round.
func ClosingInFlight(trace TraceSnapshot) (TaskSnapshot, bool) {
	candidates := make([]TaskSnapshot, 0, 1)
	for _, task := range trace.Tasks {
		if !IsClosingTask(task) {
			continue
		}
		switch task.Status {
		case protocol.TaskStatusPlanned, protocol.TaskStatusReady, protocol.TaskStatusRunning,
			protocol.TaskStatusWaitingReview, protocol.TaskStatusBlocked:
			candidates = append(candidates, task)
		}
	}
	if len(candidates) == 0 {
		return TaskSnapshot{}, false
	}
	sort.Slice(candidates, func(i, j int) bool {
		return candidates[i].TaskID < candidates[j].TaskID
	})
	return candidates[0], true
}

// HasUnfinishedClosingTask reports whether any closing round has not completed.
// It includes failed and cancelled rounds: the gate requires every non-merge
// task to be completed, so seeding another round cannot repair them and would
// only hide the stuck trail.
func HasUnfinishedClosingTask(trace TraceSnapshot) bool {
	for _, task := range trace.Tasks {
		if !IsClosingTask(task) {
			continue
		}
		if task.Status != protocol.TaskStatusCompleted {
			return true
		}
	}
	return false
}

// LatestClosingTask returns the closing round with the highest round number.
// The id is the stable identity, so a replayed or restarted ledger always
// compares against the same round.
func LatestClosingTask(trace TraceSnapshot) (TaskSnapshot, bool) {
	var latest TaskSnapshot
	found := false
	latestRound := 0
	for _, task := range trace.Tasks {
		if !IsClosingTask(task) {
			continue
		}
		round, ok := ClosingRoundNumber(task.TaskID)
		if !ok {
			continue
		}
		if !found || round > latestRound ||
			(round == latestRound && taskTime(task).After(taskTime(latest))) {
			latest = task
			latestRound = round
			found = true
		}
	}
	return latest, found
}

// TrailStage is the phase machine projected onto a whole trail. It is not a
// task phase and not a run status: a trail at the merge gate is `gate` even
// though nothing is running, which a run-derived state cannot express.
type TrailStage string

const (
	// TrailStageWork is the default: ordinary trail work is still open, or the
	// trail never entered a closing or merge phase.
	TrailStageWork TrailStage = "work"
	// TrailStageClosing means a closing round exists and has not completed.
	TrailStageClosing TrailStage = "closing"
	// TrailStageGate means the merge task waits for a human and every non-merge
	// task is completed.
	TrailStageGate TrailStage = "gate"
	// TrailStageDelivered means the merge task completed and the trail was
	// delivered.
	TrailStageDelivered TrailStage = "delivered"
)

// DeriveTrailStage projects the ledger's phase machine onto a trail using the
// same predicates the runtime uses. It is computed server-side so the client
// never has to reason about a ledger it cannot read.
func DeriveTrailStage(trace TraceSnapshot) TrailStage {
	if MergeTaskCompleted(trace) {
		return TrailStageDelivered
	}
	if MergeGateWaiting(trace) {
		return TrailStageGate
	}
	if HasUnfinishedClosingTask(trace) {
		return TrailStageClosing
	}
	return TrailStageWork
}

// MergeGateWaiting reports whether the trail is waiting on a human at the merge
// gate: a merge task sits at waiting_review and every non-merge task is
// completed, so nothing else is open.
func MergeGateWaiting(trace TraceSnapshot) bool {
	if !AllPreMergeCompleted(trace) {
		return false
	}
	for _, task := range trace.Tasks {
		if IsMergeTask(task) && task.Status == protocol.TaskStatusWaitingReview {
			return true
		}
	}
	return false
}

// WorkTaskCount counts work-phase tasks in the trail regardless of status. It
// backs the min_work_tasks predicate.
func WorkTaskCount(trace TraceSnapshot) int {
	count := 0
	for _, task := range trace.Tasks {
		if TaskPhaseOf(task) == protocol.TaskPhaseWork {
			count++
		}
	}
	return count
}

// MergeTaskCompleted reports whether the trail has already been delivered
// through a merge-phase task. A delivered trail is closed to new closing rounds.
func MergeTaskCompleted(trace TraceSnapshot) bool {
	for _, task := range trace.Tasks {
		if IsMergeTask(task) && task.Status == protocol.TaskStatusCompleted {
			return true
		}
	}
	return false
}

// ClosingCoversLatestWork reports whether the last closing round already covers
// every completed work task. A trail with no completed closing round has no
// coverage. A closing round still in flight is treated as covering the work it
// is reviewing, so the runtime never seeds a second round underneath it.
func ClosingCoversLatestWork(trace TraceSnapshot) bool {
	last, ok := LatestClosingTask(trace)
	if !ok {
		return false
	}
	if last.Status != protocol.TaskStatusCompleted {
		return true
	}
	return !WorkCompletedAfter(trace, last)
}

// WorkCompletedAfter reports whether a work-phase task completed after the
// closing round finished. This is the v1 "new work completed after the last
// closing round" comparison; spec 043's work revision replaces it later.
func WorkCompletedAfter(trace TraceSnapshot, closing TaskSnapshot) bool {
	closingAt := taskTime(closing)
	if closingAt.IsZero() {
		return false
	}
	for _, task := range trace.Tasks {
		if TaskPhaseOf(task) != protocol.TaskPhaseWork || task.Status != protocol.TaskStatusCompleted {
			continue
		}
		if taskTime(task).After(closingAt) {
			return true
		}
	}
	return false
}

func taskTime(task TaskSnapshot) time.Time {
	if !task.UpdatedAt.IsZero() {
		return task.UpdatedAt
	}
	return task.CreatedAt
}
