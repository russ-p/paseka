package runs

import (
	"encoding/json"
	"errors"
	"fmt"
	"math"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/russ-p/paseka/internal/protocol"
	"github.com/russ-p/paseka/internal/taskledger"
)

// TrailMetricsFileName is the replaceable convenience projection persisted by
// `paseka inspect metrics --write` and, later, by the work-completion checkpoint.
// It lives beside the trail's other retained evidence, so worktree-only cleanup
// leaves it readable. Historical completions reference their own immutable
// snapshots, never this file.
const TrailMetricsFileName = "metrics.json"

// TrailMetricsSchemaVersion is the version of the TrailMetrics field contract.
const TrailMetricsSchemaVersion = 1

// ErrTrailNotFound reports a trace identity with no retained evidence at all.
// It is a caller error, not an empty projection: an absent trail must not read
// as a free, idle trail.
var ErrTrailNotFound = errors.New("runs: trail not found")

// Coverage values for one measurement group.
const (
	CoverageComplete    = "complete"
	CoveragePartial     = "partial"
	CoverageUnavailable = "unavailable"
)

// Warning kinds. They keep "we found no record", "we could not read one", "two
// records disagree", and "this predates the record" distinguishable.
const (
	WarningMissing     = "missing"
	WarningUnreadable  = "unreadable"
	WarningConflicting = "conflicting"
	WarningLegacy      = "legacy"
)

// Cue ingress attribution. A cue invocation is the trail's origin only when the
// retained origin record proves it; a later invocation is an addition. Without
// an origin record the attribution stays unknown rather than labelling the
// first retained cue as the origin.
const (
	CueAttributionOrigin   = "origin"
	CueAttributionAddition = "addition"
	CueAttributionUnknown  = "unknown"
)

// TrailMetrics is a versioned projection of one trail's retained disk evidence.
//
// It is deliberately independent of NATS: the trail event log, run directories,
// task projections, retained origin, and recorded seed observations are enough
// to answer what the trail cost and what its runs did.
type TrailMetrics struct {
	SchemaVersion int       `json:"schemaVersion"`
	TraceID       string    `json:"traceId"`
	ObservedAt    time.Time `json:"observedAt"`

	// CueID names the initial cue only when the retained origin proves it.
	CueID        string              `json:"cueId,omitempty"`
	CueIngresses []MetricsCueIngress `json:"cueIngresses"`

	// Class is the entry-task descriptor. It is null when no non-merge task can
	// be identified.
	Class *MetricsClass `json:"class"`

	Work MetricsWork `json:"work"`
	// Honey is the recorded initial allocation and distinct charges.
	Honey  MetricsHoney   `json:"honey"`
	Tokens *MetricsTokens `json:"tokens"`
	Runs   MetricsRuns    `json:"runs"`

	// Tasks is the stable task-id ordered observation of every recorded task.
	Tasks []MetricsTask `json:"tasks"`
	// ByPurpose separates work, closing, and analysis. Root aggregates above
	// remain total observed activity; unknown holds historical executions whose
	// purpose was never recorded and is never folded into work or analysis.
	ByPurpose MetricsByPurpose `json:"byPurpose"`
	// VerificationFailed counts distinct recorded verification.failed events,
	// including the ones that join to no task.
	VerificationFailed int `json:"verificationFailed"`
	// GateRejections counts mechanical check rejections once each, whether the
	// rejection was recorded on a run, as a verification.failed event, or both.
	GateRejections int                  `json:"gateRejections"`
	ReviewNotes    MetricsReviewNotes   `json:"reviewNotes"`
	HumanFeedback  MetricsHumanFeedback `json:"humanFeedback"`

	// Durations are null when no usable interval exists; zero would claim a
	// measurement the record does not support.
	ActiveTimeSec *float64 `json:"activeTimeSec"`
	WallTimeSec   *float64 `json:"wallTimeSec"`

	Coverage MetricsCoverage  `json:"coverage"`
	Warnings []MetricsWarning `json:"warnings"`
}

// MetricsCueIngress is one distinct recorded cue invocation.
type MetricsCueIngress struct {
	InvocationID string    `json:"invocationId"`
	CueID        string    `json:"cueId"`
	Source       string    `json:"source,omitempty"`
	Mode         string    `json:"mode,omitempty"`
	InvokedAt    time.Time `json:"invokedAt,omitempty"`
	Attribution  string    `json:"attribution"`
	TaskIDs      []string  `json:"taskIds,omitempty"`
}

// MetricsClass is the deterministic entry-task descriptor plus the total number
// of task projections the trail retains.
type MetricsClass struct {
	TaskID    string `json:"taskId"`
	Bee       string `json:"bee,omitempty"`
	Intent    string `json:"intent,omitempty"`
	TaskCount int    `json:"taskCount"`
}

// MetricsWork is the trail's recorded work state, ledger stage, and admission
// facts. Revision and Completion stay null until the work-completion slice fills
// them; final acceptance and delivery are separate facts.
type MetricsWork struct {
	State          *string                `json:"state"`
	Stage          string                 `json:"stage"`
	Revision       *int                   `json:"revision"`
	Completion     *MetricsCompletion     `json:"completion"`
	PendingCount   int                    `json:"pendingCount"`
	PendingReasons []MetricsPendingReason `json:"pendingReasons,omitempty"`
	Pause          *MetricsEnergyPause    `json:"pause,omitempty"`
}

// MetricsCompletion is the current completion's identity and evidence
// reference. It is nil until completion evidence exists.
type MetricsCompletion struct {
	CompletionID string    `json:"completionId"`
	WorkRevision int       `json:"workRevision"`
	CompletedAt  time.Time `json:"completedAt"`
	ResultRef    string    `json:"resultRef,omitempty"`
	MetricsRef   string    `json:"metricsRef,omitempty"`
}

// MetricsPendingReason is the recorded count of deferred dispatches waiting on
// one admission reason. Individual dispatch identities live in the task ledger;
// the trail observation records the count and why they wait.
type MetricsPendingReason struct {
	Reason string `json:"reason"`
	Count  int    `json:"count"`
}

// MetricsEnergyPause is the open honey pause episode, reconstructed from the
// trail's paired energy.paused/energy.resumed observations.
type MetricsEnergyPause struct {
	EpisodeID    string    `json:"episodeId"`
	Reason       string    `json:"reason,omitempty"`
	StartedAt    time.Time `json:"startedAt,omitempty"`
	Remaining    int       `json:"remaining"`
	PendingCount int       `json:"pendingCount"`
}

// MetricsHoney is the recorded initial allocation and the distinct charges
// against it. Budget is null when no seed observation exists; Spent is null when
// the charge record itself is unavailable.
type MetricsHoney struct {
	Budget   *int           `json:"budget"`
	Spent    *int           `json:"spent"`
	ByReason map[string]int `json:"byReason,omitempty"`
}

// MetricsTokens is the summed token accounting of retained executions. It is
// null when no run reports usage; Partial is explicit when only some runs do.
type MetricsTokens struct {
	InputTokens      int64 `json:"inputTokens"`
	OutputTokens     int64 `json:"outputTokens"`
	CacheReadTokens  int64 `json:"cacheReadTokens"`
	CacheWriteTokens int64 `json:"cacheWriteTokens"`
	RunsWithUsage    int   `json:"runsWithUsage"`
	Runs             int   `json:"runs"`
	Partial          bool  `json:"partial,omitempty"`
}

// MetricsRuns counts retained executions. The dimensions overlap and must not
// be added together as a failure total.
type MetricsRuns struct {
	Retained            int            `json:"retained"`
	ByBee               map[string]int `json:"byBee,omitempty"`
	Failed              int            `json:"failed"`
	Cancelled           int            `json:"cancelled"`
	ContractFailed      int            `json:"contractFailed"`
	ContractUnavailable int            `json:"contractUnavailable"`
	Timeout             int            `json:"timeout"`
	ChecksFailed        int            `json:"checksFailed"`
	ChecksPassed        int            `json:"checksPassed"`
}

// MetricsTask is one recorded task's rework and review observation.
//
// Executions counts the invocations that performed the task's assigned work
// role. It is null when the retained record cannot prove that ownership: a
// missing historical assignment is a coverage gap, never a zero inferred from
// today's registry. Retries is max(0, executions-1); it is null for the same
// reason. A skipped task contributes no execution and no completion.
type MetricsTask struct {
	TaskID string `json:"taskId"`
	Phase  string `json:"phase"`
	// Purpose is the execution purpose derived from the recorded task phase.
	// It is work for ordinary tasks and closing for the runtime-seeded round.
	Purpose   string `json:"purpose"`
	Bee       string `json:"bee,omitempty"`
	Intent    string `json:"intent,omitempty"`
	Status    string `json:"status"`
	Started   bool   `json:"started"`
	Completed bool   `json:"completed"`
	// Runs lists every execution identity joined to the task, including roles
	// that did not perform its work (a Guard review, for example) and identities
	// whose run directory is no longer retained.
	Runs               []string `json:"runs"`
	Executions         *int     `json:"executions"`
	Retries            *int     `json:"retries"`
	VerificationFailed int      `json:"verificationFailed"`
	GateRejections     int      `json:"gateRejections"`
}

