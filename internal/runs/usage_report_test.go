package runs_test

import (
	"encoding/json"
	"errors"
	"fmt"
	"math"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/russ-p/paseka/internal/protocol"
	"github.com/russ-p/paseka/internal/runs"
)

// usageFixture is one execution directory. The zero value is a completed AFK
// run with no usage; each test changes only the part it is about.
type usageFixture struct {
	traceID        string
	agentID        string
	started        time.Time
	bee            string
	adapter        string
	implementation string
	model          string
	alias          string
	purpose        protocol.ExecutionPurpose
	state          protocol.RunStatus
	usage          *protocol.Usage
	session        *protocol.Usage
	sessionOn      bool
	noStart        bool
	noRes          bool
	// blankIdentity writes an empty Bee and Adapter, which is the legacy shape
	// the report groups under `unknown` rather than dropping.
	blankIdentity bool
}

func writeUsageFixture(t *testing.T, root string, f usageFixture) runs.Dir {
	t.Helper()
	started := f.started
	if f.noStart {
		started = time.Time{}
	}
	state := f.state
	if state == "" {
		state = protocol.StatusCompleted
	}
	bee := f.bee
	if bee == "" {
		bee = "builder"
	}
	adapter := f.adapter
	if adapter == "" {
		adapter = "opencode"
	}
	if f.blankIdentity {
		bee = ""
		adapter = ""
	}

	d := runs.Dir{ColonyRoot: root, TraceID: f.traceID, AgentID: f.agentID}
	if err := d.Prepare(); err != nil {
		t.Fatal(err)
	}
	if err := d.WriteRequest(protocol.Request{
		ProtocolVersion: protocol.Version,
		TraceID:         f.traceID,
		AgentID:         f.agentID,
		Bee:             bee,
		Adapter:         adapter,
		Workspace:       root,
		ColonyRoot:      root,
		Model:           f.model,
		ModelAlias:      f.alias,
		Purpose:         f.purpose,
		CreatedAt:       started,
	}); err != nil {
		t.Fatal(err)
	}
	if err := d.WriteStatusSnapshot(protocol.StatusSnapshot{
		ProtocolVersion: protocol.Version,
		State:           state,
		StartedAt:       started,
		FinishedAt:      finishOf(started, state),
	}); err != nil {
		t.Fatal(err)
	}
	// The implementation version is recorded by the legacy meta sidecar rather
	// than by request.json, so the fixture writes it where the projection reads
	// it.
	if f.implementation != "" {
		payload, err := json.Marshal(map[string]string{"implementation": f.implementation})
		if err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(d.MetaPath(), payload, 0o644); err != nil {
			t.Fatal(err)
		}
	}
	if f.sessionOn {
		if err := d.WriteSession(runs.SessionMeta{
			ProtocolVersion: protocol.Version,
			SessionID:       f.agentID,
			TraceID:         f.traceID,
			AgentID:         f.agentID,
			Bee:             bee,
			Adapter:         adapter,
			Workspace:       root,
			ColonyRoot:      root,
			Model:           f.model,
			State:           string(state),
			Usage:           f.session,
			Purpose:         f.purpose,
			StartedAt:       started,
		}); err != nil {
			t.Fatal(err)
		}
	}
	if !f.noRes {
		if err := d.WriteResult(protocol.Result{
			ProtocolVersion: protocol.Version,
			TraceID:         f.traceID,
			AgentID:         f.agentID,
			Status:          state,
			Summary:         "done",
			Usage:           f.usage,
			FinishedAt:      finishOf(started, state),
		}); err != nil {
			t.Fatal(err)
		}
	}
	return d
}

func finishOf(started time.Time, state protocol.RunStatus) time.Time {
	if state != protocol.StatusCompleted && state != protocol.StatusFailed && state != protocol.StatusCancelled {
		return time.Time{}
	}
	if started.IsZero() {
		return time.Time{}
	}
	return started.Add(time.Minute)
}

func usageDate(t *testing.T, value string, loc *time.Location) time.Time {
	t.Helper()
	parsed, err := time.ParseInLocation("2006-01-02 15:04", value, loc)
	if err != nil {
		t.Fatal(err)
	}
	return parsed
}

func dayByDate(t *testing.T, report runs.UsageReport, date string) runs.UsageDay {
	t.Helper()
	for _, day := range report.Days {
		if day.Date == date {
			return day
		}
	}
	t.Fatalf("report has no day %s: %+v", date, report.Days)
	return runs.UsageDay{}
}

func sumDayTokens(days []runs.UsageDay) runs.UsageTokens {
	var total runs.UsageTokens
	for _, day := range days {
		total.InputTokens += day.Tokens.InputTokens
		total.OutputTokens += day.Tokens.OutputTokens
		total.ReasoningTokens += day.Tokens.ReasoningTokens
		total.CacheReadTokens += day.Tokens.CacheReadTokens
		total.CacheWriteTokens += day.Tokens.CacheWriteTokens
		total.TotalTokens += day.Tokens.TotalTokens
	}
	return total
}

