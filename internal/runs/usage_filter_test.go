package runs_test

import (
	"encoding/json"
	"reflect"
	"testing"
	"time"

	"github.com/russ-p/paseka/internal/protocol"
	"github.com/russ-p/paseka/internal/runs"
)

// usageFilterFixture writes one colony whose four axes cross each other, so a
// filter can be proved to select a subset rather than one convenient row.
func usageFilterFixture(t *testing.T) string {
	t.Helper()
	root := t.TempDir()
	utc := time.UTC
	workDay := usageDate(t, "2026-03-02 10:00", utc)

	// An earlier legacy record: no adapter, model, Bee, or purpose recorded.
	// It is outside every filter below except `unknown`, and it is the retained
	// record's start.
	writeUsageFixture(t, root, usageFixture{
		traceID: "trace-legacy", agentID: "run-legacy",
		started:       usageDate(t, "2026-02-20 09:00", utc),
		blankIdentity: true,
		usage:         &protocol.Usage{InputTokens: 7, Source: protocol.UsageSourceCursorStreamJSON},
	})

	writeUsageFixture(t, root, usageFixture{
		traceID: "trace-app", agentID: "run-build-v1", started: workDay,
		bee: "builder", adapter: "opencode",
		model: "anthropic/claude-sonnet", alias: "sonnet", purpose: protocol.PurposeWork,
		usage: &protocol.Usage{InputTokens: 100, CostUSD: 0.5, Source: protocol.UsageSourceOpenCodeRunJSON},
	})
	writeUsageFixture(t, root, usageFixture{
		traceID: "trace-app", agentID: "run-build-v2", started: workDay.Add(time.Minute),
		bee: "builder", adapter: "opencode",
		model: "openai/gpt-5", purpose: protocol.PurposeWork,
		usage: &protocol.Usage{InputTokens: 40, Source: protocol.UsageSourceOpenCodeRunJSON},
	})
	writeUsageFixture(t, root, usageFixture{
		traceID: "trace-guard", agentID: "run-guard", started: workDay.Add(2 * time.Minute),
		bee: "guard", adapter: "cursor",
		model: "anthropic/claude-sonnet", alias: "sonnet", purpose: protocol.PurposeClosing,
		usage: &protocol.Usage{InputTokens: 200, CostUSD: 1.5, Source: protocol.UsageSourceCursorStreamJSON},
	})
	writeUsageFixture(t, root, usageFixture{
		traceID: "trace-app", agentID: "run-analysis",
		started: usageDate(t, "2026-03-03 10:00", utc),
		bee:     "builder", adapter: "opencode",
		model: "anthropic/claude-sonnet", alias: "sonnet", purpose: protocol.PurposeAnalysis,
		usage: &protocol.Usage{InputTokens: 10, Source: protocol.UsageSourceOpenCodeRunJSON},
	})
	// Undated, and matching the opencode/builder filters: it must be counted as
	// an excluded execution rather than dropped or guessed into a day.
	writeUsageFixture(t, root, usageFixture{
		traceID: "trace-app", agentID: "run-undated", noStart: true,
		bee: "builder", adapter: "opencode", purpose: protocol.PurposeWork,
		usage: &protocol.Usage{InputTokens: 3, Source: protocol.UsageSourceOpenCodeRunJSON},
	})
	return root
}

// readUsageFilterReport asks for the whole fixture window; each test varies only
// the filter set.
func readUsageFilterReport(t *testing.T, root string, query runs.UsageQuery) runs.UsageReport {
	t.Helper()
	query.Since = "2026-02-20"
	query.Until = "2026-03-04"
	query.Timezone = "UTC"
	query.Now = usageDate(t, "2026-03-04 12:00", time.UTC)
	report, err := runs.ReadUsage(root, query)
	if err != nil {
		t.Fatal(err)
	}
	return report
}

