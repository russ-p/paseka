package runs_test

import (
	"testing"
	"time"

	"github.com/russ-p/paseka/internal/protocol"
	"github.com/russ-p/paseka/internal/runs"
)

// usageTestClock is the reader's injectable clock. The cache window is measured
// against it, so an expiry test advances time instead of sleeping.
type usageTestClock struct {
	at time.Time
}

func (c *usageTestClock) now() time.Time { return c.at }

func (c *usageTestClock) advance(d time.Duration) { c.at = c.at.Add(d) }

func newUsageCacheReader(root string, clock *usageTestClock) *runs.UsageReader {
	return &runs.UsageReader{ColonyRoot: root, TTL: time.Minute, Now: clock.now}
}

// writeUsageCacheRun writes one completed execution reporting a single input
// token count, which keeps the expected totals easy to read.
func writeUsageCacheRun(t *testing.T, root, traceID, agentID string, started time.Time, adapter, model string, input int64) {
	t.Helper()
	writeUsageFixture(t, root, usageFixture{
		traceID: traceID, agentID: agentID, started: started,
		bee: "builder", adapter: adapter, model: model, purpose: protocol.PurposeWork,
		usage: &protocol.Usage{InputTokens: input, Source: protocol.UsageSourceOpenCodeRunJSON},
	})
}

func readCachedUsage(t *testing.T, reader *runs.UsageReader, q runs.UsageQuery, refresh bool) runs.UsageReport {
	t.Helper()
	report, err := reader.Read(q, refresh)
	if err != nil {
		t.Fatal(err)
	}
	return report
}

func TestUsageReaderReusesTheScanInsideTheWindowAndRescansAfterIt(t *testing.T) {
	root := t.TempDir()
	started := usageDate(t, "2026-03-02 10:00", time.UTC)
	clock := &usageTestClock{at: started.Add(12 * time.Hour)}
	writeUsageCacheRun(t, root, "trace-first", "run-1", started, "opencode", "anthropic/claude-sonnet", 100)

	reader := newUsageCacheReader(root, clock)
	query := runs.UsageQuery{Since: "2026-03-01", Until: "2026-03-03", Timezone: "UTC"}

	first := readCachedUsage(t, reader, query, false)
	if first.Period.Executions != 1 || first.Period.Tokens.InputTokens != 100 {
		t.Fatalf("first read = %+v, want the one run", first.Period)
	}

	// A run finished since the scan does not appear inside the window: the
	// answer is the earlier observation, and its observedAt says so.
	writeUsageCacheRun(t, root, "trace-second", "run-1", started.Add(time.Minute), "opencode", "openai/gpt-5", 200)
	second := readCachedUsage(t, reader, query, false)
	if !second.ObservedAt.Equal(first.ObservedAt) {
		t.Fatalf("second read observedAt = %s, want the cached %s", second.ObservedAt, first.ObservedAt)
	}
	if second.Period.Executions != 1 || second.Period.Tokens.InputTokens != 100 {
		t.Fatalf("second read = %+v, want the cached scan rather than a new one", second.Period)
	}

	// Past the window the reader scans again, so the same query sees the run
	// that finished meanwhile and carries a new observedAt.
	clock.advance(time.Minute)
	third := readCachedUsage(t, reader, query, false)
	if third.Period.Executions != 2 || third.Period.Tokens.InputTokens != 300 {
		t.Fatalf("expired read = %+v, want both runs", third.Period)
	}
	if !third.ObservedAt.After(first.ObservedAt) {
		t.Fatalf("expired read observedAt = %s, want later than %s", third.ObservedAt, first.ObservedAt)
	}
}

func TestUsageReaderRefreshBypassesTheCache(t *testing.T) {
	root := t.TempDir()
	started := usageDate(t, "2026-03-02 10:00", time.UTC)
	clock := &usageTestClock{at: started.Add(12 * time.Hour)}
	writeUsageCacheRun(t, root, "trace-first", "run-1", started, "opencode", "anthropic/claude-sonnet", 100)

	reader := newUsageCacheReader(root, clock)
	query := runs.UsageQuery{Since: "2026-03-01", Until: "2026-03-03", Timezone: "UTC"}
	before := readCachedUsage(t, reader, query, false)

	writeUsageCacheRun(t, root, "trace-second", "run-1", started.Add(time.Minute), "opencode", "openai/gpt-5", 200)
	clock.advance(time.Second)

	// The explicit refresh is the bypass path: it scans again, so a run
	// written since the previous read appears on demand.
	after := readCachedUsage(t, reader, query, true)
	if after.Period.Executions != 2 || after.Period.Tokens.InputTokens != 300 {
		t.Fatalf("refresh read = %+v, want both runs", after.Period)
	}
	if !after.ObservedAt.After(before.ObservedAt) {
		t.Fatalf("refresh observedAt = %s, want later than %s", after.ObservedAt, before.ObservedAt)
	}
}