func TestUsageScanProjectsEveryRetainedExecutionShape(t *testing.T) {
	root := t.TempDir()
	utc := time.UTC
	base := usageDate(t, "2026-02-10 10:00", utc)

	// Completed AFK run with usage.
	writeUsageFixture(t, root, usageFixture{
		traceID: "trace-a", agentID: "run-complete", started: base,
		model: "anthropic/claude-sonnet", alias: "sonnet", purpose: protocol.PurposeWork,
		usage: &protocol.Usage{InputTokens: 100, OutputTokens: 20, ReasoningTokens: 7,
			CacheReadTokens: 30, CacheWriteTokens: 4, Source: protocol.UsageSourceOpenCodeRunJSON},
	})
	// Completed AFK run that recorded no usage.
	writeUsageFixture(t, root, usageFixture{
		traceID: "trace-a", agentID: "run-silent", started: base.Add(time.Minute),
		purpose: protocol.PurposeWork,
	})
	// In-flight run.
	writeUsageFixture(t, root, usageFixture{
		traceID: "trace-a", agentID: "run-inflight", started: base.Add(2 * time.Minute),
		state: protocol.StatusRunning, purpose: protocol.PurposeWork,
	})
	// Interactive session whose usage lives in session.json. It writes no
	// result.json, which is the expected session shape rather than an
	// interrupted outcome pair.
	writeUsageFixture(t, root, usageFixture{
		traceID: "trace-b", agentID: "run-session", started: base.Add(3 * time.Minute),
		model: "openai/gpt-5", purpose: protocol.PurposeWork, noRes: true, sessionOn: true,
		session: &protocol.Usage{InputTokens: 40, OutputTokens: 8, Source: protocol.UsageSourceOpenCodeV2Session},
	})
	// A directory carrying both AFK result usage and session usage: one
	// execution, and the projection's precedence (result.json first) decides.
	writeUsageFixture(t, root, usageFixture{
		traceID: "trace-b", agentID: "run-both", started: base.Add(4 * time.Minute),
		purpose: protocol.PurposeClosing, sessionOn: true,
		usage:   &protocol.Usage{InputTokens: 55, OutputTokens: 5, Source: protocol.UsageSourceOpenCodeRunJSON},
		session: &protocol.Usage{InputTokens: 999, OutputTokens: 999, Source: protocol.UsageSourceOpenCodeV2Session},
	})
	// Resumed session: a second execution directory, counted again.
	writeUsageFixture(t, root, usageFixture{
		traceID: "trace-b", agentID: "run-session-resumed", started: base.Add(5 * time.Minute),
		purpose: protocol.PurposeWork, noRes: true, sessionOn: true,
		session: &protocol.Usage{InputTokens: 11, OutputTokens: 2, Source: protocol.UsageSourceOpenCodeV2Session},
	})
	// Unknown model.
	writeUsageFixture(t, root, usageFixture{
		traceID: "trace-c", agentID: "run-unknown-model", started: base.Add(6 * time.Minute),
		purpose: protocol.PurposeAnalysis,
		usage:   &protocol.Usage{InputTokens: 1, Source: protocol.UsageSourceCursorStreamJSON},
	})
	// Legacy run: no purpose recorded.
	writeUsageFixture(t, root, usageFixture{
		traceID: "trace-c", agentID: "run-legacy", started: base.Add(7 * time.Minute),
		usage: &protocol.Usage{InputTokens: 3, Source: protocol.UsageSourceCursorStreamJSON},
	})
	// No start time at all.
	writeUsageFixture(t, root, usageFixture{
		traceID: "trace-c", agentID: "run-undated", noStart: true,
		usage: &protocol.Usage{InputTokens: 2, Source: protocol.UsageSourceCursorStreamJSON},
	})

	// A non-execution directory inside a trail must not become a record.
	if err := os.MkdirAll(filepath.Join(root, ".paseka", "runs", "trace-a", "tasks"), 0o755); err != nil {
		t.Fatal(err)
	}

	scan, err := runs.ScanUsageRecords(root)
	if err != nil {
		t.Fatal(err)
	}
	if len(scan.Records) != 9 {
		t.Fatalf("records = %d, want 9: %+v", len(scan.Records), scan.Records)
	}

	byID := map[string]runs.UsageRecord{}
	for _, record := range scan.Records {
		byID[record.RunID] = record
	}

	complete := byID["run-complete"]
	if complete.Measurement != runs.UsageReported {
		t.Fatalf("completed run measurement = %q", complete.Measurement)
	}
	if complete.Tokens.InputTokens != 100 || complete.Tokens.OutputTokens != 20 ||
		complete.Tokens.ReasoningTokens != 7 || complete.Tokens.CacheReadTokens != 30 ||
		complete.Tokens.CacheWriteTokens != 4 || complete.Tokens.TotalTokens != 161 {
		t.Fatalf("completed run tokens = %+v", complete.Tokens)
	}
	if complete.Model != "anthropic/claude-sonnet" || complete.ModelAlias != "sonnet" {
		t.Fatalf("model/alias = %q/%q", complete.Model, complete.ModelAlias)
	}
	if complete.Purpose != "work" {
		t.Fatalf("purpose = %q", complete.Purpose)
	}

	if got := byID["run-silent"].Measurement; got != runs.UsageMissing {
		t.Fatalf("completed silent run measurement = %q, want missing", got)
	}
	if got := byID["run-inflight"].Measurement; got != runs.UsageUnknown {
		t.Fatalf("in-flight measurement = %q, want unknown", got)
	}
	if got := byID["run-session"].Measurement; got != runs.UsageReported {
		t.Fatalf("session measurement = %q, want reported", got)
	}
	both := byID["run-both"]
	if both.Measurement != runs.UsageReported || both.Tokens.InputTokens != 55 {
		t.Fatalf("both-records run = %+v, want result.json precedence and one measurement", both)
	}
	if byID["run-session-resumed"].Measurement != runs.UsageReported {
		t.Fatalf("resumed session measurement = %q", byID["run-session-resumed"].Measurement)
	}
	if byID["run-unknown-model"].Model != "" {
		t.Fatalf("unknown model = %q, want empty", byID["run-unknown-model"].Model)
	}
	if byID["run-legacy"].Purpose != "unknown" {
		t.Fatalf("legacy purpose = %q, want unknown", byID["run-legacy"].Purpose)
	}
	if !byID["run-undated"].StartedAt.IsZero() {
		t.Fatalf("undated run startedAt = %s, want zero", byID["run-undated"].StartedAt)
	}
}

