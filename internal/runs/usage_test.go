package runs_test

import (
	"testing"
	"time"

	"github.com/paseka/paseka/internal/protocol"
	"github.com/paseka/paseka/internal/runs"
)

func TestLoadRunMetaIncludesUsage(t *testing.T) {
	root := t.TempDir()
	started := time.Now().UTC()
	d := runs.Dir{ColonyRoot: root, TraceID: "trace-1", AgentID: "agent-1"}
	if err := d.Prepare(); err != nil {
		t.Fatal(err)
	}
	if err := d.WriteRequest(protocol.Request{
		ProtocolVersion: protocol.Version,
		TraceID:         "trace-1",
		AgentID:         "agent-1",
		Bee:             "builder",
		Adapter:         "cursor",
		Workspace:       root,
		ColonyRoot:      root,
		CreatedAt:       started,
	}); err != nil {
		t.Fatal(err)
	}
	if err := d.WriteResult(protocol.Result{
		ProtocolVersion: protocol.Version,
		TraceID:         "trace-1",
		AgentID:         "agent-1",
		Status:          protocol.StatusCompleted,
		Summary:         "done",
		Usage: &protocol.Usage{
			InputTokens:  100,
			OutputTokens: 20,
			Source:       protocol.UsageSourceCursorStreamJSON,
		},
		FinishedAt: started.Add(time.Minute),
	}); err != nil {
		t.Fatal(err)
	}

	meta, err := runs.LoadRunMeta(d)
	if err != nil {
		t.Fatal(err)
	}
	if meta.Usage == nil || meta.Usage.InputTokens != 100 || meta.Usage.OutputTokens != 20 {
		t.Fatalf("usage = %+v", meta.Usage)
	}
}

func TestLoadTraceSummaryAggregatesUsage(t *testing.T) {
	root := t.TempDir()
	started := time.Now().UTC()
	writeRunWithUsage(t, root, "trace-u", "agent-a", started, &protocol.Usage{
		InputTokens: 100, OutputTokens: 10, CacheReadTokens: 50,
	})
	writeRunWithUsage(t, root, "trace-u", "agent-b", started.Add(time.Minute), &protocol.Usage{
		InputTokens: 200, OutputTokens: 30, CacheWriteTokens: 5,
	})
	writeRunWithUsage(t, root, "trace-u", "agent-c", started.Add(2*time.Minute), nil)

	summary, err := runs.LoadTraceSummary(root, "trace-u")
	if err != nil {
		t.Fatal(err)
	}
	if summary.RunCount != 3 {
		t.Fatalf("runCount = %d", summary.RunCount)
	}
	if summary.Usage == nil {
		t.Fatal("expected usage aggregate")
	}
	if summary.Usage.RunCountWithUsage != 2 {
		t.Fatalf("runCountWithUsage = %d", summary.Usage.RunCountWithUsage)
	}
	if summary.Usage.InputTokens != 300 || summary.Usage.OutputTokens != 40 {
		t.Fatalf("token sum = %+v", summary.Usage)
	}
	if summary.Usage.CacheReadTokens != 50 || summary.Usage.CacheWriteTokens != 5 {
		t.Fatalf("cache sum = %+v", summary.Usage)
	}
}

func TestLoadTraceSummaryOmitsUsageWhenAbsent(t *testing.T) {
	root := t.TempDir()
	started := time.Now().UTC()
	writeRunWithUsage(t, root, "trace-empty", "agent-a", started, nil)

	summary, err := runs.LoadTraceSummary(root, "trace-empty")
	if err != nil {
		t.Fatal(err)
	}
	if summary.Usage != nil {
		t.Fatalf("expected nil usage, got %+v", summary.Usage)
	}
}

func writeRunWithUsage(t *testing.T, root, traceID, agentID string, started time.Time, usage *protocol.Usage) {
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
		Adapter:         "cursor",
		Workspace:       root,
		ColonyRoot:      root,
		CreatedAt:       started,
	}); err != nil {
		t.Fatal(err)
	}
	if err := d.WriteResult(protocol.Result{
		ProtocolVersion: protocol.Version,
		TraceID:         traceID,
		AgentID:         agentID,
		Status:          protocol.StatusCompleted,
		Summary:         "ok",
		Usage:           usage,
		FinishedAt:      started.Add(time.Minute),
	}); err != nil {
		t.Fatal(err)
	}
}