// MetricsByPurpose is the trail's observed activity split by the purpose
// admission recorded. Root aggregates remain total observed activity: the four
// buckets add up to it, and deduplication already happened before either was
// counted.
//
// Work is the completion baseline: cumulative goal work through the observed
// revision, including earlier failed work, but excluding every closing round and
// every analysis. Closing is trail overhead. Unknown holds historical
// executions whose purpose was never recorded; it is never silently folded into
// work or relabelled as analysis.
type MetricsByPurpose struct {
	Work     MetricsPurpose `json:"work"`
	Closing  MetricsPurpose `json:"closing"`
	Analysis MetricsPurpose `json:"analysis"`
	Unknown  MetricsPurpose `json:"unknown"`
	Coverage string         `json:"coverage"`
}

// MetricsPurpose is one purpose bucket's measurements. A nil pointer means the
// record does not support a measurement; zero means an observed zero. Runs and
// failure-type counters are always observed for retained executions.
type MetricsPurpose struct {
	HoneySpent         *int                  `json:"honeySpent"`
	Tokens             *MetricsPurposeTokens `json:"tokens"`
	Runs               int                   `json:"runs"`
	Failed             int                   `json:"failed"`
	Retries            *int                  `json:"retries"`
	VerificationFailed int                   `json:"verificationFailed"`
	ReviewNotes        int                   `json:"reviewNotes"`
	GateRejections     int                   `json:"gateRejections"`
	ActiveTimeSec      *float64              `json:"activeTimeSec"`
}

// MetricsPurposeTokens is the token accounting of one purpose bucket's retained
// executions with usable usage.
type MetricsPurposeTokens struct {
	InputTokens      int64 `json:"inputTokens"`
	OutputTokens     int64 `json:"outputTokens"`
	CacheReadTokens  int64 `json:"cacheReadTokens"`
	CacheWriteTokens int64 `json:"cacheWriteTokens"`
	RunsWithUsage    int   `json:"runsWithUsage"`
}

// MetricsReviewNotes counts review.note observations by the closed severity
// vocabulary. Unspecified is a note with no severity; Unknown is a historical
// value outside the vocabulary and is never folded into another bucket.
type MetricsReviewNotes struct {
	Info        int `json:"info"`
	Low         int `json:"low"`
	Medium      int `json:"medium"`
	High        int `json:"high"`
	Critical    int `json:"critical"`
	Unspecified int `json:"unspecified"`
	Unknown     int `json:"unknown"`
}

// MetricsHumanFeedback counts human.feedback observations by the closed verdict
// vocabulary. Unspecified is feedback with no verdict; Unknown is a historical
// value outside the vocabulary.
type MetricsHumanFeedback struct {
	Negative    int `json:"negative"`
	Positive    int `json:"positive"`
	Neutral     int `json:"neutral"`
	Unspecified int `json:"unspecified"`
	Unknown     int `json:"unknown"`
}

// MetricsCoverage reports completeness by measurement group.
type MetricsCoverage struct {
	Events    string `json:"events"`
	Runs      string `json:"runs"`
	Tasks     string `json:"tasks"`
	Origin    string `json:"origin"`
	Honey     string `json:"honey"`
	Tokens    string `json:"tokens"`
	Durations string `json:"durations"`
	// Purpose reports whether every retained execution carries a recorded
	// purpose. Unknown historical attribution makes it partial, not work.
	Purpose string `json:"purpose"`
}

// MetricsWarning is one concrete reason a measurement is missing, unreadable,
// conflicting, or legacy.
type MetricsWarning struct {
	Group  string `json:"group"`
	Kind   string `json:"kind"`
	Detail string `json:"detail"`
}

// TrailMetricsPath returns .paseka/runs/<traceId>/metrics.json.
func TrailMetricsPath(colonyRoot, traceID string) string {
	return filepath.Join(colonyRoot, ".paseka", "runs", traceID, TrailMetricsFileName)
}

// WriteTrailMetrics atomically persists one projection. It never declares work
// completion: the file is a replaceable convenience read, not a checkpoint.
func WriteTrailMetrics(colonyRoot, traceID string, metrics TrailMetrics) error {
	if strings.TrimSpace(colonyRoot) == "" {
		return fmt.Errorf("runs: colony root is required")
	}
	if err := validateTrailRecordID("traceId", traceID); err != nil {
		return err
	}
	data, err := json.MarshalIndent(metrics, "", "  ")
	if err != nil {
		return err
	}
	return writeFileAtomic(TrailMetricsPath(colonyRoot, traceID), append(data, '\n'), 0o644)
}

// BuildTrailMetrics projects one trail from retained disk evidence. observedAt
// is the single observation time the whole projection uses; zero means now.
//
// A trail with no retained evidence at all is ErrTrailNotFound. A readable but
// damaged or incomplete trail returns the values it does retain, with coverage
// and warnings naming every gap.
func BuildTrailMetrics(colonyRoot, traceID string, observedAt time.Time) (TrailMetrics, error) {
	if strings.TrimSpace(colonyRoot) == "" {
		return TrailMetrics{}, fmt.Errorf("runs: colony root is required")
	}
	if err := validateTrailRecordID("traceId", traceID); err != nil {
		return TrailMetrics{}, err
	}
	if observedAt.IsZero() {
		observedAt = time.Now().UTC()
	}
	observedAt = observedAt.UTC()

	m := TrailMetrics{
		SchemaVersion: TrailMetricsSchemaVersion,
		TraceID:       traceID,
		ObservedAt:    observedAt,
		CueIngresses:  []MetricsCueIngress{},
		Tasks:         []MetricsTask{},
		Warnings:      []MetricsWarning{},
		Runs:          MetricsRuns{ByBee: map[string]int{}},
	}

	traceRoot := filepath.Join(colonyRoot, ".paseka", "runs", traceID)
	rootEntries, err := os.ReadDir(traceRoot)
	if err != nil {
		if os.IsNotExist(err) {
			return TrailMetrics{}, fmt.Errorf("%w: %s", ErrTrailNotFound, traceID)
		}
		return TrailMetrics{}, fmt.Errorf("runs: list trail %s: %w", traceID, err)
	}

	events, readIssues, err := ReadTraceEventsReport(colonyRoot, traceID)
	if err != nil {
		return TrailMetrics{}, err
	}
	for _, issue := range readIssues {
		m.warn("events", WarningUnreadable, issue.Detail)
	}
	trailLogPresent := fileExists(Trail{ColonyRoot: colonyRoot, TraceID: traceID}.EventsPath())
	trailLogUnreadable, trailLogDamaged := trailLogIssues(readIssues, Trail{ColonyRoot: colonyRoot, TraceID: traceID}.EventsPath())
	if !trailLogPresent {
		m.warn("events", WarningLegacy,
			"trail-level event log is absent (legacy trail or no trail-level events)")
	}

	tasks, tasksCoverage, taskDirCount := loadMetricsTasks(&m, colonyRoot, traceID)
	m.Coverage.Tasks = tasksCoverage

	runs, runMetas, runCoverage, runUnreadable := loadMetricsRuns(&m, colonyRoot, traceID, rootEntries)
	m.Coverage.Runs = runCoverage
	m.Runs = runs

	if len(events) == 0 && runs.Retained == 0 && taskDirCount == 0 && !trailLogPresent {
		_, originOK, originErr := ReadTrailOrigin(colonyRoot, traceID)
		if originErr != nil || !originOK {
			if !hasRetainedCueInvocations(colonyRoot, traceID) {
				return TrailMetrics{}, fmt.Errorf("%w: %s", ErrTrailNotFound, traceID)
			}
		}
	}

	applyCueEvidence(&m, colonyRoot, traceID, events)
	applyClass(&m, tasks, taskDirCount)
	applyWork(&m, tasks, events)
	applyHoney(&m, events, trailLogPresent, trailLogUnreadable, trailLogDamaged)
	applyTokens(&m, runMetas, runs, runCoverage, runUnreadable)
	// Purpose is read from the run record and the task phase, never from a Bee
	// name. The same maps feed the root counters and byPurpose, so the buckets
	// cannot drift from the total they split.
	taskPurposes := metricsTaskPurposes(tasks)
	runPurposes := metricsRunPurposes(runMetas)
	verification := applyVerification(&m, events, taskPurposes, runPurposes)
	gate := applyGateRejections(&m, runMetas, events, taskPurposes, runPurposes)
	applyTasks(&m, colonyRoot, traceID, tasks, runMetas, verification.perTask, gate.perTask)
	reviewNotes := applyReviewNotes(&m, events, taskPurposes, runPurposes)
	applyHumanFeedback(&m, events)
	runDurations := applyDurations(&m, runMetas, tasks, events, observedAt, runUnreadable)
	applyByPurpose(&m, events, runMetas, taskPurposes, runDurations, verification.byPurpose, gate.byPurpose, reviewNotes)
	applyPurposeCoverage(&m, runMetas)
	applyEventsCoverage(&m, trailLogPresent, trailLogUnreadable, len(readIssues) > 0)
	applyTaskRunLinkageWarnings(&m, colonyRoot, traceID)
	applyRunTaskJoinWarnings(&m, tasks, runMetas)

	sort.SliceStable(m.Warnings, func(i, j int) bool {
		if m.Warnings[i].Group != m.Warnings[j].Group {
			return m.Warnings[i].Group < m.Warnings[j].Group
		}
		if m.Warnings[i].Kind != m.Warnings[j].Kind {
			return m.Warnings[i].Kind < m.Warnings[j].Kind
		}
		return m.Warnings[i].Detail < m.Warnings[j].Detail
	})
	return m, nil
}

func (m *TrailMetrics) warn(group, kind, detail string) {
	if strings.TrimSpace(detail) == "" {
		return
	}
	m.Warnings = append(m.Warnings, MetricsWarning{Group: group, Kind: kind, Detail: detail})
}

