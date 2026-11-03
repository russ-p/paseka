package runs

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/russ-p/paseka/internal/protocol"
	"github.com/russ-p/paseka/internal/taskledger"
)

func metricsEvent(t *testing.T, traceID, agentID string, typ protocol.EventType, payload any, at time.Time) protocol.Event {
	t.Helper()
	ev, err := protocol.NewEvent(traceID, agentID, 0, typ, payload)
	if err != nil {
		t.Fatal(err)
	}
	ev.CreatedAt = at
	return ev
}

func appendMetricsEvent(t *testing.T, root, traceID string, ev protocol.Event) {
	t.Helper()
	if err := (Trail{ColonyRoot: root, TraceID: traceID}).AppendEvent(ev); err != nil {
		t.Fatal(err)
	}
}

type metricsRunSpec struct {
	agentID      string
	bee          string
	taskID       string
	started      time.Time
	finished     time.Time
	state        protocol.RunStatus
	reason       string
	usage        *protocol.Usage
	sessionUsage *protocol.Usage
	gate         *protocol.GateResult
	contract     *protocol.ContractRecord
}

func writeMetricsRun(t *testing.T, root, traceID string, spec metricsRunSpec) {
	t.Helper()
	if spec.state == "" {
		spec.state = protocol.StatusCompleted
	}
	d := Dir{ColonyRoot: root, TraceID: traceID, AgentID: spec.agentID}
	if err := d.Prepare(); err != nil {
		t.Fatal(err)
	}
	if err := d.WriteRequest(protocol.Request{
		ProtocolVersion: protocol.Version,
		TraceID:         traceID,
		AgentID:         spec.agentID,
		Bee:             spec.bee,
		Adapter:         "cursor",
		Workspace:       root,
		ColonyRoot:      root,
		TaskID:          spec.taskID,
		CreatedAt:       spec.started,
	}); err != nil {
		t.Fatal(err)
	}
	if err := d.WriteStatusSnapshot(protocol.StatusSnapshot{
		ProtocolVersion: protocol.Version,
		State:           spec.state,
		Reason:          spec.reason,
		StartedAt:       spec.started,
		FinishedAt:      spec.finished,
		Gate:            spec.gate,
	}); err != nil {
		t.Fatal(err)
	}
	if err := d.WriteResult(protocol.Result{
		ProtocolVersion: protocol.Version,
		TraceID:         traceID,
		AgentID:         spec.agentID,
		Status:          spec.state,
		Reason:          spec.reason,
		Summary:         "ok",
		Usage:           spec.usage,
		FinishedAt:      spec.finished,
		Gate:            spec.gate,
		Contract:        spec.contract,
	}); err != nil {
		t.Fatal(err)
	}
	if spec.sessionUsage != nil {
		if err := d.WriteSession(SessionMeta{
			ProtocolVersion: protocol.Version,
			SessionID:       "session-" + spec.agentID,
			TraceID:         traceID,
			AgentID:         spec.agentID,
			Bee:             spec.bee,
			Adapter:         "cursor",
			Workspace:       root,
			ColonyRoot:      root,
			State:           string(spec.state),
			Usage:           spec.sessionUsage,
			StartedAt:       spec.started,
			FinishedAt:      spec.finished,
		}); err != nil {
			t.Fatal(err)
		}
	}
}

func writeMetricsTask(t *testing.T, root, traceID string, task taskledger.TaskSnapshot) {
	t.Helper()
	if task.Status == "" {
		task.Status = protocol.TaskStatusCompleted
	}
	if err := WriteTaskSnapshot(root, traceID, task); err != nil {
		t.Fatal(err)
	}
}

func writeMetricsOrigin(t *testing.T, root, traceID string, origin TrailOrigin) {
	t.Helper()
	if _, _, err := EnsureTrailOrigin(root, traceID, origin); err != nil {
		t.Fatal(err)
	}
}

func writeMetricsInvocation(t *testing.T, root, traceID string, inv CueInvocation) {
	t.Helper()
	if err := WriteCueInvocation(root, traceID, inv); err != nil {
		t.Fatal(err)
	}
}

