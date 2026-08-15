package hiveview

import (
	"fmt"
	"strings"
	"time"

	"github.com/paseka/paseka/internal/bus"
	"github.com/paseka/paseka/internal/colony"
	"github.com/paseka/paseka/internal/homestate"
	"github.com/paseka/paseka/internal/protocol"
	"github.com/paseka/paseka/internal/runs"
	"github.com/paseka/paseka/internal/runtime"
	"github.com/paseka/paseka/internal/sessions"
	"github.com/paseka/paseka/internal/tasks"
)

const (
	snapshotTraceLimit     = 10
	snapshotAttentionLimit = 10
	snapshotLiveBeeDisplay = 3
)

// ColonySnapshot is the read-only colony status contract (schemaVersion 1).
type ColonySnapshot struct {
	SchemaVersion   int                   `json:"schemaVersion"`
	GeneratedAt     string                `json:"generatedAt"`
	Slug            string                `json:"slug"`
	ColonyRoot      string                `json:"colonyRoot"`
	Runtime         SnapshotRuntime       `json:"runtime"`
	NATS            SnapshotNATS          `json:"nats"`
	Agents          AgentsView            `json:"agents"`
	ActiveWorktrees int                   `json:"activeWorktrees"`
	TaskCounts      map[string]int        `json:"taskCounts"`
	Energy          SnapshotEnergy        `json:"energy"`
	Attention       SnapshotAttention     `json:"attention"`
	RecentTraces    []SnapshotRecentTrace `json:"recentTraces"`
}

// SnapshotRuntime is hive runtime status without colony identity fields.
type SnapshotRuntime struct {
	Status          string `json:"status"`
	Alive           bool   `json:"alive"`
	PID             int    `json:"pid,omitempty"`
	StartedAt       string `json:"startedAt,omitempty"`
	LastHeartbeatAt string `json:"lastHeartbeatAt,omitempty"`
	SubjectPrefix   string `json:"subjectPrefix,omitempty"`
}

// SnapshotNATS is light NATS connectivity for status snapshots.
type SnapshotNATS struct {
	Configured bool `json:"configured"`
	Connected  bool `json:"connected"`
}

// SnapshotEnergy is honey reserve for recent Flight Trails.
type SnapshotEnergy struct {
	Available bool                  `json:"available"`
	Traces    []SnapshotEnergyTrace `json:"traces"`
}

// SnapshotEnergyTrace is remaining/budget for one trace in the energy window.
type SnapshotEnergyTrace struct {
	TraceID   string `json:"traceId"`
	Remaining int    `json:"remaining"`
	Budget    int    `json:"budget"`
}

// SnapshotAttention lists items that may need Beekeeper or interface-bee action.
type SnapshotAttention struct {
	RuntimeStale    bool                      `json:"runtimeStale"`
	NatsDown        bool                      `json:"natsDown"`
	WaitingReview   []SnapshotAttentionTask   `json:"waitingReview"`
	PendingInvites  []SnapshotAttentionInvite `json:"pendingInvites"`
	FailedTasks     []SnapshotAttentionTask   `json:"failedTasks"`
	LowEnergyTraces []SnapshotLowEnergy       `json:"lowEnergyTraces"`
}

// SnapshotAttentionTask is a task row in the attention block.
type SnapshotAttentionTask struct {
	TraceID string `json:"traceId"`
	TaskID  string `json:"taskId"`
	Title   string `json:"title,omitempty"`
	Review  string `json:"review,omitempty"`
}

// SnapshotAttentionInvite is a pending Human Gateway invite.
type SnapshotAttentionInvite struct {
	InviteID string `json:"inviteId"`
	TraceID  string `json:"traceId"`
	Bee      string `json:"bee"`
}

// SnapshotLowEnergy is a trace with exhausted honey in the recent window.
type SnapshotLowEnergy struct {
	TraceID   string `json:"traceId"`
	Remaining int    `json:"remaining"`
	Budget    int    `json:"budget"`
}

// SnapshotRecentTrace is a short recent Flight Trail row.
type SnapshotRecentTrace struct {
	TraceID   string `json:"traceId"`
	Title     string `json:"title,omitempty"`
	UpdatedAt string `json:"updatedAt"`
}

