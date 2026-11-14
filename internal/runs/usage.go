package runs

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/russ-p/paseka/internal/protocol"
)

// Usage report constants. The range is bounded so one request cannot become an
// unbounded walk of every run directory a colony ever wrote, and the default
// answers "this month" without asking a caller to know that.
const (
	// UsageMaxDays is the longest range a usage report will build.
	UsageMaxDays = 366
	// UsageDefaultDays is the range used when a caller names no since date.
	UsageDefaultDays = 30
)

// Measurement states of one execution's recorded usage.
const (
	// UsageReported means the execution finished and left a readable usage record.
	UsageReported = "reported"
	// UsageMissing means the execution finished without a usage record. It is
	// unmeasured, never a zero-token run.
	UsageMissing = "missing"
	// UsageUnknown means the execution is in flight or its outcome record is not
	// readable yet. It is unfinished, not free.
	UsageUnknown = "unknown"
)

// usagePurposeUnknown is the reader-side bucket for an execution whose purpose
// was never recorded. It is not work, closing, or analysis by inference.
const usagePurposeUnknown = "unknown"

// UsageQuery selects one colony-wide usage report.
//
// Since and Until are local calendar dates (YYYY-MM-DD) in Timezone. Since is
// inclusive and Until is exclusive, both local midnights. An empty Since means
// UsageDefaultDays ending now; an empty Until means now. Timezone is an IANA
// zone name; empty uses the host's local zone. Now is injectable so a report is
// reproducible in a test; zero means time.Now().UTC().
type UsageQuery struct {
	Since    string
	Until    string
	Timezone string
	Now      time.Time
}

// UsageError is a request the reader refuses with a reason a caller can show.
// The console answers it as a 400 rather than treating a typo as an empty
// report.
type UsageError struct {
	Reason string
}

func (e *UsageError) Error() string { return e.Reason }

func usageErrorf(format string, args ...any) error {
	return &UsageError{Reason: fmt.Sprintf(format, args...)}
}

// UsageTokens is the component split recorded for one execution, a day, or a
// range. Components stay separate: reasoning is never folded into output, and
// cache kinds are never folded into input, because providers disagree about
// what those fields already contain.
//
// TotalTokens is the sum of the components this record reported. It is not
// presented as billable tokens.
type UsageTokens struct {
	InputTokens      int64 `json:"inputTokens"`
	OutputTokens     int64 `json:"outputTokens"`
	ReasoningTokens  int64 `json:"reasoningTokens"`
	CacheReadTokens  int64 `json:"cacheReadTokens"`
	CacheWriteTokens int64 `json:"cacheWriteTokens"`
	TotalTokens      int64 `json:"totalTokens"`
}

func (t *UsageTokens) add(u *protocol.Usage) {
	if u == nil {
		return
	}
	t.InputTokens += u.InputTokens
	t.OutputTokens += u.OutputTokens
	t.ReasoningTokens += u.ReasoningTokens
	t.CacheReadTokens += u.CacheReadTokens
	t.CacheWriteTokens += u.CacheWriteTokens
	t.recomputeTotal()
}

func (t *UsageTokens) addTokens(other UsageTokens) {
	t.InputTokens += other.InputTokens
	t.OutputTokens += other.OutputTokens
	t.ReasoningTokens += other.ReasoningTokens
	t.CacheReadTokens += other.CacheReadTokens
	t.CacheWriteTokens += other.CacheWriteTokens
	t.recomputeTotal()
}

func (t *UsageTokens) recomputeTotal() {
	t.TotalTokens = t.InputTokens + t.OutputTokens + t.ReasoningTokens +
		t.CacheReadTokens + t.CacheWriteTokens
}

// UsageRecord is one retained execution, read through the existing run
// projection. It is the unit the daily buckets count: one execution directory
// produces one record, and a resumed session is a new directory and therefore a
// new record.
type UsageRecord struct {
	TraceID        string      `json:"traceId"`
	RunID          string      `json:"runId"`
	Bee            string      `json:"bee,omitempty"`
	Adapter        string      `json:"adapter,omitempty"`
	Implementation string      `json:"implementation,omitempty"`
	Model          string      `json:"model,omitempty"`
	ModelAlias     string      `json:"modelAlias,omitempty"`
	Purpose        string      `json:"purpose"`
	StartedAt      time.Time   `json:"startedAt,omitempty"`
	Measurement    string      `json:"measurement"`
	Tokens         UsageTokens `json:"tokens"`
	RecordedCost   float64     `json:"recordedCostUsd,omitempty"`
	CostReported   bool        `json:"costReported"`
}