func TestBuildTrailMetricsCostAndRunHalf(t *testing.T) {
	root := t.TempDir()
	traceID := "trace-metrics"
	base := time.Now().UTC().Truncate(time.Second).Add(-time.Hour)
	observedAt := base.Add(10 * time.Minute)

	writeMetricsOrigin(t, root, traceID, TrailOrigin{
		Kind:         OriginKindCue,
		Source:       "cli",
		InvocationID: "cueinv-1",
		CueID:        "cue-a",
	})
	writeMetricsInvocation(t, root, traceID, CueInvocation{
		InvocationID: "cueinv-1", TraceID: traceID, CueID: "cue-a", Source: "cli",
		Mode: "task", InvokedAt: base, TaskIDs: []string{"task-1"},
	})
	writeMetricsInvocation(t, root, traceID, CueInvocation{
		InvocationID: "cueinv-2", TraceID: traceID, CueID: "cue-b", Source: "telegram",
		Mode: "signal", InvokedAt: base.Add(time.Minute),
	})

	writeMetricsTask(t, root, traceID, taskledger.TaskSnapshot{
		TaskID: "task-1", Bee: "builder", Intent: "feature", Status: protocol.TaskStatusCompleted,
		CreatedAt: base.Add(time.Second),
	})
	writeMetricsTask(t, root, traceID, taskledger.TaskSnapshot{
		TaskID: "task-2", Bee: "reviewer", Intent: "feature", Status: protocol.TaskStatusCompleted,
		CreatedAt: base.Add(2 * time.Second),
	})

	appendMetricsEvent(t, root, traceID, metricsEvent(t, traceID, "runtime", protocol.EventSignal, protocol.EnergySeededPayload{
		Kind: protocol.SignalEnergySeeded, SeedID: "seed-1", Budget: 12, SeededAt: base,
	}, base))
	charge := metricsEvent(t, traceID, "runtime", protocol.EventSignal, protocol.EnergyConsumePayload{
		Kind: protocol.SignalEnergyConsume, Amount: 2, Reason: "task.dispatch", TaskID: "task-1",
	}, base.Add(time.Minute))
	// The trail record is at-least-once: the same charge may be appended twice.
	appendMetricsEvent(t, root, traceID, charge)
	appendMetricsEvent(t, root, traceID, charge)
	appendMetricsEvent(t, root, traceID, metricsEvent(t, traceID, "runtime", protocol.EventSignal, protocol.EnergyConsumePayload{
		Kind: protocol.SignalEnergyConsume, Amount: 1, Reason: "task.dispatch", TaskID: "task-2",
	}, base.Add(2*time.Minute)))
	appendMetricsEvent(t, root, traceID, metricsEvent(t, traceID, "cli", protocol.EventSignal, protocol.EnergyAddPayload{
		Kind: protocol.SignalEnergyAdd, Amount: 5,
	}, base.Add(3*time.Minute)))

	writeMetricsRun(t, root, traceID, metricsRunSpec{
		agentID: "run-a", bee: "builder", taskID: "task-1",
		started: base.Add(time.Minute), finished: base.Add(2 * time.Minute),
		usage: &protocol.Usage{InputTokens: 100, OutputTokens: 20, CacheReadTokens: 5},
	})
	writeMetricsRun(t, root, traceID, metricsRunSpec{
		agentID: "run-b", bee: "builder", taskID: "task-2",
		started: base.Add(90 * time.Second), finished: base.Add(3 * time.Minute),
		usage: &protocol.Usage{InputTokens: 50, OutputTokens: 10, CacheWriteTokens: 1},
	})

	m, err := BuildTrailMetrics(root, traceID, observedAt)
	if err != nil {
		t.Fatal(err)
	}
	if m.SchemaVersion != 1 || m.TraceID != traceID || !m.ObservedAt.Equal(observedAt) {
		t.Fatalf("identity = %+v", m)
	}
	if m.CueID != "cue-a" {
		t.Fatalf("cueId = %q, want cue-a", m.CueID)
	}
	if len(m.CueIngresses) != 2 {
		t.Fatalf("cueIngresses = %+v, want 2", m.CueIngresses)
	}
	if m.CueIngresses[0].Attribution != CueAttributionOrigin || m.CueIngresses[1].Attribution != CueAttributionAddition {
		t.Fatalf("cue attribution = %+v", m.CueIngresses)
	}
	if m.Class == nil || m.Class.TaskID != "task-1" || m.Class.Bee != "builder" || m.Class.TaskCount != 2 {
		t.Fatalf("class = %+v", m.Class)
	}
	if m.Work.State == nil || *m.Work.State != "completed" || m.Work.Stage != "work" {
		t.Fatalf("work = %+v", m.Work)
	}
	if m.Work.Revision != nil || m.Work.Completion != nil {
		t.Fatalf("completion slot must stay null until the completion slice: %+v", m.Work)
	}
	if m.Honey.Budget == nil || *m.Honey.Budget != 12 {
		t.Fatalf("budget = %v, want 12", m.Honey.Budget)
	}
	if m.Honey.Spent == nil || *m.Honey.Spent != 3 {
		t.Fatalf("spent = %v, want 3 distinct charges", m.Honey.Spent)
	}
	if m.Honey.ByReason["task.dispatch"] != 3 {
		t.Fatalf("byReason = %+v", m.Honey.ByReason)
	}
	if m.Tokens == nil {
		t.Fatal("tokens = nil, want measured usage")
	}
	if m.Tokens.InputTokens != 150 || m.Tokens.OutputTokens != 30 ||
		m.Tokens.CacheReadTokens != 5 || m.Tokens.CacheWriteTokens != 1 ||
		m.Tokens.RunsWithUsage != 2 || m.Tokens.Runs != 2 || m.Tokens.Partial {
		t.Fatalf("tokens = %+v", m.Tokens)
	}
	if m.Runs.Retained != 2 || m.Runs.ByBee["builder"] != 2 {
		t.Fatalf("runs = %+v", m.Runs)
	}
	if m.ActiveTimeSec == nil || *m.ActiveTimeSec != 150 {
		t.Fatalf("activeTimeSec = %v, want 150 (parallel runs add)", m.ActiveTimeSec)
	}
	if m.WallTimeSec == nil || *m.WallTimeSec != 599 {
		t.Fatalf("wallTimeSec = %v, want 599 (review wait stays in wall time)", m.WallTimeSec)
	}
	for label, got := range map[string]string{
		"events": m.Coverage.Events, "runs": m.Coverage.Runs, "tasks": m.Coverage.Tasks,
		"origin": m.Coverage.Origin, "honey": m.Coverage.Honey,
		"tokens": m.Coverage.Tokens, "durations": m.Coverage.Durations,
	} {
		if got != CoverageComplete {
			t.Fatalf("coverage %s = %q, want complete (warnings: %+v)", label, got, m.Warnings)
		}
	}
	if len(m.Warnings) != 0 {
		t.Fatalf("warnings = %+v, want none", m.Warnings)
	}
}