// BuildColonySnapshot assembles the observe-only colony status projection.
func BuildColonySnapshot(ctx colony.Context, sup *runtime.Supervisor, mgr *sessions.Manager) (ColonySnapshot, error) {
	if sup == nil {
		sup = runtime.DefaultSupervisor()
	}
	if mgr == nil {
		mgr = sessions.NewManager()
	}

	snap := ColonySnapshot{
		SchemaVersion: 1,
		GeneratedAt:   time.Now().UTC().Format(time.RFC3339),
		Slug:          ctx.Slug,
		ColonyRoot:    ctx.ColonyRoot,
		TaskCounts:    map[string]int{},
		Energy: SnapshotEnergy{
			Traces: []SnapshotEnergyTrace{},
		},
		Attention: SnapshotAttention{
			WaitingReview:   []SnapshotAttentionTask{},
			PendingInvites:  []SnapshotAttentionInvite{},
			FailedTasks:     []SnapshotAttentionTask{},
			LowEnergyTraces: []SnapshotLowEnergy{},
		},
		RecentTraces: []SnapshotRecentTrace{},
	}

	rtView, err := GetRuntime(ctx, sup)
	if err != nil {
		return ColonySnapshot{}, err
	}
	snap.Runtime = snapshotRuntimeFromView(rtView)

	agents, err := GetAgents(ctx, mgr)
	if err != nil {
		return ColonySnapshot{}, err
	}
	if agents.Items == nil {
		agents.Items = []AgentItem{}
	}
	snap.Agents = agents

	natsPing, err := bus.Ping(ctx)
	if err != nil {
		return ColonySnapshot{}, err
	}
	snap.NATS = SnapshotNATS{
		Configured: natsPing.Configured,
		Connected:  natsPing.Connected,
	}

	st, err := homestate.LoadState(ctx.Slug)
	if err != nil {
		return ColonySnapshot{}, err
	}
	snap.ActiveWorktrees = len(st.Worktrees)

	board, err := taskBoardForSnapshot(ctx)
	if err != nil {
		return ColonySnapshot{}, err
	}
	if board.TaskCounts != nil {
		snap.TaskCounts = board.TaskCounts
	}

	invites, err := ListInvites(ctx, protocol.InviteStatusPending)
	if err != nil {
		return ColonySnapshot{}, err
	}

	traceViews, err := ListTraces(ctx, snapshotTraceLimit)
	if err != nil {
		return ColonySnapshot{}, err
	}
	for _, tr := range traceViews {
		snap.RecentTraces = append(snap.RecentTraces, SnapshotRecentTrace{
			TraceID:   tr.TraceID,
			Title:     tr.Title,
			UpdatedAt: tr.LastActivityAt.UTC().Format(time.RFC3339),
		})
	}

	snap.Energy = collectSnapshotEnergy(ctx, traceViews)
	snap.Attention = buildSnapshotAttention(snap.Runtime, snap.NATS, board, invites, snap.Energy)

	return snap, nil
}

// SubstrateHealthy reports whether the hive can choreograph (runtime alive, NATS ok if configured).
func (s ColonySnapshot) SubstrateHealthy() bool {
	if !s.Runtime.Alive {
		return false
	}
	if s.NATS.Configured && !s.NATS.Connected {
		return false
	}
	return true
}

func snapshotRuntimeFromView(view RuntimeView) SnapshotRuntime {
	out := SnapshotRuntime{
		Status:          view.Status,
		Alive:           view.Alive,
		PID:             view.PID,
		StartedAt:       view.StartedAt,
		LastHeartbeatAt: view.LastHeartbeatAt,
		SubjectPrefix:   view.SubjectPrefix,
	}
	return out
}

func taskBoardForSnapshot(ctx colony.Context) (TaskBoardView, error) {
	board, err := ListTaskBoard(ctx)
	if err == nil {
		return board, nil
	}
	return taskBoardFromFilesystem(ctx)
}

func taskBoardFromFilesystem(ctx colony.Context) (TaskBoardView, error) {
	traceSummaries, err := runs.ScanRecentTraces(ctx.ColonyRoot, TaskBoardTraceLimit)
	if err != nil {
		return TaskBoardView{}, err
	}
	var items []TaskListItem
	for _, trace := range traceSummaries {
		snap, err := runs.LoadTraceTasksFromFS(ctx.ColonyRoot, trace.TraceID)
		if err != nil {
			continue
		}
		items = append(items, taskItemsFromSnapshot(ctx, trace.TraceID, snap)...)
	}
	return buildTaskBoard(items), nil
}

