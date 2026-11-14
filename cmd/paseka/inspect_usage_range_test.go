package main

import (
	"bytes"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"reflect"
	"sort"
	"strings"
	"testing"
	"time"

	"github.com/russ-p/paseka/internal/colony"
	"github.com/russ-p/paseka/internal/console"
	"github.com/russ-p/paseka/internal/protocol"
	"github.com/russ-p/paseka/internal/runs"
)

func TestInspectUsageRangeTextStatesRangeZoneAndRows(t *testing.T) {
	root := t.TempDir()
	writeInspectRun(t, root, "trace-range", "reported", time.Date(2026, 3, 1, 10, 0, 0, 0, time.UTC), &protocol.Usage{
		InputTokens: 100, OutputTokens: 10, ReasoningTokens: 3,
		CacheReadTokens: 5, CacheWriteTokens: 2, CostUSD: 0.25,
		Source: protocol.UsageSourceOpenCodeRunJSON,
	})
	writeInspectRun(t, root, "trace-range", "silent", time.Date(2026, 3, 2, 10, 0, 0, 0, time.UTC), nil)
	writeInspectRun(t, root, "trace-range", "undated", time.Time{}, &protocol.Usage{
		InputTokens: 7, Source: protocol.UsageSourceOpenCodeRunJSON,
	})

	var buf bytes.Buffer
	err := printUsageRange(&buf, root, runs.UsageQuery{
		Since: "2026-03-01", Until: "2026-03-04", Timezone: "UTC",
	}, false)
	if err != nil {
		t.Fatal(err)
	}
	got := buf.String()

	for _, want := range []string{
		"Usage range: 2026-03-01T00:00:00Z .. 2026-03-04T00:00:00Z\n",
		"Time zone:   UTC\n",
		"Coverage: 2 executions (1 reported, 1 unmeasured) in 1 trail\n",
		"Period tokens:\n",
		"  input:       100\n",
		"  output:      10\n",
		"  reasoning:   3\n",
		"  cache read:  5\n",
		"  cache write: 2\n",
		"  total:       120\n",
		"Period recorded cost: 0.25 USD across 1 execution\n",
		"Days (3):\n",
		"2026-03-01",
		"2026-03-02",
		"unmeasured",
		"Excluded undated: 1 execution has no usable start time\n",
		"Warnings:\n",
	} {
		if !strings.Contains(got, want) {
			t.Fatalf("output missing %q:\n%s", want, got)
		}
	}
	if strings.Contains(dayRow(t, got, "2026-03-03"), "unmeasured") {
		t.Fatalf("an untouched day must read as a genuine zero:\n%s", got)
	}
	if !strings.Contains(dayRow(t, got, "2026-03-03"), "no executions") {
		t.Fatalf("an untouched day must be distinguishable from an unmeasured one:\n%s", got)
	}
	if !strings.Contains(dayRow(t, got, "2026-03-02"), "unmeasured") {
		t.Fatalf("a day whose executions reported nothing must say so:\n%s", got)
	}
}

