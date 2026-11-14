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
	d := runs.Dir{ColonyRoot: root, TraceID: traceID, AgentID: agentID}
	if err := d.Prepare(); err != nil {
		t.Fatal(err)
	}
	if err := d.WriteRequest(protocol.Request{
		ProtocolVersion: protocol.Version,
		TraceID:         traceID,
		AgentID:         agentID,
		Bee:             "builder",
		Adapter:         "opencode",
		Workspace:       root,
		ColonyRoot:      root,
		Model:           "anthropic/claude-sonnet",
		ModelAlias:      "sonnet",
		Purpose:         protocol.PurposeWork,
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