func collectSnapshotEnergy(ctx colony.Context, traces []TraceSummaryView) SnapshotEnergy {
	out := SnapshotEnergy{
		Available: false,
		Traces:    []SnapshotEnergyTrace{},
	}
	if len(traces) == 0 {
		return out
	}

	session, err := tasks.OpenLedger(ctx)
	if err != nil || session.Ledger == nil {
		return out
	}
	defer session.Close()

	for _, tr := range traces {
		ledgerSnap, err := session.Ledger.Snapshot(tr.TraceID)
		if err != nil || ledgerSnap.EnergyBudget == 0 {
			continue
		}
		out.Available = true
		out.Traces = append(out.Traces, SnapshotEnergyTrace{
			TraceID:   tr.TraceID,
			Remaining: ledgerSnap.EnergyRemaining,
			Budget:    ledgerSnap.EnergyBudget,
		})
	}
	return out
}

func buildSnapshotAttention(
	rt SnapshotRuntime,
	nats SnapshotNATS,
	board TaskBoardView,
	invites []InviteView,
	energy SnapshotEnergy,
) SnapshotAttention {
	att := SnapshotAttention{
		RuntimeStale:    rt.Status == runtime.RuntimeStatusStale,
		NatsDown:        nats.Configured && !nats.Connected,
		WaitingReview:   []SnapshotAttentionTask{},
		PendingInvites:  []SnapshotAttentionInvite{},
		FailedTasks:     []SnapshotAttentionTask{},
		LowEnergyTraces: []SnapshotLowEnergy{},
	}

	for _, group := range board.Groups {
		switch group.Status {
		case string(protocol.TaskStatusWaitingReview):
			for _, task := range group.Tasks {
				if len(att.WaitingReview) >= snapshotAttentionLimit {
					break
				}
				att.WaitingReview = append(att.WaitingReview, snapshotAttentionTaskFromItem(task))
			}
		case string(protocol.TaskStatusFailed):
			for _, task := range group.Tasks {
				if len(att.FailedTasks) >= snapshotAttentionLimit {
					break
				}
				att.FailedTasks = append(att.FailedTasks, snapshotAttentionTaskFromItem(task))
			}
		}
	}

	for _, inv := range invites {
		if len(att.PendingInvites) >= snapshotAttentionLimit {
			break
		}
		att.PendingInvites = append(att.PendingInvites, SnapshotAttentionInvite{
			InviteID: inv.InviteID,
			TraceID:  inv.TraceID,
			Bee:      inv.Bee,
		})
	}

	for _, tr := range energy.Traces {
		if tr.Budget > 0 && tr.Remaining <= 0 {
			if len(att.LowEnergyTraces) >= snapshotAttentionLimit {
				break
			}
			att.LowEnergyTraces = append(att.LowEnergyTraces, SnapshotLowEnergy{
				TraceID:   tr.TraceID,
				Remaining: tr.Remaining,
				Budget:    tr.Budget,
			})
		}
	}

	return att
}

func snapshotAttentionTaskFromItem(task TaskListItem) SnapshotAttentionTask {
	return SnapshotAttentionTask{
		TraceID: task.TraceID,
		TaskID:  task.TaskID,
		Title:   task.Title,
		Review:  task.Review,
	}
}