func TestInspectUsageRangeDefaultsNameTheResolvedRange(t *testing.T) {
	repo := initInspectFixtureRepo(t)

	jsonOut, err := runInspectUsageCLI(t, repo, "--json")
	if err != nil {
		t.Fatalf("json run: %v", err)
	}
	var report runs.UsageReport
	if err := json.Unmarshal([]byte(jsonOut), &report); err != nil {
		t.Fatalf("decode report: %v\n%s", err, jsonOut)
	}
	if len(report.Days) != runs.UsageDefaultDays {
		t.Fatalf("days = %d, want %d", len(report.Days), runs.UsageDefaultDays)
	}
	if strings.TrimSpace(report.Timezone) == "" {
		t.Fatal("the report echoes no timezone")
	}

	textOut, err := runInspectUsageCLI(t, repo)
	if err != nil {
		t.Fatalf("text run: %v", err)
	}
	if !strings.Contains(textOut, "Usage range: "+report.Since+" .. ") {
		t.Fatalf("text does not name the resolved since %q:\n%s", report.Since, textOut)
	}
	if !strings.Contains(textOut, "Time zone:   "+report.Timezone+"\n") {
		t.Fatalf("text does not name the resolved timezone %q:\n%s", report.Timezone, textOut)
	}
	if !strings.Contains(textOut, "Days (30):") {
		t.Fatalf("both range defaults were not applied:\n%s", textOut)
	}
	rangeLine := linePrefix(t, textOut, "Usage range: ")
	parts := strings.Split(strings.TrimPrefix(rangeLine, "Usage range: "), " .. ")
	if len(parts) != 2 {
		t.Fatalf("range line = %q", rangeLine)
	}
	if _, err := time.Parse(time.RFC3339, strings.TrimSpace(parts[1])); err != nil {
		t.Fatalf("resolved until is not named: %q (%v)", parts[1], err)
	}
}

func TestInspectUsageRangeJSONEqualsTheAPIReport(t *testing.T) {
	root := initInspectFixtureRepo(t)
	started := time.Date(2026, 3, 2, 10, 0, 0, 0, time.UTC)
	writeInspectRun(t, root, "trace-api", "run-1", started, &protocol.Usage{
		InputTokens: 1200, OutputTokens: 300, Source: protocol.UsageSourceOpenCodeRunJSON,
	})

	const query = "since=2026-03-01&until=2026-03-03&timezone=UTC"
	srv := console.NewServer(console.Options{
		Addr:   "127.0.0.1:0",
		Colony: colony.Context{ColonyRoot: root},
	})
	rec := httptest.NewRecorder()
	srv.Handler().ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/api/usage?"+query, nil))
	if rec.Code != http.StatusOK {
		t.Fatalf("api status = %d body=%s", rec.Code, rec.Body.String())
	}

	cliOut, err := runInspectUsageCLI(t, root, "--since", "2026-03-01", "--until", "2026-03-03", "--timezone", "UTC", "--json")
	if err != nil {
		t.Fatalf("cli run: %v", err)
	}

	var apiReport, cliReport map[string]any
	if err := json.Unmarshal(rec.Body.Bytes(), &apiReport); err != nil {
		t.Fatal(err)
	}
	if err := json.Unmarshal([]byte(cliOut), &cliReport); err != nil {
		t.Fatalf("decode cli report: %v\n%s", err, cliOut)
	}
	// The two reads happen at different instants; every other field is the
	// same observation of the same fixture.
	delete(apiReport, "observedAt")
	delete(cliReport, "observedAt")

	if !reflect.DeepEqual(fieldSet(apiReport), fieldSet(cliReport)) {
		t.Fatalf("field sets differ: api=%v cli=%v", fieldSet(apiReport), fieldSet(cliReport))
	}
	if cliReport["timezone"] != "UTC" {
		t.Fatalf("cli timezone = %v, want UTC", cliReport["timezone"])
	}
	if !reflect.DeepEqual(apiReport, cliReport) {
		t.Fatalf("cli report differs from the API report\napi: %v\ncli: %v", apiReport, cliReport)
	}
}

