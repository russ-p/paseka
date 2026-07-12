package colony

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/paseka/paseka/internal/gitroot"
)

// PurgeTarget selects which artifact classes to remove.
type PurgeTarget struct {
	Runs      bool
	Worktrees bool
	Cache     bool
	State     bool
	Bus       bool
	TraceID   string
}

// Any reports whether at least one target is selected.
func (t PurgeTarget) Any() bool {
	return t.Runs || t.Worktrees || t.Cache || t.State || t.Bus
}

// BusPurgePlan describes JetStream artifacts that would be removed for one trace.
type BusPurgePlan struct {
	TraceID       string
	TaskLedgerKey bool
	EventCount    int
	Artifacts     []string
}

// Empty reports whether bus purge would affect nothing.
func (p BusPurgePlan) Empty() bool {
	return !p.TaskLedgerKey && p.EventCount == 0 && len(p.Artifacts) == 0
}

// BusPurgePlanFromTrace builds a bus purge plan from bus inspection results.
func BusPurgePlanFromTrace(traceID string, taskLedgerKey bool, eventCount int, artifacts []string) *BusPurgePlan {
	return &BusPurgePlan{
		TraceID:       traceID,
		TaskLedgerKey: taskLedgerKey,
		EventCount:    eventCount,
		Artifacts:     artifacts,
	}
}

// PurgePlan describes what will be removed before confirmation.
type PurgePlan struct {
	Runs      []string
	Worktrees []string
	Cache     bool
	State     bool
	Bus       *BusPurgePlan
}

// BusPurgeResult reports bus artifacts removed for one trace.
type BusPurgeResult struct {
	KeysRemoved    []string
	EventsRemoved  int
	ObjectsRemoved []string
}

// PurgeResult reports what was removed.
type PurgeResult struct {
	Removed []string
	Bus     *BusPurgeResult
}

// PlanPurge lists paths and flags that would be affected.
func PlanPurge(ctx Context, target PurgeTarget) (PurgePlan, error) {
	var plan PurgePlan

	if target.Runs {
		runsRoot := PasekaPath(ctx.ColonyRoot, "runs")
		entries, err := listChildDirs(runsRoot)
		if err != nil {
			return plan, err
		}
		plan.Runs = entries
	}

	if target.Worktrees {
		wtRoot := PasekaPath(ctx.ColonyRoot, "worktrees")
		entries, err := listChildDirs(wtRoot)
		if err != nil {
			return plan, err
		}
		plan.Worktrees = entries
	}

	if target.Cache {
		cacheRoot := PasekaPath(ctx.ColonyRoot, "cache")
		if fi, err := os.Stat(cacheRoot); err == nil && fi.IsDir() {
			plan.Cache = true
		}
	}

	if target.State {
		st, err := LoadState(ctx.Slug)
		if err != nil {
			return plan, err
		}
		plan.State = len(st.Worktrees) > 0
	}

	return plan, nil
}

// Purge removes selected colony artifacts.
func Purge(ctx Context, target PurgeTarget) (PurgeResult, error) {
	var res PurgeResult

	if target.Worktrees {
		wtRoot := PasekaPath(ctx.ColonyRoot, "worktrees")
		entries, err := listChildDirs(wtRoot)
		if err != nil {
			return res, err
		}
		for _, traceID := range entries {
			path := filepath.Join(wtRoot, traceID)
			if err := removeGitWorktree(ctx.ColonyRoot, path); err != nil {
				return res, err
			}
			res.Removed = append(res.Removed, path)
		}
		if err := pruneWorktrees(ctx.ColonyRoot); err != nil {
			return res, err
		}
	}

	if target.Runs {
		runsRoot := PasekaPath(ctx.ColonyRoot, "runs")
		entries, err := listChildDirs(runsRoot)
		if err != nil {
			return res, err
		}
		for _, traceID := range entries {
			path := filepath.Join(runsRoot, traceID)
			if err := os.RemoveAll(path); err != nil {
				return res, fmt.Errorf("purge runs %s: %w", path, err)
			}
			res.Removed = append(res.Removed, path)
		}
	}

	if target.Cache {
		cacheRoot := PasekaPath(ctx.ColonyRoot, "cache")
		if _, err := os.Stat(cacheRoot); err == nil {
			if err := os.RemoveAll(cacheRoot); err != nil {
				return res, fmt.Errorf("purge cache: %w", err)
			}
			res.Removed = append(res.Removed, cacheRoot)
		}
	}

	if target.State || target.Worktrees {
		if err := SaveState(ctx.Slug, State{}); err != nil {
			return res, err
		}
		if target.State {
			res.Removed = append(res.Removed, "state.json")
		}
	}

	return res, nil
}

