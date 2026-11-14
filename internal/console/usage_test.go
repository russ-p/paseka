package console_test

import (
	"encoding/json"
	"io"
	"io/fs"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"testing"
	"time"

	"github.com/russ-p/paseka/internal/console"
	"github.com/russ-p/paseka/internal/protocol"
	"github.com/russ-p/paseka/internal/runs"
)

func writeUsageConsoleRun(t *testing.T, root, traceID, agentID string, started time.Time, usage *protocol.Usage) {
	t.Helper()
	writeUsageConsoleRunFor(t, root, traceID, agentID, started, "builder", "opencode", "anthropic/claude-sonnet", protocol.PurposeWork, usage)
}

// writeUsageConsoleRunFor writes one execution with the identity fields a
// filter test varies.
func writeUsageConsoleRunFor(t *testing.T, root, traceID, agentID string, started time.Time, bee, adapter, model string, purpose protocol.ExecutionPurpose, usage *protocol.Usage) {
	t.Helper()
	d := runs.Dir{ColonyRoot: root, TraceID: traceID, AgentID: agentID}
	if err := d.Prepare(); err != nil {
		t.Fatal(err)
	}
	if err := d.WriteRequest(protocol.Request{
		ProtocolVersion: protocol.Version,
		TraceID:         traceID,
		AgentID:         agentID,
		Bee:             bee,
		Adapter:         adapter,
		Workspace:       root,
		ColonyRoot:      root,
		Model:           model,
		ModelAlias:      "sonnet",
		Purpose:         purpose,
		CreatedAt:       started,
	}); err != nil {
		t.Fatal(err)
	}
	if err := d.WriteStatusSnapshot(protocol.StatusSnapshot{
		ProtocolVersion: protocol.Version,
		State:           protocol.StatusCompleted,
		StartedAt:       started,
		FinishedAt:      started.Add(time.Minute),
	}); err != nil {
		t.Fatal(err)
	}
	if err := d.WriteResult(protocol.Result{
		ProtocolVersion: protocol.Version,
		TraceID:         traceID,
		AgentID:         agentID,
		Status:          protocol.StatusCompleted,
		Summary:         "done",
		Usage:           usage,
		FinishedAt:      started.Add(time.Minute),
	}); err != nil {
		t.Fatal(err)
	}
}

func usageAPIRequest(t *testing.T, srv *console.Server, target string) *httptest.ResponseRecorder {
	t.Helper()
	rec := httptest.NewRecorder()
	srv.Handler().ServeHTTP(rec, httptest.NewRequest(http.MethodGet, target, nil))
	return rec
}

func usageAPIReport(t *testing.T, srv *console.Server, target string) runs.UsageReport {
	t.Helper()
	rec := usageAPIRequest(t, srv, target)
	if rec.Code != http.StatusOK {
		t.Fatalf("%s: status = %d body=%s", target, rec.Code, rec.Body.String())
	}
	var report runs.UsageReport
	if err := json.NewDecoder(rec.Body).Decode(&report); err != nil {
		t.Fatal(err)
	}
	return report
}