func TestInspectUsageRangeTimezoneMovesTheDay(t *testing.T) {
	root := t.TempDir()
	// 02:00 UTC is still the previous evening in New York.
	started := time.Date(2026, 3, 3, 2, 0, 0, 0, time.UTC)
	writeInspectRun(t, root, "trace-zone", "run-1", started, &protocol.Usage{
		InputTokens: 777, Source: protocol.UsageSourceOpenCodeRunJSON,
	})

	query := runs.UsageQuery{Since: "2026-03-02", Until: "2026-03-04"}

	query.Timezone = "UTC"
	utcReport, err := runs.ReadUsage(root, query)
	if err != nil {
		t.Fatal(err)
	}
	query.Timezone = "America/New_York"
	nyReport, err := runs.ReadUsage(root, query)
	if err != nil {
		t.Fatal(err)
	}

	if utcReport.Days[0].Executions != 0 || utcReport.Days[1].Executions != 1 {
		t.Fatalf("UTC days = %+v, want the run on the 3rd", utcReport.Days)
	}
	if nyReport.Days[0].Executions != 1 || nyReport.Days[1].Executions != 0 {
		t.Fatalf("New York days = %+v, want the run on the 2nd", nyReport.Days)
	}

	var utcText, nyText bytes.Buffer
	if err := printUsageReport(&utcText, utcReport); err != nil {
		t.Fatal(err)
	}
	if err := printUsageReport(&nyText, nyReport); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(utcText.String(), "Time zone:   UTC\n") {
		t.Fatalf("UTC text does not echo its zone:\n%s", utcText.String())
	}
	if !strings.Contains(nyText.String(), "Time zone:   America/New_York\n") {
		t.Fatalf("New York text does not echo its zone:\n%s", nyText.String())
	}
	if strings.Contains(dayRow(t, utcText.String(), "2026-03-02"), "777") {
		t.Fatalf("UTC text placed the run on the 2nd:\n%s", utcText.String())
	}
	if !strings.Contains(dayRow(t, nyText.String(), "2026-03-02"), "777") {
		t.Fatalf("New York text did not place the run on the 2nd:\n%s", nyText.String())
	}
}

func TestInspectUsageRangeRefusalsExitNonZeroAndPrintNoReport(t *testing.T) {
	repo := initInspectFixtureRepo(t)

	tests := []struct {
		name string
		args []string
	}{
		{"unknown zone", []string{"--timezone", "Mars/Olympus"}},
		{"unparsable since", []string{"--since", "yesterday"}},
		{"unparsable until", []string{"--until", "2026-02-30"}},
		{"reversed", []string{"--since", "2026-03-10", "--until", "2026-03-01"}},
		{"same day", []string{"--since", "2026-03-10", "--until", "2026-03-10"}},
		{"over long", []string{"--since", "2024-01-01", "--until", "2026-03-01"}},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			args := append([]string{"inspect", "usage", "-C", repo}, test.args...)
			root := newRoot()
			var stdout, stderr bytes.Buffer
			root.SetOut(&stdout)
			root.SetErr(&stderr)
			root.SetArgs(args)
			err := root.Execute()
			if err == nil {
				t.Fatalf("execute succeeded, want a refusal")
			}
			var usageErr *runs.UsageError
			if !errors.As(err, &usageErr) {
				t.Fatalf("err = %v, want *runs.UsageError", err)
			}
			if strings.TrimSpace(usageErr.Reason) == "" {
				t.Fatal("refusal carries no reason")
			}
			if stdout.Len() != 0 {
				t.Fatalf("refusal wrote a report:\n%s", stdout.String())
			}
		})
	}
}

func runInspectUsageCLI(t *testing.T, repo string, args ...string) (string, error) {
	t.Helper()
	root := newRoot()
	var out bytes.Buffer
	root.SetOut(&out)
	root.SetErr(&out)
	root.SetArgs(append([]string{"inspect", "usage", "-C", repo}, args...))
	err := root.Execute()
	return out.String(), err
}

// dayRow returns the daily table line for date.
func dayRow(t *testing.T, output, date string) string {
	t.Helper()
	return linePrefix(t, output, "  "+date)
}

func linePrefix(t *testing.T, output, prefix string) string {
	t.Helper()
	for _, line := range strings.Split(output, "\n") {
		if strings.HasPrefix(line, prefix) {
			return line
		}
	}
	t.Fatalf("output has no line starting %q:\n%s", prefix, output)
	return ""
}

func fieldSet(m map[string]any) []string {
	keys := make([]string, 0, len(m))
	for key := range m {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	return keys
}