// UsageWarning is one reason a usage report is partial. It names the group it
// is about, the kind of gap, and what was found.
type UsageWarning struct {
	Group  string `json:"group"`
	Kind   string `json:"kind"`
	Detail string `json:"detail"`
}

// UsageScan is the retained-record read before time bucketing: every execution
// record the reader could project, plus what it could not read.
type UsageScan struct {
	Records  []UsageRecord
	Warnings []UsageWarning
}

// UsageTotals is a component split plus the coverage that makes it honest:
// how many executions it covers, how many reported usage, and how many
// executions recorded a provider cost.
type UsageTotals struct {
	Executions             int         `json:"executions"`
	ReportedExecutions     int         `json:"reportedExecutions"`
	DistinctTrails         int         `json:"distinctTrails"`
	Tokens                 UsageTokens `json:"tokens"`
	RecordedCost           float64     `json:"recordedCostUsd"`
	CostReportedExecutions int         `json:"costReportedExecutions"`
}

// UsageDay is one zero-filled local calendar day in the requested range.
type UsageDay struct {
	Date string `json:"date"`
	UsageTotals
	// Unmeasured distinguishes "executions, none of which reported usage"
	// from a day with no executions at all.
	Unmeasured bool `json:"unmeasured"`
	// Incomplete marks a day with at least one execution whose outcome is not
	// final yet.
	Incomplete bool `json:"incomplete"`
}

// UsageReport is the colony-wide observation. Period totals and the day list
// are derived from the same records, and the period is summed from the days so
// a headline cannot disagree with the table under it.
type UsageReport struct {
	Since              string         `json:"since"`
	Until              string         `json:"until"`
	Timezone           string         `json:"timezone"`
	ObservedAt         time.Time      `json:"observedAt"`
	EarliestRetainedAt *time.Time     `json:"earliestRetainedAt,omitempty"`
	Period             UsageTotals    `json:"period"`
	Days               []UsageDay     `json:"days"`
	ExcludedUndated    int            `json:"excludedUndated"`
	Warnings           []UsageWarning `json:"warnings"`
}

// ScanUsageRecords walks .paseka/runs and projects every retained execution
// carrying a request record. Non-execution directories inside a trail are
// skipped, an unreadable execution warns rather than disappearing, and a
// missing runs directory is an empty scan rather than an error.
func ScanUsageRecords(colonyRoot string) (UsageScan, error) {
	scan := UsageScan{Records: []UsageRecord{}, Warnings: []UsageWarning{}}
	runsRoot := filepath.Join(colonyRoot, ".paseka", "runs")
	traceDirs, err := os.ReadDir(runsRoot)
	if err != nil {
		if os.IsNotExist(err) {
			return scan, nil
		}
		return scan, err
	}

	for _, traceEntry := range traceDirs {
		if !traceEntry.IsDir() {
			continue
		}
		traceID := traceEntry.Name()
		tracePath := filepath.Join(runsRoot, traceID)
		agentDirs, err := os.ReadDir(tracePath)
		if err != nil {
			scan.warn("runs", WarningUnreadable, fmt.Sprintf("trail %s: %v", traceID, err))
			continue
		}
		for _, agentEntry := range agentDirs {
			if !agentEntry.IsDir() || IsReservedTraceSubdir(agentEntry.Name()) {
				continue
			}
			d := Dir{
				ColonyRoot: colonyRoot,
				TraceID:    traceID,
				AgentID:    agentEntry.Name(),
			}
			// A directory without a request record is not a retained execution:
			// it may be a half-prepared launch. It contributes no record and no
			// warning, because there is no execution to be honest about yet.
			if !fileExists(d.RequestPath()) {
				continue
			}
			meta, err := LoadRunMeta(d)
			if err != nil {
				scan.warn("runs", WarningUnreadable,
					fmt.Sprintf("run %s/%s: %v", traceID, agentEntry.Name(), err))
				continue
			}
			scan.Records = append(scan.Records, newUsageRecord(meta))
		}
	}

	sort.SliceStable(scan.Records, func(i, j int) bool {
		a, b := scan.Records[i], scan.Records[j]
		if !a.StartedAt.Equal(b.StartedAt) {
			return a.StartedAt.Before(b.StartedAt)
		}
		if a.TraceID != b.TraceID {
			return a.TraceID < b.TraceID
		}
		return a.RunID < b.RunID
	})
	sortUsageWarnings(scan.Warnings)
	return scan, nil
}

func (s *UsageScan) warn(group, kind, detail string) {
	s.Warnings = append(s.Warnings, UsageWarning{Group: group, Kind: kind, Detail: detail})
}