func TestUsageAPIReturnsTheColonyReport(t *testing.T) {
	repo := initConsoleRepo(t)
	started := time.Date(2026, 3, 2, 10, 0, 0, 0, time.UTC)
	writeUsageConsoleRun(t, repo, "trace-usage", "run-1", started, &protocol.Usage{
		InputTokens: 1200, OutputTokens: 300, ReasoningTokens: 40,
		CacheReadTokens: 500, CacheWriteTokens: 20, CostUSD: 0.25,
		Source: protocol.UsageSourceOpenCodeRunJSON,
	})
	srv := console.NewServer(console.Options{Addr: "127.0.0.1:0", Colony: setupConsoleHome(t, repo)})

	rec := usageAPIRequest(t, srv, "/api/usage?since=2026-03-01&until=2026-03-03&timezone=UTC")
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d body=%s", rec.Code, rec.Body.String())
	}
	var report runs.UsageReport
	if err := json.NewDecoder(rec.Body).Decode(&report); err != nil {
		t.Fatal(err)
	}
	if report.Timezone != "UTC" {
		t.Fatalf("timezone = %q, want UTC", report.Timezone)
	}
	if report.Since != "2026-03-01T00:00:00Z" || report.Until != "2026-03-03T00:00:00Z" {
		t.Fatalf("range echo = %s..%s", report.Since, report.Until)
	}
	if len(report.Days) != 2 {
		t.Fatalf("days = %d, want 2", len(report.Days))
	}
	if report.Period.Executions != 1 || report.Period.ReportedExecutions != 1 {
		t.Fatalf("period = %+v", report.Period)
	}
	if report.Period.Tokens.InputTokens != 1200 || report.Period.Tokens.TotalTokens != 2060 {
		t.Fatalf("tokens = %+v", report.Period.Tokens)
	}
	if report.Period.RecordedCost != 0.25 || report.Period.CostReportedExecutions != 1 {
		t.Fatalf("recorded cost = %v in %d executions", report.Period.RecordedCost, report.Period.CostReportedExecutions)
	}
	// The period's top trails come from the same records as the days, so the
	// page can link a trail without a second aggregation.
	if report.Trails.MeasuredTrails != 1 || len(report.Trails.Entries) != 1 {
		t.Fatalf("trail ranking = %+v, want the one measured trail", report.Trails)
	}
	if report.Trails.Entries[0].TraceID != "trace-usage" ||
		report.Trails.Entries[0].Tokens.TotalTokens != 2060 ||
		report.Trails.Entries[0].ReportedExecutions != 1 {
		t.Fatalf("trail row = %+v", report.Trails.Entries[0])
	}
	// Every axis is in the same payload, so the page switches axes without a
	// second read; each sums to the totals beside it.
	day := report.Days[1]
	for name, entries := range map[string][]runs.UsageAxisEntry{
		"adapter": day.Breakdown.Adapter,
		"model":   day.Breakdown.Model,
		"bee":     day.Breakdown.Bee,
		"purpose": day.Breakdown.Purpose,
	} {
		var tokens int64
		var executions int
		for _, entry := range entries {
			tokens += entry.Tokens.TotalTokens
			executions += entry.Executions
		}
		if tokens != day.Tokens.TotalTokens || executions != day.Executions {
			t.Fatalf("%s axis = %d tokens / %d executions, day = %d / %d",
				name, tokens, executions, day.Tokens.TotalTokens, day.Executions)
		}
	}
	if len(report.Breakdown.Adapter) != 1 || report.Breakdown.Adapter[0].Key != "opencode" {
		t.Fatalf("period adapter ranking = %+v", report.Breakdown.Adapter)
	}
	if len(report.Breakdown.Model) != 1 || report.Breakdown.Model[0].Label != "sonnet" {
		t.Fatalf("period model ranking = %+v, want the alias beside the resolved id", report.Breakdown.Model)
	}
}

// The API carries the counts a page needs to say what its totals could not see:
// the executions it could not date and the records it could not open.
func TestUsageAPICarriesExcludedUndatedAndUnreadableCoverage(t *testing.T) {
	repo := initConsoleRepo(t)
	writeUsageConsoleRun(t, repo, "trace-dated", "run-1",
		time.Date(2026, 3, 2, 10, 0, 0, 0, time.UTC),
		&protocol.Usage{InputTokens: 5, Source: protocol.UsageSourceOpenCodeRunJSON})
	writeUsageConsoleRun(t, repo, "trace-undated", "run-2", time.Time{},
		&protocol.Usage{InputTokens: 7, Source: protocol.UsageSourceOpenCodeRunJSON})
	broken := filepath.Join(repo, ".paseka", "runs", "trace-dated", "broken")
	if err := os.MkdirAll(broken, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(broken, runs.RequestFileName), []byte("{not json"), 0o644); err != nil {
		t.Fatal(err)
	}
	srv := console.NewServer(console.Options{Addr: "127.0.0.1:0", Colony: setupConsoleHome(t, repo)})

	report := usageAPIReport(t, srv, "/api/usage?since=2026-03-01&until=2026-03-03&timezone=UTC")
	if report.ExcludedUndated != 1 {
		t.Fatalf("excludedUndated = %d, want the undated execution counted", report.ExcludedUndated)
	}
	// The unreadable record is not in the execution count, and the rest of the
	// report still renders.
	if report.Period.Executions != 1 || report.Period.ReportedExecutions != 1 {
		t.Fatalf("period = %+v, want only the readable execution covered", report.Period)
	}
	found := false
	for _, warning := range report.Warnings {
		if warning.Group == "runs" && warning.Kind == runs.WarningUnreadable {
			found = true
		}
	}
	if !found {
		t.Fatalf("warnings = %+v, want the unreadable record named", report.Warnings)
	}
}