func applyEventsCoverage(m *TrailMetrics, trailLogPresent, trailLogUnreadable, anyIssue bool) {
	switch {
	case !trailLogPresent:
		m.Coverage.Events = CoveragePartial
	case trailLogUnreadable:
		m.Coverage.Events = CoverageUnavailable
	case anyIssue:
		m.Coverage.Events = CoveragePartial
	default:
		m.Coverage.Events = CoverageComplete
	}
}

// trailLogIssues reports whether the trail-level record itself was unreadable or
// damaged. A damaged run log does not make trail-level charges unreadable; a
// damaged trail record does, because that is where they are recorded.
func trailLogIssues(issues []TrailReadIssue, trailLogPath string) (unreadable, damaged bool) {
	for _, issue := range issues {
		if issue.Path != trailLogPath {
			continue
		}
		switch issue.Kind {
		case TrailIssueUnreadable:
			unreadable = true
		case TrailIssueDamaged:
			damaged = true
		}
	}
	return unreadable, damaged
}

// loadMetricsTasks reads the task projections leniently: one damaged task.md
// costs its own task and is reported, never the whole trail.
func loadMetricsTasks(m *TrailMetrics, colonyRoot, traceID string) (taskledger.TraceSnapshot, string, int) {
	snapshot := taskledger.TraceSnapshot{TraceID: traceID, Tasks: map[string]taskledger.TaskSnapshot{}}
	root := TraceTasksRoot(colonyRoot, traceID)
	entries, err := os.ReadDir(root)
	if err != nil {
		if os.IsNotExist(err) {
			return snapshot, CoverageComplete, 0
		}
		m.warn("tasks", WarningUnreadable, fmt.Sprintf("task projection directory: %v", err))
		return snapshot, CoverageUnavailable, 0
	}

	coverage := CoverageComplete
	dirCount := 0
	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}
		dirCount++
		taskID := entry.Name()
		d := TaskDir{ColonyRoot: colonyRoot, TraceID: traceID, TaskID: taskID}
		fm, _, err := d.ReadTask()
		if err != nil {
			m.warn("tasks", WarningUnreadable, fmt.Sprintf("task %s projection: %v", taskID, err))
			coverage = CoveragePartial
			continue
		}
		task := taskledger.TaskSnapshot{
			TaskID:          fm.TaskID,
			Title:           fm.Title,
			Bee:             fm.Bee,
			Sector:          fm.Sector,
			Intent:          fm.Intent,
			CueID:           fm.CueID,
			CueInvocationID: fm.CueInvocationID,
			Review:          fm.Review,
			Phase:           protocol.DeriveTaskPhase(fm.Phase, fm.Review),
			Status:          fm.Status,
			DependsOn:       append([]string(nil), fm.DependsOn...),
			Summary:         fm.Summary,
			Commit:          fm.Commit,
		}
		if task.TaskID == "" {
			task.TaskID = taskID
		}
		if fm.CreatedAt != "" {
			ts, err := time.Parse(time.RFC3339, fm.CreatedAt)
			if err != nil {
				m.warn("tasks", WarningUnreadable, fmt.Sprintf("task %s createdAt %q: %v", taskID, fm.CreatedAt, err))
				coverage = CoveragePartial
			} else {
				task.CreatedAt = ts
			}
		}
		if fm.UpdatedAt != "" {
			ts, err := time.Parse(time.RFC3339, fm.UpdatedAt)
			if err != nil {
				m.warn("tasks", WarningUnreadable, fmt.Sprintf("task %s updatedAt %q: %v", taskID, fm.UpdatedAt, err))
				coverage = CoveragePartial
			} else {
				task.UpdatedAt = ts
			}
		}
		snapshot.Tasks[task.TaskID] = task
	}
	return snapshot, coverage, dirCount
}

// loadMetricsRuns reads retained run directories, excluding reserved trace
// subdirectories such as tasks/, artifacts/, proposals/, and cue-invocations/.
func loadMetricsRuns(m *TrailMetrics, colonyRoot, traceID string, entries []os.DirEntry) (MetricsRuns, []RunMeta, string, bool) {
	runs := MetricsRuns{ByBee: map[string]int{}}
	var metas []RunMeta
	coverage := CoverageComplete
	unreadable := false
	for _, entry := range entries {
		if !entry.IsDir() || IsReservedTraceSubdir(entry.Name()) {
			continue
		}
		agentID := entry.Name()
		d := Dir{ColonyRoot: colonyRoot, TraceID: traceID, AgentID: agentID}
		if !fileExists(d.RequestPath()) {
			// A directory without request.json is not a retained execution.
			continue
		}
		meta, err := LoadRunMeta(d)
		if err != nil {
			m.warn("runs", WarningUnreadable, fmt.Sprintf("run %s record: %v", agentID, err))
			coverage = CoveragePartial
			unreadable = true
			continue
		}
		metas = append(metas, meta)
		runs.Retained++
		if meta.Bee != "" {
			runs.ByBee[meta.Bee]++
		}
		switch protocol.RunStatus(meta.State) {
		case protocol.StatusFailed:
			runs.Failed++
		case protocol.StatusCancelled:
			runs.Cancelled++
		}
		if meta.Reason == protocol.ReasonTimeout {
			runs.Timeout++
		}
		switch meta.Contract.Status {
		case protocol.ContractFailed:
			runs.ContractFailed++
		case protocol.ContractUnavailable:
			runs.ContractUnavailable++
		}
		if meta.Gate != nil {
			if meta.Gate.Passed {
				runs.ChecksPassed++
			} else {
				runs.ChecksFailed++
			}
		}
		if meta.Records != nil {
			m.warn("runs", WarningConflicting,
				fmt.Sprintf("run %s outcome records: %s (%s)", agentID, meta.Records.Status, meta.Records.Detail))
			coverage = CoveragePartial
		}
	}
	return runs, metas, coverage, unreadable
}

// applyCueEvidence reads the retained origin and cue invocation records. A
// later cue on a manually created trail is an addition; a missing legacy origin
// leaves attribution unknown instead of promoting the first retained cue.
func applyCueEvidence(m *TrailMetrics, colonyRoot, traceID string, events []protocol.Event) {
	byInvocation := map[string]MetricsCueIngress{}
	origin, originOK, originErr := ReadTrailOrigin(colonyRoot, traceID)
	originKind := ""
	switch {
	case originErr != nil:
		m.warn("origin", WarningUnreadable, fmt.Sprintf("trail origin record: %v", originErr))
		m.Coverage.Origin = CoverageUnavailable
	case !originOK:
		m.warn("origin", WarningLegacy,
			"trail origin record is absent (legacy trail); the initial cue is unknown")
		m.Coverage.Origin = CoveragePartial
	default:
		originKind = origin.Kind
		m.Coverage.Origin = CoverageComplete
	}

	recordCoverage := loadCueInvocationRecords(m, colonyRoot, traceID, byInvocation)
	eventCoverage := loadCueInvocationEvents(m, events, byInvocation)
	if originKind == "" && (recordCoverage == CoverageUnavailable || eventCoverage == CoverageUnavailable) {
		m.Coverage.Origin = CoverageUnavailable
	} else if m.Coverage.Origin == CoverageComplete && (recordCoverage == CoveragePartial || eventCoverage == CoveragePartial) {
		m.Coverage.Origin = CoveragePartial
	}

	attribution := func(invocationID string) string {
		switch originKind {
		case OriginKindCue:
			if invocationID == origin.InvocationID {
				return CueAttributionOrigin
			}
			return CueAttributionAddition
		case OriginKindManual, OriginKindFork, OriginKindOther:
			return CueAttributionAddition
		default:
			return CueAttributionUnknown
		}
	}

	ingresses := make([]MetricsCueIngress, 0, len(byInvocation))
	for _, ingress := range byInvocation {
		ingress.Attribution = attribution(ingress.InvocationID)
		ingresses = append(ingresses, ingress)
	}
	sort.SliceStable(ingresses, func(i, j int) bool {
		if !ingresses[i].InvokedAt.Equal(ingresses[j].InvokedAt) {
			return ingresses[i].InvokedAt.Before(ingresses[j].InvokedAt)
		}
		return ingresses[i].InvocationID < ingresses[j].InvocationID
	})

	if originKind == OriginKindCue {
		m.CueID = origin.CueID
		found := false
		for _, ingress := range ingresses {
			if ingress.InvocationID == origin.InvocationID {
				found = true
				break
			}
		}
		if !found {
			// The origin is durable evidence in its own right. Report it even
			// when its invocation record is gone, and say the record is missing.
			ingresses = append([]MetricsCueIngress{{
				InvocationID: origin.InvocationID,
				CueID:        origin.CueID,
				Source:       origin.Source,
				InvokedAt:    origin.CreatedAt,
				Attribution:  CueAttributionOrigin,
			}}, ingresses...)
			m.warn("origin", WarningMissing,
				fmt.Sprintf("origin invocation %s has no retained cue invocation record", origin.InvocationID))
			if m.Coverage.Origin == CoverageComplete {
				m.Coverage.Origin = CoveragePartial
			}
		}
	}
	m.CueIngresses = ingresses
}