func TestBuildTrailMetricsMissingSeedKeepsBudgetUnknown(t *testing.T) {
	root := t.TempDir()
	traceID := "trace-no-seed"
	base := time.Now().UTC()

	appendMetricsEvent(t, root, traceID, metricsEvent(t, traceID, "cli", protocol.EventSignal, protocol.EnergyAddPayload{
		Kind: protocol.SignalEnergyAdd, Amount: 5,
	}, base))
	appendMetricsEvent(t, root, traceID, metricsEvent(t, traceID, "runtime", protocol.EventSignal, protocol.EnergyConsumePayload{
		Kind: protocol.SignalEnergyConsume, Amount: 2, Reason: "task.dispatch",
	}, base.Add(time.Minute)))

	m, err := BuildTrailMetrics(root, traceID, base.Add(5*time.Minute))
	if err != nil {
		t.Fatal(err)
	}
	if m.Honey.Budget != nil {
		t.Fatalf("budget = %v, want unknown (a top-up must not replace the seed)", *m.Honey.Budget)
	}
	if m.Honey.Spent == nil || *m.Honey.Spent != 2 {
		t.Fatalf("spent = %v, want 2", m.Honey.Spent)
	}
	if m.Coverage.Honey != CoveragePartial {
		t.Fatalf("honey coverage = %q, want partial", m.Coverage.Honey)
	}
	assertWarning(t, m.Warnings, "honey", WarningMissing)
}

func TestBuildTrailMetricsTokensMissingReportedZeroAndPartial(t *testing.T) {
	base := time.Now().UTC()
	t.Run("no usage is null", func(t *testing.T) {
		root := t.TempDir()
		traceID := "trace-no-usage"
		writeMetricsRun(t, root, traceID, metricsRunSpec{
			agentID: "run-a", bee: "builder", started: base, finished: base.Add(time.Minute),
		})
		m, err := BuildTrailMetrics(root, traceID, base.Add(2*time.Minute))
		if err != nil {
			t.Fatal(err)
		}
		if m.Tokens != nil {
			t.Fatalf("tokens = %+v, want null", m.Tokens)
		}
		if m.Coverage.Tokens != CoverageUnavailable {
			t.Fatalf("tokens coverage = %q, want unavailable", m.Coverage.Tokens)
		}
	})
	t.Run("reported zero is a measurement", func(t *testing.T) {
		root := t.TempDir()
		traceID := "trace-zero-usage"
		writeMetricsRun(t, root, traceID, metricsRunSpec{
			agentID: "run-a", bee: "builder", started: base, finished: base.Add(time.Minute),
			usage: &protocol.Usage{Source: protocol.UsageSourceCursorStreamJSON},
		})
		m, err := BuildTrailMetrics(root, traceID, base.Add(2*time.Minute))
		if err != nil {
			t.Fatal(err)
		}
		if m.Tokens == nil || m.Tokens.RunsWithUsage != 1 || m.Tokens.InputTokens != 0 {
			t.Fatalf("tokens = %+v, want a measured zero", m.Tokens)
		}
		if m.Coverage.Tokens != CoverageComplete {
			t.Fatalf("tokens coverage = %q, want complete", m.Coverage.Tokens)
		}
	})
	t.Run("partial reporting is explicit", func(t *testing.T) {
		root := t.TempDir()
		traceID := "trace-partial-usage"
		writeMetricsRun(t, root, traceID, metricsRunSpec{
			agentID: "run-a", bee: "builder", started: base, finished: base.Add(time.Minute),
			usage: &protocol.Usage{InputTokens: 10},
		})
		writeMetricsRun(t, root, traceID, metricsRunSpec{
			agentID: "run-b", bee: "builder", started: base.Add(2 * time.Minute), finished: base.Add(3 * time.Minute),
		})
		m, err := BuildTrailMetrics(root, traceID, base.Add(4*time.Minute))
		if err != nil {
			t.Fatal(err)
		}
		if m.Tokens == nil || !m.Tokens.Partial || m.Tokens.RunsWithUsage != 1 || m.Tokens.Runs != 2 {
			t.Fatalf("tokens = %+v, want explicit partial reporting", m.Tokens)
		}
		if m.Coverage.Tokens != CoveragePartial {
			t.Fatalf("tokens coverage = %q, want partial", m.Coverage.Tokens)
		}
	})
}