func TestUsageReaderKeepsReportsApartByQuery(t *testing.T) {
	started := usageDate(t, "2026-03-02 10:00", time.UTC)

	cases := []struct {
		name       string
		other      runs.UsageQuery
		wantTokens int64
		wantExecs  int
	}{
		{
			name:       "range",
			other:      runs.UsageQuery{Since: "2026-02-28", Until: "2026-03-04", Timezone: "UTC"},
			wantTokens: 300,
			wantExecs:  2,
		},
		{
			name:       "zone",
			other:      runs.UsageQuery{Since: "2026-03-01", Until: "2026-03-03", Timezone: "Europe/Berlin"},
			wantTokens: 300,
			wantExecs:  2,
		},
		{
			name:       "filter set",
			other:      runs.UsageQuery{Since: "2026-03-01", Until: "2026-03-03", Timezone: "UTC", Adapters: []string{"opencode"}},
			wantTokens: 300,
			wantExecs:  2,
		},
		{
			name:       "filter value",
			other:      runs.UsageQuery{Since: "2026-03-01", Until: "2026-03-03", Timezone: "UTC", Models: []string{"openai/gpt-5"}},
			wantTokens: 200,
			wantExecs:  1,
		},
	}

	for _, test := range cases {
		t.Run(test.name, func(t *testing.T) {
			root := t.TempDir()
			clock := &usageTestClock{at: started.Add(12 * time.Hour)}
			writeUsageCacheRun(t, root, "trace-first", "run-1", started, "opencode", "anthropic/claude-sonnet", 100)

			reader := newUsageCacheReader(root, clock)
			base := runs.UsageQuery{Since: "2026-03-01", Until: "2026-03-03", Timezone: "UTC"}
			cached := readCachedUsage(t, reader, base, false)
			if cached.Period.Executions != 1 {
				t.Fatalf("base read = %+v, want the one run", cached.Period)
			}

			// A second run lands after the base scan. The different query must
			// scan rather than reuse the base report.
			writeUsageCacheRun(t, root, "trace-second", "run-1", started.Add(time.Minute), "opencode", "openai/gpt-5", 200)
			other := readCachedUsage(t, reader, test.other, false)
			if other.Period.Executions != test.wantExecs || other.Period.Tokens.InputTokens != test.wantTokens {
				t.Fatalf("different %s read = %+v, want a fresh scan of the runs that match", test.name, other.Period)
			}

			// The base query's own report is untouched by that scan.
			again := readCachedUsage(t, reader, base, false)
			if again.Period.Executions != 1 || !again.ObservedAt.Equal(cached.ObservedAt) {
				t.Fatalf("base reread = %+v at %s, want the cached scan %s", again.Period, again.ObservedAt, cached.ObservedAt)
			}
		})
	}
}

// A zero TTL is the uncached reader: every read scans, which keeps a caller
// that wants no reuse from having to bypass an entry it never filled.
func TestUsageReaderWithoutTTLAlwaysScans(t *testing.T) {
	root := t.TempDir()
	started := usageDate(t, "2026-03-02 10:00", time.UTC)
	clock := &usageTestClock{at: started.Add(12 * time.Hour)}
	writeUsageCacheRun(t, root, "trace-first", "run-1", started, "opencode", "anthropic/claude-sonnet", 100)

	reader := &runs.UsageReader{ColonyRoot: root, TTL: 0, Now: clock.now}
	query := runs.UsageQuery{Since: "2026-03-01", Until: "2026-03-03", Timezone: "UTC"}
	readCachedUsage(t, reader, query, false)

	writeUsageCacheRun(t, root, "trace-second", "run-1", started.Add(time.Minute), "opencode", "openai/gpt-5", 200)
	again := readCachedUsage(t, reader, query, false)
	if again.Period.Executions != 2 {
		t.Fatalf("read = %+v, want both runs with caching disabled", again.Period)
	}
}