// loadCueInvocationRecords reads cue-invocations/ leniently.
func loadCueInvocationRecords(m *TrailMetrics, colonyRoot, traceID string, out map[string]MetricsCueIngress) string {
	root := CueInvocationsRoot(colonyRoot, traceID)
	entries, err := os.ReadDir(root)
	if err != nil {
		if os.IsNotExist(err) {
			return CoverageComplete
		}
		m.warn("origin", WarningUnreadable, fmt.Sprintf("cue invocation directory: %v", err))
		return CoverageUnavailable
	}
	coverage := CoverageComplete
	for _, entry := range entries {
		if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".json") {
			continue
		}
		invocationID := strings.TrimSuffix(entry.Name(), ".json")
		inv, ok, err := ReadCueInvocation(colonyRoot, traceID, invocationID)
		if err != nil {
			m.warn("origin", WarningUnreadable,
				fmt.Sprintf("cue invocation %s record: %v", invocationID, err))
			coverage = CoveragePartial
			continue
		}
		if !ok {
			continue
		}
		out[inv.InvocationID] = MetricsCueIngress{
			InvocationID: inv.InvocationID,
			CueID:        inv.CueID,
			Source:       inv.Source,
			Mode:         inv.Mode,
			InvokedAt:    inv.InvokedAt,
			TaskIDs:      append([]string(nil), inv.TaskIDs...),
		}
	}
	return coverage
}

// loadCueInvocationEvents adds ingress observations that have no durable
// invocation record, and reports a record that disagrees with its observation.
func loadCueInvocationEvents(m *TrailMetrics, events []protocol.Event, out map[string]MetricsCueIngress) string {
	coverage := CoverageComplete
	for _, ev := range events {
		if ev.Type != protocol.EventSignal || protocol.PayloadKind(ev.Payload) != protocol.SignalCueIngress {
			continue
		}
		var payload protocol.CueIngressPayload
		if err := json.Unmarshal(ev.Payload, &payload); err != nil {
			m.warn("origin", WarningUnreadable, fmt.Sprintf("cue.ingress event %s payload: %v", ev.ID, err))
			coverage = CoveragePartial
			continue
		}
		if payload.InvocationID == "" {
			continue
		}
		if recorded, ok := out[payload.InvocationID]; ok {
			if recorded.CueID != payload.CueID || recorded.Mode != string(payload.Mode) {
				m.warn("origin", WarningConflicting,
					fmt.Sprintf("cue invocation %s record disagrees with its cue.ingress event", payload.InvocationID))
				coverage = CoveragePartial
			}
			continue
		}
		out[payload.InvocationID] = MetricsCueIngress{
			InvocationID: payload.InvocationID,
			CueID:        payload.CueID,
			Source:       payload.Source,
			Mode:         string(payload.Mode),
			InvokedAt:    payload.InvokedAt,
			TaskIDs:      append([]string(nil), payload.TaskIDs...),
		}
		m.warn("origin", WarningLegacy,
			fmt.Sprintf("cue invocation %s is retained only as its cue.ingress event", payload.InvocationID))
		coverage = CoveragePartial
	}
	return coverage
}

// applyClass selects the entry-task descriptor deterministically from known
// non-merge tasks: earliest createdAt, then taskId. A task with no recorded
// creation time makes the classification partial rather than letting a zero
// time decide it.
func applyClass(m *TrailMetrics, tasks taskledger.TraceSnapshot, taskCount int) {
	candidates := make([]taskledger.TaskSnapshot, 0, len(tasks.Tasks))
	for _, task := range tasks.Tasks {
		if taskledger.IsMergeTask(task) {
			continue
		}
		candidates = append(candidates, task)
	}
	if len(candidates) == 0 {
		return
	}
	unknownTime := 0
	timed := make([]taskledger.TaskSnapshot, 0, len(candidates))
	for _, task := range candidates {
		if task.CreatedAt.IsZero() {
			unknownTime++
			continue
		}
		timed = append(timed, task)
	}
	if unknownTime > 0 {
		m.warn("tasks", WarningMissing,
			fmt.Sprintf("%d task(s) have no recorded createdAt; entry-task classification is partial", unknownTime))
		if m.Coverage.Tasks == CoverageComplete {
			m.Coverage.Tasks = CoveragePartial
		}
	}
	var entry taskledger.TaskSnapshot
	switch {
	case len(timed) > 0:
		entry = timed[0]
		for _, task := range timed[1:] {
			if task.CreatedAt.Before(entry.CreatedAt) ||
				(task.CreatedAt.Equal(entry.CreatedAt) && task.TaskID < entry.TaskID) {
				entry = task
			}
		}
	default:
		// No creation time is known at all. Fall back to the stable task id so
		// the descriptor is deterministic, and say why it is not causal.
		entry = candidates[0]
		for _, task := range candidates[1:] {
			if task.TaskID < entry.TaskID {
				entry = task
			}
		}
	}
	m.Class = &MetricsClass{
		TaskID:    entry.TaskID,
		Bee:       entry.Bee,
		Intent:    entry.Intent,
		TaskCount: taskCount,
	}
}

// applyWork derives the recorded work state and the same ledger stage the
// Console's trail detail derives from the same task evidence.
func applyWork(m *TrailMetrics, tasks taskledger.TraceSnapshot, events []protocol.Event) {
	m.Work.Stage = string(taskledger.DeriveTrailStage(tasks))
	m.Work.Revision = nil
	m.Work.Completion = nil

	pause := openEnergyPause(m, events)
	m.Work.Pause = pause
	if pause != nil {
		m.Work.PendingCount = pause.PendingCount
		if pause.PendingCount > 0 {
			reason := strings.TrimSpace(pause.Reason)
			if reason == "" {
				reason = "unspecified"
			}
			m.Work.PendingReasons = []MetricsPendingReason{{Reason: reason, Count: pause.PendingCount}}
		}
	}

	state := deriveWorkState(tasks.Tasks, pause)
	if state != "" {
		m.Work.State = &state
	}
}

func deriveWorkState(tasks map[string]taskledger.TaskSnapshot, pause *MetricsEnergyPause) string {
	anyNonMerge := false
	anyActive := false
	anyFailed := false
	anyCancelled := false
	anyBlocked := false
	anySkipped := false
	allCompleted := true
	for _, task := range tasks {
		if taskledger.IsMergeTask(task) {
			continue
		}
		anyNonMerge = true
		switch task.Status {
		case protocol.TaskStatusFailed:
			anyFailed = true
		case protocol.TaskStatusCancelled:
			anyCancelled = true
		case protocol.TaskStatusBlocked:
			anyBlocked = true
		case protocol.TaskStatusSkipped:
			anySkipped = true
		}
		switch task.Status {
		case protocol.TaskStatusPlanned, protocol.TaskStatusReady,
			protocol.TaskStatusRunning, protocol.TaskStatusWaitingReview:
			anyActive = true
		}
		if task.Status != protocol.TaskStatusCompleted {
			allCompleted = false
		}
	}
	switch {
	case anyFailed:
		return string(protocol.TaskStatusFailed)
	case anyCancelled:
		return string(protocol.TaskStatusCancelled)
	case anyBlocked:
		return string(protocol.TaskStatusBlocked)
	case anyNonMerge && allCompleted:
		// Accepted work is complete even if a later dispatch is paused: the
		// pause is an admission fact, not a regression of finished work.
		return string(protocol.TaskStatusCompleted)
	case pause != nil && pause.PendingCount > 0:
		// Energy-deferred work is not progress: a pause must not read as an
		// idle or completed trail.
		return string(protocol.TaskStatusBlocked)
	case anyActive:
		return "open"
	case anySkipped:
		return string(protocol.TaskStatusSkipped)
	default:
		return ""
	}
}

// openEnergyPause reconstructs the open pause episode from the trail's paired
// observations. The ledger is the authority for admission; these events are its
// retained trail record.
func openEnergyPause(m *TrailMetrics, events []protocol.Event) *MetricsEnergyPause {
	var open *MetricsEnergyPause
	for _, ev := range events {
		if ev.Type != protocol.EventSignal {
			continue
		}
		switch protocol.PayloadKind(ev.Payload) {
		case string(protocol.SignalEnergyPaused):
			var payload protocol.EnergyPausedPayload
			if err := json.Unmarshal(ev.Payload, &payload); err != nil {
				m.warn("work", WarningUnreadable, fmt.Sprintf("energy.paused event %s payload: %v", ev.ID, err))
				continue
			}
			if payload.EpisodeID == "" {
				m.warn("work", WarningUnreadable, fmt.Sprintf("energy.paused event %s has no episode identity", ev.ID))
				continue
			}
			open = &MetricsEnergyPause{
				EpisodeID:    payload.EpisodeID,
				Reason:       payload.Reason,
				StartedAt:    ev.CreatedAt,
				Remaining:    payload.Remaining,
				PendingCount: payload.PendingCount,
			}
		case string(protocol.SignalEnergyResumed):
			var payload protocol.EnergyResumedPayload
			if err := json.Unmarshal(ev.Payload, &payload); err != nil {
				m.warn("work", WarningUnreadable, fmt.Sprintf("energy.resumed event %s payload: %v", ev.ID, err))
				continue
			}
			if open != nil && (payload.EpisodeID == "" || payload.EpisodeID == open.EpisodeID) {
				open = nil
			}
		}
	}
	return open
}