// The API is read-only: it creates no file and appends no event. The snapshot
// covers the whole repository so a read that materializes a cache beside the
// runs fails here too.
func TestUsageAPIReadChangesNoFile(t *testing.T) {
	repo := initConsoleRepo(t)
	started := time.Date(2026, 3, 2, 10, 0, 0, 0, time.UTC)
	writeUsageConsoleRun(t, repo, "trace-readonly", "run-1", started, &protocol.Usage{
		InputTokens: 10, Source: protocol.UsageSourceOpenCodeRunJSON,
	})
	srv := console.NewServer(console.Options{Addr: "127.0.0.1:0", Colony: setupConsoleHome(t, repo)})

	before := snapshotTree(t, repo)
	rec := usageAPIRequest(t, srv, "/api/usage?since=2026-03-01&until=2026-03-03&timezone=UTC")
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d body=%s", rec.Code, rec.Body.String())
	}
	// The explicit refresh is a second path through the same reader; neither
	// may leave a cache file or an aggregate behind.
	rec = usageAPIRequest(t, srv, "/api/usage?since=2026-03-01&until=2026-03-03&timezone=UTC&refresh=1")
	if rec.Code != http.StatusOK {
		t.Fatalf("refresh status = %d body=%s", rec.Code, rec.Body.String())
	}
	after := snapshotTree(t, repo)

	if strings.Join(before, "\n") != strings.Join(after, "\n") {
		t.Fatalf("a usage read changed the tree\nbefore:\n%s\nafter:\n%s",
			strings.Join(before, "\n"), strings.Join(after, "\n"))
	}
}

func snapshotTree(t *testing.T, root string) []string {
	t.Helper()
	var entries []string
	err := filepath.WalkDir(root, func(path string, entry fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		rel, err := filepath.Rel(root, path)
		if err != nil {
			return err
		}
		if entry.IsDir() {
			entries = append(entries, "dir "+rel)
			return nil
		}
		data, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		entries = append(entries, "file "+rel+" "+string(data))
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	sort.Strings(entries)
	return entries
}

func TestUsageAPIRejectsBadRangesWithAReason(t *testing.T) {
	repo := initConsoleRepo(t)
	srv := console.NewServer(console.Options{Addr: "127.0.0.1:0", Colony: setupConsoleHome(t, repo)})

	tests := []struct {
		name   string
		target string
	}{
		{"unknown zone", "/api/usage?timezone=Mars/Olympus"},
		{"unparsable since", "/api/usage?since=yesterday&timezone=UTC"},
		{"unparsable until", "/api/usage?until=2026-02-30&timezone=UTC"},
		{"reversed", "/api/usage?since=2026-03-10&until=2026-03-01&timezone=UTC"},
		{"over long", "/api/usage?since=2024-01-01&until=2026-03-01&timezone=UTC"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			rec := usageAPIRequest(t, srv, test.target)
			if rec.Code != http.StatusBadRequest {
				t.Fatalf("status = %d body=%s, want 400", rec.Code, rec.Body.String())
			}
			if got := rec.Header().Get("Content-Type"); !strings.HasPrefix(got, "text/plain") {
				t.Fatalf("Content-Type = %q, want plain text", got)
			}
			if strings.TrimSpace(rec.Body.String()) == "" {
				t.Fatal("400 carries no reason")
			}
		})
	}
}

func TestUsageAPIMissingRunsDirectoryIsAnEmptyReport(t *testing.T) {
	repo := initConsoleRepo(t)
	srv := console.NewServer(console.Options{Addr: "127.0.0.1:0", Colony: setupConsoleHome(t, repo)})

	rec := usageAPIRequest(t, srv, "/api/usage?since=2026-03-01&until=2026-03-03&timezone=UTC")
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d body=%s", rec.Code, rec.Body.String())
	}
	var report runs.UsageReport
	if err := json.NewDecoder(rec.Body).Decode(&report); err != nil {
		t.Fatal(err)
	}
	if len(report.Days) != 2 || report.Period.Executions != 0 {
		t.Fatalf("report = %+v, want two zero-filled days and no executions", report)
	}
}