func TestUsageReportAppliesRepeatedFiltersBeforeAggregation(t *testing.T) {
	root := usageFilterFixture(t)
	report := readUsageFilterReport(t, root, runs.UsageQuery{
		Adapters: []string{"opencode", "cursor"},
	})

	// Both repeated values are alternatives: every dated opencode and cursor
	// execution is counted, and the unknown-adapter legacy run is not.
	if report.Period.Executions != 4 || report.Period.ReportedExecutions != 4 {
		t.Fatalf("period = %+v, want the four dated matching executions", report.Period)
	}
	if report.Period.Tokens.InputTokens != 350 || report.Period.Tokens.TotalTokens != 350 {
		t.Fatalf("period tokens = %+v, want 100+40+200+10", report.Period.Tokens)
	}
	if report.Period.DistinctTrails != 2 {
		t.Fatalf("distinct trails = %d, want trace-app and trace-guard", report.Period.DistinctTrails)
	}
	// The undated record matches the filter, so it stays an honest exclusion.
	if report.ExcludedUndated != 1 {
		t.Fatalf("excludedUndated = %d, want the matching undated execution", report.ExcludedUndated)
	}

	// The range and day boundaries are the requested ones, not the filtered
	// subset's: twelve zero-filled days from 20 February to 3 March.
	if len(report.Days) != 12 {
		t.Fatalf("days = %d, want the unchanged twelve-day range", len(report.Days))
	}
	if report.Days[0].Date != "2026-02-20" || report.Days[11].Date != "2026-03-03" {
		t.Fatalf("range = %s..%s", report.Days[0].Date, report.Days[11].Date)
	}
	if got := dayByDate(t, report, "2026-02-20").Executions; got != 0 {
		t.Fatalf("legacy day executions = %d, want it excluded by the adapter filter", got)
	}
	if got := dayByDate(t, report, "2026-03-02").Executions; got != 3 {
		t.Fatalf("work day executions = %d, want the two opencode runs and the cursor run", got)
	}

	// Every axis is the filtered set and sums to the filtered total.
	for name, entries := range map[string][]runs.UsageAxisEntry{
		"adapter": report.Breakdown.Adapter,
		"model":   report.Breakdown.Model,
		"bee":     report.Breakdown.Bee,
		"purpose": report.Breakdown.Purpose,
	} {
		assertAxisSums(t, name, entries, report.Period)
	}
	if entries := report.Breakdown.Adapter; len(entries) != 2 {
		t.Fatalf("adapter axis = %+v, want exactly opencode and cursor", entries)
	}
	if entries := report.Breakdown.Purpose; len(entries) != 3 {
		t.Fatalf("purpose axis = %+v, want exactly work, closing, and analysis", entries)
	}

	// The top-trails ranking is the same filtered set: the legacy trail is not a
	// zero row, and the shown rows reconcile with the period.
	if report.Trails.MeasuredTrails != 2 || len(report.Trails.Entries) != 2 {
		t.Fatalf("trails = %+v, want the two matching trails", report.Trails)
	}
	var shown int64
	for _, entry := range report.Trails.Entries {
		shown += entry.Tokens.TotalTokens
		if entry.TraceID == "trace-legacy" {
			t.Fatalf("a filtered-out trail was ranked: %+v", entry)
		}
	}
	if report.Trails.Entries[0].TraceID != "trace-guard" {
		t.Fatalf("trail ranking = %+v, want the largest first", report.Trails.Entries)
	}
	if shown+report.Trails.OutsideTokens != report.Period.Tokens.TotalTokens {
		t.Fatalf("shown %d != period %d", shown, report.Period.Tokens.TotalTokens)
	}

	// The period is still the sum of the days under it.
	if sum := sumDayTokens(report.Days); sum != report.Period.Tokens {
		t.Fatalf("period tokens = %+v, day sum = %+v", report.Period.Tokens, sum)
	}
	// And the echo is the set that was applied.
	if !reflect.DeepEqual(report.Filters.Adapter, []string{"cursor", "opencode"}) {
		t.Fatalf("filter echo = %+v", report.Filters)
	}
}

func TestUsageReportCombinesAxesAndNarrowsToOneRecord(t *testing.T) {
	root := usageFilterFixture(t)
	report := readUsageFilterReport(t, root, runs.UsageQuery{
		Adapters: []string{"opencode"},
		Purposes: []string{"work"},
	})

	// Only the two work runs of the opencode adapter survive; the closing run,
	// the analysis run, and the legacy record are all excluded.
	if report.Period.Executions != 2 || report.Period.Tokens.InputTokens != 140 {
		t.Fatalf("period = %+v, want the two matching work executions", report.Period)
	}
	if got := dayByDate(t, report, "2026-03-02").Executions; got != 2 {
		t.Fatalf("03-02 executions = %d, want the two work runs", got)
	}
	if got := dayByDate(t, report, "2026-03-03").Executions; got != 0 {
		t.Fatalf("03-03 executions = %d, want the analysis run excluded by purpose", got)
	}
	if len(report.Breakdown.Purpose) != 1 || report.Breakdown.Purpose[0].Key != "work" {
		t.Fatalf("purpose axis = %+v, want work only", report.Breakdown.Purpose)
	}
	if len(report.Breakdown.Bee) != 1 || report.Breakdown.Bee[0].Key != "builder" {
		t.Fatalf("bee axis = %+v", report.Breakdown.Bee)
	}
	if len(report.Breakdown.Model) != 2 {
		t.Fatalf("model axis = %+v, want both work models", report.Breakdown.Model)
	}
}