// applyHoney reads the durable seed observation and the distinct charges. The
// initial budget comes only from energy.seeded; a top-up or stipend never
// replaces it. Missing seed evidence leaves the budget unknown.
func applyHoney(m *TrailMetrics, events []protocol.Event, trailLogPresent, trailLogUnreadable, trailLogDamaged bool) {
	spent := 0
	byReason := map[string]int{}
	// A charge total is observed when the trail record is readable, or when the
	// merged read actually holds a charge. With no readable record and no
	// charge, zero would be an invented total.
	chargesRecorded := (trailLogPresent && !trailLogUnreadable) || hasEnergyConsume(events)
	partial := trailLogDamaged || !trailLogPresent || (trailLogUnreadable && chargesRecorded)
	var budget *int
	for _, ev := range events {
		if ev.Type != protocol.EventSignal {
			continue
		}
		switch protocol.PayloadKind(ev.Payload) {
		case string(protocol.SignalEnergySeeded):
			var payload protocol.EnergySeededPayload
			if err := json.Unmarshal(ev.Payload, &payload); err != nil {
				m.warn("honey", WarningUnreadable, fmt.Sprintf("energy.seeded event %s payload: %v", ev.ID, err))
				partial = true
				continue
			}
			if budget == nil {
				value := payload.Budget
				budget = &value
			} else if *budget != payload.Budget {
				m.warn("honey", WarningConflicting,
					fmt.Sprintf("energy.seeded events disagree on the initial budget (%d vs %d)", *budget, payload.Budget))
				partial = true
			}
		case string(protocol.SignalEnergyConsume):
			var payload protocol.EnergyConsumePayload
			if err := json.Unmarshal(ev.Payload, &payload); err != nil {
				m.warn("honey", WarningUnreadable, fmt.Sprintf("energy.consume event %s payload: %v", ev.ID, err))
				partial = true
				continue
			}
			spent += payload.Amount
			reason := strings.TrimSpace(payload.Reason)
			if reason == "" {
				reason = "unspecified"
			}
			byReason[reason] += payload.Amount
		}
	}
	m.Honey.Budget = budget
	if chargesRecorded {
		value := spent
		m.Honey.Spent = &value
		if len(byReason) > 0 {
			m.Honey.ByReason = byReason
		}
	}
	if trailLogDamaged {
		m.warn("honey", WarningUnreadable,
			"trail event log is damaged; honey charges may be incomplete")
	}
	if !trailLogPresent && chargesRecorded {
		m.warn("honey", WarningLegacy,
			"trail-level event log is absent; honey charges come from run records only")
	}
	if trailLogUnreadable && chargesRecorded {
		m.warn("honey", WarningUnreadable,
			"trail event log is unreadable; honey charges may be incomplete")
	}
	switch {
	case !chargesRecorded:
		m.Coverage.Honey = CoverageUnavailable
		m.warn("honey", WarningMissing, "no trail event record; honey charges are unavailable")
	case budget == nil:
		m.Coverage.Honey = CoveragePartial
		m.warn("honey", WarningMissing,
			"no energy.seeded observation; the initial honey budget is unknown")
	case partial:
		m.Coverage.Honey = CoveragePartial
	default:
		m.Coverage.Honey = CoverageComplete
	}
}

// applyTokens sums retained execution usage. It is null when no run reports
// usage; reported zeros are a measurement and partial reporting is explicit.
func applyTokens(m *TrailMetrics, metas []RunMeta, runs MetricsRuns, runCoverage string, runUnreadable bool) {
	if runCoverage == CoverageUnavailable {
		m.Coverage.Tokens = CoverageUnavailable
		m.warn("tokens", WarningUnreadable, "run records are unavailable; token usage is unavailable")
		return
	}
	var tokens MetricsTokens
	withUsage := 0
	for _, meta := range metas {
		if meta.Usage == nil {
			continue
		}
		if meta.Usage.Source == protocol.UsageSourceUnavailable {
			continue
		}
		withUsage++
		tokens.InputTokens += meta.Usage.InputTokens
		tokens.OutputTokens += meta.Usage.OutputTokens
		tokens.CacheReadTokens += meta.Usage.CacheReadTokens
		tokens.CacheWriteTokens += meta.Usage.CacheWriteTokens
	}
	if withUsage == 0 {
		m.Coverage.Tokens = CoverageUnavailable
		if runs.Retained == 0 {
			m.warn("tokens", WarningMissing, "no retained execution reports token usage")
		} else {
			m.warn("tokens", WarningMissing,
				fmt.Sprintf("none of the %d retained execution(s) reports token usage", runs.Retained))
		}
		return
	}
	tokens.RunsWithUsage = withUsage
	tokens.Runs = runs.Retained
	missingUsage := runs.Retained - withUsage
	tokens.Partial = missingUsage > 0 || runUnreadable
	if tokens.Partial {
		m.Coverage.Tokens = CoveragePartial
		if missingUsage > 0 {
			m.warn("tokens", WarningMissing,
				fmt.Sprintf("%d of %d retained execution(s) report no usable token usage", missingUsage, runs.Retained))
		} else {
			m.warn("tokens", WarningUnreadable,
				"some run records are unreadable; token usage may be incomplete")
		}
	} else {
		m.Coverage.Tokens = CoverageComplete
	}
	m.Tokens = &tokens
}

// metricsPurposeUnknown is the bucket for an execution whose purpose was not
// recorded. It is a reader-side bucket name, not a value admission writes.
const metricsPurposeUnknown = "unknown"

// metricsPurposeKey maps a recorded purpose to its bucket. Anything outside the
// closed vocabulary is unknown: a reader never invents work or analysis for it.
func metricsPurposeKey(purpose protocol.ExecutionPurpose) string {
	if protocol.IsKnownExecutionPurpose(purpose) {
		return string(purpose)
	}
	return metricsPurposeUnknown
}

// metricsTaskPurposes derives each task's execution purpose from its recorded
// phase. The phase vocabulary is closed, so every task has a purpose: work or
// closing. A merge gate is not dispatched and is not goal work.
func metricsTaskPurposes(tasks taskledger.TraceSnapshot) map[string]protocol.ExecutionPurpose {
	out := make(map[string]protocol.ExecutionPurpose, len(tasks.Tasks))
	for taskID, task := range tasks.Tasks {
		out[taskID] = protocol.PurposeForTaskPhase(task.Phase)
	}
	return out
}

// metricsRunPurposes indexes the purpose recorded on each retained execution.
// Empty or unsupported values stay absent, which makes them unknown.
func metricsRunPurposes(metas []RunMeta) map[string]protocol.ExecutionPurpose {
	out := make(map[string]protocol.ExecutionPurpose, len(metas))
	for _, meta := range metas {
		purpose := protocol.ExecutionPurpose(meta.Purpose)
		if protocol.IsKnownExecutionPurpose(purpose) {
			out[meta.AgentID] = purpose
		}
	}
	return out
}

// metricsEventPurpose attributes an observation to the execution that produced
// it, then to the task it names. A legacy event produced by the runtime and
// naming no task stays unknown.
func metricsEventPurpose(ev protocol.Event, taskID string, taskPurposes map[string]protocol.ExecutionPurpose, runPurposes map[string]protocol.ExecutionPurpose) protocol.ExecutionPurpose {
	if purpose, ok := runPurposes[ev.AgentID]; ok {
		return purpose
	}
	if taskID != "" {
		return taskPurposes[taskID]
	}
	return ""
}

// verificationEvidence is the trail's verification.failed observation, split
// per task and per purpose. The per-task map may sum to less than the trail
// total: a taskless verification stays in the total and in one purpose bucket.
type verificationEvidence struct {
	perTask   map[string]int
	byPurpose map[string]int
}

// applyVerification counts distinct verification.failed observations and joins
// them to their recorded task and purpose.
func applyVerification(m *TrailMetrics, events []protocol.Event, taskPurposes map[string]protocol.ExecutionPurpose, runPurposes map[string]protocol.ExecutionPurpose) verificationEvidence {
	evidence := verificationEvidence{
		perTask:   map[string]int{},
		byPurpose: map[string]int{},
	}
	total := 0
	for _, ev := range events {
		if ev.Type != protocol.EventVerification {
			continue
		}
		if protocol.PayloadKind(ev.Payload) != string(protocol.VerificationFailed) {
			continue
		}
		var payload protocol.VerificationPayload
		if err := json.Unmarshal(ev.Payload, &payload); err != nil {
			m.warn("events", WarningUnreadable,
				fmt.Sprintf("verification.failed event %s payload: %v", ev.ID, err))
			continue
		}
		total++
		taskID := strings.TrimSpace(payload.TaskID)
		if taskID != "" {
			evidence.perTask[taskID]++
		}
		purpose := metricsEventPurpose(ev, taskID, taskPurposes, runPurposes)
		evidence.byPurpose[metricsPurposeKey(purpose)]++
	}
	m.VerificationFailed = total
	return evidence
}

// gateEvidence is the deduplicated mechanical rejection count, split per task
// and per purpose.
type gateEvidence struct {
	perTask   map[string]int
	byPurpose map[string]int
}