func TestBuildTrailMetricsInteractiveUsageCountedOnce(t *testing.T) {
	root := t.TempDir()
	traceID := "trace-interactive-usage"
	base := time.Now().UTC()

	// Both records describe the same execution: the AFK result wins and the
	// session usage is not added on top.
	writeMetricsRun(t, root, traceID, metricsRunSpec{
		agentID: "run-a", bee: "builder", started: base, finished: base.Add(time.Minute),
		usage:        &protocol.Usage{InputTokens: 100},
		sessionUsage: &protocol.Usage{InputTokens: 999},
	})
	// A session-only execution contributes its recorded usage once.
	writeMetricsRun(t, root, traceID, metricsRunSpec{
		agentID: "run-b", bee: "builder", started: base.Add(2 * time.Minute), finished: base.Add(3 * time.Minute),
		sessionUsage: &protocol.Usage{InputTokens: 50},
	})

	m, err := BuildTrailMetrics(root, traceID, base.Add(4*time.Minute))
	if err != nil {
		t.Fatal(err)
	}
	if m.Tokens == nil || m.Tokens.InputTokens != 150 || m.Tokens.RunsWithUsage != 2 {
		t.Fatalf("tokens = %+v, want 150 across 2 executions", m.Tokens)
	}
}

func TestBuildTrailMetricsDurationsPartialAndUnavailable(t *testing.T) {
	base := time.Now().UTC()
	t.Run("missing timestamp makes only that interval partial", func(t *testing.T) {
		root := t.TempDir()
		traceID := "trace-partial-duration"
		writeMetricsRun(t, root, traceID, metricsRunSpec{
			agentID: "run-a", bee: "builder", started: base, finished: base.Add(time.Minute),
		})
		writeMetricsRun(t, root, traceID, metricsRunSpec{
			agentID: "run-b", bee: "builder", started: time.Time{}, finished: base.Add(2 * time.Minute),
		})
		m, err := BuildTrailMetrics(root, traceID, base.Add(5*time.Minute))
		if err != nil {
			t.Fatal(err)
		}
		if m.ActiveTimeSec == nil || *m.ActiveTimeSec != 60 {
			t.Fatalf("activeTimeSec = %v, want the one usable interval", m.ActiveTimeSec)
		}
		if m.Coverage.Durations != CoveragePartial {
			t.Fatalf("durations coverage = %q, want partial", m.Coverage.Durations)
		}
		assertWarning(t, m.Warnings, "durations", WarningMissing)
	})
	t.Run("no usable interval is null", func(t *testing.T) {
		root := t.TempDir()
		traceID := "trace-no-duration"
		writeMetricsRun(t, root, traceID, metricsRunSpec{
			agentID: "run-a", bee: "builder", started: time.Time{}, finished: time.Time{},
		})
		m, err := BuildTrailMetrics(root, traceID, base.Add(5*time.Minute))
		if err != nil {
			t.Fatal(err)
		}
		if m.ActiveTimeSec != nil {
			t.Fatalf("activeTimeSec = %v, want null", *m.ActiveTimeSec)
		}
		if m.Coverage.Durations != CoverageUnavailable {
			t.Fatalf("durations coverage = %q, want unavailable", m.Coverage.Durations)
		}
	})
}

