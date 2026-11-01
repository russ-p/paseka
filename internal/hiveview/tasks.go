package hiveview

import (
	"fmt"
	"sort"
	"time"

	"github.com/russ-p/paseka/internal/colony"
	"github.com/russ-p/paseka/internal/homestate"
	"github.com/russ-p/paseka/internal/protocol"
	"github.com/russ-p/paseka/internal/review"
	"github.com/russ-p/paseka/internal/runs"
	"github.com/russ-p/paseka/internal/taskledger"
	"github.com/russ-p/paseka/internal/tasks"
)

// TaskBoardTraceLimit is the number of recent traces scanned for board/queue projections.
const TaskBoardTraceLimit = 30

var taskStatusOrder = []string{
	string(protocol.TaskStatusReady),
	string(protocol.TaskStatusRunning),
	string(protocol.TaskStatusWaitingReview),
	string(protocol.TaskStatusPlanned),
	string(protocol.TaskStatusBlocked),
	string(protocol.TaskStatusSkipped),
	string(protocol.TaskStatusFailed),
	string(protocol.TaskStatusCompleted),
}

// TaskRunView links a task to one agent run.
type TaskRunView struct {
	AgentID    string    `json:"agentId"`
	Bee        string    `json:"bee,omitempty"`
	RunDir     string    `json:"runDir,omitempty"`
	RunStatus  string    `json:"runStatus,omitempty"`
	StartedAt  time.Time `json:"startedAt,omitempty"`
	FinishedAt time.Time `json:"finishedAt,omitempty"`
}

// TaskListItem is one task row on the board.
type TaskListItem struct {
	TraceID string `json:"traceId"`
	TaskID  string `json:"taskId"`
	Title   string `json:"title"`
	Status  string `json:"status"`
	// Phase is the ledger phase (work, closing, merge), a separate axis from
	// status: a closing card is still planned, running, or failed.
	Phase  string `json:"phase"`
	Review string `json:"review,omitempty"`
	Bee    string `json:"bee,omitempty"`
	Sector string `json:"sector,omitempty"`
	// PendingDispatch marks a task whose dispatch waits in the trail's
	// admission queue. It is not a lifecycle status: Status stays what the task
	// actually is (usually ready), and this is the admission axis beside it.
	PendingDispatch bool `json:"pendingDispatch,omitempty"`
	// ApproveBlockedReason is why the merge gate cannot be approved yet, when a
	// non-merge task is still incomplete. Empty means approve is available.
	ApproveBlockedReason string           `json:"approveBlockedReason,omitempty"`
	DependsOn            []string         `json:"dependsOn,omitempty"`
	RunCount             int              `json:"runCount"`
	CanStart             bool             `json:"canStart"`
	CanRetry             bool             `json:"canRetry"`
	CanApprove           bool             `json:"canApprove"`
	CanReject            bool             `json:"canReject"`
	CanDiscard           bool             `json:"canDiscard"`
	CanRequestChanges    bool             `json:"canRequestChanges,omitempty"`
	ReworkTaskID         string           `json:"reworkTaskId,omitempty"`
	ReworkStatus         string           `json:"reworkStatus,omitempty"`
	IsFinal              bool             `json:"isFinal"`
	ProposalWorkspace    string           `json:"proposalWorkspace,omitempty"`
	UpdatedAt            time.Time        `json:"updatedAt,omitempty"`
	Delivery             string           `json:"delivery,omitempty"`
	PRTitle              string           `json:"prTitle,omitempty"`
	PRBody               string           `json:"prBody,omitempty"`
	PullRequest          *PullRequestView `json:"pullRequest,omitempty"`
}

// TaskStatusGroup groups tasks by lifecycle status.
type TaskStatusGroup struct {
	Status string         `json:"status"`
	Tasks  []TaskListItem `json:"tasks"`
}

// TaskBoardView is the colony-wide task board projection.
type TaskBoardView struct {
	Groups     []TaskStatusGroup `json:"groups"`
	TaskCounts map[string]int    `json:"taskCounts"`
	// PendingDispatches counts deferred dispatches awaiting admission across
	// the scanned trails. It is deliberately not a task status: taskCounts
	// stays status-based, and a pending task keeps the status it really has.
	PendingDispatches int `json:"pendingDispatches"`
}

// TaskDetailView is a full task inspection view.
type TaskDetailView struct {
	TaskListItem
	Body         string        `json:"body,omitempty"`
	Intent       string        `json:"intent,omitempty"`
	Summary      string        `json:"summary,omitempty"`
	TraceSummary string        `json:"traceSummary,omitempty"`
	Commit       string        `json:"commit,omitempty"`
	Runs         []TaskRunView `json:"runs"`
	Source       string        `json:"source"`
}