// applyGateRejections counts mechanical check rejections once per rejection.
//
// A rejection is normally recorded twice: the run carries the failed gate
// verdict, and the runtime records a verification.failed with reason
// checks_failed. They describe the same event, so neither recorder may be
// added on top of the other. A failed gate with no surviving event still
// counts, and a surviving event whose run record was overwritten still counts.
//
// The purpose split uses the same deduplicated counts: a task rejection takes
// the task's phase purpose, and a taskless rejection keeps the purpose of its
// run record when that is unambiguous.
func applyGateRejections(m *TrailMetrics, metas []RunMeta, events []protocol.Event, taskPurposes map[string]protocol.ExecutionPurpose, runPurposes map[string]protocol.ExecutionPurpose) gateEvidence {
	runGates := map[string]int{}
	tasklessRunGates := 0
	tasklessRunPurposes := map[string]struct{}{}
	for _, meta := range metas {
		if meta.Gate == nil || meta.Gate.Passed {
			continue
		}
		if taskID := strings.TrimSpace(meta.TaskID); taskID != "" {
			runGates[taskID]++
		} else {
			tasklessRunGates++
			tasklessRunPurposes[metricsPurposeKey(protocol.ExecutionPurpose(meta.Purpose))] = struct{}{}
		}
	}

	checkEvents := map[string]int{}
	tasklessEvents := 0
	tasklessEventPurposes := map[string]struct{}{}
	for _, ev := range events {
		if ev.Type != protocol.EventVerification {
			continue
		}
		if protocol.PayloadKind(ev.Payload) != string(protocol.VerificationFailed) {
			continue
		}
		var payload protocol.VerificationPayload
		if err := json.Unmarshal(ev.Payload, &payload); err != nil {
			continue
		}
		if strings.TrimSpace(payload.Reason) != protocol.ReasonChecksFailed {
			continue
		}
		if taskID := strings.TrimSpace(payload.TaskID); taskID != "" {
			checkEvents[taskID]++
		} else {
			tasklessEvents++
			purpose := metricsPurposeKey(metricsEventPurpose(ev, "", taskPurposes, runPurposes))
			tasklessEventPurposes[purpose] = struct{}{}
		}
	}

	perTask := make(map[string]int, len(runGates)+len(checkEvents))
	for taskID, count := range runGates {
		perTask[taskID] = count
	}
	for taskID, recorded := range checkEvents {
		if recorded > perTask[taskID] {
			perTask[taskID] = recorded
		}
	}
	taskless := tasklessRunGates
	if tasklessEvents > taskless {
		taskless = tasklessEvents
	}
	total := taskless
	for _, count := range perTask {
		total += count
	}

	byPurpose := map[string]int{}
	for taskID, count := range perTask {
		byPurpose[metricsPurposeKey(taskPurposes[taskID])] += count
	}
	if taskless > 0 {
		// The two recorders of a taskless rejection share one identity, so the
		// count is deduplicated before attribution. If their purposes disagree,
		// or no purpose is recorded, the rejection stays unknown.
		purposes := tasklessRunPurposes
		if tasklessRunGates == 0 {
			purposes = tasklessEventPurposes
		} else if tasklessEvents > 0 {
			purposes = unionPurposeSets(tasklessRunPurposes, tasklessEventPurposes)
		}
		byPurpose[singlePurposeOrUnknown(purposes)] += taskless
	}

	m.GateRejections = total
	return gateEvidence{perTask: perTask, byPurpose: byPurpose}
}

func unionPurposeSets(a, b map[string]struct{}) map[string]struct{} {
	out := make(map[string]struct{}, len(a)+len(b))
	for key := range a {
		out[key] = struct{}{}
	}
	for key := range b {
		out[key] = struct{}{}
	}
	return out
}

func singlePurposeOrUnknown(purposes map[string]struct{}) string {
	if len(purposes) != 1 {
		return metricsPurposeUnknown
	}
	for purpose := range purposes {
		if purpose == metricsPurposeUnknown {
			return metricsPurposeUnknown
		}
		return purpose
	}
	return metricsPurposeUnknown
}

// applyTasks builds the stable task-id ordered observation. Executions count
// invocations that performed the task's assigned work role, resolved from the
// ownership recorded when each execution ran:
//
//   - a task's runs.ndjson entry is a recorded task dispatch, so it performed
//     the assigned role whatever that role is called;
//   - a direct-dispatch or HITL run joined through request.json counts when its
//     recorded Bee matches the task's recorded assignment (or the Bee recorded
//     on a task dispatch of the same task).
//
// A Guard or other associated role therefore stays in the joined run list
// without becoming an execution or a retry. Missing ownership is reported as a
// coverage gap instead of being guessed: executions and retries stay null.
// A skipped task contributes no execution, retry, or completion.
func applyTasks(m *TrailMetrics, colonyRoot, traceID string, tasks taskledger.TraceSnapshot, metas []RunMeta, verification, gate map[string]int) {
	metaByAgent := make(map[string]RunMeta, len(metas))
	runsByTask := map[string][]string{}
	for _, meta := range metas {
		metaByAgent[meta.AgentID] = meta
		if taskID := strings.TrimSpace(meta.TaskID); taskID != "" {
			runsByTask[taskID] = append(runsByTask[taskID], meta.AgentID)
		}
	}

	taskIDs := make([]string, 0, len(tasks.Tasks))
	for taskID := range tasks.Tasks {
		taskIDs = append(taskIDs, taskID)
	}
	sort.Strings(taskIDs)

	rows := make([]MetricsTask, 0, len(taskIDs))
	for _, mapID := range taskIDs {
		task := tasks.Tasks[mapID]
		taskID := task.TaskID
		if taskID == "" {
			taskID = mapID
		}
		d := TaskDir{ColonyRoot: colonyRoot, TraceID: traceID, TaskID: taskID}
		entries, err := d.ReadTaskRuns()
		if err != nil {
			m.warn("tasks", WarningUnreadable, fmt.Sprintf("task %s run linkage: %v", taskID, err))
			markTasksPartial(m)
		}

		entryByAgent := make(map[string]TaskRunEntry, len(entries))
		joined := map[string]struct{}{}
		workRoles := map[string]struct{}{}
		if bee := strings.TrimSpace(task.Bee); bee != "" {
			workRoles[bee] = struct{}{}
		}
		for _, entry := range entries {
			if entry.AgentID == "" {
				continue
			}
			entryByAgent[entry.AgentID] = entry
			joined[entry.AgentID] = struct{}{}
			if bee := strings.TrimSpace(entry.Bee); bee != "" {
				workRoles[bee] = struct{}{}
			}
		}
		for _, agentID := range runsByTask[taskID] {
			joined[agentID] = struct{}{}
		}

		runIDs := make([]string, 0, len(joined))
		for agentID := range joined {
			runIDs = append(runIDs, agentID)
		}
		sort.Strings(runIDs)

		row := MetricsTask{
			TaskID:             taskID,
			Phase:              string(task.Phase),
			Purpose:            string(protocol.PurposeForTaskPhase(task.Phase)),
			Bee:                task.Bee,
			Intent:             task.Intent,
			Status:             string(task.Status),
			Runs:               runIDs,
			VerificationFailed: verification[taskID],
			GateRejections:     gate[taskID],
		}

		switch task.Status {
		case protocol.TaskStatusSkipped:
			// A superseded standing allocation never counts as work, however
			// many records the trail still retains around it.
			zero := 0
			row.Executions = &zero
			row.Retries = &zero
			row.Started = false
			row.Completed = false
		default:
			executions := 0
			ownershipUnknown := false
			for _, agentID := range runIDs {
				if _, ok := entryByAgent[agentID]; ok {
					// A recorded task dispatch is the assigned work role by
					// construction, whatever the role is named.
					executions++
					continue
				}
				meta, ok := metaByAgent[agentID]
				if !ok {
					// The identity is joined but its run record is gone; it
					// stays visible without contributing a proven execution.
					continue
				}
				bee := strings.TrimSpace(meta.Bee)
				if bee != "" {
					if _, ok := workRoles[bee]; ok {
						executions++
						continue
					}
				}
				// Either the assignment is unknown, or the run records no role
				// at all: the execution count cannot be proven.
				if len(workRoles) == 0 || bee == "" {
					ownershipUnknown = true
				}
			}
			if ownershipUnknown {
				m.warn("tasks", WarningMissing,
					fmt.Sprintf("task %s has joined executions without a recorded assigned role; executions and retries are unknown", taskID))
				markTasksPartial(m)
			} else {
				retries := executions - 1
				if retries < 0 {
					retries = 0
				}
				value := executions
				row.Executions = &value
				row.Retries = &retries
				row.Started = taskStarted(executions, task.Status, energyDeferred(m))
				row.Completed = task.Status == protocol.TaskStatusCompleted
			}
		}
		rows = append(rows, row)
	}
	m.Tasks = rows
}

// taskStarted reports whether a task actually started. An execution proves it
// did. Without one, an open energy pause pending entry keeps a task that still
// looks active from counting as started, whatever its lifecycle status says;
// only a terminal status is evidence in its own right.
func taskStarted(executions int, status protocol.TaskStatus, deferred bool) bool {
	if executions > 0 {
		return true
	}
	if deferred && !terminalTaskStatus(status) {
		return false
	}
	return taskStartedByStatus(status)
}

// terminalTaskStatus reports whether the lifecycle status itself proves the
// task had started.
func terminalTaskStatus(status protocol.TaskStatus) bool {
	switch status {
	case protocol.TaskStatusCompleted, protocol.TaskStatusFailed, protocol.TaskStatusCancelled:
		return true
	default:
		return false
	}
}

// energyDeferred reports whether the trail has an open pause with work waiting
// behind it.
func energyDeferred(m *TrailMetrics) bool {
	return m.Work.Pause != nil && m.Work.Pause.PendingCount > 0
}

// taskStartedByStatus reports whether the lifecycle status itself proves the
// task started. planned and ready do not: an energy-deferred entry keeps its
// lifecycle status while the trail cannot admit it.
func taskStartedByStatus(status protocol.TaskStatus) bool {
	switch status {
	case protocol.TaskStatusRunning, protocol.TaskStatusWaitingReview,
		protocol.TaskStatusCompleted, protocol.TaskStatusFailed, protocol.TaskStatusCancelled:
		return true
	default:
		return false
	}
}

