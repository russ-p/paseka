package runs_test

import (
	"testing"
	"time"

	"github.com/paseka/paseka/internal/protocol"
	"github.com/paseka/paseka/internal/runs"
)

func TestLatestWorktreeBranchFromInsight(t *testing.T) {
	root := t.TempDir()
	traceID := "trace-wt-branch"
	d := runs.Dir{ColonyRoot: root, TraceID: traceID, AgentID: "scout"}
	if err := d.Prepare(); err != nil {
		t.Fatal(err)
	}

	old, err := protocol.NewEvent(traceID, "scout", 1, protocol.EventInsight, protocol.WorktreeBranchPayload{
		Kind:   protocol.InsightWorktreeBranch,
		Branch: "feature/old-name",
	})
	if err != nil {
		t.Fatal(err)
	}
	old.CreatedAt = time.Now().UTC().Add(-time.Minute)
	if err := d.AppendEvent(old); err != nil {
		t.Fatal(err)
	}

	newEv, err := protocol.NewEvent(traceID, "scout", 2, protocol.EventInsight, protocol.WorktreeBranchPayload{
		Kind:   protocol.InsightWorktreeBranch,
		Branch: "feature/live-bees-header",
	})
	if err != nil {
		t.Fatal(err)
	}
	newEv.CreatedAt = time.Now().UTC()
	if err := d.AppendEvent(newEv); err != nil {
		t.Fatal(err)
	}

	got, err := runs.LatestWorktreeBranch(root, traceID)
	if err != nil {
		t.Fatal(err)
	}
	if got != "feature/live-bees-header" {
		t.Fatalf("branch = %q", got)
	}
}

func TestResolveWorktreeBranchDefault(t *testing.T) {
	root := t.TempDir()
	got, err := runs.ResolveWorktreeBranch(root, "trace/abc")
	if err != nil {
		t.Fatal(err)
	}
	if got != "paseka/trace-abc" {
		t.Fatalf("branch = %q, want paseka/trace-abc", got)
	}
}