func TestBuildTrailMetricsStageMatchesTrailDetail(t *testing.T) {
	base := time.Now().UTC()
	cases := []struct {
		name  string
		tasks []taskledger.TaskSnapshot
		want  taskledger.TrailStage
	}{
		{
			name: "work",
			tasks: []taskledger.TaskSnapshot{
				{TaskID: "task-1", Status: protocol.TaskStatusRunning, CreatedAt: base},
			},
			want: taskledger.TrailStageWork,
		},
		{
			name: "closing",
			tasks: []taskledger.TaskSnapshot{
				{TaskID: "task-1", Status: protocol.TaskStatusCompleted, CreatedAt: base},
				{TaskID: "closing-1", Phase: protocol.TaskPhaseClosing, Status: protocol.TaskStatusPlanned, CreatedAt: base.Add(time.Minute)},
			},
			want: taskledger.TrailStageClosing,
		},
		{
			name: "gate",
			tasks: []taskledger.TaskSnapshot{
				{TaskID: "task-1", Status: protocol.TaskStatusCompleted, CreatedAt: base},
				{TaskID: "merge-1", Review: protocol.TaskReviewFinal, Status: protocol.TaskStatusWaitingReview, CreatedAt: base.Add(time.Minute)},
			},
			want: taskledger.TrailStageGate,
		},
		{
			name: "delivered",
			tasks: []taskledger.TaskSnapshot{
				{TaskID: "task-1", Status: protocol.TaskStatusCompleted, CreatedAt: base},
				{TaskID: "merge-1", Review: protocol.TaskReviewFinal, Status: protocol.TaskStatusCompleted, CreatedAt: base.Add(time.Minute)},
			},
			want: taskledger.TrailStageDelivered,
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			root := t.TempDir()
			traceID := "trace-stage-" + tc.name
			snapshot := taskledger.TraceSnapshot{TraceID: traceID, Tasks: map[string]taskledger.TaskSnapshot{}}
			for _, task := range tc.tasks {
				snapshot.Tasks[task.TaskID] = task
				writeMetricsTask(t, root, traceID, task)
			}
			m, err := BuildTrailMetrics(root, traceID, base.Add(10*time.Minute))
			if err != nil {
				t.Fatal(err)
			}
			want := string(taskledger.DeriveTrailStage(snapshot))
			if want != string(tc.want) {
				t.Fatalf("fixture stage = %q, want %q", want, tc.want)
			}
			if m.Work.Stage != want {
				t.Fatalf("metrics stage = %q, trail detail stage = %q", m.Work.Stage, want)
			}
		})
	}
}

func TestBuildTrailMetricsOriginAndAdditionCoverage(t *testing.T) {
	base := time.Now().UTC()
	t.Run("manual origin makes a later cue an addition", func(t *testing.T) {
		root := t.TempDir()
		traceID := "trace-manual-origin"
		writeMetricsOrigin(t, root, traceID, TrailOrigin{Kind: OriginKindManual, Source: "cli"})
		writeMetricsInvocation(t, root, traceID, CueInvocation{
			InvocationID: "cueinv-late", TraceID: traceID, CueID: "cue-late", Mode: "task", InvokedAt: base,
		})
		m, err := BuildTrailMetrics(root, traceID, base.Add(time.Minute))
		if err != nil {
			t.Fatal(err)
		}
		if m.CueID != "" {
			t.Fatalf("cueId = %q, want empty (the trail has no cue origin)", m.CueID)
		}
		if len(m.CueIngresses) != 1 || m.CueIngresses[0].Attribution != CueAttributionAddition {
			t.Fatalf("cueIngresses = %+v, want one addition", m.CueIngresses)
		}
		if m.Coverage.Origin != CoverageComplete {
			t.Fatalf("origin coverage = %q, want complete", m.Coverage.Origin)
		}
	})
	t.Run("missing legacy origin leaves attribution unknown", func(t *testing.T) {
		root := t.TempDir()
		traceID := "trace-legacy-origin"
		writeMetricsInvocation(t, root, traceID, CueInvocation{
			InvocationID: "cueinv-1", TraceID: traceID, CueID: "cue-a", Mode: "task", InvokedAt: base,
		})
		m, err := BuildTrailMetrics(root, traceID, base.Add(time.Minute))
		if err != nil {
			t.Fatal(err)
		}
		if m.CueID != "" {
			t.Fatalf("cueId = %q, want empty", m.CueID)
		}
		if len(m.CueIngresses) != 1 || m.CueIngresses[0].Attribution != CueAttributionUnknown {
			t.Fatalf("cueIngresses = %+v, want unknown attribution", m.CueIngresses)
		}
		if m.Coverage.Origin != CoveragePartial {
			t.Fatalf("origin coverage = %q, want partial", m.Coverage.Origin)
		}
		assertWarning(t, m.Warnings, "origin", WarningLegacy)
	})
	t.Run("origin survives a missing invocation record", func(t *testing.T) {
		root := t.TempDir()
		traceID := "trace-origin-no-record"
		writeMetricsOrigin(t, root, traceID, TrailOrigin{
			Kind: OriginKindCue, Source: "cli", InvocationID: "cueinv-gone", CueID: "cue-a",
		})
		m, err := BuildTrailMetrics(root, traceID, base.Add(time.Minute))
		if err != nil {
			t.Fatal(err)
		}
		if m.CueID != "cue-a" {
			t.Fatalf("cueId = %q, want cue-a", m.CueID)
		}
		if len(m.CueIngresses) != 1 || m.CueIngresses[0].Attribution != CueAttributionOrigin {
			t.Fatalf("cueIngresses = %+v, want the retained origin", m.CueIngresses)
		}
		if m.Coverage.Origin != CoveragePartial {
			t.Fatalf("origin coverage = %q, want partial", m.Coverage.Origin)
		}
		assertWarning(t, m.Warnings, "origin", WarningMissing)
	})
}