// applyReviewNotes counts review.note observations by the closed severity
// vocabulary and attributes each note to the execution that produced it.
func applyReviewNotes(m *TrailMetrics, events []protocol.Event, taskPurposes map[string]protocol.ExecutionPurpose, runPurposes map[string]protocol.ExecutionPurpose) map[string]int {
	byPurpose := map[string]int{}
	for _, ev := range events {
		if ev.Type != protocol.EventInsight {
			continue
		}
		if protocol.PayloadKind(ev.Payload) != string(protocol.InsightReviewNote) {
			continue
		}
		var payload protocol.NarrativeInsightPayload
		if err := json.Unmarshal(ev.Payload, &payload); err != nil {
			m.warn("events", WarningUnreadable,
				fmt.Sprintf("review.note event %s payload: %v", ev.ID, err))
			continue
		}
		purpose := metricsEventPurpose(ev, strings.TrimSpace(payload.TaskID), taskPurposes, runPurposes)
		byPurpose[metricsPurposeKey(purpose)]++
		switch protocol.ClassifyReviewSeverity(payload.Severity) {
		case protocol.ReviewSeverityInfo:
			m.ReviewNotes.Info++
		case protocol.ReviewSeverityLow:
			m.ReviewNotes.Low++
		case protocol.ReviewSeverityMedium:
			m.ReviewNotes.Medium++
		case protocol.ReviewSeverityHigh:
			m.ReviewNotes.High++
		case protocol.ReviewSeverityCritical:
			m.ReviewNotes.Critical++
		case protocol.ReviewSeverityUnknown:
			m.ReviewNotes.Unknown++
		default:
			m.ReviewNotes.Unspecified++
		}
	}
	return byPurpose
}

// applyHumanFeedback counts human.feedback observations by the closed verdict
// vocabulary.
func applyHumanFeedback(m *TrailMetrics, events []protocol.Event) {
	for _, ev := range events {
		if ev.Type != protocol.EventInsight {
			continue
		}
		if protocol.PayloadKind(ev.Payload) != string(protocol.InsightHumanFeedback) {
			continue
		}
		var payload protocol.HumanFeedbackPayload
		if err := json.Unmarshal(ev.Payload, &payload); err != nil {
			m.warn("events", WarningUnreadable,
				fmt.Sprintf("human.feedback event %s payload: %v", ev.ID, err))
			continue
		}
		switch protocol.ClassifyFeedbackVerdict(payload.Verdict) {
		case protocol.FeedbackVerdictNegative:
			m.HumanFeedback.Negative++
		case protocol.FeedbackVerdictPositive:
			m.HumanFeedback.Positive++
		case protocol.FeedbackVerdictNeutral:
			m.HumanFeedback.Neutral++
		case protocol.FeedbackVerdictUnknown:
			m.HumanFeedback.Unknown++
		default:
			m.HumanFeedback.Unspecified++
		}
	}
}

func markTasksPartial(m *TrailMetrics) {
	if m.Coverage.Tasks == CoverageComplete {
		m.Coverage.Tasks = CoveragePartial
	}
}

// runIntervalProblem classifies why an execution has no usable active interval.
type runIntervalProblem int

const (
	runIntervalOK runIntervalProblem = iota
	runIntervalMissingStart
	runIntervalMissingEnd
	runIntervalConflicting
)

// runActiveSeconds returns one execution's usable active interval. A running
// execution ends at observedAt; a terminal one needs its recorded finish time.
func runActiveSeconds(meta RunMeta, observedAt time.Time) (float64, runIntervalProblem) {
	if meta.StartedAt.IsZero() {
		return 0, runIntervalMissingStart
	}
	end := meta.FinishedAt
	if end.IsZero() {
		if terminalRunState(meta.State) {
			return 0, runIntervalMissingEnd
		}
		end = observedAt
	}
	if end.Before(meta.StartedAt) {
		return 0, runIntervalConflicting
	}
	return end.Sub(meta.StartedAt).Seconds(), runIntervalOK
}

// applyDurations computes active and wall time from recorded timestamps.
// Overlapping executions add to active time; waiting between them stays in wall
// time. A missing or invalid timestamp makes only that interval unusable. The
// returned map holds each usable execution's interval so byPurpose can sum the
// same measurements without a second, possibly divergent, read.
func applyDurations(m *TrailMetrics, metas []RunMeta, tasks taskledger.TraceSnapshot, events []protocol.Event, observedAt time.Time, runUnreadable bool) map[string]float64 {
	active := 0.0
	usable := 0
	partial := runUnreadable
	durations := make(map[string]float64, len(metas))
	for _, meta := range metas {
		seconds, problem := runActiveSeconds(meta, observedAt)
		switch problem {
		case runIntervalMissingStart:
			m.warn("durations", WarningMissing, fmt.Sprintf("run %s has no recorded start time", meta.AgentID))
			partial = true
			continue
		case runIntervalMissingEnd:
			m.warn("durations", WarningMissing,
				fmt.Sprintf("run %s is %s but has no recorded finish time", meta.AgentID, meta.State))
			partial = true
			continue
		case runIntervalConflicting:
			m.warn("durations", WarningConflicting,
				fmt.Sprintf("run %s finishes before it starts", meta.AgentID))
			partial = true
			continue
		}
		active += seconds
		durations[meta.AgentID] = seconds
		usable++
	}

	start, startKnown := earliestEvidence(tasks, metas, events)
	wallKnown := false
	switch {
	case !startKnown:
		m.warn("durations", WarningMissing, "no retained ingress, task, or execution timestamp")
	case observedAt.Before(start):
		m.warn("durations", WarningConflicting, "observation time is before the earliest retained evidence")
		wall := 0.0
		m.WallTimeSec = &wall
		wallKnown = true
		partial = true
	default:
		wall := roundSeconds(observedAt.Sub(start).Seconds())
		m.WallTimeSec = &wall
		wallKnown = true
	}

	if usable > 0 {
		value := roundSeconds(active)
		m.ActiveTimeSec = &value
	}
	switch {
	case usable == 0 && !wallKnown:
		m.Coverage.Durations = CoverageUnavailable
		m.warn("durations", WarningMissing, "no usable execution interval")
	case usable == 0 || !wallKnown || partial:
		m.Coverage.Durations = CoveragePartial
	default:
		m.Coverage.Durations = CoverageComplete
	}
	return durations
}

// applyByPurpose splits the same measured evidence the root aggregates count.
// It never recounts a rejection: the deduplicated verification and gate maps
// arrive already counted, so the buckets add up to the trail total rather than
// to a second, looser pass over the same events.
func applyByPurpose(
	m *TrailMetrics,
	events []protocol.Event,
	metas []RunMeta,
	taskPurposes map[string]protocol.ExecutionPurpose,
	runDurations map[string]float64,
	verificationByPurpose, gateByPurpose, reviewNotesByPurpose map[string]int,
) {
	buckets := map[string]*MetricsPurpose{
		string(protocol.PurposeWork):     &m.ByPurpose.Work,
		string(protocol.PurposeClosing):  &m.ByPurpose.Closing,
		string(protocol.PurposeAnalysis): &m.ByPurpose.Analysis,
		metricsPurposeUnknown:            &m.ByPurpose.Unknown,
	}
	bucketFor := func(purpose protocol.ExecutionPurpose) *MetricsPurpose {
		return buckets[metricsPurposeKey(purpose)]
	}

	// Honey charges are divided, never the shared budget allocation. A charge
	// recorded with its purpose is authoritative; an older charge falls back to
	// the task it names, and a charge with neither stays unknown.
	honeyByPurpose := map[string]int{}
	if m.Honey.Spent != nil {
		for _, ev := range events {
			if ev.Type != protocol.EventSignal || protocol.PayloadKind(ev.Payload) != string(protocol.SignalEnergyConsume) {
				continue
			}
			var payload protocol.EnergyConsumePayload
			if err := json.Unmarshal(ev.Payload, &payload); err != nil {
				continue
			}
			honeyByPurpose[metricsPurposeKey(chargePurpose(payload, taskPurposes))] += payload.Amount
		}
	}

	tokensByPurpose := map[string]*MetricsPurposeTokens{}
	activeByPurpose := map[string]float64{}
	activeSeen := map[string]bool{}
	for _, meta := range metas {
		purpose := protocol.ExecutionPurpose(meta.Purpose)
		bucket := bucketFor(purpose)
		bucket.Runs++
		if protocol.RunStatus(meta.State) == protocol.StatusFailed {
			bucket.Failed++
		}
		if meta.Usage != nil && meta.Usage.Source != protocol.UsageSourceUnavailable {
			key := metricsPurposeKey(purpose)
			tokens := tokensByPurpose[key]
			if tokens == nil {
				tokens = &MetricsPurposeTokens{}
				tokensByPurpose[key] = tokens
			}
			tokens.RunsWithUsage++
			tokens.InputTokens += meta.Usage.InputTokens
			tokens.OutputTokens += meta.Usage.OutputTokens
			tokens.CacheReadTokens += meta.Usage.CacheReadTokens
			tokens.CacheWriteTokens += meta.Usage.CacheWriteTokens
		}
		if seconds, ok := runDurations[meta.AgentID]; ok {
			key := metricsPurposeKey(purpose)
			activeByPurpose[key] += seconds
			activeSeen[key] = true
		}
	}

	for key, bucket := range buckets {
		if m.Honey.Spent != nil {
			value := honeyByPurpose[key]
			bucket.HoneySpent = &value
		}
		bucket.Tokens = tokensByPurpose[key]
		if activeSeen[key] {
			value := roundSeconds(activeByPurpose[key])
			bucket.ActiveTimeSec = &value
		}
		bucket.VerificationFailed = verificationByPurpose[key]
		bucket.GateRejections = gateByPurpose[key]
		bucket.ReviewNotes = reviewNotesByPurpose[key]
	}

	applyPurposeRetries(buckets, m.Tasks, metas)
}