func listChildDirs(root string) ([]string, error) {
	entries, err := os.ReadDir(root)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, err
	}
	var names []string
	for _, e := range entries {
		if e.IsDir() {
			names = append(names, e.Name())
		}
	}
	return names, nil
}

func removeGitWorktree(colonyRoot, path string) error {
	if gitroot.IsInsideWorkTree(path) {
		cmd := exec.Command("git", "-C", colonyRoot, "worktree", "remove", "--force", path)
		if out, err := cmd.CombinedOutput(); err != nil {
			return fmt.Errorf("git worktree remove %s: %w: %s", path, err, strings.TrimSpace(string(out)))
		}
		return nil
	}
	return os.RemoveAll(path)
}

func pruneWorktrees(colonyRoot string) error {
	cmd := exec.Command("git", "-C", colonyRoot, "worktree", "prune")
	if out, err := cmd.CombinedOutput(); err != nil {
		return fmt.Errorf("git worktree prune: %w: %s", err, strings.TrimSpace(string(out)))
	}
	return nil
}

// FormatPlan returns a human-readable summary of a purge plan.
func FormatPlan(plan PurgePlan) string {
	var b strings.Builder
	if len(plan.Runs) > 0 {
		fmt.Fprintf(&b, "  runs (%d traces):\n", len(plan.Runs))
		for _, id := range plan.Runs {
			fmt.Fprintf(&b, "    - %s\n", id)
		}
	}
	if len(plan.Worktrees) > 0 {
		fmt.Fprintf(&b, "  worktrees (%d):\n", len(plan.Worktrees))
		for _, id := range plan.Worktrees {
			fmt.Fprintf(&b, "    - %s\n", id)
		}
	}
	if plan.Cache {
		b.WriteString("  cache/\n")
	}
	if plan.State {
		b.WriteString("  state.json (worktree registry)\n")
	}
	if plan.Bus != nil {
		fmt.Fprintf(&b, "  bus (trace %s):\n", plan.Bus.TraceID)
		if plan.Bus.TaskLedgerKey {
			fmt.Fprintf(&b, "    - task ledger key: %s\n", plan.Bus.TraceID)
		}
		if plan.Bus.EventCount > 0 {
			fmt.Fprintf(&b, "    - %d stream event(s)\n", plan.Bus.EventCount)
		}
		if len(plan.Bus.Artifacts) > 0 {
			fmt.Fprintf(&b, "    - %d artifact object(s):\n", len(plan.Bus.Artifacts))
			for _, name := range plan.Bus.Artifacts {
				fmt.Fprintf(&b, "      - %s\n", name)
			}
		}
	}
	return strings.TrimRight(b.String(), "\n")
}

// PlanEmpty reports whether nothing would be removed.
func PlanEmpty(plan PurgePlan) bool {
	fsEmpty := len(plan.Runs) == 0 && len(plan.Worktrees) == 0 && !plan.Cache && !plan.State
	if plan.Bus == nil {
		return fsEmpty
	}
	return fsEmpty && plan.Bus.Empty()
}