func newUsageRecord(meta RunMeta) UsageRecord {
	rec := UsageRecord{
		TraceID:        meta.TraceID,
		RunID:          meta.AgentID,
		Bee:            meta.Bee,
		Adapter:        meta.Adapter,
		Implementation: meta.Implementation,
		Model:          meta.Model,
		ModelAlias:     meta.ModelAlias,
		Purpose:        usagePurposeKey(meta.Purpose),
		StartedAt:      meta.StartedAt,
		Measurement:    usageMeasurement(meta),
	}
	if rec.Measurement == UsageReported {
		rec.Tokens.add(meta.Usage)
		// The protocol carries no "cost was reported" flag. A non-zero value is
		// the only evidence that a provider recorded money, and an absent value
		// is left out of the count rather than summed as a free execution.
		if meta.Usage != nil && meta.Usage.CostUSD != 0 {
			rec.RecordedCost = meta.Usage.CostUSD
			rec.CostReported = true
		}
	}
	return rec
}

// usageMeasurement decides what a retained execution can honestly say about its
// usage. A finished run without a readable measurement is missing, never zero;
// an unfinished run or an outcome pair a reader cannot prove is unknown.
func usageMeasurement(meta RunMeta) string {
	if !usageTerminal(meta.State) {
		return UsageUnknown
	}
	if meta.Usage == nil || meta.Usage.Source == protocol.UsageSourceUnavailable {
		if meta.Records != nil {
			return UsageUnknown
		}
		return UsageMissing
	}
	return UsageReported
}

func usageTerminal(state string) bool {
	switch protocol.RunStatus(state) {
	case protocol.StatusCompleted, protocol.StatusFailed, protocol.StatusCancelled:
		return true
	}
	return false
}

func usagePurposeKey(purpose protocol.ExecutionPurpose) string {
	if protocol.IsKnownExecutionPurpose(purpose) {
		return string(purpose)
	}
	return usagePurposeUnknown
}

// ReadUsage builds the colony-wide report for one range. It reads retained run
// records only: it changes no file, publishes no event, and needs no bus.
func ReadUsage(colonyRoot string, q UsageQuery) (UsageReport, error) {
	now := q.Now
	if now.IsZero() {
		now = time.Now().UTC()
	}
	loc, zoneName, err := usageLocation(q.Timezone)
	if err != nil {
		return UsageReport{}, err
	}
	since, until, days, err := usageRange(q, now, loc)
	if err != nil {
		return UsageReport{}, err
	}

	scan, err := ScanUsageRecords(colonyRoot)
	if err != nil {
		return UsageReport{}, err
	}

	report := UsageReport{
		Since:      since.Format(time.RFC3339),
		Until:      until.Format(time.RFC3339),
		Timezone:   zoneName,
		ObservedAt: now.UTC(),
		Days:       []UsageDay{},
		Warnings:   append([]UsageWarning{}, scan.Warnings...),
	}
	indexByDate := make(map[string]int, len(days))
	dayTrails := make([]map[string]struct{}, len(days))
	for i, day := range days {
		date := day.Format("2006-01-02")
		indexByDate[date] = i
		report.Days = append(report.Days, UsageDay{Date: date})
		dayTrails[i] = map[string]struct{}{}
	}

	var earliest time.Time
	for _, rec := range scan.Records {
		if rec.StartedAt.IsZero() {
			report.ExcludedUndated++
			continue
		}
		if earliest.IsZero() || rec.StartedAt.Before(earliest) {
			earliest = rec.StartedAt
		}
		// A record is placed by its start time, in whole: a run crossing
		// midnight lands entirely on its start day rather than inventing a
		// distribution the record does not contain.
		date := rec.StartedAt.In(loc).Format("2006-01-02")
		idx, ok := indexByDate[date]
		if !ok {
			continue
		}
		day := &report.Days[idx]
		day.Executions++
		dayTrails[idx][rec.TraceID] = struct{}{}
		switch rec.Measurement {
		case UsageReported:
			day.ReportedExecutions++
			day.Tokens.addTokens(rec.Tokens)
			if rec.CostReported {
				day.RecordedCost += rec.RecordedCost
				day.CostReportedExecutions++
			}
		case UsageUnknown:
			day.Incomplete = true
		}
	}

	for i := range report.Days {
		day := &report.Days[i]
		day.DistinctTrails = len(dayTrails[i])
		day.Unmeasured = day.Executions > 0 && day.ReportedExecutions == 0
	}

	// The period is the sum of the days rather than a second pass over the
	// records, so a headline cannot disagree with the table beneath it.
	periodTrails := map[string]struct{}{}
	for i := range report.Days {
		day := report.Days[i]
		report.Period.Executions += day.Executions
		report.Period.ReportedExecutions += day.ReportedExecutions
		report.Period.Tokens.addTokens(day.Tokens)
		report.Period.RecordedCost += day.RecordedCost
		report.Period.CostReportedExecutions += day.CostReportedExecutions
		for trail := range dayTrails[i] {
			periodTrails[trail] = struct{}{}
		}
	}
	report.Period.DistinctTrails = len(periodTrails)

	if !earliest.IsZero() {
		utc := earliest.UTC()
		report.EarliestRetainedAt = &utc
		// Only the days before the retained record's own day are unmeasured:
		// a range that starts at the retained run's local midnight has no whole
		// earlier day to warn about.
		if startOfLocalDay(earliest.In(loc), loc).After(since) {
			report.Warnings = append(report.Warnings, UsageWarning{
				Group:  "coverage",
				Kind:   WarningMissing,
				Detail: fmt.Sprintf("range begins before the earliest retained run (%s); earlier days are not measured", utc.Format(time.RFC3339)),
			})
		}
	}
	if report.ExcludedUndated > 0 {
		report.Warnings = append(report.Warnings, UsageWarning{
			Group:  "range",
			Kind:   WarningMissing,
			Detail: fmt.Sprintf("%d execution(s) have no usable start time and are excluded from the daily buckets", report.ExcludedUndated),
		})
	}
	sortUsageWarnings(report.Warnings)
	if report.Warnings == nil {
		report.Warnings = []UsageWarning{}
	}
	return report, nil
}

