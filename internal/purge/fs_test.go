package purge_test

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/paseka/paseka/internal/colony"
	"github.com/paseka/paseka/internal/gitroot"
	"github.com/paseka/paseka/internal/homestate"
	"github.com/paseka/paseka/internal/purge"
	"github.com/paseka/paseka/internal/runs"
	"github.com/paseka/paseka/internal/worktree"
)

func TestPurgeRuns(t *testing.T) {
	repo := initTestRepo(t)
	slug := setupPurgeHome(t, repo)

	d := runs.Dir{ColonyRoot: repo, TraceID: "trace-1", AgentID: "agent-a"}
	if err := d.Prepare(); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(d.ResultPath(), []byte("ok"), 0o644); err != nil {
		t.Fatal(err)
	}

	ctx := colony.Context{ColonyRoot: repo, Slug: slug}
	plan, err := purge.Plan(ctx, purge.PurgeTarget{Runs: true})
	if err != nil {
		t.Fatal(err)
	}
	if len(plan.Runs) != 1 || plan.Runs[0] != "trace-1" {
		t.Fatalf("plan runs = %+v", plan.Runs)
	}

	res, err := purge.Execute(ctx, purge.PurgeTarget{Runs: true})
	if err != nil {
		t.Fatal(err)
	}
	if len(res.Removed) != 1 {
		t.Fatalf("removed = %+v", res.Removed)
	}
	if _, err := os.Stat(filepath.Join(repo, ".paseka", "runs", "trace-1")); !os.IsNotExist(err) {
		t.Fatalf("runs dir still exists: %v", err)
	}
}

func TestPurgeWorktrees(t *testing.T) {
	repo := initTestRepo(t)
	slug := setupPurgeHome(t, repo)

	if _, err := worktree.Ensure(worktree.EnsureOptions{
		ColonyRoot: repo,
		TraceID:    "trace-wt",
		Slug:       slug,
	}); err != nil {
		t.Fatal(err)
	}

	ctx := colony.Context{ColonyRoot: repo, Slug: slug}
	plan, err := purge.Plan(ctx, purge.PurgeTarget{Worktrees: true})
	if err != nil {
		t.Fatal(err)
	}
	if len(plan.Worktrees) != 1 {
		t.Fatalf("plan worktrees = %+v", plan.Worktrees)
	}

	res, err := purge.Execute(ctx, purge.PurgeTarget{Worktrees: true})
	if err != nil {
		t.Fatal(err)
	}
	if len(res.Removed) != 1 {
		t.Fatalf("removed = %+v", res.Removed)
	}

	wtPath := filepath.Join(repo, ".paseka", "worktrees", "trace-wt")
	if _, err := os.Stat(wtPath); !os.IsNotExist(err) {
		t.Fatalf("worktree dir still exists: %v", err)
	}
	if gitroot.IsInsideWorkTree(wtPath) {
		t.Fatal("git still considers path a worktree")
	}

	st, err := homestate.LoadState(slug)
	if err != nil {
		t.Fatal(err)
	}
	if len(st.Worktrees) != 0 {
		t.Fatalf("state worktrees = %+v", st.Worktrees)
	}
}

func TestPurgeStateOnly(t *testing.T) {
	repo := initTestRepo(t)
	slug := setupPurgeHome(t, repo)

	if err := homestate.RegisterWorktree(slug, homestate.WorktreeEntry{
		TraceID: "trace-old",
		Path:    "/tmp/gone",
	}); err != nil {
		t.Fatal(err)
	}

	ctx := colony.Context{ColonyRoot: repo, Slug: slug}
	plan, err := purge.Plan(ctx, purge.PurgeTarget{State: true})
	if err != nil {
		t.Fatal(err)
	}
	if !plan.State {
		t.Fatal("expected state in plan")
	}

	_, err = purge.Execute(ctx, purge.PurgeTarget{State: true})
	if err != nil {
		t.Fatal(err)
	}

	st, err := homestate.LoadState(slug)
	if err != nil {
		t.Fatal(err)
	}
	if len(st.Worktrees) != 0 {
		t.Fatalf("state worktrees = %+v", st.Worktrees)
	}
}

func TestPurgeCache(t *testing.T) {
	repo := initTestRepo(t)
	slug := setupPurgeHome(t, repo)

	cacheDir := filepath.Join(repo, ".paseka", "cache", "tmp")
	if err := os.MkdirAll(cacheDir, 0o755); err != nil {
		t.Fatal(err)
	}

	ctx := colony.Context{ColonyRoot: repo, Slug: slug}
	plan, err := purge.Plan(ctx, purge.PurgeTarget{Cache: true})
	if err != nil {
		t.Fatal(err)
	}
	if !plan.Cache {
		t.Fatal("expected cache in plan")
	}

	res, err := purge.Execute(ctx, purge.PurgeTarget{Cache: true})
	if err != nil {
		t.Fatal(err)
	}
	if len(res.Removed) != 1 {
		t.Fatalf("removed = %+v", res.Removed)
	}
}

func TestFormatBusPurgePlan(t *testing.T) {
	plan := purge.PurgePlan{
		Bus: &purge.BusPurgePlan{
			TraceID:       "trace-bus",
			TaskLedgerKey: true,
			EventCount:    2,
			Artifacts:     []string{"trace-bus-agent-1.diff"},
		},
	}
	out := purge.FormatPlan(plan)
	if !strings.Contains(out, "bus (trace trace-bus)") {
		t.Fatalf("format plan = %q", out)
	}
	if !strings.Contains(out, "task ledger key: trace-bus") {
		t.Fatalf("format plan = %q", out)
	}
	if !strings.Contains(out, "2 stream event(s)") {
		t.Fatalf("format plan = %q", out)
	}
	if !strings.Contains(out, "trace-bus-agent-1.diff") {
		t.Fatalf("format plan = %q", out)
	}
}

func TestPlanEmptyBus(t *testing.T) {
	empty := purge.PurgePlan{Bus: &purge.BusPurgePlan{TraceID: "trace-bus"}}
	if !purge.PlanEmpty(empty) {
		t.Fatal("expected empty bus plan")
	}
	populated := purge.PurgePlan{Bus: &purge.BusPurgePlan{TraceID: "trace-bus", EventCount: 1}}
	if purge.PlanEmpty(populated) {
		t.Fatal("expected non-empty bus plan")
	}
}

func TestBusPurgePlanFromTrace(t *testing.T) {
	plan := purge.BusPurgePlanFromTrace("trace-bus", true, 2, []string{"trace-bus-agent-1.diff"})
	if plan.TraceID != "trace-bus" || !plan.TaskLedgerKey || plan.EventCount != 2 {
		t.Fatalf("plan = %#v", plan)
	}
}