func TestUsageReportFilterMatchesTheUnknownGroup(t *testing.T) {
	root := usageFilterFixture(t)

	byModel := readUsageFilterReport(t, root, runs.UsageQuery{Models: []string{runs.UsageUnknownKey}})
	if byModel.Period.Executions != 1 || byModel.Period.Tokens.InputTokens != 7 {
		t.Fatalf("unknown-model report = %+v, want the legacy run only", byModel.Period)
	}
	if entry := findAxisEntry(t, byModel.Breakdown.Model, runs.UsageUnknownKey, ""); entry.Tokens.InputTokens != 7 {
		t.Fatalf("unknown model entry = %+v", entry)
	}

	byAdapter := readUsageFilterReport(t, root, runs.UsageQuery{Adapters: []string{runs.UsageUnknownKey}})
	if byAdapter.Period.Executions != 1 || byAdapter.Period.Tokens.InputTokens != 7 {
		t.Fatalf("unknown-adapter report = %+v, want the legacy run only", byAdapter.Period)
	}

	byBee := readUsageFilterReport(t, root, runs.UsageQuery{Bees: []string{"builder"}})
	if byBee.Period.Executions != 3 || byBee.Period.Tokens.InputTokens != 150 {
		t.Fatalf("builder report = %+v, want the three builder executions", byBee.Period)
	}
}

func TestUsageReportFilterThatMatchesNothingKeepsTheRange(t *testing.T) {
	root := usageFilterFixture(t)
	report := readUsageFilterReport(t, root, runs.UsageQuery{
		Models:   []string{"model-that-never-ran"},
		Purposes: []string{"work"},
	})

	if len(report.Days) != 12 {
		t.Fatalf("days = %d, want the unchanged range", len(report.Days))
	}
	if report.Period.Executions != 0 || report.Period.Tokens.TotalTokens != 0 {
		t.Fatalf("period = %+v, want an empty filtered report", report.Period)
	}
	// The unmatched filter does not turn the matching undated record into an
	// exclusion it never selected.
	if report.ExcludedUndated != 0 {
		t.Fatalf("excludedUndated = %d, want none for an unmatched filter", report.ExcludedUndated)
	}
	for _, day := range report.Days {
		if day.Executions != 0 || day.Unmeasured || day.Incomplete {
			t.Fatalf("day %s = %+v, want a genuine zero", day.Date, day)
		}
		if day.Breakdown.Adapter == nil || day.Breakdown.Model == nil ||
			day.Breakdown.Bee == nil || day.Breakdown.Purpose == nil {
			t.Fatalf("day %s encodes an axis as null", day.Date)
		}
	}
	if report.Trails.Entries == nil {
		t.Fatal("empty trail ranking encodes entries as null")
	}
	if report.Trails.MeasuredTrails != 0 || len(report.Trails.Entries) != 0 {
		t.Fatalf("trails = %+v, want no ranked trail", report.Trails)
	}
	if !reflect.DeepEqual(report.Filters.Model, []string{"model-that-never-ran"}) ||
		!reflect.DeepEqual(report.Filters.Purpose, []string{"work"}) {
		t.Fatalf("filter echo = %+v", report.Filters)
	}
}

func TestUsageReportFilterEchoNormalizesAndNeverMovesRetention(t *testing.T) {
	root := usageFilterFixture(t)
	report := readUsageFilterReport(t, root, runs.UsageQuery{
		Adapters: []string{"  opencode  ", "opencode", ""},
	})

	if !reflect.DeepEqual(report.Filters.Adapter, []string{"opencode"}) {
		t.Fatalf("adapter echo = %+v, want one trimmed deduplicated value", report.Filters.Adapter)
	}
	// The retained record's start describes retention, not the filter, so a
	// filter that excludes the earliest record does not move it.
	if report.EarliestRetainedAt == nil {
		t.Fatal("earliestRetainedAt missing")
	}
	wantEarliest := usageDate(t, "2026-02-20 09:00", time.UTC)
	if !report.EarliestRetainedAt.Equal(wantEarliest) {
		t.Fatalf("earliestRetainedAt = %s, want the scan-wide %s", report.EarliestRetainedAt, wantEarliest)
	}

	// An unfiltered report carries the empty set as empty lists, not nulls, so a
	// client can read the field without a presence test.
	unfiltered := readUsageFilterReport(t, root, runs.UsageQuery{})
	if !unfiltered.Filters.IsEmpty() {
		t.Fatalf("unfiltered echo = %+v", unfiltered.Filters)
	}
	data, err := json.Marshal(unfiltered)
	if err != nil {
		t.Fatal(err)
	}
	var decoded map[string]any
	if err := json.Unmarshal(data, &decoded); err != nil {
		t.Fatal(err)
	}
	filters, ok := decoded["filters"].(map[string]any)
	if !ok {
		t.Fatalf("report carries no filters object: %s", data)
	}
	for _, axis := range []string{"adapter", "model", "bee", "purpose"} {
		if filters[axis] == nil {
			t.Fatalf("filters.%s is null, want an empty list", axis)
		}
	}
}