// FormatColonySnapshot renders a compact human text snapshot.
func FormatColonySnapshot(s ColonySnapshot) string {
	lines := []string{
		fmt.Sprintf("Paseka · %s", s.Slug),
		"",
		fmt.Sprintf("Runtime: %s (alive=%v)", s.Runtime.Status, s.Runtime.Alive),
	}
	if s.Runtime.PID > 0 {
		lines[len(lines)-1] = fmt.Sprintf("Runtime: %s pid=%d (alive=%v)", s.Runtime.Status, s.Runtime.PID, s.Runtime.Alive)
	}
	if s.Runtime.SubjectPrefix != "" {
		lines = append(lines, fmt.Sprintf("Subject: %s", s.Runtime.SubjectPrefix))
	}

	natsLine := "NATS: not configured"
	if s.NATS.Configured {
		if s.NATS.Connected {
			natsLine = "NATS: connected"
		} else {
			natsLine = "NATS: configured, disconnected"
		}
	}
	lines = append(lines, natsLine)
	lines = append(lines, fmt.Sprintf("Worktrees: %d", s.ActiveWorktrees))

	lines = append(lines, "")
	lines = append(lines, fmt.Sprintf("Live bees: %d (%d afk, %d session)", s.Agents.Count, s.Agents.AFK, s.Agents.Sessions))
	displayed := 0
	for _, item := range s.Agents.Items {
		if displayed >= snapshotLiveBeeDisplay {
			break
		}
		line := formatLiveBeeLine(item)
		lines = append(lines, "  "+line)
		displayed++
	}
	if s.Agents.Count > displayed {
		lines = append(lines, fmt.Sprintf("  +%d more", s.Agents.Count-displayed))
	}

	lines = append(lines, "")
	lines = append(lines, fmt.Sprintf("Tasks: running=%d waiting_review=%d failed=%d",
		s.TaskCounts[string(protocol.TaskStatusRunning)],
		s.TaskCounts[string(protocol.TaskStatusWaitingReview)],
		s.TaskCounts[string(protocol.TaskStatusFailed)],
	))

	if s.Energy.Available && len(s.Energy.Traces) > 0 {
		lines = append(lines, "")
		lines = append(lines, "Honey:")
		for _, tr := range s.Energy.Traces {
			lines = append(lines, fmt.Sprintf("  %s: %d/%d remaining", tr.TraceID, tr.Remaining, tr.Budget))
		}
	} else if s.NATS.Configured {
		lines = append(lines, "")
		lines = append(lines, "Honey: unavailable")
	}

	if attentionHasItems(s.Attention) {
		lines = append(lines, "")
		lines = append(lines, "Attention:")
		if s.Attention.RuntimeStale {
			lines = append(lines, "  runtime stale")
		}
		if s.Attention.NatsDown {
			lines = append(lines, "  nats down")
		}
		if len(s.Attention.WaitingReview) > 0 {
			lines = append(lines, fmt.Sprintf("  waiting review: %d", len(s.Attention.WaitingReview)))
			for _, task := range s.Attention.WaitingReview {
				lines = append(lines, fmt.Sprintf("    %s/%s", task.TraceID, task.TaskID))
			}
		}
		if len(s.Attention.PendingInvites) > 0 {
			lines = append(lines, fmt.Sprintf("  pending invites: %d", len(s.Attention.PendingInvites)))
			for _, inv := range s.Attention.PendingInvites {
				lines = append(lines, fmt.Sprintf("    %s %s/%s", inv.InviteID, inv.TraceID, inv.Bee))
			}
		}
		if len(s.Attention.FailedTasks) > 0 {
			lines = append(lines, fmt.Sprintf("  failed tasks: %d", len(s.Attention.FailedTasks)))
			for _, task := range s.Attention.FailedTasks {
				lines = append(lines, fmt.Sprintf("    %s/%s", task.TraceID, task.TaskID))
			}
		}
		if len(s.Attention.LowEnergyTraces) > 0 {
			lines = append(lines, fmt.Sprintf("  low energy traces: %d", len(s.Attention.LowEnergyTraces)))
			for _, tr := range s.Attention.LowEnergyTraces {
				lines = append(lines, fmt.Sprintf("    %s (%d/%d)", tr.TraceID, tr.Remaining, tr.Budget))
			}
		}
	}

	if len(s.RecentTraces) > 0 {
		lines = append(lines, "")
		lines = append(lines, "Recent traces:")
		for _, tr := range s.RecentTraces {
			title := tr.Title
			if title == "" {
				title = tr.TraceID
			}
			lines = append(lines, fmt.Sprintf("  %s: %s", tr.TraceID, title))
		}
	}

	return strings.Join(lines, "\n")
}

func formatLiveBeeLine(item AgentItem) string {
	if item.Kind == "session" && item.SessionID != "" {
		return fmt.Sprintf("%s/session %s", item.Bee, item.SessionID)
	}
	if item.PID > 0 {
		return fmt.Sprintf("%s/%d", item.Bee, item.PID)
	}
	return item.Bee
}

func attentionHasItems(att SnapshotAttention) bool {
	return att.RuntimeStale || att.NatsDown ||
		len(att.WaitingReview) > 0 || len(att.PendingInvites) > 0 ||
		len(att.FailedTasks) > 0 || len(att.LowEnergyTraces) > 0
}