func TestUsageAPIReusesTheScanUntilRefreshBypassesIt(t *testing.T) {
	repo := initConsoleRepo(t)
	started := time.Date(2026, 3, 2, 10, 0, 0, 0, time.UTC)
	writeUsageConsoleRun(t, repo, "trace-first", "run-1", started, &protocol.Usage{
		InputTokens: 100, Source: protocol.UsageSourceOpenCodeRunJSON,
	})
	srv := console.NewServer(console.Options{Addr: "127.0.0.1:0", Colony: setupConsoleHome(t, repo)})
	target := "/api/usage?since=2026-03-01&until=2026-03-03&timezone=UTC"

	first := usageAPIReport(t, srv, target)
	if first.Period.Executions != 1 {
		t.Fatalf("first read = %+v, want the one run", first.Period)
	}

	// A run that finished since the scan does not appear inside the cache
	// window; the repeated read is the earlier observation, observedAt included.
	writeUsageConsoleRun(t, repo, "trace-second", "run-1", started.Add(time.Minute), &protocol.Usage{
		InputTokens: 200, Source: protocol.UsageSourceOpenCodeRunJSON,
	})
	cached := usageAPIReport(t, srv, target)
	if cached.Period.Executions != 1 || cached.Period.Tokens.InputTokens != 100 {
		t.Fatalf("cached read = %+v, want the first scan", cached.Period)
	}
	if !cached.ObservedAt.Equal(first.ObservedAt) {
		t.Fatalf("cached observedAt = %s, want %s", cached.ObservedAt, first.ObservedAt)
	}

	// The refresh control's query is the bypass: the new run is visible and the
	// report says it was observed later.
	fresh := usageAPIReport(t, srv, target+"&refresh=1")
	if fresh.Period.Executions != 2 || fresh.Period.Tokens.InputTokens != 300 {
		t.Fatalf("refreshed read = %+v, want both runs", fresh.Period)
	}
	if !fresh.ObservedAt.After(first.ObservedAt) {
		t.Fatalf("refreshed observedAt = %s, want later than %s", fresh.ObservedAt, first.ObservedAt)
	}
}