// ListTaskBoard returns tasks grouped by status across recent traces.
func ListTaskBoard(ctx colony.Context) (TaskBoardView, error) {
	items, pending, err := collectRecentTaskItems(ctx)
	if err != nil {
		return TaskBoardView{}, err
	}
	view := buildTaskBoard(items)
	view.PendingDispatches = pending
	return view, nil
}

// ListTraceTasks returns tasks for one trace.
func ListTraceTasks(ctx colony.Context, traceID string) ([]TaskListItem, error) {
	if traceID == "" {
		return nil, fmt.Errorf("trace id is required")
	}
	session, err := tasks.OpenLedger(ctx)
	if err != nil {
		return nil, err
	}
	defer session.Close()

	snap, _, err := tasks.LoadTrace(ctx, session.Ledger, traceID)
	if err != nil {
		return nil, err
	}
	return taskItemsFromSnapshot(ctx, traceID, snap), nil
}

// GetTask returns one task detail view.
func GetTask(ctx colony.Context, traceID, taskID string) (TaskDetailView, bool, error) {
	if traceID == "" || taskID == "" {
		return TaskDetailView{}, false, nil
	}
	session, err := tasks.OpenLedger(ctx)
	if err != nil {
		return TaskDetailView{}, false, err
	}
	defer session.Close()

	snap, source, err := tasks.LoadTrace(ctx, session.Ledger, traceID)
	if err != nil {
		return TaskDetailView{}, false, err
	}
	task, ok := snap.Tasks[taskID]
	if !ok {
		return TaskDetailView{}, false, nil
	}

	item := TaskItemFromSnapshot(ctx, traceID, snap, task)
	view := TaskDetailView{
		TaskListItem: item,
		Body:         task.Body,
		Intent:       task.Intent,
		Summary:      task.Summary,
		Commit:       task.Commit,
		Source:       string(source),
	}
	if taskledger.IsFinalReviewTask(task) {
		if traceSummary, err := runs.ResolveTraceSummary(ctx.ColonyRoot, traceID); err == nil {
			view.TraceSummary = traceSummary
		}
		if manifest, err := colony.LoadColony(ctx.ColonyRoot); err == nil {
			view.Delivery = manifest.Defaults.ResolvedDelivery()
		}
		if pubCopy, err := review.ResolvePublishCopy(ctx.ColonyRoot, traceID, "", ""); err == nil {
			view.PRTitle = pubCopy.Title
			view.PRBody = pubCopy.Body
		}
		if pr, ok, err := homestate.FindPullRequest(ctx.Slug, traceID); err == nil && ok {
			view.PullRequest = &PullRequestView{
				URL:    pr.URL,
				Number: pr.Number,
				Head:   pr.Head,
				State:  pr.State,
				Draft:  pr.Draft,
			}
		}
	}

	taskDir, err := runs.NewTaskDir(ctx.ColonyRoot, traceID, taskID)
	if err != nil {
		return TaskDetailView{}, false, err
	}
	runEntries, err := taskDir.ReadTaskRuns()
	if err != nil {
		return TaskDetailView{}, false, err
	}
	for _, entry := range runEntries {
		view.Runs = append(view.Runs, TaskRunView{
			AgentID:    entry.AgentID,
			Bee:        entry.Bee,
			RunDir:     entry.RunDir,
			RunStatus:  entry.RunStatus,
			StartedAt:  entry.StartedAt,
			FinishedAt: entry.FinishedAt,
		})
	}
	return view, true, nil
}

// TaskItemFromSnapshot builds one task list row from ledger state. It derives
// the pending-task set for a single-row caller; bulk projections derive the set
// once per snapshot and call TaskItemFromSnapshotWithPending instead.
func TaskItemFromSnapshot(ctx colony.Context, traceID string, snap taskledger.TraceSnapshot, task taskledger.TaskSnapshot) TaskListItem {
	return TaskItemFromSnapshotWithPending(ctx, traceID, snap, task, taskledger.PendingTaskIDs(snap))
}