func usageLocation(name string) (*time.Location, string, error) {
	name = strings.TrimSpace(name)
	if name == "" {
		return time.Local, time.Local.String(), nil
	}
	loc, err := time.LoadLocation(name)
	if err != nil {
		return nil, "", usageErrorf("unknown timezone %q", name)
	}
	return loc, loc.String(), nil
}

func usageRange(q UsageQuery, now time.Time, loc *time.Location) (time.Time, time.Time, []time.Time, error) {
	since, err := parseUsageDate("since", q.Since, loc)
	if err != nil {
		return time.Time{}, time.Time{}, nil, err
	}
	until, err := parseUsageDate("until", q.Until, loc)
	if err != nil {
		return time.Time{}, time.Time{}, nil, err
	}
	if since.IsZero() {
		today := startOfLocalDay(now.In(loc), loc)
		since = today.AddDate(0, 0, -(UsageDefaultDays - 1))
	}
	if until.IsZero() {
		until = now
	}
	if !until.After(since) {
		return time.Time{}, time.Time{}, nil, usageErrorf("until %s must be after since %s",
			until.Format(time.RFC3339), since.Format(time.RFC3339))
	}

	days := make([]time.Time, 0, UsageDefaultDays)
	for day := since; day.Before(until); day = day.AddDate(0, 0, 1) {
		if len(days) == UsageMaxDays {
			return time.Time{}, time.Time{}, nil, usageErrorf("range is longer than %d days", UsageMaxDays)
		}
		days = append(days, day)
	}
	if len(days) == 0 {
		return time.Time{}, time.Time{}, nil, usageErrorf("range contains no days")
	}
	return since, until, days, nil
}

func parseUsageDate(field, value string, loc *time.Location) (time.Time, error) {
	value = strings.TrimSpace(value)
	if value == "" {
		return time.Time{}, nil
	}
	parsed, err := time.ParseInLocation("2006-01-02", value, loc)
	if err != nil {
		return time.Time{}, usageErrorf("%s %q is not a date (want YYYY-MM-DD)", field, value)
	}
	return parsed, nil
}

// startOfLocalDay returns the first instant of t's local calendar day. Asking
// for the date explicitly is what keeps a daylight-saving transition from
// shifting or duplicating a boundary.
func startOfLocalDay(t time.Time, loc *time.Location) time.Time {
	return time.Date(t.Year(), t.Month(), t.Day(), 0, 0, 0, 0, loc)
}

func sortUsageWarnings(warnings []UsageWarning) {
	sort.SliceStable(warnings, func(i, j int) bool {
		if warnings[i].Group != warnings[j].Group {
			return warnings[i].Group < warnings[j].Group
		}
		if warnings[i].Kind != warnings[j].Kind {
			return warnings[i].Kind < warnings[j].Kind
		}
		return warnings[i].Detail < warnings[j].Detail
	})
}