func TestUsageReportBucketsByStartDayAndZeroFills(t *testing.T) {
	root := t.TempDir()
	utc := time.UTC

	// Starts on the 1st, finishes on the 2nd: it belongs to the 1st.
	writeUsageFixture(t, root, usageFixture{
		traceID: "trace-day", agentID: "crosses-midnight", started: usageDate(t, "2026-01-01 23:30", utc),
		usage: &protocol.Usage{InputTokens: 10, OutputTokens: 1, Source: protocol.UsageSourceOpenCodeRunJSON},
	})
	// Exactly at the inclusive boundary.
	writeUsageFixture(t, root, usageFixture{
		traceID: "trace-day", agentID: "at-since", started: usageDate(t, "2026-01-01 00:00", utc),
		usage: &protocol.Usage{InputTokens: 20, OutputTokens: 2, Source: protocol.UsageSourceOpenCodeRunJSON},
	})
	// Exactly at the exclusive boundary: not in the range.
	writeUsageFixture(t, root, usageFixture{
		traceID: "trace-day", agentID: "at-until", started: usageDate(t, "2026-01-04 00:00", utc),
		usage: &protocol.Usage{InputTokens: 99, OutputTokens: 9, Source: protocol.UsageSourceOpenCodeRunJSON},
	})

	report, err := runs.ReadUsage(root, runs.UsageQuery{
		Since: "2026-01-01", Until: "2026-01-04", Timezone: "UTC",
		Now: usageDate(t, "2026-01-10 12:00", utc),
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(report.Days) != 3 {
		t.Fatalf("days = %d, want 3: %+v", len(report.Days), report.Days)
	}
	if report.Days[0].Date != "2026-01-01" || report.Days[1].Date != "2026-01-02" || report.Days[2].Date != "2026-01-03" {
		t.Fatalf("dates = %+v", report.Days)
	}

	first := dayByDate(t, report, "2026-01-01")
	if first.Executions != 2 || first.ReportedExecutions != 2 {
		t.Fatalf("first day = %+v, want the midnight-crossing run on its start day", first)
	}
	if first.Tokens.InputTokens != 30 {
		t.Fatalf("first day input = %d, want 30 (the 4th is excluded)", first.Tokens.InputTokens)
	}

	empty := dayByDate(t, report, "2026-01-02")
	if empty.Executions != 0 || empty.Unmeasured || empty.Incomplete {
		t.Fatalf("empty day = %+v, want a genuine zero with no flags", empty)
	}

	// Every day's component total is the sum of the components it reports.
	for _, day := range report.Days {
		want := day.Tokens.InputTokens + day.Tokens.OutputTokens + day.Tokens.ReasoningTokens +
			day.Tokens.CacheReadTokens + day.Tokens.CacheWriteTokens
		if day.Tokens.TotalTokens != want {
			t.Fatalf("%s total = %d, components sum to %d", day.Date, day.Tokens.TotalTokens, want)
		}
	}

	// The period is the sum of the days, not a second pass over the records.
	sum := sumDayTokens(report.Days)
	if report.Period.Tokens != sum {
		t.Fatalf("period tokens = %+v, day sum = %+v", report.Period.Tokens, sum)
	}
	if report.Period.Executions != 2 || report.Period.ReportedExecutions != 2 {
		t.Fatalf("period = %+v", report.Period)
	}
}

func TestUsageReportDaylightSavingKeepsOneBoundaryPerDay(t *testing.T) {
	root := t.TempDir()
	loc, err := time.LoadLocation("Europe/Berlin")
	if err != nil {
		t.Fatal(err)
	}

	// Spring forward on 2026-03-29 and fall back on 2026-10-25.
	springStart := usageDate(t, "2026-03-28 12:00", loc)
	fallStart := usageDate(t, "2026-10-24 12:00", loc)
	writeUsageFixture(t, root, usageFixture{
		traceID: "trace-dst", agentID: "spring", started: springStart,
		usage: &protocol.Usage{InputTokens: 1, Source: protocol.UsageSourceOpenCodeRunJSON},
	})
	writeUsageFixture(t, root, usageFixture{
		traceID: "trace-dst", agentID: "fall", started: fallStart,
		usage: &protocol.Usage{InputTokens: 2, Source: protocol.UsageSourceOpenCodeRunJSON},
	})

	for _, span := range []struct {
		since, until string
		dates        []string
	}{
		{"2026-03-28", "2026-03-31", []string{"2026-03-28", "2026-03-29", "2026-03-30"}},
		{"2026-10-24", "2026-10-27", []string{"2026-10-24", "2026-10-25", "2026-10-26"}},
	} {
		report, err := runs.ReadUsage(root, runs.UsageQuery{
			Since: span.since, Until: span.until, Timezone: "Europe/Berlin",
			Now: usageDate(t, "2026-11-01 12:00", loc),
		})
		if err != nil {
			t.Fatal(err)
		}
		if len(report.Days) != len(span.dates) {
			t.Fatalf("%s..%s days = %d, want %d: %+v", span.since, span.until, len(report.Days), len(span.dates), report.Days)
		}
		for i, date := range span.dates {
			if report.Days[i].Date != date {
				t.Fatalf("%s..%s day %d = %q, want %q", span.since, span.until, i, report.Days[i].Date, date)
			}
		}
	}

	spring, err := runs.ReadUsage(root, runs.UsageQuery{
		Since: "2026-03-28", Until: "2026-03-31", Timezone: "Europe/Berlin",
		Now: usageDate(t, "2026-11-01 12:00", loc),
	})
	if err != nil {
		t.Fatal(err)
	}
	if got := dayByDate(t, spring, "2026-03-28").Executions; got != 1 {
		t.Fatalf("spring run executions = %d, want 1", got)
	}
	if spring.Period.Executions != 1 {
		t.Fatalf("spring period executions = %d, want 1 (no duplicated day)", spring.Period.Executions)
	}
}

func TestUsageReportMissingIsNotZeroAndInFlightMarksTheDayIncomplete(t *testing.T) {
	root := t.TempDir()
	utc := time.UTC
	writeUsageFixture(t, root, usageFixture{
		traceID: "trace-state", agentID: "silent", started: usageDate(t, "2026-04-01 09:00", utc),
	})
	writeUsageFixture(t, root, usageFixture{
		traceID: "trace-state", agentID: "inflight", started: usageDate(t, "2026-04-02 09:00", utc),
		state: protocol.StatusRunning,
	})

	report, err := runs.ReadUsage(root, runs.UsageQuery{
		Since: "2026-04-01", Until: "2026-04-04", Timezone: "UTC",
		Now: usageDate(t, "2026-04-05 12:00", utc),
	})
	if err != nil {
		t.Fatal(err)
	}

	silent := dayByDate(t, report, "2026-04-01")
	if silent.Executions != 1 || silent.ReportedExecutions != 0 {
		t.Fatalf("silent day = %+v", silent)
	}
	if !silent.Unmeasured || silent.Incomplete {
		t.Fatalf("silent day flags = unmeasured:%v incomplete:%v", silent.Unmeasured, silent.Incomplete)
	}
	if silent.Tokens.TotalTokens != 0 {
		t.Fatalf("silent day invented %d tokens", silent.Tokens.TotalTokens)
	}

	inflight := dayByDate(t, report, "2026-04-02")
	if !inflight.Unmeasured || !inflight.Incomplete {
		t.Fatalf("in-flight day flags = unmeasured:%v incomplete:%v", inflight.Unmeasured, inflight.Incomplete)
	}
	if inflight.Tokens.TotalTokens != 0 {
		t.Fatalf("in-flight day invented %d tokens", inflight.Tokens.TotalTokens)
	}

	quiet := dayByDate(t, report, "2026-04-03")
	if quiet.Unmeasured {
		t.Fatalf("a day with no executions must not be unmeasured: %+v", quiet)
	}
}

func TestUsageReportWarnsAboutUnreadableRunsInsteadOfDroppingThem(t *testing.T) {
	root := t.TempDir()
	utc := time.UTC
	writeUsageFixture(t, root, usageFixture{
		traceID: "trace-readable", agentID: "ok", started: usageDate(t, "2026-05-01 09:00", utc),
		usage: &protocol.Usage{InputTokens: 5, Source: protocol.UsageSourceOpenCodeRunJSON},
	})
	broken := filepath.Join(root, ".paseka", "runs", "trace-readable", "broken")
	if err := os.MkdirAll(broken, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(broken, runs.RequestFileName), []byte("{not json"), 0o644); err != nil {
		t.Fatal(err)
	}

	report, err := runs.ReadUsage(root, runs.UsageQuery{
		Since: "2026-05-01", Until: "2026-05-02", Timezone: "UTC",
		Now: usageDate(t, "2026-05-02 12:00", utc),
	})
	if err != nil {
		t.Fatal(err)
	}
	if report.Period.Executions != 1 {
		t.Fatalf("executions = %d, want the readable run still counted", report.Period.Executions)
	}
	found := false
	for _, warning := range report.Warnings {
		if warning.Group == "runs" && warning.Kind == runs.WarningUnreadable {
			found = true
		}
	}
	if !found {
		t.Fatalf("warnings = %+v, want an unreadable-run warning", report.Warnings)
	}
}

func TestUsageReportExcludesUndatedWithAWarning(t *testing.T) {
	root := t.TempDir()
	utc := time.UTC
	writeUsageFixture(t, root, usageFixture{
		traceID: "trace-undated", agentID: "no-start", noStart: true,
		usage: &protocol.Usage{InputTokens: 42, Source: protocol.UsageSourceOpenCodeRunJSON},
	})

	report, err := runs.ReadUsage(root, runs.UsageQuery{
		Since: "2026-06-01", Until: "2026-06-02", Timezone: "UTC",
		Now: usageDate(t, "2026-06-02 12:00", utc),
	})
	if err != nil {
		t.Fatal(err)
	}
	if report.ExcludedUndated != 1 {
		t.Fatalf("excludedUndated = %d, want 1", report.ExcludedUndated)
	}
	if report.Period.Executions != 0 || report.Period.Tokens.TotalTokens != 0 {
		t.Fatalf("undated run was guessed into the range: %+v", report.Period)
	}
	if len(report.Warnings) == 0 {
		t.Fatal("expected a warning for the undated execution")
	}
}

func TestUsageReportWarnsWhenRangePredatesRetainedRecords(t *testing.T) {
	root := t.TempDir()
	utc := time.UTC
	writeUsageFixture(t, root, usageFixture{
		traceID: "trace-retained", agentID: "run", started: usageDate(t, "2026-07-10 09:00", utc),
		usage: &protocol.Usage{InputTokens: 5, Source: protocol.UsageSourceOpenCodeRunJSON},
	})

	early, err := runs.ReadUsage(root, runs.UsageQuery{
		Since: "2026-07-01", Until: "2026-07-11", Timezone: "UTC",
		Now: usageDate(t, "2026-07-11 12:00", utc),
	})
	if err != nil {
		t.Fatal(err)
	}
	coverage := false
	for _, warning := range early.Warnings {
		if warning.Group == "coverage" {
			coverage = true
		}
	}
	if !coverage {
		t.Fatalf("warnings = %+v, want a coverage warning", early.Warnings)
	}
	if early.EarliestRetainedAt == nil {
		t.Fatal("earliestRetainedAt missing")
	}

	inside, err := runs.ReadUsage(root, runs.UsageQuery{
		Since: "2026-07-10", Until: "2026-07-11", Timezone: "UTC",
		Now: usageDate(t, "2026-07-11 12:00", utc),
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(inside.Warnings) != 0 {
		t.Fatalf("warnings = %+v, want none when the range starts at the retained record", inside.Warnings)
	}
}

func TestUsageReportCountsRunsWhoseWorktreeWasPruned(t *testing.T) {
	root := t.TempDir()
	utc := time.UTC
	// The run records remain; no worktree directory exists under .paseka/worktrees.
	writeUsageFixture(t, root, usageFixture{
		traceID: "trace-pruned", agentID: "run", started: usageDate(t, "2026-08-01 09:00", utc),
		usage: &protocol.Usage{InputTokens: 12, OutputTokens: 3, Source: protocol.UsageSourceOpenCodeRunJSON},
	})

	report, err := runs.ReadUsage(root, runs.UsageQuery{
		Since: "2026-08-01", Until: "2026-08-02", Timezone: "UTC",
		Now: usageDate(t, "2026-08-02 12:00", utc),
	})
	if err != nil {
		t.Fatal(err)
	}
	if report.Period.Executions != 1 || report.Period.DistinctTrails != 1 {
		t.Fatalf("period = %+v, want the retained trail counted", report.Period)
	}
}

func TestUsageReportDefaultRangeIsThirtyDaysEndingNow(t *testing.T) {
	root := t.TempDir()
	utc := time.UTC
	now := usageDate(t, "2026-09-30 15:00", utc)

	report, err := runs.ReadUsage(root, runs.UsageQuery{Timezone: "UTC", Now: now})
	if err != nil {
		t.Fatal(err)
	}
	if len(report.Days) != runs.UsageDefaultDays {
		t.Fatalf("days = %d, want %d", len(report.Days), runs.UsageDefaultDays)
	}
	if report.Days[0].Date != "2026-09-01" || report.Days[len(report.Days)-1].Date != "2026-09-30" {
		t.Fatalf("range = %s..%s", report.Days[0].Date, report.Days[len(report.Days)-1].Date)
	}
}

func TestUsageReportMissingRunsDirectoryIsEmpty(t *testing.T) {
	root := t.TempDir()
	report, err := runs.ReadUsage(root, runs.UsageQuery{
		Since: "2026-10-01", Until: "2026-10-03", Timezone: "UTC",
		Now: usageDate(t, "2026-10-03 12:00", time.UTC),
	})
	if err != nil {
		t.Fatalf("missing runs directory returned an error: %v", err)
	}
	if len(report.Days) != 2 {
		t.Fatalf("days = %d, want 2 zero-filled days", len(report.Days))
	}
	if report.Period.Executions != 0 || report.Period.Tokens.TotalTokens != 0 {
		t.Fatalf("period = %+v, want empty", report.Period)
	}
}

func TestUsageQueryRefusalsAreClientErrors(t *testing.T) {
	root := t.TempDir()
	tests := []struct {
		name  string
		query runs.UsageQuery
	}{
		{"unknown zone", runs.UsageQuery{Timezone: "Mars/Olympus"}},
		{"unparsable since", runs.UsageQuery{Since: "yesterday", Timezone: "UTC"}},
		{"unparsable until", runs.UsageQuery{Until: "2026-13-01", Timezone: "UTC"}},
		{"reversed", runs.UsageQuery{Since: "2026-10-10", Until: "2026-10-01", Timezone: "UTC"}},
		{"same day", runs.UsageQuery{Since: "2026-10-10", Until: "2026-10-10", Timezone: "UTC"}},
		{"over long", runs.UsageQuery{Since: "2025-01-01", Until: "2026-10-01", Timezone: "UTC"}},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			_, err := runs.ReadUsage(root, test.query)
			var usageErr *runs.UsageError
			if !errors.As(err, &usageErr) {
				t.Fatalf("err = %v, want *runs.UsageError", err)
			}
			if strings.TrimSpace(usageErr.Reason) == "" {
				t.Fatal("refusal carries no reason")
			}
		})
	}
}

// The JSON contract deliberately has no price, rate, or estimate field. This
// reads the encoded report rather than the struct so a field added later is
// caught on the wire.
func TestUsageReportJSONCarriesNoPriceFields(t *testing.T) {
	root := t.TempDir()
	report, err := runs.ReadUsage(root, runs.UsageQuery{
		Since: "2026-10-01", Until: "2026-10-02", Timezone: "UTC",
		Now: usageDate(t, "2026-10-02 12:00", time.UTC),
	})
	if err != nil {
		t.Fatal(err)
	}
	data, err := json.Marshal(report)
	if err != nil {
		t.Fatal(err)
	}
	var decoded map[string]any
	if err := json.Unmarshal(data, &decoded); err != nil {
		t.Fatal(err)
	}
	var walk func(prefix string, value any)
	walk = func(prefix string, value any) {
		switch typed := value.(type) {
		case map[string]any:
			for key, child := range typed {
				lower := strings.ToLower(key)
				for _, forbidden := range []string{"price", "rate", "estimate", "currency"} {
					if strings.Contains(lower, forbidden) {
						t.Fatalf("usage report carries %s field %q", forbidden, prefix+key)
					}
				}
				walk(prefix+key+".", child)
			}
		case []any:
			for _, child := range typed {
				walk(prefix, child)
			}
		}
	}
	walk("", decoded)
}

// findAxisEntry returns the one entry matching key and label, or fails.
func findAxisEntry(t *testing.T, entries []runs.UsageAxisEntry, key, label string) runs.UsageAxisEntry {
	t.Helper()
	for _, entry := range entries {
		if entry.Key == key && entry.Label == label {
			return entry
		}
	}
	t.Fatalf("axis has no entry %q/%q: %+v", key, label, entries)
	return runs.UsageAxisEntry{}
}

// assertAxisSums proves one axis is a complete view of the totals beside it.
func assertAxisSums(t *testing.T, name string, entries []runs.UsageAxisEntry, totals runs.UsageTotals) {
	t.Helper()
	var (
		tokens     runs.UsageTokens
		executions int
		cost       float64
		costCount  int
	)
	for _, entry := range entries {
		tokens.InputTokens += entry.Tokens.InputTokens
		tokens.OutputTokens += entry.Tokens.OutputTokens
		tokens.ReasoningTokens += entry.Tokens.ReasoningTokens
		tokens.CacheReadTokens += entry.Tokens.CacheReadTokens
		tokens.CacheWriteTokens += entry.Tokens.CacheWriteTokens
		tokens.TotalTokens += entry.Tokens.TotalTokens
		executions += entry.Executions
		cost += entry.RecordedCost
		costCount += entry.CostReportedExecutions
	}
	if tokens != totals.Tokens {
		t.Fatalf("%s axis tokens = %+v, totals = %+v", name, tokens, totals.Tokens)
	}
	if executions != totals.Executions {
		t.Fatalf("%s axis executions = %d, totals = %d", name, executions, totals.Executions)
	}
	if costCount != totals.CostReportedExecutions || math.Abs(cost-totals.RecordedCost) > 1e-9 {
		t.Fatalf("%s axis cost = %v in %d, totals = %v in %d",
			name, cost, costCount, totals.RecordedCost, totals.CostReportedExecutions)
	}
}

func TestUsageReportBreaksEachDayIntoFourAxesThatSumToTheDay(t *testing.T) {
	root := t.TempDir()
	utc := time.UTC
	base := usageDate(t, "2026-02-10 10:00", utc)

	// A v1 proxy run: the adapter name, no implementation.
	writeUsageFixture(t, root, usageFixture{
		traceID: "trace-axes", agentID: "proxy-v1", started: base,
		bee: "builder", adapter: "opencode",
		model: "anthropic/claude-sonnet", alias: "sonnet", purpose: protocol.PurposeWork,
		usage: &protocol.Usage{InputTokens: 100, OutputTokens: 10, CostUSD: 0.5,
			Source: protocol.UsageSourceOpenCodeRunJSON},
	})
	// The same adapter name on its v2 implementation: a second adapter group,
	// not a merge into the first one.
	writeUsageFixture(t, root, usageFixture{
		traceID: "trace-axes", agentID: "proxy-v2", started: base.Add(time.Minute),
		bee: "guard", adapter: "opencode", implementation: "opencodev2",
		model: "anthropic/claude-sonnet", alias: "sonnet", purpose: protocol.PurposeClosing,
		usage: &protocol.Usage{InputTokens: 50, CostUSD: 0.25,
			Source: protocol.UsageSourceOpenCodeRunJSON},
	})
	// An implementation equal to the name is not a second group.
	writeUsageFixture(t, root, usageFixture{
		traceID: "trace-axes", agentID: "same-version", started: base.Add(2 * time.Minute),
		bee: "builder", adapter: "opencode", implementation: "opencode",
		model: "openai/gpt-5", purpose: protocol.PurposeAnalysis,
		usage: &protocol.Usage{InputTokens: 30, Source: protocol.UsageSourceOpenCodeRunJSON},
	})
	// A legacy record with no adapter, Bee, model, or purpose: every axis names
	// it rather than dropping it.
	writeUsageFixture(t, root, usageFixture{
		traceID: "trace-axes", agentID: "legacy", started: base.Add(3 * time.Minute),
		blankIdentity: true,
		usage:         &protocol.Usage{InputTokens: 7, Source: protocol.UsageSourceCursorStreamJSON},
	})

	report, err := runs.ReadUsage(root, runs.UsageQuery{
		Since: "2026-02-10", Until: "2026-02-11", Timezone: "UTC",
		Now: usageDate(t, "2026-02-11 12:00", utc),
	})
	if err != nil {
		t.Fatal(err)
	}
	day := dayByDate(t, report, "2026-02-10")

	// The adapter axis keeps the implementation distinct from the name.
	if len(day.Breakdown.Adapter) != 3 {
		t.Fatalf("adapter entries = %+v, want three (v1, v2, unknown)", day.Breakdown.Adapter)
	}
	v1 := findAxisEntry(t, day.Breakdown.Adapter, "opencode", "")
	if v1.Executions != 2 {
		t.Fatalf("v1 adapter executions = %d, want the run whose implementation equals its name folded in", v1.Executions)
	}
	v2 := findAxisEntry(t, day.Breakdown.Adapter, "opencode", "opencodev2")
	if v2.Tokens.InputTokens != 50 || v2.Executions != 1 {
		t.Fatalf("v2 adapter entry = %+v", v2)
	}
	unknownAdapter := findAxisEntry(t, day.Breakdown.Adapter, runs.UsageUnknownKey, "")
	if unknownAdapter.Tokens.InputTokens != 7 {
		t.Fatalf("unknown adapter = %+v", unknownAdapter)
	}
	// The ranking puts the largest volume first.
	if day.Breakdown.Adapter[0].Key != "opencode" || day.Breakdown.Adapter[0].Label != "" {
		t.Fatalf("adapter ranking = %+v, want the v1 group first", day.Breakdown.Adapter)
	}

	// The model axis groups by the resolved id and shows the alias beside it.
	if len(day.Breakdown.Model) != 3 {
		t.Fatalf("model entries = %+v, want two models and unknown", day.Breakdown.Model)
	}
	sonnet := findAxisEntry(t, day.Breakdown.Model, "anthropic/claude-sonnet", "sonnet")
	if sonnet.Executions != 2 || sonnet.Tokens.InputTokens != 150 {
		t.Fatalf("sonnet model entry = %+v, want both aliased runs grouped by resolved id", sonnet)
	}
	findAxisEntry(t, day.Breakdown.Model, "openai/gpt-5", "")
	findAxisEntry(t, day.Breakdown.Model, runs.UsageUnknownKey, "")

	// Bee and purpose name their unknown groups too.
	findAxisEntry(t, day.Breakdown.Bee, "builder", "")
	findAxisEntry(t, day.Breakdown.Bee, "guard", "")
	findAxisEntry(t, day.Breakdown.Bee, runs.UsageUnknownKey, "")
	findAxisEntry(t, day.Breakdown.Purpose, "work", "")
	findAxisEntry(t, day.Breakdown.Purpose, "closing", "")
	findAxisEntry(t, day.Breakdown.Purpose, "analysis", "")
	findAxisEntry(t, day.Breakdown.Purpose, runs.UsageUnknownKey, "")

	assertAxisSums(t, "adapter", day.Breakdown.Adapter, day.UsageTotals)
	assertAxisSums(t, "model", day.Breakdown.Model, day.UsageTotals)
	assertAxisSums(t, "bee", day.Breakdown.Bee, day.UsageTotals)
	assertAxisSums(t, "purpose", day.Breakdown.Purpose, day.UsageTotals)
}

func TestUsageReportPeriodRankingIsTheSumOfItsDays(t *testing.T) {
	root := t.TempDir()
	utc := time.UTC
	writeUsageFixture(t, root, usageFixture{
		traceID: "trace-period", agentID: "day-1", started: usageDate(t, "2026-02-10 10:00", utc),
		bee: "builder", adapter: "opencode", model: "openai/gpt-5", purpose: protocol.PurposeWork,
		usage: &protocol.Usage{InputTokens: 100, CostUSD: 0.1, Source: protocol.UsageSourceOpenCodeRunJSON},
	})
	writeUsageFixture(t, root, usageFixture{
		traceID: "trace-period", agentID: "day-2", started: usageDate(t, "2026-02-11 10:00", utc),
		bee: "builder", adapter: "opencode", model: "openai/gpt-5", purpose: protocol.PurposeWork,
		usage: &protocol.Usage{InputTokens: 40, CostUSD: 0.2, Source: protocol.UsageSourceOpenCodeRunJSON},
	})

	report, err := runs.ReadUsage(root, runs.UsageQuery{
		Since: "2026-02-10", Until: "2026-02-12", Timezone: "UTC",
		Now: usageDate(t, "2026-02-12 12:00", utc),
	})
	if err != nil {
		t.Fatal(err)
	}

	model := findAxisEntry(t, report.Breakdown.Model, "openai/gpt-5", "")
	if model.Executions != 2 || model.Tokens.InputTokens != 140 {
		t.Fatalf("period model entry = %+v, want both days summed", model)
	}
	if model.RecordedCost < 0.2999 || model.RecordedCost > 0.3001 || model.CostReportedExecutions != 2 {
		t.Fatalf("period model cost = %v in %d", model.RecordedCost, model.CostReportedExecutions)
	}
	assertAxisSums(t, "adapter", report.Breakdown.Adapter, report.Period)
	assertAxisSums(t, "model", report.Breakdown.Model, report.Period)
	assertAxisSums(t, "bee", report.Breakdown.Bee, report.Period)
	assertAxisSums(t, "purpose", report.Breakdown.Purpose, report.Period)

	// And each period axis total is the sum of that axis over the days.
	var periodTotal int64
	for _, day := range report.Days {
		for _, entry := range day.Breakdown.Model {
			periodTotal += entry.Tokens.TotalTokens
		}
	}
	if periodTotal != report.Period.Tokens.TotalTokens {
		t.Fatalf("days sum to %d, period = %d", periodTotal, report.Period.Tokens.TotalTokens)
	}
}

func TestUsageReportLegacyPurposeIsUnknownNotWork(t *testing.T) {
	root := t.TempDir()
	utc := time.UTC
	writeUsageFixture(t, root, usageFixture{
		traceID: "trace-legacy-purpose", agentID: "run",
		started: usageDate(t, "2026-02-10 10:00", utc),
		usage:   &protocol.Usage{InputTokens: 5, Source: protocol.UsageSourceOpenCodeRunJSON},
	})
	report, err := runs.ReadUsage(root, runs.UsageQuery{
		Since: "2026-02-10", Until: "2026-02-11", Timezone: "UTC",
		Now: usageDate(t, "2026-02-11 12:00", utc),
	})
	if err != nil {
		t.Fatal(err)
	}
	day := dayByDate(t, report, "2026-02-10")
	findAxisEntry(t, day.Breakdown.Purpose, runs.UsageUnknownKey, "")
	for _, entry := range day.Breakdown.Purpose {
		if entry.Key == "work" {
			t.Fatalf("legacy record was labelled work: %+v", entry)
		}
	}
}

// The top-trails table is a period ranking over reported tokens. A silent trail
// is absent rather than a zero row, the remainder is stated so the shown rows
// plus it are the period total, and only the shown rows pay for title reads.
func TestUsageReportRanksTopTrailsByReportedTokens(t *testing.T) {
	root := t.TempDir()
	utc := time.UTC
	base := usageDate(t, "2026-02-10 10:00", utc)

	// A titled trail at the top, with two executions and a recorded cost.
	titled := writeUsageFixture(t, root, usageFixture{
		traceID: "trace-top", agentID: "run-1", started: base,
		usage: &protocol.Usage{InputTokens: 500, OutputTokens: 50, CostUSD: 1.25,
			Source: protocol.UsageSourceOpenCodeRunJSON},
	})
	writeUsageFixture(t, root, usageFixture{
		traceID: "trace-top", agentID: "run-2", started: base.Add(time.Minute),
		usage: &protocol.Usage{InputTokens: 250, CostUSD: 0.75,
			Source: protocol.UsageSourceOpenCodeRunJSON},
	})
	title, err := protocol.NewEvent("trace-top", "scout", 1, protocol.EventInsight, protocol.TraceTitlePayload{
		Kind:  protocol.InsightTraceTitle,
		Title: "Top spending trail",
	})
	if err != nil {
		t.Fatal(err)
	}
	if err := titled.AppendEvent(title); err != nil {
		t.Fatal(err)
	}

	// Enough further measured trails to push past the carried rows.
	for i := 1; i <= runs.UsageTrailLimit; i++ {
		input := int64(100 - i)
		writeUsageFixture(t, root, usageFixture{
			traceID: fmt.Sprintf("trace-measured-%02d", i), agentID: "run",
			started: base.Add(time.Duration(i+1) * time.Minute),
			usage:   &protocol.Usage{InputTokens: input, Source: protocol.UsageSourceOpenCodeRunJSON},
		})
	}

	// Neither of these may appear: a finished run without usage is a coverage
	// gap, and an in-flight run is unfinished rather than free.
	writeUsageFixture(t, root, usageFixture{
		traceID: "trace-silent", agentID: "run", started: base.Add(30 * time.Minute),
	})
	writeUsageFixture(t, root, usageFixture{
		traceID: "trace-inflight", agentID: "run", started: base.Add(31 * time.Minute),
		state: protocol.StatusRunning,
		usage: &protocol.Usage{InputTokens: 9999, Source: protocol.UsageSourceOpenCodeRunJSON},
	})

	report, err := runs.ReadUsage(root, runs.UsageQuery{
		Since: "2026-02-10", Until: "2026-02-11", Timezone: "UTC",
		Now: usageDate(t, "2026-02-11 12:00", utc),
	})
	if err != nil {
		t.Fatal(err)
	}

	ranking := report.Trails
	if ranking.MeasuredTrails != runs.UsageTrailLimit+1 {
		t.Fatalf("measured trails = %d, want %d from a silent and an in-flight trail excluded",
			ranking.MeasuredTrails, runs.UsageTrailLimit+1)
	}
	if len(ranking.Entries) != runs.UsageTrailLimit {
		t.Fatalf("entries = %d, want the carried top %d", len(ranking.Entries), runs.UsageTrailLimit)
	}
	if ranking.OutsideTrails != 1 || ranking.OutsideTokens == 0 {
		t.Fatalf("remainder = %d trails / %d tokens, want the one row outside the table",
			ranking.OutsideTrails, ranking.OutsideTokens)
	}

	first := ranking.Entries[0]
	if first.TraceID != "trace-top" || first.Title != "Top spending trail" {
		t.Fatalf("top entry identity = %q/%q, want the titled top trail", first.TraceID, first.Title)
	}
	if first.Executions != 2 || first.ReportedExecutions != 2 || first.Tokens.TotalTokens != 800 {
		t.Fatalf("top entry usage = %+v, want both executions summed", first)
	}
	if math.Abs(first.RecordedCost-2.0) > 1e-9 || first.CostReportedExecutions != 2 {
		t.Fatalf("top entry cost = %v in %d", first.RecordedCost, first.CostReportedExecutions)
	}

	// The ranking is largest first, and the remainder reconciles with the
	// period total the headline reports.
	for i := 1; i < len(ranking.Entries); i++ {
		if ranking.Entries[i-1].Tokens.TotalTokens < ranking.Entries[i].Tokens.TotalTokens {
			t.Fatalf("entries are not ranked by reported tokens: %+v", ranking.Entries)
		}
	}
	var shown int64
	for _, entry := range ranking.Entries {
		shown += entry.Tokens.TotalTokens
		if entry.TraceID == "trace-silent" || entry.TraceID == "trace-inflight" {
			t.Fatalf("a trail without reported usage was ranked: %+v", entry)
		}
	}
	if shown+ranking.OutsideTokens != report.Period.Tokens.TotalTokens {
		t.Fatalf("shown %d + outside %d != period %d", shown, ranking.OutsideTokens, report.Period.Tokens.TotalTokens)
	}
}

func TestUsageReportTopTrailsHandleEmptyAndSingleTrailPeriods(t *testing.T) {
	root := t.TempDir()
	utc := time.UTC

	empty, err := runs.ReadUsage(root, runs.UsageQuery{
		Since: "2026-02-10", Until: "2026-02-11", Timezone: "UTC",
		Now: usageDate(t, "2026-02-11 12:00", utc),
	})
	if err != nil {
		t.Fatal(err)
	}
	if empty.Trails.MeasuredTrails != 0 || empty.Trails.OutsideTrails != 0 ||
		empty.Trails.OutsideTokens != 0 || len(empty.Trails.Entries) != 0 {
		t.Fatalf("empty ranking = %+v, want no rows and no remainder", empty.Trails)
	}
	if empty.Trails.Entries == nil {
		t.Fatal("empty ranking encodes entries as null instead of an empty list")
	}

	writeUsageFixture(t, root, usageFixture{
		traceID: "trace-only", agentID: "run", started: usageDate(t, "2026-02-10 09:00", utc),
		usage: &protocol.Usage{InputTokens: 40, Source: protocol.UsageSourceOpenCodeRunJSON},
	})
	single, err := runs.ReadUsage(root, runs.UsageQuery{
		Since: "2026-02-10", Until: "2026-02-11", Timezone: "UTC",
		Now: usageDate(t, "2026-02-11 12:00", utc),
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(single.Trails.Entries) != 1 || single.Trails.OutsideTrails != 0 ||
		single.Trails.MeasuredTrails != 1 {
		t.Fatalf("single-trail ranking = %+v", single.Trails)
	}
	row := single.Trails.Entries[0]
	// No title is recorded, so the report leaves Title empty and the caller
	// falls back to the id rather than inventing a name.
	if row.TraceID != "trace-only" || row.Title != "" {
		t.Fatalf("single row = %+v", row)
	}
	if row.Tokens.TotalTokens != 40 || row.Executions != 1 || row.ReportedExecutions != 1 {
		t.Fatalf("single row usage = %+v", row)
	}
}