func TestUsageAPIAcceptsRepeatedFilters(t *testing.T) {
	repo := initConsoleRepo(t)
	started := time.Date(2026, 3, 2, 10, 0, 0, 0, time.UTC)
	writeUsageConsoleRunFor(t, repo, "trace-opencode", "run-1", started,
		"builder", "opencode", "anthropic/claude-sonnet", protocol.PurposeWork,
		&protocol.Usage{InputTokens: 100, Source: protocol.UsageSourceOpenCodeRunJSON})
	writeUsageConsoleRunFor(t, repo, "trace-cursor", "run-1", started.Add(time.Minute),
		"guard", "cursor", "anthropic/claude-sonnet", protocol.PurposeClosing,
		&protocol.Usage{InputTokens: 200, Source: protocol.UsageSourceCursorStreamJSON})
	writeUsageConsoleRunFor(t, repo, "trace-compact", "run-1", started.Add(2*time.Minute),
		"builder", "compact", "openai/gpt-5", protocol.PurposeWork,
		&protocol.Usage{InputTokens: 50, Source: protocol.UsageSourceOpenCodeRunJSON})
	srv := console.NewServer(console.Options{Addr: "127.0.0.1:0", Colony: setupConsoleHome(t, repo)})

	read := func(target string) runs.UsageReport {
		t.Helper()
		rec := usageAPIRequest(t, srv, target)
		if rec.Code != http.StatusOK {
			t.Fatalf("%s: status = %d body=%s", target, rec.Code, rec.Body.String())
		}
		var report runs.UsageReport
		if err := json.NewDecoder(rec.Body).Decode(&report); err != nil {
			t.Fatal(err)
		}
		return report
	}

	// Repeated values are one filter set on that axis: both adapters pass, the
	// third does not, and the range is the one that was asked for.
	both := read("/api/usage?since=2026-03-01&until=2026-03-03&timezone=UTC&adapter=opencode&adapter=cursor")
	if both.Period.Executions != 2 || both.Period.Tokens.InputTokens != 300 {
		t.Fatalf("period = %+v, want the two named adapters", both.Period)
	}
	if len(both.Days) != 2 || both.Since != "2026-03-01T00:00:00Z" || both.Until != "2026-03-03T00:00:00Z" {
		t.Fatalf("range = %s..%s days=%d, want the requested range unchanged", both.Since, both.Until, len(both.Days))
	}
	if len(both.Filters.Adapter) != 2 || both.Filters.Adapter[0] != "cursor" || both.Filters.Adapter[1] != "opencode" {
		t.Fatalf("filter echo = %+v, want the normalized set", both.Filters.Adapter)
	}
	if len(both.Breakdown.Adapter) != 2 || len(both.Trails.Entries) != 2 {
		t.Fatalf("breakdown/trails = %+v / %+v, want only the matching records", both.Breakdown.Adapter, both.Trails)
	}

	// Different axes combine.
	narrow := read("/api/usage?since=2026-03-01&until=2026-03-03&timezone=UTC&adapter=opencode&bee=builder&purpose=work")
	if narrow.Period.Executions != 1 || narrow.Period.Tokens.InputTokens != 100 {
		t.Fatalf("period = %+v, want the one opencode/builder/work run", narrow.Period)
	}
	if len(narrow.Filters.Bee) != 1 || narrow.Filters.Bee[0] != "builder" {
		t.Fatalf("bee echo = %+v", narrow.Filters.Bee)
	}

	// A filter that matches nothing answers a full, zero-filled report rather
	// than an error: the empty view is the page's to explain.
	empty := read("/api/usage?since=2026-03-01&until=2026-03-03&timezone=UTC&model=never-ran")
	if empty.Period.Executions != 0 || len(empty.Days) != 2 {
		t.Fatalf("empty report = %+v days=%d", empty.Period, len(empty.Days))
	}
	if len(empty.Filters.Model) != 1 || empty.Filters.Model[0] != "never-ran" {
		t.Fatalf("empty filter echo = %+v", empty.Filters)
	}
}

func TestUsageAPIReturnsNoPriceOrEstimateField(t *testing.T) {
	repo := initConsoleRepo(t)
	started := time.Date(2026, 3, 2, 10, 0, 0, 0, time.UTC)
	writeUsageConsoleRun(t, repo, "trace-price", "run-1", started, &protocol.Usage{
		InputTokens: 10, CostUSD: 0.5, Source: protocol.UsageSourceOpenCodeRunJSON,
	})
	srv := console.NewServer(console.Options{Addr: "127.0.0.1:0", Colony: setupConsoleHome(t, repo)})

	rec := usageAPIRequest(t, srv, "/api/usage?since=2026-03-01&until=2026-03-03&timezone=UTC")
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d body=%s", rec.Code, rec.Body.String())
	}
	body, err := io.ReadAll(rec.Body)
	if err != nil {
		t.Fatal(err)
	}
	var decoded map[string]any
	if err := json.Unmarshal(body, &decoded); err != nil {
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
						t.Fatalf("usage response carries %s field %q", forbidden, prefix+key)
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
	if !strings.Contains(string(body), `"recordedCostUsd":0.5`) {
		t.Fatalf("recorded cost is absent from the report: %s", body)
	}
}

func TestUsageAPIRejectsOtherMethods(t *testing.T) {
	repo := initConsoleRepo(t)
	srv := console.NewServer(console.Options{Addr: "127.0.0.1:0", Colony: setupConsoleHome(t, repo)})

	rec := httptest.NewRecorder()
	srv.Handler().ServeHTTP(rec, httptest.NewRequest(http.MethodPost, "/api/usage", nil))
	if rec.Code != http.StatusMethodNotAllowed {
		t.Fatalf("status = %d, want 405", rec.Code)
	}
}
