package colony_test

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/paseka/paseka/internal/colony"
	"github.com/paseka/paseka/internal/gitroot"
	"github.com/paseka/paseka/internal/worktree"
)

func TestPurgeRuns(t *testing.T) {
	repo := initTestRepo(t)
	slug := setupPurgeHome(t, repo)

	runDir := filepath.Join(repo, ".paseka", "runs", "trace-1", "agent-a")
	if err := os.MkdirAll(runDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(runDir, "result.txt"), []byte("ok"), 0o644); err != nil {
		t.Fatal(err)
	}

	ctx := colony.Context{ColonyRoot: repo, Slug: slug}
	plan, err := colony.PlanPurge(ctx, colony.PurgeTarget{Runs: true})
	if err != nil {
		t.Fatal(err)
	}
	if len(plan.Runs) != 1 || plan.Runs[0] != "trace-1" {
		t.Fatalf("plan runs = %+v", plan.Runs)
	}

	res, err := colony.Purge(ctx, colony.PurgeTarget{Runs: true})
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
	plan, err := colony.PlanPurge(ctx, colony.PurgeTarget{Worktrees: true})
	if err != nil {
		t.Fatal(err)
	}
	if len(plan.Worktrees) != 1 {
		t.Fatalf("plan worktrees = %+v", plan.Worktrees)
	}

	res, err := colony.Purge(ctx, colony.PurgeTarget{Worktrees: true})
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

	st, err := colony.LoadState(slug)
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

	if err := colony.RegisterWorktree(slug, colony.WorktreeEntry{
		TraceID: "trace-old",
		Path:    "/tmp/gone",
	}); err != nil {
		t.Fatal(err)
	}

	ctx := colony.Context{ColonyRoot: repo, Slug: slug}
	plan, err := colony.PlanPurge(ctx, colony.PurgeTarget{State: true})
	if err != nil {
		t.Fatal(err)
	}
	if !plan.State {
		t.Fatal("expected state in plan")
	}

	_, err = colony.Purge(ctx, colony.PurgeTarget{State: true})
	if err != nil {
		t.Fatal(err)
	}

	st, err := colony.LoadState(slug)
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
	plan, err := colony.PlanPurge(ctx, colony.PurgeTarget{Cache: true})
	if err != nil {
		t.Fatal(err)
	}
	if !plan.Cache {
		t.Fatal("expected cache in plan")
	}

	res, err := colony.Purge(ctx, colony.PurgeTarget{Cache: true})
	if err != nil {
		t.Fatal(err)
	}
	if len(res.Removed) != 1 {
		t.Fatalf("removed = %+v", res.Removed)
	}
}

func TestFormatBusPurgePlan(t *testing.T) {
	plan := colony.PurgePlan{
		Bus: &colony.BusPurgePlan{
			TraceID:       "trace-bus",
			TaskLedgerKey: true,
			EventCount:    2,
			Artifacts:     []string{"trace-bus-agent-1.diff"},
		},
	}
	out := colony.FormatPlan(plan)
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
	empty := colony.PurgePlan{Bus: &colony.BusPurgePlan{TraceID: "trace-bus"}}
	if !colony.PlanEmpty(empty) {
		t.Fatal("expected empty bus plan")
	}
	populated := colony.PurgePlan{Bus: &colony.BusPurgePlan{TraceID: "trace-bus", EventCount: 1}}
	if colony.PlanEmpty(populated) {
		t.Fatal("expected non-empty bus plan")
	}
}

func TestBusPurgePlanFromTrace(t *testing.T) {
	plan := colony.BusPurgePlanFromTrace("trace-bus", true, 2, []string{"trace-bus-agent-1.diff"})
	if plan.TraceID != "trace-bus" || !plan.TaskLedgerKey || plan.EventCount != 2 {
		t.Fatalf("plan = %#v", plan)
	}
}

func setupPurgeHome(t *testing.T, repo string) string {
	t.Helper()
	base := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", base)

	slug := "purge-test"
	homeDir, err := colony.HomeDir(slug)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Join(homeDir, "adapters"), 0o755); err != nil {
		t.Fatal(err)
	}
	cfg := []byte("colony_root: " + repo + "\nslug: " + slug + "\n")
	if err := os.WriteFile(filepath.Join(homeDir, "config.yaml"), cfg, 0o600); err != nil {
		t.Fatal(err)
	}
	return slug
}