func TestBuildTrailMetricsMissingTrailIsError(t *testing.T) {
	_, err := BuildTrailMetrics(t.TempDir(), "trace-absent", time.Now().UTC())
	if !errors.Is(err, ErrTrailNotFound) {
		t.Fatalf("err = %v, want ErrTrailNotFound", err)
	}
}

func TestBuildTrailMetricsDamagedEvidenceKeepsRetainedValues(t *testing.T) {
	root := t.TempDir()
	traceID := "trace-damaged"
	base := time.Now().UTC()

	appendMetricsEvent(t, root, traceID, metricsEvent(t, traceID, "runtime", protocol.EventSignal, protocol.EnergySeededPayload{
		Kind: protocol.SignalEnergySeeded, SeedID: "seed-1", Budget: 9, SeededAt: base,
	}, base))
	appendMetricsEvent(t, root, traceID, metricsEvent(t, traceID, "runtime", protocol.EventSignal, protocol.EnergyConsumePayload{
		Kind: protocol.SignalEnergyConsume, Amount: 4, Reason: "task.dispatch",
	}, base.Add(time.Minute)))
	// A torn append leaves half a line behind; the complete history survives.
	appendRaw(t, Trail{ColonyRoot: root, TraceID: traceID}.EventsPath(), `{"id":"evt-torn`)

	writeMetricsTask(t, root, traceID, taskledger.TaskSnapshot{
		TaskID: "task-1", Bee: "builder", Status: protocol.TaskStatusCompleted, CreatedAt: base,
	})
	broken := TaskDir{ColonyRoot: root, TraceID: traceID, TaskID: "task-2"}
	if err := os.MkdirAll(broken.Root(), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(broken.TaskPath(), []byte("---\ntaskId: [unterminated\n---\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	writeMetricsRun(t, root, traceID, metricsRunSpec{
		agentID: "run-a", bee: "builder", started: base, finished: base.Add(time.Minute),
	})
	// Make the outcome pair disagree: the retained counts must not report the
	// friendlier completed value as a proven outcome.
	d := Dir{ColonyRoot: root, TraceID: traceID, AgentID: "run-a"}
	if err := d.WriteResult(protocol.Result{
		ProtocolVersion: protocol.Version, TraceID: traceID, AgentID: "run-a",
		Status: protocol.StatusFailed, Summary: "adapter failed", FinishedAt: base.Add(time.Minute),
	}); err != nil {
		t.Fatal(err)
	}

	m, err := BuildTrailMetrics(root, traceID, base.Add(5*time.Minute))
	if err != nil {
		t.Fatal(err)
	}
	if m.Honey.Budget == nil || *m.Honey.Budget != 9 || m.Honey.Spent == nil || *m.Honey.Spent != 4 {
		t.Fatalf("honey = %+v, want retained seed and charge", m.Honey)
	}
	if m.Runs.Retained != 1 || m.Runs.Failed != 1 {
		t.Fatalf("runs = %+v, want the retained conflicting run counted as failed", m.Runs)
	}
	if m.Class == nil || m.Class.TaskID != "task-1" {
		t.Fatalf("class = %+v, want the readable task", m.Class)
	}
	if m.Coverage.Events != CoveragePartial || m.Coverage.Tasks != CoveragePartial || m.Coverage.Runs != CoveragePartial {
		t.Fatalf("coverage = %+v, want partial events/tasks/runs", m.Coverage)
	}
	assertWarning(t, m.Warnings, "events", WarningUnreadable)
	assertWarning(t, m.Warnings, "tasks", WarningUnreadable)
	assertWarning(t, m.Warnings, "runs", WarningConflicting)
}

func TestBuildTrailMetricsOverwrittenRunEvidence(t *testing.T) {
	root := t.TempDir()
	traceID := "trace-overwritten"
	base := time.Now().UTC()

	writeMetricsRun(t, root, traceID, metricsRunSpec{
		agentID: "run-a", bee: "builder", taskID: "task-1",
		started: base, finished: base.Add(time.Minute),
	})
	writeMetricsTask(t, root, traceID, taskledger.TaskSnapshot{
		TaskID: "task-1", Bee: "builder", Status: protocol.TaskStatusCompleted, CreatedAt: base,
	})
	taskDir := TaskDir{ColonyRoot: root, TraceID: traceID, TaskID: "task-1"}
	for _, entry := range []TaskRunEntry{
		{AgentID: "run-a", Bee: "builder", AttemptID: "attempt-1", StartedAt: base},
		{AgentID: "run-a", Bee: "builder", AttemptID: "attempt-2", StartedAt: base.Add(time.Hour)},
	} {
		if err := taskDir.AppendTaskRun(entry); err != nil {
			t.Fatal(err)
		}
	}

	m, err := BuildTrailMetrics(root, traceID, base.Add(2*time.Hour))
	if err != nil {
		t.Fatal(err)
	}
	if m.Runs.Retained != 1 {
		t.Fatalf("retained = %d, want one retained execution directory", m.Runs.Retained)
	}
	assertWarning(t, m.Warnings, "runs", WarningConflicting)
	if m.Coverage.Runs != CoveragePartial {
		t.Fatalf("runs coverage = %q, want partial", m.Coverage.Runs)
	}
}

func TestBuildTrailMetricsExcludesRecordDirectories(t *testing.T) {
	root := t.TempDir()
	traceID := "trace-record-dirs"
	base := time.Now().UTC()

	writeMetricsRun(t, root, traceID, metricsRunSpec{
		agentID: "run-a", bee: "builder", started: base, finished: base.Add(time.Minute),
	})
	for _, name := range []string{"tasks", "artifacts", "proposals", "cue-invocations", "config", "fork-requests"} {
		dir := filepath.Join(root, ".paseka", "runs", traceID, name)
		if err := os.MkdirAll(dir, 0o755); err != nil {
			t.Fatal(err)
		}
		// A request record inside a reserved directory must not become an
		// execution count.
		if err := os.WriteFile(filepath.Join(dir, RequestFileName), []byte("{}"), 0o644); err != nil {
			t.Fatal(err)
		}
	}

	m, err := BuildTrailMetrics(root, traceID, base.Add(2*time.Minute))
	if err != nil {
		t.Fatal(err)
	}
	if m.Runs.Retained != 1 {
		t.Fatalf("retained = %d, want reserved record directories excluded", m.Runs.Retained)
	}
}

func TestBuildTrailMetricsWorktreeCleanupLeavesReadAvailable(t *testing.T) {
	root := t.TempDir()
	traceID := "trace-worktree-cleanup"
	base := time.Now().UTC()

	writeMetricsRun(t, root, traceID, metricsRunSpec{
		agentID: "run-a", bee: "builder", started: base, finished: base.Add(time.Minute),
		usage: &protocol.Usage{InputTokens: 7},
	})
	worktree := filepath.Join(root, ".paseka", "worktrees", traceID)
	if err := os.MkdirAll(worktree, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(worktree, "README"), []byte("checkout"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.RemoveAll(worktree); err != nil {
		t.Fatal(err)
	}

	m, err := BuildTrailMetrics(root, traceID, base.Add(2*time.Minute))
	if err != nil {
		t.Fatalf("worktree-only cleanup must leave the metrics read intact: %v", err)
	}
	if m.Runs.Retained != 1 || m.Tokens == nil || m.Tokens.InputTokens != 7 {
		t.Fatalf("metrics = %+v, want the retained run and usage", m)
	}
}

func TestWriteTrailMetricsPersistsTheSameProjection(t *testing.T) {
	root := t.TempDir()
	traceID := "trace-write-metrics"
	base := time.Now().UTC()

	writeMetricsRun(t, root, traceID, metricsRunSpec{
		agentID: "run-a", bee: "builder", started: base, finished: base.Add(time.Minute),
	})
	observedAt := base.Add(5 * time.Minute)
	m, err := BuildTrailMetrics(root, traceID, observedAt)
	if err != nil {
		t.Fatal(err)
	}
	if err := WriteTrailMetrics(root, traceID, m); err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(TrailMetricsPath(root, traceID))
	if err != nil {
		t.Fatal(err)
	}
	var persisted TrailMetrics
	if err := json.Unmarshal(data, &persisted); err != nil {
		t.Fatalf("persisted metrics are not valid JSON: %v\n%s", err, data)
	}
	if !persisted.ObservedAt.Equal(observedAt) || persisted.TraceID != traceID {
		t.Fatalf("persisted identity = %+v, want %s at %v", persisted, traceID, observedAt)
	}
	// metrics.json is a file, not a run directory: it must not change counts.
	again, err := BuildTrailMetrics(root, traceID, observedAt)
	if err != nil {
		t.Fatal(err)
	}
	if again.Runs.Retained != 1 {
		t.Fatalf("retained = %d after writing metrics.json", again.Runs.Retained)
	}
}

func TestBuildTrailMetricsWorkPauseAndPending(t *testing.T) {
	root := t.TempDir()
	traceID := "trace-pause"
	base := time.Now().UTC()

	writeMetricsTask(t, root, traceID, taskledger.TaskSnapshot{
		TaskID: "task-1", Bee: "builder", Status: protocol.TaskStatusReady, CreatedAt: base,
	})
	appendMetricsEvent(t, root, traceID, metricsEvent(t, traceID, "runtime", protocol.EventSignal, protocol.EnergySeededPayload{
		Kind: protocol.SignalEnergySeeded, SeedID: "seed-1", Budget: 1, SeededAt: base,
	}, base))
	paused := metricsEvent(t, traceID, "runtime", protocol.EventSignal, protocol.EnergyPausedPayload{
		Kind: protocol.SignalEnergyPaused, EpisodeID: "pause-1", Reason: "Honey reserve exhausted",
		Remaining: 0, PendingCount: 2,
	}, base.Add(time.Minute))
	appendMetricsEvent(t, root, traceID, paused)

	m, err := BuildTrailMetrics(root, traceID, base.Add(2*time.Minute))
	if err != nil {
		t.Fatal(err)
	}
	if m.Work.Pause == nil || m.Work.Pause.EpisodeID != "pause-1" || m.Work.Pause.PendingCount != 2 {
		t.Fatalf("pause = %+v, want the open episode", m.Work.Pause)
	}
	if m.Work.PendingCount != 2 || len(m.Work.PendingReasons) != 1 ||
		m.Work.PendingReasons[0].Reason != "Honey reserve exhausted" ||
		m.Work.PendingReasons[0].Count != 2 {
		t.Fatalf("pending = %+v/%+v, want the pause's reasons", m.Work.PendingCount, m.Work.PendingReasons)
	}
	if m.Work.State == nil || *m.Work.State != "blocked" {
		t.Fatalf("state = %v, want blocked while work is energy-deferred", m.Work.State)
	}

	appendMetricsEvent(t, root, traceID, metricsEvent(t, traceID, "runtime", protocol.EventSignal, protocol.EnergyResumedPayload{
		Kind: protocol.SignalEnergyResumed, EpisodeID: "pause-1", Source: protocol.EnergyResumeSourceAdd,
		Remaining: 1, PendingBefore: 2, PendingAfter: 2, Dispatched: 0,
	}, base.Add(3*time.Minute)))
	m, err = BuildTrailMetrics(root, traceID, base.Add(4*time.Minute))
	if err != nil {
		t.Fatal(err)
	}
	if m.Work.Pause != nil || m.Work.PendingCount != 0 {
		t.Fatalf("pause = %+v, want the episode closed", m.Work.Pause)
	}
	if m.Work.State == nil || *m.Work.State != "open" {
		t.Fatalf("state = %v, want open after the pause closed", m.Work.State)
	}
}

func TestBuildTrailMetricsRunDimensions(t *testing.T) {
	root := t.TempDir()
	traceID := "trace-run-dimensions"
	base := time.Now().UTC()

	writeMetricsRun(t, root, traceID, metricsRunSpec{
		agentID: "run-timeout", bee: "builder", started: base, finished: base.Add(time.Minute),
		state: protocol.StatusCancelled, reason: protocol.ReasonTimeout,
		gate:     &protocol.GateResult{Passed: false, Reason: "exit"},
		contract: &protocol.ContractRecord{Status: protocol.ContractFailed},
	})
	writeMetricsRun(t, root, traceID, metricsRunSpec{
		agentID: "run-checks", bee: "builder", started: base.Add(2 * time.Minute), finished: base.Add(3 * time.Minute),
		state: protocol.StatusFailed, reason: protocol.ReasonChecksFailed,
		gate:     &protocol.GateResult{Passed: false, Reason: "exit"},
		contract: &protocol.ContractRecord{Status: protocol.ContractUnavailable},
	})
	writeMetricsRun(t, root, traceID, metricsRunSpec{
		agentID: "run-pass", bee: "reviewer", started: base.Add(4 * time.Minute), finished: base.Add(5 * time.Minute),
		gate:     &protocol.GateResult{Passed: true},
		contract: &protocol.ContractRecord{Status: protocol.ContractPassed},
	})

	m, err := BuildTrailMetrics(root, traceID, base.Add(10*time.Minute))
	if err != nil {
		t.Fatal(err)
	}
	if m.Runs.Retained != 3 || m.Runs.ByBee["builder"] != 2 || m.Runs.ByBee["reviewer"] != 1 {
		t.Fatalf("runs = %+v", m.Runs)
	}
	if m.Runs.Failed != 1 || m.Runs.Cancelled != 1 {
		t.Fatalf("primary outcomes = %+v, want one failed and one cancelled", m.Runs)
	}
	if m.Runs.Timeout != 1 || m.Runs.ChecksFailed != 2 || m.Runs.ChecksPassed != 1 {
		t.Fatalf("gate dimensions = %+v", m.Runs)
	}
	if m.Runs.ContractFailed != 1 || m.Runs.ContractUnavailable != 1 {
		t.Fatalf("contract dimensions = %+v", m.Runs)
	}
}
func assertWarning(t *testing.T, warnings []MetricsWarning, group, kind string) {
	t.Helper()
	for _, warning := range warnings {
		if warning.Group == group && warning.Kind == kind {
			return
		}
	}
	t.Fatalf("missing warning %s/%s in %+v", group, kind, warnings)
}
