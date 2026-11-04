package runs

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"reflect"
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
	agentID        string
	bee            string
	taskID         string
	started        time.Time
	finished       time.Time
	state          protocol.RunStatus
	reason         string
	purpose        protocol.ExecutionPurpose
	analysisOf     string
	legacyPurpose  bool
	configRef      string
	configRevision int
	usage          *protocol.Usage
	sessionUsage   *protocol.Usage
	gate           *protocol.GateResult
	contract       *protocol.ContractRecord
}

func writeMetricsRun(t *testing.T, root, traceID string, spec metricsRunSpec) {
	t.Helper()
	if spec.state == "" {
		spec.state = protocol.StatusCompleted
	}
	purpose := spec.purpose
	if purpose == "" && !spec.legacyPurpose {
		purpose = protocol.PurposeWork
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
		Purpose:         purpose,
		AnalysisOf:      spec.analysisOf,
		CreatedAt:       spec.started,
		ConfigRef:       spec.configRef,
		ConfigRevision:  spec.configRevision,
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

// writeMetricsManifest writes the retained spec 042 configuration binding. The
// fixture is raw JSON so the test pins the on-disk contract the metrics reader
// depends on, not the reader's own struct tags.
func writeMetricsManifest(t *testing.T, root, traceID string, manifest map[string]any) {
	t.Helper()
	path := metricsConfigManifestPath(root, traceID)
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if _, ok := manifest["version"]; !ok {
		manifest["version"] = 1
	}
	if _, ok := manifest["traceId"]; !ok {
		manifest["traceId"] = traceID
	}
	data, err := json.MarshalIndent(manifest, "", "  ")
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, append(data, '\n'), 0o644); err != nil {
		t.Fatal(err)
	}
}

// writeMetricsForkRecord writes the retained spec 042 fork lineage record.
func writeMetricsForkRecord(t *testing.T, root, traceID string, record map[string]any) {
	t.Helper()
	path := metricsForkLineagePath(root, traceID)
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	data, err := json.MarshalIndent(record, "", "  ")
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, append(data, '\n'), 0o644); err != nil {
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
	writeMetricsManifest(t, root, traceID, map[string]any{
		"initialConfigRef": "sha256:cfg-a",
		"activeConfigRef":  "sha256:cfg-a",
		"configRevision":   1,
		"policy":           "editable",
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
		configRef: "sha256:cfg-a", configRevision: 1,
		usage: &protocol.Usage{InputTokens: 100, OutputTokens: 20, CacheReadTokens: 5},
	})
	writeMetricsRun(t, root, traceID, metricsRunSpec{
		agentID: "run-b", bee: "builder", taskID: "task-2",
		started: base.Add(90 * time.Second), finished: base.Add(3 * time.Minute),
		configRef: "sha256:cfg-a", configRevision: 1,
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
		"purpose": m.Coverage.Purpose, "lineage": m.Coverage.Lineage,
		"configuration": m.Coverage.Configuration,
	} {
		if label == "lineage" {
			// This trail is known ordinary: its origin is a cue, not a fork.
			if got != CoverageAbsent {
				t.Fatalf("coverage %s = %q, want absent", label, got)
			}
			continue
		}
		if got != CoverageComplete {
			t.Fatalf("coverage %s = %q, want complete (warnings: %+v)", label, got, m.Warnings)
		}
	}
	if m.ForkOf != nil {
		t.Fatalf("forkOf = %+v, want null for an ordinary trail", m.ForkOf)
	}
	if m.Configuration == nil || m.Configuration.MultipleVersions {
		t.Fatalf("configuration = %+v, want one retained version", m.Configuration)
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

func findMetricsTask(t *testing.T, m TrailMetrics, taskID string) MetricsTask {
	t.Helper()
	for _, task := range m.Tasks {
		if task.TaskID == taskID {
			return task
		}
	}
	t.Fatalf("task %s not found in %+v", taskID, m.Tasks)
	return MetricsTask{}
}

func assertCount(t *testing.T, label string, got *int, want int) {
	t.Helper()
	if got == nil {
		t.Fatalf("%s = nil, want %d", label, want)
	}
	if *got != want {
		t.Fatalf("%s = %d, want %d", label, *got, want)
	}
}

func TestBuildTrailMetricsTaskExecutionsIgnoreAssociatedRoles(t *testing.T) {
	base := time.Now().UTC()
	root := t.TempDir()
	traceID := "trace-rework-roles"

	writeMetricsTask(t, root, traceID, taskledger.TaskSnapshot{
		TaskID: "task-1", Bee: "builder", Intent: "feature",
		Status: protocol.TaskStatusCompleted, CreatedAt: base,
	})
	writeMetricsRun(t, root, traceID, metricsRunSpec{
		agentID: "run-work-1", bee: "builder", taskID: "task-1",
		started: base, finished: base.Add(time.Minute),
	})
	writeMetricsRun(t, root, traceID, metricsRunSpec{
		agentID: "run-guard", bee: "guard", taskID: "task-1",
		started: base.Add(time.Minute), finished: base.Add(2 * time.Minute),
	})

	m, err := BuildTrailMetrics(root, traceID, base.Add(5*time.Minute))
	if err != nil {
		t.Fatal(err)
	}
	task := findMetricsTask(t, m, "task-1")
	if len(task.Runs) != 2 {
		t.Fatalf("joined runs = %+v, want the work and the Guard execution", task.Runs)
	}
	// The Guard execution stays a joined run, but it is not a retry.
	assertCount(t, "executions", task.Executions, 1)
	assertCount(t, "retries", task.Retries, 0)

	// A second execution of the assigned work role is one retry.
	writeMetricsRun(t, root, traceID, metricsRunSpec{
		agentID: "run-work-2", bee: "builder", taskID: "task-1",
		started: base.Add(2 * time.Minute), finished: base.Add(3 * time.Minute),
	})
	m, err = BuildTrailMetrics(root, traceID, base.Add(6*time.Minute))
	if err != nil {
		t.Fatal(err)
	}
	task = findMetricsTask(t, m, "task-1")
	assertCount(t, "executions", task.Executions, 2)
	assertCount(t, "retries", task.Retries, 1)
}

func TestBuildTrailMetricsTaskExecutionsCustomRoleAndDirectRework(t *testing.T) {
	base := time.Now().UTC()
	root := t.TempDir()
	traceID := "trace-custom-role"

	writeMetricsTask(t, root, traceID, taskledger.TaskSnapshot{
		TaskID: "task-1", Bee: "mason", Status: protocol.TaskStatusCompleted, CreatedAt: base,
	})
	// The task dispatch records its own execution-time role, even when it is a
	// custom name no registry would recognise today.
	taskDir := TaskDir{ColonyRoot: root, TraceID: traceID, TaskID: "task-1"}
	if err := taskDir.AppendTaskRun(TaskRunEntry{
		AgentID: "run-mason-1", Bee: "mason", AttemptID: "attempt-1", StartedAt: base,
	}); err != nil {
		t.Fatal(err)
	}
	writeMetricsRun(t, root, traceID, metricsRunSpec{
		agentID: "run-mason-1", bee: "mason", taskID: "task-1",
		started: base, finished: base.Add(time.Minute),
	})
	// A direct-dispatch rework that performs the same assigned work role counts.
	writeMetricsRun(t, root, traceID, metricsRunSpec{
		agentID: "run-mason-rework", bee: "mason", taskID: "task-1",
		started: base.Add(time.Minute), finished: base.Add(2 * time.Minute),
	})

	m, err := BuildTrailMetrics(root, traceID, base.Add(5*time.Minute))
	if err != nil {
		t.Fatal(err)
	}
	task := findMetricsTask(t, m, "task-1")
	if task.Phase != string(protocol.TaskPhaseWork) {
		t.Fatalf("phase = %q, want work", task.Phase)
	}
	assertCount(t, "executions", task.Executions, 2)
	assertCount(t, "retries", task.Retries, 1)
}

func TestBuildTrailMetricsTaskExecutionsUseRecordedDispatchOwnership(t *testing.T) {
	base := time.Now().UTC()
	root := t.TempDir()
	traceID := "trace-renamed-role"

	// The task's recorded role was renamed after dispatch. The run is still the
	// task's work execution because its dispatch entry carries the role used at
	// execution time, not because today's record says "builder".
	writeMetricsTask(t, root, traceID, taskledger.TaskSnapshot{
		TaskID: "task-1", Bee: "builder", Status: protocol.TaskStatusCompleted, CreatedAt: base,
	})
	taskDir := TaskDir{ColonyRoot: root, TraceID: traceID, TaskID: "task-1"}
	if err := taskDir.AppendTaskRun(TaskRunEntry{
		AgentID: "run-1", Bee: "builder-v2", AttemptID: "attempt-1", StartedAt: base,
	}); err != nil {
		t.Fatal(err)
	}
	writeMetricsRun(t, root, traceID, metricsRunSpec{
		agentID: "run-1", bee: "builder-v2", taskID: "task-1",
		started: base, finished: base.Add(time.Minute),
	})

	m, err := BuildTrailMetrics(root, traceID, base.Add(5*time.Minute))
	if err != nil {
		t.Fatal(err)
	}
	task := findMetricsTask(t, m, "task-1")
	assertCount(t, "executions", task.Executions, 1)
	assertCount(t, "retries", task.Retries, 0)
}

func TestBuildTrailMetricsMissingOwnershipIsCoverageGap(t *testing.T) {
	base := time.Now().UTC()
	root := t.TempDir()
	traceID := "trace-missing-ownership"

	// A legacy task.md carries no assigned role, and only the run record joins
	// the execution. The reader cannot prove the run performed the assignment.
	writeMetricsTask(t, root, traceID, taskledger.TaskSnapshot{
		TaskID: "task-1", Status: protocol.TaskStatusCompleted, CreatedAt: base,
	})
	writeMetricsRun(t, root, traceID, metricsRunSpec{
		agentID: "run-1", bee: "builder", taskID: "task-1",
		started: base, finished: base.Add(time.Minute),
	})

	m, err := BuildTrailMetrics(root, traceID, base.Add(5*time.Minute))
	if err != nil {
		t.Fatal(err)
	}
	task := findMetricsTask(t, m, "task-1")
	if len(task.Runs) != 1 {
		t.Fatalf("joined runs = %+v, want the retained execution", task.Runs)
	}
	if task.Executions != nil || task.Retries != nil {
		t.Fatalf("executions/retries = %v/%v, want unknown rather than a guess", task.Executions, task.Retries)
	}
	if m.Coverage.Tasks != CoveragePartial {
		t.Fatalf("tasks coverage = %q, want partial", m.Coverage.Tasks)
	}
	assertWarning(t, m.Warnings, "tasks", WarningMissing)
}

func TestBuildTrailMetricsVerificationAndGateRejections(t *testing.T) {
	base := time.Now().UTC()
	root := t.TempDir()
	traceID := "trace-gate-rework"

	writeMetricsTask(t, root, traceID, taskledger.TaskSnapshot{
		TaskID: "task-1", Bee: "builder", Status: protocol.TaskStatusCompleted, CreatedAt: base,
	})
	// The rejected run records its mechanical gate verdict and the runtime
	// records the matching verification.failed.
	writeMetricsRun(t, root, traceID, metricsRunSpec{
		agentID: "run-check", bee: "builder", taskID: "task-1",
		started: base, finished: base.Add(time.Minute),
		state: protocol.StatusFailed, reason: protocol.ReasonChecksFailed,
		gate: &protocol.GateResult{Passed: false, Reason: "exit"},
	})
	appendMetricsEvent(t, root, traceID, metricsEvent(t, traceID, protocol.RuntimeAgentID, protocol.EventVerification,
		protocol.VerificationPayload{
			Kind: protocol.VerificationFailed, TaskID: "task-1", Reason: protocol.ReasonChecksFailed,
			Summary: "checks failed",
		}, base.Add(time.Minute)))
	// The builder reworks: one execution of the assigned work role more.
	writeMetricsRun(t, root, traceID, metricsRunSpec{
		agentID: "run-rework", bee: "builder", taskID: "task-1",
		started: base.Add(2 * time.Minute), finished: base.Add(3 * time.Minute),
	})

	// A verification failure without another execution does not invent a retry.
	writeMetricsTask(t, root, traceID, taskledger.TaskSnapshot{
		TaskID: "task-2", Bee: "reviewer", Status: protocol.TaskStatusFailed, CreatedAt: base.Add(time.Second),
	})
	writeMetricsRun(t, root, traceID, metricsRunSpec{
		agentID: "run-review", bee: "reviewer", taskID: "task-2",
		started: base, finished: base.Add(time.Minute),
	})
	appendMetricsEvent(t, root, traceID, metricsEvent(t, traceID, "reviewer-1", protocol.EventVerification,
		protocol.VerificationPayload{Kind: protocol.VerificationFailed, TaskID: "task-2", Summary: "please revise"},
		base.Add(time.Minute)))

	// A taskless verification stays in the trail total.
	appendMetricsEvent(t, root, traceID, metricsEvent(t, traceID, "cli", protocol.EventVerification,
		protocol.VerificationPayload{Kind: protocol.VerificationFailed, Summary: "unjoined failure"},
		base.Add(4*time.Minute)))

	m, err := BuildTrailMetrics(root, traceID, base.Add(10*time.Minute))
	if err != nil {
		t.Fatal(err)
	}
	task := findMetricsTask(t, m, "task-1")
	assertCount(t, "executions", task.Executions, 2)
	assertCount(t, "retries", task.Retries, 1)
	if task.VerificationFailed != 1 || task.GateRejections != 1 {
		t.Fatalf("task-1 verification/gate = %d/%d, want 1/1", task.VerificationFailed, task.GateRejections)
	}
	// The run's failed gate and its matching event are one rejection, not two.
	if m.GateRejections != 1 {
		t.Fatalf("trail gate rejections = %d, want the one recorded rejection", m.GateRejections)
	}

	review := findMetricsTask(t, m, "task-2")
	assertCount(t, "review executions", review.Executions, 1)
	assertCount(t, "review retries", review.Retries, 0)
	if review.VerificationFailed != 1 {
		t.Fatalf("task-2 verification = %d, want 1", review.VerificationFailed)
	}

	if m.VerificationFailed != 3 {
		t.Fatalf("trail verification = %d, want all three distinct events", m.VerificationFailed)
	}
	perTask := 0
	for _, row := range m.Tasks {
		perTask += row.VerificationFailed
	}
	if perTask >= m.VerificationFailed {
		t.Fatalf("per-task sum = %d, trail total = %d: the taskless failure must stay only in the total",
			perTask, m.VerificationFailed)
	}
}

func TestBuildTrailMetricsSkippedAndDeferredTasks(t *testing.T) {
	base := time.Now().UTC()
	root := t.TempDir()
	traceID := "trace-skipped-deferred"

	// A superseded standing tick keeps a retained execution record but must not
	// count as work, an execution, a retry, or a completion.
	writeMetricsTask(t, root, traceID, taskledger.TaskSnapshot{
		TaskID: "task-skipped", Bee: "builder", Status: protocol.TaskStatusSkipped,
		CreatedAt: base, Summary: protocol.SupersededByStipendSummary(4),
	})
	writeMetricsRun(t, root, traceID, metricsRunSpec{
		agentID: "run-old-tick", bee: "builder", taskID: "task-skipped",
		started: base, finished: base.Add(time.Minute),
	})

	// Energy-deferred work keeps its lifecycle status while the trail cannot
	// admit it. It has not started.
	writeMetricsTask(t, root, traceID, taskledger.TaskSnapshot{
		TaskID: "task-deferred", Bee: "builder", Status: protocol.TaskStatusReady,
		CreatedAt: base.Add(time.Second),
	})
	// A deferred rework may leave the lifecycle status looking active. Without
	// an execution the deferred entry still means it did not start.
	writeMetricsTask(t, root, traceID, taskledger.TaskSnapshot{
		TaskID: "task-deferred-active", Bee: "builder", Status: protocol.TaskStatusRunning,
		CreatedAt: base.Add(2 * time.Second),
	})
	appendMetricsEvent(t, root, traceID, metricsEvent(t, traceID, protocol.RuntimeAgentID, protocol.EventSignal,
		protocol.EnergyPausedPayload{
			Kind: protocol.SignalEnergyPaused, EpisodeID: "pause-1", Reason: "Honey reserve exhausted",
			Remaining: 0, PendingCount: 1,
		}, base.Add(time.Minute)))

	m, err := BuildTrailMetrics(root, traceID, base.Add(5*time.Minute))
	if err != nil {
		t.Fatal(err)
	}
	skipped := findMetricsTask(t, m, "task-skipped")
	assertCount(t, "skipped executions", skipped.Executions, 0)
	assertCount(t, "skipped retries", skipped.Retries, 0)
	if skipped.Started || skipped.Completed {
		t.Fatalf("skipped task started/completed = %t/%t, want false/false", skipped.Started, skipped.Completed)
	}
	deferred := findMetricsTask(t, m, "task-deferred")
	assertCount(t, "deferred executions", deferred.Executions, 0)
	assertCount(t, "deferred retries", deferred.Retries, 0)
	if deferred.Started || deferred.Completed {
		t.Fatalf("deferred task started/completed = %t/%t, want false/false", deferred.Started, deferred.Completed)
	}
	deferredActive := findMetricsTask(t, m, "task-deferred-active")
	assertCount(t, "deferred active executions", deferredActive.Executions, 0)
	if deferredActive.Started {
		t.Fatalf("energy-deferred task with status %q counted as started", deferredActive.Status)
	}
	if m.Work.Pause == nil || m.Work.Pause.PendingCount != 1 {
		t.Fatalf("pause = %+v, want the open energy-deferred episode", m.Work.Pause)
	}
}

func TestBuildTrailMetricsReviewNotesAndHumanFeedbackBuckets(t *testing.T) {
	base := time.Now().UTC()
	root := t.TempDir()
	traceID := "trace-review-buckets"

	for i, severity := range []string{"info", "low", "medium", "high", "critical", "", "severe"} {
		appendMetricsEvent(t, root, traceID, metricsEvent(t, traceID, "reviewer", protocol.EventInsight,
			protocol.NarrativeInsightPayload{Kind: protocol.InsightReviewNote, Summary: "note", Severity: severity},
			base.Add(time.Duration(i)*time.Second)))
	}
	for i, verdict := range []string{"negative", "positive", "neutral", "", "blocking"} {
		appendMetricsEvent(t, root, traceID, metricsEvent(t, traceID, "beekeeper", protocol.EventInsight,
			protocol.HumanFeedbackPayload{Kind: protocol.InsightHumanFeedback, TaskID: "task-1", Message: "feedback", Verdict: verdict},
			base.Add(time.Duration(i)*time.Second)))
	}

	m, err := BuildTrailMetrics(root, traceID, base.Add(time.Minute))
	if err != nil {
		t.Fatal(err)
	}
	wantNotes := MetricsReviewNotes{Info: 1, Low: 1, Medium: 1, High: 1, Critical: 1, Unspecified: 1, Unknown: 1}
	if m.ReviewNotes != wantNotes {
		t.Fatalf("review notes = %+v, want %+v", m.ReviewNotes, wantNotes)
	}
	wantFeedback := MetricsHumanFeedback{Negative: 1, Positive: 1, Neutral: 1, Unspecified: 1, Unknown: 1}
	if m.HumanFeedback != wantFeedback {
		t.Fatalf("human feedback = %+v, want %+v", m.HumanFeedback, wantFeedback)
	}
}

func TestBuildTrailMetricsTasksAreStableTaskIDOrdered(t *testing.T) {
	base := time.Now().UTC()
	root := t.TempDir()
	traceID := "trace-task-order"

	for _, taskID := range []string{"task-3", "task-1", "task-2"} {
		writeMetricsTask(t, root, traceID, taskledger.TaskSnapshot{
			TaskID: taskID, Bee: "builder", Status: protocol.TaskStatusCompleted, CreatedAt: base,
		})
	}
	m, err := BuildTrailMetrics(root, traceID, base.Add(time.Minute))
	if err != nil {
		t.Fatal(err)
	}
	got := make([]string, 0, len(m.Tasks))
	for _, task := range m.Tasks {
		got = append(got, task.TaskID)
	}
	want := []string{"task-1", "task-2", "task-3"}
	if len(got) != len(want) {
		t.Fatalf("tasks = %+v, want %+v", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("tasks = %+v, want stable task-id order %+v", got, want)
		}
	}
}

func TestBuildTrailMetricsSplitsObservedActivityByPurpose(t *testing.T) {
	root := t.TempDir()
	traceID := "trace-by-purpose"
	base := time.Now().UTC().Truncate(time.Second).Add(-time.Hour)
	observedAt := base.Add(10 * time.Minute)

	writeMetricsTask(t, root, traceID, taskledger.TaskSnapshot{
		TaskID: "task-work", Bee: "builder", Status: protocol.TaskStatusCompleted, CreatedAt: base.Add(time.Second),
	})
	writeMetricsTask(t, root, traceID, taskledger.TaskSnapshot{
		TaskID: "task-closing", Bee: "reviewer", Phase: protocol.TaskPhaseClosing,
		Status: protocol.TaskStatusCompleted, CreatedAt: base.Add(2 * time.Second),
	})

	appendMetricsEvent(t, root, traceID, metricsEvent(t, traceID, "runtime", protocol.EventSignal, protocol.EnergySeededPayload{
		Kind: protocol.SignalEnergySeeded, SeedID: "seed-1", Budget: 20, SeededAt: base,
	}, base))
	charge := func(amount int, taskID string, purpose protocol.ExecutionPurpose, analysisOf string, at time.Time) {
		appendMetricsEvent(t, root, traceID, metricsEvent(t, traceID, "runtime", protocol.EventSignal, protocol.EnergyConsumePayload{
			Kind: protocol.SignalEnergyConsume, Amount: amount, Reason: "task.dispatch",
			TaskID: taskID, Purpose: purpose, AnalysisOf: analysisOf,
		}, at))
	}
	charge(2, "task-work", protocol.PurposeWork, "", base.Add(10*time.Second))
	charge(1, "task-closing", protocol.PurposeClosing, "", base.Add(70*time.Second))
	charge(1, "", protocol.PurposeAnalysis, "completion-r1", base.Add(110*time.Second))
	charge(1, "", "", "", base.Add(111*time.Second))

	writeMetricsRun(t, root, traceID, metricsRunSpec{
		agentID: "run-work-1", bee: "builder", taskID: "task-work", purpose: protocol.PurposeWork,
		started: base.Add(10 * time.Second), finished: base.Add(40 * time.Second),
		usage: &protocol.Usage{InputTokens: 100, OutputTokens: 10},
	})
	writeMetricsRun(t, root, traceID, metricsRunSpec{
		agentID: "run-work-2", bee: "builder", taskID: "task-work", purpose: protocol.PurposeWork,
		started: base.Add(50 * time.Second), finished: base.Add(60 * time.Second),
		usage: &protocol.Usage{InputTokens: 50, OutputTokens: 5},
	})
	writeMetricsRun(t, root, traceID, metricsRunSpec{
		agentID: "run-closing", bee: "reviewer", taskID: "task-closing", purpose: protocol.PurposeClosing,
		started: base.Add(70 * time.Second), finished: base.Add(100 * time.Second),
		state: protocol.StatusFailed, reason: protocol.ReasonChecksFailed,
		gate:  &protocol.GateResult{Passed: false},
		usage: &protocol.Usage{InputTokens: 200, OutputTokens: 20},
	})
	writeMetricsRun(t, root, traceID, metricsRunSpec{
		agentID: "run-analysis", purpose: protocol.PurposeAnalysis, analysisOf: "completion-r1",
		started: base.Add(110 * time.Second), finished: base.Add(120 * time.Second),
		state: protocol.StatusFailed,
		usage: &protocol.Usage{InputTokens: 10, OutputTokens: 1},
	})

	appendMetricsEvent(t, root, traceID, metricsEvent(t, traceID, "run-work-1", protocol.EventVerification, protocol.VerificationPayload{
		Kind: protocol.VerificationFailed, TaskID: "task-work", Summary: "review",
	}, base.Add(45*time.Second)))
	appendMetricsEvent(t, root, traceID, metricsEvent(t, traceID, "runtime", protocol.EventVerification, protocol.VerificationPayload{
		Kind: protocol.VerificationFailed, TaskID: "task-closing", Reason: protocol.ReasonChecksFailed,
	}, base.Add(101*time.Second)))
	appendMetricsEvent(t, root, traceID, metricsEvent(t, traceID, "run-analysis", protocol.EventVerification, protocol.VerificationPayload{
		Kind: protocol.VerificationFailed, Summary: "analysis finding",
	}, base.Add(121*time.Second)))

	note := func(agentID, taskID, severity string, at time.Time) {
		appendMetricsEvent(t, root, traceID, metricsEvent(t, traceID, agentID, protocol.EventInsight, protocol.NarrativeInsightPayload{
			Kind: protocol.InsightReviewNote, TaskID: taskID, Severity: severity, Summary: "note",
		}, at))
	}
	note("run-work-1", "task-work", "medium", base.Add(46*time.Second))
	note("run-closing", "task-closing", "high", base.Add(102*time.Second))
	note("run-analysis", "", "low", base.Add(122*time.Second))

	m, err := BuildTrailMetrics(root, traceID, observedAt)
	if err != nil {
		t.Fatal(err)
	}
	if m.Coverage.Purpose != CoverageComplete {
		t.Fatalf("purpose coverage = %q, want complete (warnings %+v)", m.Coverage.Purpose, m.Warnings)
	}

	work := m.ByPurpose.Work
	closing := m.ByPurpose.Closing
	analysis := m.ByPurpose.Analysis
	unknown := m.ByPurpose.Unknown

	if work.Runs != 2 || work.Failed != 0 || work.VerificationFailed != 1 || work.ReviewNotes != 1 ||
		work.GateRejections != 0 || work.Retries == nil || *work.Retries != 1 {
		t.Fatalf("work bucket = %+v", work)
	}
	if work.HoneySpent == nil || *work.HoneySpent != 2 {
		t.Fatalf("work honey = %v, want 2", work.HoneySpent)
	}
	if work.Tokens == nil || work.Tokens.InputTokens != 150 || work.Tokens.OutputTokens != 15 || work.Tokens.RunsWithUsage != 2 {
		t.Fatalf("work tokens = %+v", work.Tokens)
	}
	if work.ActiveTimeSec == nil || *work.ActiveTimeSec != 40 {
		t.Fatalf("work active time = %v, want 40", work.ActiveTimeSec)
	}

	if closing.Runs != 1 || closing.Failed != 1 || closing.VerificationFailed != 1 ||
		closing.ReviewNotes != 1 || closing.GateRejections != 1 || closing.Retries == nil || *closing.Retries != 0 {
		t.Fatalf("closing bucket = %+v", closing)
	}
	if closing.HoneySpent == nil || *closing.HoneySpent != 1 {
		t.Fatalf("closing honey = %v, want 1", closing.HoneySpent)
	}
	if closing.Tokens == nil || closing.Tokens.InputTokens != 200 || closing.Tokens.OutputTokens != 20 {
		t.Fatalf("closing tokens = %+v", closing.Tokens)
	}
	if closing.ActiveTimeSec == nil || *closing.ActiveTimeSec != 30 {
		t.Fatalf("closing active time = %v, want 30", closing.ActiveTimeSec)
	}

	if analysis.Runs != 1 || analysis.Failed != 1 || analysis.VerificationFailed != 1 ||
		analysis.ReviewNotes != 1 || analysis.GateRejections != 0 || analysis.Retries == nil || *analysis.Retries != 0 {
		t.Fatalf("analysis bucket = %+v", analysis)
	}
	if analysis.HoneySpent == nil || *analysis.HoneySpent != 1 {
		t.Fatalf("analysis honey = %v, want 1", analysis.HoneySpent)
	}
	if analysis.Tokens == nil || analysis.Tokens.InputTokens != 10 || analysis.Tokens.OutputTokens != 1 {
		t.Fatalf("analysis tokens = %+v", analysis.Tokens)
	}
	if analysis.ActiveTimeSec == nil || *analysis.ActiveTimeSec != 10 {
		t.Fatalf("analysis active time = %v, want 10", analysis.ActiveTimeSec)
	}

	if unknown.Runs != 0 || unknown.HoneySpent == nil || *unknown.HoneySpent != 1 {
		t.Fatalf("unknown bucket = %+v, want an unattributed legacy charge and no runs", unknown)
	}

	// Root aggregates stay total observed activity; the buckets split that same
	// deduplicated evidence rather than recounting it.
	if got := work.Runs + closing.Runs + analysis.Runs + unknown.Runs; got != m.Runs.Retained {
		t.Fatalf("bucket runs sum = %d, root retained = %d", got, m.Runs.Retained)
	}
	if got := *work.HoneySpent + *closing.HoneySpent + *analysis.HoneySpent + *unknown.HoneySpent; got != *m.Honey.Spent {
		t.Fatalf("bucket honey sum = %d, root spent = %d", got, *m.Honey.Spent)
	}
	if got := work.Tokens.InputTokens + closing.Tokens.InputTokens + analysis.Tokens.InputTokens; got != m.Tokens.InputTokens {
		t.Fatalf("bucket token sum = %d, root input = %d", got, m.Tokens.InputTokens)
	}
	if got := work.VerificationFailed + closing.VerificationFailed + analysis.VerificationFailed + unknown.VerificationFailed; got != m.VerificationFailed {
		t.Fatalf("bucket verification sum = %d, root = %d", got, m.VerificationFailed)
	}
	if got := work.GateRejections + closing.GateRejections + analysis.GateRejections + unknown.GateRejections; got != m.GateRejections {
		t.Fatalf("bucket gate sum = %d, root = %d (one rejection recorded twice)", got, m.GateRejections)
	}
	if m.GateRejections != 1 || m.VerificationFailed != 3 {
		t.Fatalf("root verification %d gate %d, want 3 and 1", m.VerificationFailed, m.GateRejections)
	}
	if *work.ActiveTimeSec+*closing.ActiveTimeSec+*analysis.ActiveTimeSec != *m.ActiveTimeSec {
		t.Fatalf("bucket active time does not sum to root")
	}

	// Task observations carry the phase-derived purpose.
	for _, task := range m.Tasks {
		switch task.TaskID {
		case "task-work":
			if task.Purpose != string(protocol.PurposeWork) {
				t.Fatalf("task-work purpose = %q", task.Purpose)
			}
		case "task-closing":
			if task.Purpose != string(protocol.PurposeClosing) {
				t.Fatalf("task-closing purpose = %q", task.Purpose)
			}
		}
	}
}

func TestBuildTrailMetricsLegacyPurposeStaysUnknown(t *testing.T) {
	root := t.TempDir()
	traceID := "trace-legacy-purpose"
	base := time.Now().UTC().Truncate(time.Second).Add(-time.Hour)
	observedAt := base.Add(10 * time.Minute)

	writeMetricsTask(t, root, traceID, taskledger.TaskSnapshot{
		TaskID: "task-work", Bee: "builder", Status: protocol.TaskStatusCompleted, CreatedAt: base.Add(time.Second),
	})
	writeMetricsRun(t, root, traceID, metricsRunSpec{
		agentID: "run-work", bee: "builder", taskID: "task-work", purpose: protocol.PurposeWork,
		started: base.Add(10 * time.Second), finished: base.Add(20 * time.Second),
		usage: &protocol.Usage{InputTokens: 10, OutputTokens: 1},
	})
	writeMetricsRun(t, root, traceID, metricsRunSpec{
		agentID: "run-legacy", bee: "builder", legacyPurpose: true,
		started: base.Add(30 * time.Second), finished: base.Add(40 * time.Second),
		state: protocol.StatusFailed,
		usage: &protocol.Usage{InputTokens: 5, OutputTokens: 1},
	})

	appendMetricsEvent(t, root, traceID, metricsEvent(t, traceID, "runtime", protocol.EventSignal, protocol.EnergyConsumePayload{
		Kind: protocol.SignalEnergyConsume, Amount: 1, Reason: "task.dispatch", TaskID: "task-work",
		Purpose: protocol.PurposeWork,
	}, base.Add(11*time.Second)))
	appendMetricsEvent(t, root, traceID, metricsEvent(t, traceID, "runtime", protocol.EventSignal, protocol.EnergyConsumePayload{
		Kind: protocol.SignalEnergyConsume, Amount: 1, Reason: "task.dispatch",
	}, base.Add(31*time.Second)))

	m, err := BuildTrailMetrics(root, traceID, observedAt)
	if err != nil {
		t.Fatal(err)
	}
	if m.Coverage.Purpose != CoveragePartial {
		t.Fatalf("purpose coverage = %q, want partial", m.Coverage.Purpose)
	}
	assertWarning(t, m.Warnings, "purpose", WarningLegacy)

	if m.ByPurpose.Work.Runs != 1 || m.ByPurpose.Work.Failed != 0 {
		t.Fatalf("work bucket = %+v, want the one attributed run", m.ByPurpose.Work)
	}
	if m.ByPurpose.Unknown.Runs != 1 || m.ByPurpose.Unknown.Failed != 1 {
		t.Fatalf("unknown bucket = %+v, want the legacy run", m.ByPurpose.Unknown)
	}
	if m.ByPurpose.Analysis.Runs != 0 {
		t.Fatalf("analysis bucket = %+v, a legacy run must not become analysis", m.ByPurpose.Analysis)
	}
	if m.ByPurpose.Unknown.Retries != nil {
		t.Fatalf("unknown retries = %v, want null", *m.ByPurpose.Unknown.Retries)
	}
	if m.ByPurpose.Work.HoneySpent == nil || *m.ByPurpose.Work.HoneySpent != 1 {
		t.Fatalf("work honey = %v, want 1", m.ByPurpose.Work.HoneySpent)
	}
	if m.ByPurpose.Unknown.HoneySpent == nil || *m.ByPurpose.Unknown.HoneySpent != 1 {
		t.Fatalf("unknown honey = %v, want 1", m.ByPurpose.Unknown.HoneySpent)
	}
	if m.Runs.Retained != 2 || m.Honey.Spent == nil || *m.Honey.Spent != 2 {
		t.Fatalf("root totals = runs %d honey %v, want 2 and 2", m.Runs.Retained, m.Honey.Spent)
	}
}

func TestBuildTrailMetricsWorkBaselineExcludesClosingAndAnalysis(t *testing.T) {
	base := time.Now().UTC().Truncate(time.Second).Add(-time.Hour)
	observedAt := base.Add(10 * time.Minute)

	build := func(t *testing.T, traceID string, overhead bool) TrailMetrics {
		t.Helper()
		root := t.TempDir()
		writeMetricsTask(t, root, traceID, taskledger.TaskSnapshot{
			TaskID: "task-work", Bee: "builder", Status: protocol.TaskStatusCompleted, CreatedAt: base.Add(time.Second),
		})
		writeMetricsRun(t, root, traceID, metricsRunSpec{
			agentID: "run-work", bee: "builder", taskID: "task-work", purpose: protocol.PurposeWork,
			started: base.Add(10 * time.Second), finished: base.Add(20 * time.Second),
			usage: &protocol.Usage{InputTokens: 100, OutputTokens: 10},
		})
		appendMetricsEvent(t, root, traceID, metricsEvent(t, traceID, "runtime", protocol.EventSignal, protocol.EnergyConsumePayload{
			Kind: protocol.SignalEnergyConsume, Amount: 2, Reason: "task.dispatch", TaskID: "task-work",
			Purpose: protocol.PurposeWork,
		}, base.Add(11*time.Second)))

		if overhead {
			writeMetricsTask(t, root, traceID, taskledger.TaskSnapshot{
				TaskID: "closing-1", Bee: "reviewer", Phase: protocol.TaskPhaseClosing,
				Status: protocol.TaskStatusCompleted, CreatedAt: base.Add(2 * time.Second),
			})
			writeMetricsRun(t, root, traceID, metricsRunSpec{
				agentID: "run-closing", bee: "reviewer", taskID: "closing-1", purpose: protocol.PurposeClosing,
				started: base.Add(30 * time.Second), finished: base.Add(time.Minute),
				state: protocol.StatusFailed, reason: protocol.ReasonChecksFailed,
				gate:  &protocol.GateResult{Passed: false},
				usage: &protocol.Usage{InputTokens: 500, OutputTokens: 50},
			})
			writeMetricsRun(t, root, traceID, metricsRunSpec{
				agentID: "run-analysis", purpose: protocol.PurposeAnalysis, analysisOf: "completion-r1",
				started: base.Add(70 * time.Second), finished: base.Add(90 * time.Second),
				usage: &protocol.Usage{InputTokens: 1000, OutputTokens: 100},
			})
			appendMetricsEvent(t, root, traceID, metricsEvent(t, traceID, "runtime", protocol.EventSignal, protocol.EnergyConsumePayload{
				Kind: protocol.SignalEnergyConsume, Amount: 3, Reason: "task.dispatch", TaskID: "closing-1",
				Purpose: protocol.PurposeClosing,
			}, base.Add(31*time.Second)))
			appendMetricsEvent(t, root, traceID, metricsEvent(t, traceID, "runtime", protocol.EventSignal, protocol.EnergyConsumePayload{
				Kind: protocol.SignalEnergyConsume, Amount: 5, Reason: "task.dispatch",
				Purpose: protocol.PurposeAnalysis, AnalysisOf: "completion-r1",
			}, base.Add(71*time.Second)))
		}

		m, err := BuildTrailMetrics(root, traceID, observedAt)
		if err != nil {
			t.Fatal(err)
		}
		return m
	}

	baseline := build(t, "trace-baseline", false)
	wrapped := build(t, "trace-wrapped", true)

	if !reflect.DeepEqual(baseline.ByPurpose.Work, wrapped.ByPurpose.Work) {
		t.Fatalf("work baseline changed by closing/analysis:\nbaseline %+v\nwrapped  %+v",
			baseline.ByPurpose.Work, wrapped.ByPurpose.Work)
	}
	if wrapped.ByPurpose.Work.HoneySpent == nil || *wrapped.ByPurpose.Work.HoneySpent != 2 {
		t.Fatalf("wrapped work honey = %v, want the goal work only", wrapped.ByPurpose.Work.HoneySpent)
	}
	if baseline.Honey.Spent == nil || wrapped.Honey.Spent == nil || *wrapped.Honey.Spent <= *baseline.Honey.Spent {
		t.Fatalf("root honey did not grow with overhead: baseline %v wrapped %v",
			baseline.Honey.Spent, wrapped.Honey.Spent)
	}
	if wrapped.ByPurpose.Closing.Runs != 1 || wrapped.ByPurpose.Analysis.Runs != 1 {
		t.Fatalf("closing/analysis buckets = %+v / %+v", wrapped.ByPurpose.Closing, wrapped.ByPurpose.Analysis)
	}
	if wrapped.ByPurpose.Closing.HoneySpent == nil || *wrapped.ByPurpose.Closing.HoneySpent != 3 {
		t.Fatalf("closing honey = %v, want 3", wrapped.ByPurpose.Closing.HoneySpent)
	}
	if wrapped.ByPurpose.Analysis.HoneySpent == nil || *wrapped.ByPurpose.Analysis.HoneySpent != 5 {
		t.Fatalf("analysis honey = %v, want 5", wrapped.ByPurpose.Analysis.HoneySpent)
	}
}

func TestBuildTrailMetricsForkLineageComesFromRetainedRecord(t *testing.T) {
	root := t.TempDir()
	forkTrace := "trace-fork"
	sourceTrace := "trace-source"
	base := time.Now().UTC().Truncate(time.Second).Add(-time.Hour)
	observedAt := base.Add(10 * time.Minute)

	writeMetricsManifest(t, root, forkTrace, map[string]any{
		"initialConfigRef": "sha256:candidate",
		"activeConfigRef":  "sha256:candidate",
		"configRevision":   1,
		"policy":           "pinned",
	})
	writeMetricsForkRecord(t, root, forkTrace, map[string]any{
		"version":          1,
		"forkId":           "fork-1",
		"traceId":          forkTrace,
		"sourceTrace":      sourceTrace,
		"sourceTask":       "source-task",
		"sourceAttempt":    "attempt-source",
		"sourceRun":        "run-source",
		"sourceConfigRef":  "sha256:source-cfg",
		"sourceRevision":   3,
		"configRef":        "sha256:candidate",
		"policy":           "pinned",
		"state":            "runnable",
		"delta":            map[string]any{"form": "none"},
		"createdAt":        base,
		"sourceTaskDigest": "sha256:digest",
		"sourceBaseSha":    "abc",
		"baseSha":          "abc",
	})
	writeMetricsOrigin(t, root, forkTrace, TrailOrigin{
		Kind: OriginKindFork, SourceTraceID: sourceTrace, ForkID: "fork-1",
	})
	writeMetricsRun(t, root, forkTrace, metricsRunSpec{
		agentID: "run-fork", bee: "builder", taskID: "fork-task",
		started: base, finished: base.Add(time.Minute),
		configRef: "sha256:candidate", configRevision: 1,
	})

	m, err := BuildTrailMetrics(root, forkTrace, observedAt)
	if err != nil {
		t.Fatal(err)
	}
	want := MetricsForkOf{
		SourceTrace:          sourceTrace,
		SourceTask:           "source-task",
		SourceAttempt:        "attempt-source",
		SourceRun:            "run-source",
		SourceConfigRef:      "sha256:source-cfg",
		SourceConfigRevision: 3,
		ForkID:               "fork-1",
		Delta:                "none",
		State:                "runnable",
	}
	if m.ForkOf == nil || *m.ForkOf != want {
		t.Fatalf("forkOf = %+v, want %+v", m.ForkOf, want)
	}
	if m.Coverage.Lineage != CoverageComplete {
		t.Fatalf("lineage coverage = %q, want complete (warnings %+v)", m.Coverage.Lineage, m.Warnings)
	}
	if m.Configuration == nil {
		t.Fatal("configuration = nil, want the retained binding")
	}
	if m.Configuration.InitialConfigRef != "sha256:candidate" ||
		m.Configuration.ActiveConfigRef != "sha256:candidate" ||
		m.Configuration.ConfigRevision != 1 ||
		m.Configuration.MultipleVersions {
		t.Fatalf("configuration = %+v", m.Configuration)
	}
	if len(m.Configuration.Used) != 1 ||
		m.Configuration.Used[0].ConfigRef != "sha256:candidate" ||
		m.Configuration.Used[0].ConfigRevision != 1 ||
		m.Configuration.Used[0].Runs != 1 {
		t.Fatalf("used = %+v", m.Configuration.Used)
	}
	if m.Coverage.Configuration != CoverageComplete {
		t.Fatalf("configuration coverage = %q, want complete (warnings %+v)", m.Coverage.Configuration, m.Warnings)
	}

	// The source trail is not consulted. Applying a new configuration and
	// changing its task state after the fork must not rewrite the fork's
	// retained lineage.
	writeMetricsManifest(t, root, sourceTrace, map[string]any{
		"initialConfigRef": "sha256:new-source-cfg",
		"activeConfigRef":  "sha256:new-source-cfg",
		"configRevision":   5,
		"policy":           "editable",
	})
	writeMetricsTask(t, root, sourceTrace, taskledger.TaskSnapshot{
		TaskID: "source-task", Bee: "builder", Status: protocol.TaskStatusCompleted,
		CreatedAt: base,
	})

	again, err := BuildTrailMetrics(root, forkTrace, observedAt)
	if err != nil {
		t.Fatal(err)
	}
	if again.ForkOf == nil || *again.ForkOf != want {
		t.Fatalf("forkOf changed with the source trail: %+v, want %+v", again.ForkOf, want)
	}
	if !reflect.DeepEqual(again.Configuration, m.Configuration) {
		t.Fatalf("fork configuration changed with the source trail:\nbefore %+v\nafter  %+v",
			m.Configuration, again.Configuration)
	}
}

func TestBuildTrailMetricsConfigurationMultipleVersions(t *testing.T) {
	root := t.TempDir()
	traceID := "trace-multi-config"
	base := time.Now().UTC().Truncate(time.Second)

	writeMetricsManifest(t, root, traceID, map[string]any{
		"initialConfigRef": "sha256:cfg-a",
		"activeConfigRef":  "sha256:cfg-b",
		"configRevision":   2,
		"policy":           "editable",
	})
	writeMetricsRun(t, root, traceID, metricsRunSpec{
		agentID: "run-a", bee: "builder", taskID: "task-1",
		started: base, finished: base.Add(time.Minute),
		configRef: "sha256:cfg-a", configRevision: 1,
	})
	writeMetricsRun(t, root, traceID, metricsRunSpec{
		agentID: "run-b", bee: "builder", taskID: "task-2",
		started: base.Add(2 * time.Minute), finished: base.Add(3 * time.Minute),
		configRef: "sha256:cfg-b", configRevision: 2,
	})

	m, err := BuildTrailMetrics(root, traceID, base.Add(10*time.Minute))
	if err != nil {
		t.Fatal(err)
	}
	if m.Configuration == nil {
		t.Fatal("configuration = nil")
	}
	if !m.Configuration.MultipleVersions {
		t.Fatalf("multipleVersions = false, want true for two retained versions: %+v", m.Configuration)
	}
	if m.Configuration.InitialConfigRef != "sha256:cfg-a" || m.Configuration.ActiveConfigRef != "sha256:cfg-b" {
		t.Fatalf("initial/active = %q/%q", m.Configuration.InitialConfigRef, m.Configuration.ActiveConfigRef)
	}
	wantUsed := []MetricsConfigUse{
		{ConfigRef: "sha256:cfg-a", ConfigRevision: 1, Runs: 1},
		{ConfigRef: "sha256:cfg-b", ConfigRevision: 2, Runs: 1},
	}
	if !reflect.DeepEqual(m.Configuration.Used, wantUsed) {
		t.Fatalf("used = %+v, want %+v", m.Configuration.Used, wantUsed)
	}
	if m.Coverage.Configuration != CoverageComplete {
		t.Fatalf("configuration coverage = %q, want complete (warnings %+v)", m.Coverage.Configuration, m.Warnings)
	}
}

func TestBuildTrailMetricsOrdinaryTrailLineageIsKnownAbsent(t *testing.T) {
	root := t.TempDir()
	traceID := "trace-ordinary"
	base := time.Now().UTC()

	writeMetricsOrigin(t, root, traceID, TrailOrigin{Kind: OriginKindManual, Source: "cli"})

	m, err := BuildTrailMetrics(root, traceID, base)
	if err != nil {
		t.Fatal(err)
	}
	if m.ForkOf != nil {
		t.Fatalf("forkOf = %+v, want null", m.ForkOf)
	}
	if m.Coverage.Lineage != CoverageAbsent {
		t.Fatalf("lineage coverage = %q, want %q (known ordinary)", m.Coverage.Lineage, CoverageAbsent)
	}
	for _, warning := range m.Warnings {
		if warning.Group == "lineage" {
			t.Fatalf("ordinary trail warned about lineage: %+v", warning)
		}
	}
	if m.Configuration != nil {
		t.Fatalf("configuration = %+v, want null without a binding", m.Configuration)
	}
	if m.Coverage.Configuration != CoverageUnavailable {
		t.Fatalf("configuration coverage = %q, want unavailable", m.Coverage.Configuration)
	}
	assertWarning(t, m.Warnings, "configuration", WarningMissing)
}

func TestBuildTrailMetricsMissingLineageIsUnavailable(t *testing.T) {
	root := t.TempDir()
	traceID := "trace-pinned-no-lineage"
	base := time.Now().UTC()

	writeMetricsManifest(t, root, traceID, map[string]any{
		"initialConfigRef": "sha256:cfg-a",
		"activeConfigRef":  "sha256:cfg-a",
		"configRevision":   1,
		"policy":           "pinned",
	})
	writeMetricsRun(t, root, traceID, metricsRunSpec{
		agentID: "run-a", bee: "builder", started: base, finished: base.Add(time.Minute),
		configRef: "sha256:cfg-a", configRevision: 1,
	})

	m, err := BuildTrailMetrics(root, traceID, base.Add(5*time.Minute))
	if err != nil {
		t.Fatal(err)
	}
	if m.ForkOf != nil {
		t.Fatalf("forkOf = %+v, want null", m.ForkOf)
	}
	if m.Coverage.Lineage != CoverageUnavailable {
		t.Fatalf("lineage coverage = %q, want unavailable for a pinned trail", m.Coverage.Lineage)
	}
	assertWarning(t, m.Warnings, "lineage", WarningMissing)
	// The rest of the read still works.
	if m.Runs.Retained != 1 {
		t.Fatalf("runs = %+v, want the retained execution", m.Runs)
	}
	if m.Configuration == nil || m.Coverage.Configuration != CoverageComplete {
		t.Fatalf("configuration = %+v (coverage %q), want complete", m.Configuration, m.Coverage.Configuration)
	}
}

func TestBuildTrailMetricsUnreadableEvidenceKeepsReadAvailable(t *testing.T) {
	base := time.Now().UTC()

	t.Run("fork lineage", func(t *testing.T) {
		root := t.TempDir()
		traceID := "trace-bad-lineage"
		writeMetricsOrigin(t, root, traceID, TrailOrigin{Kind: OriginKindFork, SourceTraceID: "trace-source"})
		writeMetricsRun(t, root, traceID, metricsRunSpec{
			agentID: "run-a", bee: "builder", started: base, finished: base.Add(time.Minute),
		})
		path := metricsForkLineagePath(root, traceID)
		if err := os.WriteFile(path, []byte("{not json"), 0o644); err != nil {
			t.Fatal(err)
		}

		m, err := BuildTrailMetrics(root, traceID, base.Add(2*time.Minute))
		if err != nil {
			t.Fatal(err)
		}
		if m.ForkOf != nil {
			t.Fatalf("forkOf = %+v, want null", m.ForkOf)
		}
		if m.Coverage.Lineage != CoverageUnavailable {
			t.Fatalf("lineage coverage = %q, want unavailable", m.Coverage.Lineage)
		}
		assertWarning(t, m.Warnings, "lineage", WarningUnreadable)
		if m.Runs.Retained != 1 {
			t.Fatalf("runs = %+v, want the retained execution", m.Runs)
		}
	})

	t.Run("configuration manifest", func(t *testing.T) {
		root := t.TempDir()
		traceID := "trace-bad-config"
		writeMetricsOrigin(t, root, traceID, TrailOrigin{Kind: OriginKindManual})
		writeMetricsRun(t, root, traceID, metricsRunSpec{
			agentID: "run-a", bee: "builder", started: base, finished: base.Add(time.Minute),
			configRef: "sha256:cfg-a", configRevision: 2,
		})
		path := metricsConfigManifestPath(root, traceID)
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, []byte("{not json"), 0o644); err != nil {
			t.Fatal(err)
		}

		m, err := BuildTrailMetrics(root, traceID, base.Add(2*time.Minute))
		if err != nil {
			t.Fatal(err)
		}
		if m.Coverage.Configuration != CoverageUnavailable {
			t.Fatalf("configuration coverage = %q, want unavailable", m.Coverage.Configuration)
		}
		assertWarning(t, m.Warnings, "configuration", WarningUnreadable)
		if m.Configuration == nil || len(m.Configuration.Used) != 1 ||
			m.Configuration.Used[0].ConfigRef != "sha256:cfg-a" {
			t.Fatalf("configuration = %+v, want the recorded execution reference", m.Configuration)
		}
		// A readable origin still answers the lineage question.
		if m.Coverage.Lineage != CoverageAbsent {
			t.Fatalf("lineage coverage = %q, want absent", m.Coverage.Lineage)
		}
	})
}

func TestBuildTrailMetricsLegacyRunConfigurationStaysUnknown(t *testing.T) {
	root := t.TempDir()
	traceID := "trace-legacy-config"
	base := time.Now().UTC()

	writeMetricsManifest(t, root, traceID, map[string]any{
		"initialConfigRef": "sha256:cfg-a",
		"activeConfigRef":  "sha256:cfg-a",
		"configRevision":   1,
		"policy":           "editable",
	})
	// A run recorded before configuration provenance existed. The manifest's
	// active reference is not evidence of what this execution used.
	writeMetricsRun(t, root, traceID, metricsRunSpec{
		agentID: "run-legacy", bee: "builder", started: base, finished: base.Add(time.Minute),
	})

	m, err := BuildTrailMetrics(root, traceID, base.Add(5*time.Minute))
	if err != nil {
		t.Fatal(err)
	}
	if m.Configuration == nil {
		t.Fatal("configuration = nil, want the retained binding")
	}
	if len(m.Configuration.Used) != 0 {
		t.Fatalf("used = %+v, want empty: a legacy execution must not be backfilled", m.Configuration.Used)
	}
	if m.Configuration.UnknownRuns != 1 {
		t.Fatalf("unknownRuns = %d, want 1", m.Configuration.UnknownRuns)
	}
	if m.Configuration.InitialConfigRef != "sha256:cfg-a" || m.Configuration.ActiveConfigRef != "sha256:cfg-a" {
		t.Fatalf("binding = %+v, want the retained initial/active references", m.Configuration)
	}
	if m.Configuration.MultipleVersions {
		t.Fatalf("multipleVersions = true, want false for one retained version")
	}
	if m.Coverage.Configuration != CoveragePartial {
		t.Fatalf("configuration coverage = %q, want partial", m.Coverage.Configuration)
	}
	assertWarning(t, m.Warnings, "configuration", WarningLegacy)
}

func TestBuildTrailMetricsConfigurationWithoutBindingKeepsRecordedUses(t *testing.T) {
	root := t.TempDir()
	traceID := "trace-unbound-config"
	base := time.Now().UTC()

	writeMetricsOrigin(t, root, traceID, TrailOrigin{Kind: OriginKindManual})
	writeMetricsRun(t, root, traceID, metricsRunSpec{
		agentID: "run-a", bee: "builder", started: base, finished: base.Add(time.Minute),
		configRef: "sha256:cfg-a", configRevision: 1,
	})

	m, err := BuildTrailMetrics(root, traceID, base.Add(5*time.Minute))
	if err != nil {
		t.Fatal(err)
	}
	if m.Configuration == nil {
		t.Fatal("configuration = nil, want the recorded execution reference")
	}
	if m.Configuration.InitialConfigRef != "" || m.Configuration.ActiveConfigRef != "" {
		t.Fatalf("binding = %+v, want no invented initial/active reference", m.Configuration)
	}
	if len(m.Configuration.Used) != 1 || m.Configuration.Used[0].ConfigRef != "sha256:cfg-a" {
		t.Fatalf("used = %+v", m.Configuration.Used)
	}
	if m.Configuration.MultipleVersions {
		t.Fatalf("multipleVersions = true, want false for one used reference")
	}
	if m.Coverage.Configuration != CoveragePartial {
		t.Fatalf("configuration coverage = %q, want partial", m.Coverage.Configuration)
	}
	assertWarning(t, m.Warnings, "configuration", WarningMissing)
}

func TestAnalysisRetryStaysInAnalysis(t *testing.T) {
	root := t.TempDir()
	traceID := "trace-analysis-retry"
	base := time.Now().UTC().Add(-time.Hour)

	writeMetricsRun(t, root, traceID, metricsRunSpec{
		agentID: "analysis-1", bee: "analyst", started: base, finished: base.Add(10 * time.Second),
		state: protocol.StatusFailed, reason: "analysis broke",
		purpose: protocol.PurposeAnalysis, analysisOf: "completion-r1",
	})
	writeMetricsRun(t, root, traceID, metricsRunSpec{
		agentID: "analysis-2", bee: "analyst", started: base.Add(time.Minute), finished: base.Add(time.Minute + 10*time.Second),
		state:   protocol.StatusCompleted,
		purpose: protocol.PurposeAnalysis, analysisOf: "completion-r1",
	})

	m, err := BuildTrailMetrics(root, traceID, base.Add(2*time.Minute))
	if err != nil {
		t.Fatal(err)
	}
	analysis := m.ByPurpose.Analysis
	if analysis.Runs != 2 || analysis.Failed != 1 {
		t.Fatalf("analysis bucket = %+v", analysis)
	}
	if analysis.Retries == nil || *analysis.Retries != 1 {
		t.Fatalf("analysis retries = %v, want 1", analysis.Retries)
	}
	if m.ByPurpose.Work.Runs != 0 || m.ByPurpose.Work.Failed != 0 {
		t.Fatalf("analysis retry leaked into work: %+v", m.ByPurpose.Work)
	}
	if m.Runs.Retained != 2 || m.Runs.Failed != 1 {
		t.Fatalf("total runs = %+v", m.Runs)
	}
}
