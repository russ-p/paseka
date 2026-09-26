package hiveview

import (
	"net/url"
	"testing"
	"time"

	"github.com/russ-p/paseka/internal/colony"
	"github.com/russ-p/paseka/internal/protocol"
	"github.com/russ-p/paseka/internal/runs"
)

func TestParseTracePageQueryDefaults(t *testing.T) {
	query, err := ParseTracePageQuery(url.Values{})
	if err != nil {
		t.Fatalf("ParseTracePageQuery: %v", err)
	}
	if query.Limit != DefaultTracePageLimit || query.Before != "" {
		t.Fatalf("query = %+v", query)
	}
}

func TestParseTracePageQueryClampsLimit(t *testing.T) {
	query, err := ParseTracePageQuery(url.Values{"limit": []string{"5000"}})
	if err != nil {
		t.Fatalf("ParseTracePageQuery: %v", err)
	}
	if query.Limit != MaxTracePageLimit {
		t.Fatalf("limit = %d, want %d", query.Limit, MaxTracePageLimit)
	}
}

func TestParseTracePageQueryRejectsBadInput(t *testing.T) {
	for name, values := range map[string]url.Values{
		"zero limit":     {"limit": []string{"0"}},
		"negative limit": {"limit": []string{"-3"}},
		"word limit":     {"limit": []string{"many"}},
		"cursor no id":   {"before": []string{"2026-09-25T18:04:22Z|"}},
		"cursor no bar":  {"before": []string{"2026-09-25T18:04:22Z"}},
		"cursor no time": {"before": []string{"|trace-1"}},
		"cursor bad time": {"before": []string{
			"not-a-time|trace-1",
		}},
	} {
		t.Run(name, func(t *testing.T) {
			if _, err := ParseTracePageQuery(values); err == nil {
				t.Fatalf("expected an error for %v", values)
			}
		})
	}
}

func TestParseTraceCursorRoundTrips(t *testing.T) {
	summary := runs.TraceSummary{
		TraceID:        "trace-abc",
		LastActivityAt: time.Date(2026, 9, 25, 18, 4, 22, 123456789, time.UTC),
	}
	at, id, err := ParseTraceCursor(TraceCursorFor(summary))
	if err != nil {
		t.Fatalf("ParseTraceCursor: %v", err)
	}
	if id != summary.TraceID {
		t.Fatalf("id = %q, want %q", id, summary.TraceID)
	}
	if !at.Equal(summary.LastActivityAt) {
		t.Fatalf("at = %s, want %s", at, summary.LastActivityAt)
	}
}

// seedTracePage writes n traces whose last activity is one hour apart, newest last.
func seedTracePage(t *testing.T, repo string, n int) {
	t.Helper()
	base := time.Date(2026, 9, 25, 12, 0, 0, 0, time.UTC)
	for i := range n {
		traceID := "trace-" + string(rune('a'+i))
		started := base.Add(time.Duration(i) * time.Hour)
		d := runs.Dir{ColonyRoot: repo, TraceID: traceID, AgentID: "agent-1"}
		if err := d.Prepare(); err != nil {
			t.Fatal(err)
		}
		if err := d.WriteRequest(protocol.Request{
			ProtocolVersion: protocol.Version,
			TraceID:         traceID,
			AgentID:         "agent-1",
			Bee:             "scout",
			Adapter:         "cursor",
			Workspace:       repo,
			ColonyRoot:      repo,
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
	}
}

func TestListTracesPageWalksTheWholeHistory(t *testing.T) {
	repo := t.TempDir()
	seedTracePage(t, repo, 5)
	ctx := colony.Context{ColonyRoot: repo, Slug: "test"}

	seen := map[string]bool{}
	var order []string
	cursor := ""
	for step := range 10 {
		page, err := ListTracesPage(ctx, TracePageQuery{Limit: 2, Before: cursor})
		if err != nil {
			t.Fatalf("page %d: %v", step, err)
		}
		if len(page) == 0 {
			break
		}
		for _, view := range page {
			if seen[view.TraceID] {
				t.Fatalf("page %d repeated %s: %v", step, view.TraceID, order)
			}
			seen[view.TraceID] = true
			order = append(order, view.TraceID)
		}
		if len(page) < 2 {
			break
		}
		cursor = page[len(page)-1].LastActivityAt.UTC().Format(time.RFC3339Nano) + "|" + page[len(page)-1].TraceID
	}
	if len(order) != 5 {
		t.Fatalf("walked %v, want all 5 traces", order)
	}
	if order[0] != "trace-e" || order[4] != "trace-a" {
		t.Fatalf("order = %v, want newest first", order)
	}
}

func TestListTracesPageBeforeIsExclusive(t *testing.T) {
	repo := t.TempDir()
	seedTracePage(t, repo, 3)
	ctx := colony.Context{ColonyRoot: repo, Slug: "test"}

	first, err := ListTracesPage(ctx, TracePageQuery{Limit: 3})
	if err != nil {
		t.Fatal(err)
	}
	if len(first) != 3 {
		t.Fatalf("first page = %+v", first)
	}
	cursor := TraceCursorFor(runs.TraceSummary{
		TraceID:        first[0].TraceID,
		LastActivityAt: first[0].LastActivityAt,
	})
	second, err := ListTracesPage(ctx, TracePageQuery{Limit: 3, Before: cursor})
	if err != nil {
		t.Fatal(err)
	}
	if len(second) != 2 {
		t.Fatalf("second page = %+v, want the 2 older traces", second)
	}
	for _, view := range second {
		if view.TraceID == first[0].TraceID {
			t.Fatalf("cursor row repeated: %+v", second)
		}
	}
}