// TaskItemFromSnapshotWithPending builds one task list row using a pending-task
// set already derived from the same snapshot. A nil or empty set reads as "no
// pending task", so an older snapshot needs no migration.
func TaskItemFromSnapshotWithPending(ctx colony.Context, traceID string, snap taskledger.TraceSnapshot, task taskledger.TaskSnapshot, pending map[string]bool) TaskListItem {
	title := task.Title
	if title == "" {
		title = task.TaskID
	}
	status := string(task.Status)
	if status == "" {
		status = string(protocol.TaskStatusPlanned)
	}

	runCount := 0
	if taskDir, err := runs.NewTaskDir(ctx.ColonyRoot, traceID, task.TaskID); err == nil {
		if entries, err := taskDir.ReadTaskRuns(); err == nil {
			runCount = len(entries)
		}
	}

	reviewPolicy := string(protocol.NormalizeTaskReviewPolicy(task.Review))
	canApprove, canReject := reviewActionsForTask(task)
	canDiscard := canReject
	proposalWorkspace := string(task.ProposalWorkspace)
	item := TaskListItem{
		TraceID:              traceID,
		TaskID:               task.TaskID,
		Title:                title,
		Status:               status,
		Phase:                string(taskledger.TaskPhaseOf(task)),
		Review:               reviewPolicy,
		Bee:                  task.Bee,
		Sector:               task.Sector,
		PendingDispatch:      pending[task.TaskID],
		ApproveBlockedReason: review.ApproveBlockedReason(snap, task.TaskID),
		DependsOn:            append([]string(nil), task.DependsOn...),
		RunCount:             runCount,
		CanStart:             tasks.CanStartTask(snap, task.TaskID),
		CanRetry:             tasks.CanRetryTask(snap, task.TaskID),
		CanApprove:           canApprove,
		CanReject:            canReject,
		CanDiscard:           canDiscard,
		IsFinal:              taskledger.IsFinalReviewTask(task),
		ProposalWorkspace:    proposalWorkspace,
		UpdatedAt:            task.UpdatedAt,
	}
	if item.IsFinal {
		item.CanRequestChanges = taskledger.CanRequestChanges(snap, task)
		if inflight, ok := taskledger.InFlightRework(snap); ok {
			item.ReworkTaskID = inflight.TaskID
			item.ReworkStatus = string(inflight.Status)
		}
	}
	return item
}

func collectRecentTaskItems(ctx colony.Context) ([]TaskListItem, int, error) {
	traceSummaries, err := runs.ScanRecentTraces(ctx.ColonyRoot, TaskBoardTraceLimit)
	if err != nil {
		return nil, 0, err
	}

	session, err := tasks.OpenLedger(ctx)
	if err != nil {
		return nil, 0, err
	}
	defer session.Close()

	var items []TaskListItem
	pending := 0
	for _, trace := range traceSummaries {
		snap, _, err := tasks.LoadTrace(ctx, session.Ledger, trace.TraceID)
		if err != nil {
			continue
		}
		pending += taskledger.ActivePendingCount(snap)
		items = append(items, taskItemsFromSnapshot(ctx, trace.TraceID, snap)...)
	}
	return items, pending, nil
}

func buildTaskBoard(items []TaskListItem) TaskBoardView {
	counts := map[string]int{}
	byStatus := map[string][]TaskListItem{}
	for _, item := range items {
		status := item.Status
		if status == "" {
			status = string(protocol.TaskStatusPlanned)
		}
		counts[status]++
		byStatus[status] = append(byStatus[status], item)
	}

	// Groups starts empty for the same reason a trace's tasks do: the board is a
	// JSON `groups` array to the console, and an empty board must be `[]`.
	view := TaskBoardView{Groups: []TaskStatusGroup{}, TaskCounts: counts}
	for _, status := range taskStatusOrder {
		groupItems := byStatus[status]
		if len(groupItems) == 0 {
			continue
		}
		sort.Slice(groupItems, func(i, j int) bool {
			if !groupItems[i].UpdatedAt.Equal(groupItems[j].UpdatedAt) {
				return groupItems[i].UpdatedAt.After(groupItems[j].UpdatedAt)
			}
			return groupItems[i].TaskID < groupItems[j].TaskID
		})
		view.Groups = append(view.Groups, TaskStatusGroup{
			Status: status,
			Tasks:  groupItems,
		})
		delete(byStatus, status)
	}
	remaining := make([]string, 0, len(byStatus))
	for status := range byStatus {
		remaining = append(remaining, status)
	}
	sort.Strings(remaining)
	for _, status := range remaining {
		groupItems := byStatus[status]
		sort.Slice(groupItems, func(i, j int) bool {
			return groupItems[i].TaskID < groupItems[j].TaskID
		})
		view.Groups = append(view.Groups, TaskStatusGroup{
			Status: status,
			Tasks:  groupItems,
		})
	}
	return view
}

func taskItemsFromSnapshot(ctx colony.Context, traceID string, snap taskledger.TraceSnapshot) []TaskListItem {
	ids := make([]string, 0, len(snap.Tasks))
	for id := range snap.Tasks {
		ids = append(ids, id)
	}
	sort.Strings(ids)
	// One pending set per trace snapshot, shared by every row in this projection.
	pending := taskledger.PendingTaskIDs(snap)
	out := make([]TaskListItem, 0, len(ids))
	for _, id := range ids {
		out = append(out, TaskItemFromSnapshotWithPending(ctx, traceID, snap, snap.Tasks[id], pending))
	}
	return out
}

func reviewActionsForTask(task taskledger.TaskSnapshot) (canApprove, canReject bool) {
	if task.Status != protocol.TaskStatusWaitingReview {
		return false, false
	}
	if !taskledger.IsReviewGate(task) {
		return false, false
	}
	return true, true
}