// applyPurposeRetries attributes recorded rework to the purpose that performed
// it. A task's retries are max(0, executions-1) over its recorded work role; an
// analysis has no task row, so repeated executions for the same studied
// milestone are its retries. An attribution the record cannot prove stays null.
func applyPurposeRetries(buckets map[string]*MetricsPurpose, tasks []MetricsTask, metas []RunMeta) {
	taskRetries := map[string]int{}
	taskSeen := map[string]bool{}
	unknown := map[string]bool{}
	for _, task := range tasks {
		key := metricsPurposeKey(protocol.ExecutionPurpose(task.Purpose))
		taskSeen[key] = true
		if task.Retries == nil {
			unknown[key] = true
			continue
		}
		taskRetries[key] += *task.Retries
	}

	analysisGroups := map[string]int{}
	analysisRuns := 0
	analysisMissingMilestone := false
	for _, meta := range metas {
		if protocol.ExecutionPurpose(meta.Purpose) != protocol.PurposeAnalysis {
			continue
		}
		analysisRuns++
		milestone := strings.TrimSpace(meta.AnalysisOf)
		if milestone == "" {
			analysisMissingMilestone = true
			continue
		}
		analysisGroups[milestone]++
	}
	analysisRetries := 0
	for _, count := range analysisGroups {
		if count > 1 {
			analysisRetries += count - 1
		}
	}

	for key, bucket := range buckets {
		switch key {
		case string(protocol.PurposeAnalysis):
			if analysisRuns == 0 {
				zero := 0
				bucket.Retries = &zero
				continue
			}
			if analysisMissingMilestone {
				continue
			}
			value := analysisRetries
			bucket.Retries = &value
		case metricsPurposeUnknown:
			if bucket.Runs > 0 {
				continue
			}
			zero := 0
			bucket.Retries = &zero
		default:
			if unknown[key] {
				continue
			}
			if !taskSeen[key] && bucket.Runs > 0 {
				// Direct-only work has no task ownership evidence.
				continue
			}
			value := taskRetries[key]
			bucket.Retries = &value
		}
	}
}

// chargePurpose resolves a honey charge's purpose from the recorded admission
// attribution, falling back to the task it names. It never guesses from the
// charge reason or a Bee name.
func chargePurpose(payload protocol.EnergyConsumePayload, taskPurposes map[string]protocol.ExecutionPurpose) protocol.ExecutionPurpose {
	if protocol.IsKnownExecutionPurpose(payload.Purpose) {
		return payload.Purpose
	}
	if taskID := strings.TrimSpace(payload.TaskID); taskID != "" {
		if purpose, ok := taskPurposes[taskID]; ok {
			return purpose
		}
	}
	return ""
}

// applyPurposeCoverage reports how much of the trail's execution history has a
// recorded purpose. Legacy executions remain unknown: they are never defaulted
// to work, and never reclassified as analysis.
func applyPurposeCoverage(m *TrailMetrics, metas []RunMeta) {
	legacy := 0
	invalid := 0
	analysisWithoutMilestone := 0
	for _, meta := range metas {
		purpose := protocol.ExecutionPurpose(meta.Purpose)
		switch {
		case strings.TrimSpace(string(meta.Purpose)) == "":
			legacy++
		case !protocol.IsKnownExecutionPurpose(purpose):
			invalid++
			m.warn("purpose", WarningConflicting,
				fmt.Sprintf("run %s records unsupported purpose %q", meta.AgentID, meta.Purpose))
		}
		if purpose == protocol.PurposeAnalysis && strings.TrimSpace(meta.AnalysisOf) == "" {
			analysisWithoutMilestone++
		}
	}

	switch {
	case len(metas) == 0:
		m.Coverage.Purpose = CoverageComplete
	case legacy > 0 || invalid > 0 || analysisWithoutMilestone > 0:
		m.Coverage.Purpose = CoveragePartial
	default:
		m.Coverage.Purpose = CoverageComplete
	}
	m.ByPurpose.Coverage = m.Coverage.Purpose

	if legacy > 0 {
		m.warn("purpose", WarningLegacy,
			fmt.Sprintf("%d of %d retained execution(s) have no recorded purpose; they are counted as unknown, not work or analysis",
				legacy, len(metas)))
	}
	if analysisWithoutMilestone > 0 {
		m.warn("purpose", WarningMissing,
			fmt.Sprintf("%d analysis execution(s) name no studied milestone (analysisOf); their retry attribution is unknown",
				analysisWithoutMilestone))
	}
}

// terminalRunState reports whether a run state is terminal.
func terminalRunState(state string) bool {
	switch protocol.RunStatus(state) {
	case protocol.StatusCompleted, protocol.StatusFailed, protocol.StatusCancelled:
		return true
	}
	return false
}

// earliestEvidence returns the earliest retained ingress/task/execution time.
// Platform observations describe the trail rather than work, so they do not
// start the wall clock.
func earliestEvidence(tasks taskledger.TraceSnapshot, metas []RunMeta, events []protocol.Event) (time.Time, bool) {
	var earliest time.Time
	known := false
	consider := func(at time.Time) {
		if at.IsZero() {
			return
		}
		if !known || at.Before(earliest) {
			earliest = at
			known = true
		}
	}
	for _, meta := range metas {
		consider(meta.StartedAt)
	}
	for _, task := range tasks.Tasks {
		consider(task.CreatedAt)
	}
	for _, ev := range events {
		if ev.CreatedAt.IsZero() {
			continue
		}
		kind := protocol.PayloadKind(ev.Payload)
		if protocol.IsPlatformObservation(kind) || kind == "trail.work.completed" {
			continue
		}
		consider(ev.CreatedAt)
	}
	return earliest, known
}

// applyTaskRunLinkageWarnings reports linkage evidence that the retained run
// directories cannot prove: a linked execution with no run record, and a run
// directory reused for more than one dispatch (whose earlier results were
// overwritten).
func applyTaskRunLinkageWarnings(m *TrailMetrics, colonyRoot, traceID string) {
	root := TraceTasksRoot(colonyRoot, traceID)
	entries, err := os.ReadDir(root)
	if err != nil {
		return
	}
	type linkage struct {
		attempt string
		started time.Time
	}
	seen := map[string]linkage{}
	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}
		d := TaskDir{ColonyRoot: colonyRoot, TraceID: traceID, TaskID: entry.Name()}
		links, err := d.ReadTaskRuns()
		if err != nil {
			m.warn("runs", WarningUnreadable, fmt.Sprintf("task %s run linkage: %v", entry.Name(), err))
			if m.Coverage.Runs == CoverageComplete {
				m.Coverage.Runs = CoveragePartial
			}
			continue
		}
		for _, link := range links {
			if link.AgentID == "" {
				continue
			}
			if !fileExists((Dir{ColonyRoot: colonyRoot, TraceID: traceID, AgentID: link.AgentID}).RequestPath()) {
				m.warn("runs", WarningMissing,
					fmt.Sprintf("task %s links execution %s with no retained run record", entry.Name(), link.AgentID))
				if m.Coverage.Runs == CoverageComplete {
					m.Coverage.Runs = CoveragePartial
				}
				continue
			}
			current := linkage{attempt: link.AttemptID, started: link.StartedAt}
			if previous, ok := seen[link.AgentID]; ok {
				if previous.attempt != current.attempt || !previous.started.Equal(current.started) {
					m.warn("runs", WarningConflicting,
						fmt.Sprintf("execution directory %s was reused; earlier dispatch results are overwritten", link.AgentID))
					if m.Coverage.Runs == CoverageComplete {
						m.Coverage.Runs = CoveragePartial
					}
				}
				continue
			}
			seen[link.AgentID] = current
		}
	}
}

// applyRunTaskJoinWarnings reports an execution whose recorded task has no
// retained projection. The execution still counts in trail totals; only the
// join is missing.
func applyRunTaskJoinWarnings(m *TrailMetrics, tasks taskledger.TraceSnapshot, metas []RunMeta) {
	for _, meta := range metas {
		if meta.TaskID == "" {
			continue
		}
		if _, ok := tasks.Tasks[meta.TaskID]; ok {
			continue
		}
		m.warn("runs", WarningMissing,
			fmt.Sprintf("run %s names task %s with no retained task projection", meta.AgentID, meta.TaskID))
		if m.Coverage.Runs == CoverageComplete {
			m.Coverage.Runs = CoveragePartial
		}
	}
}

func roundSeconds(value float64) float64 {
	return math.Round(value*1000) / 1000
}

// hasEnergyConsume reports whether the merged read holds any charge event. It
// keeps a missing trail record from turning "we saw no charge" into "zero was
// spent".
func hasEnergyConsume(events []protocol.Event) bool {
	for _, ev := range events {
		if ev.Type == protocol.EventSignal && protocol.PayloadKind(ev.Payload) == string(protocol.SignalEnergyConsume) {
			return true
		}
	}
	return false
}

// hasRetainedCueInvocations reports whether the trail retains any cue ingress
// record. Such a record is trail evidence even before the first run, task, or
// event.
func hasRetainedCueInvocations(colonyRoot, traceID string) bool {
	entries, err := os.ReadDir(CueInvocationsRoot(colonyRoot, traceID))
	if err != nil {
		return false
	}
	for _, entry := range entries {
		if !entry.IsDir() && strings.HasSuffix(entry.Name(), ".json") {
			return true
		}
	}
	return false
}
