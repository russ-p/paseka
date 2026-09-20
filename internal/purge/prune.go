package purge

import (
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/russ-p/paseka/internal/colony"
	"github.com/russ-p/paseka/internal/gitroot"
	"github.com/russ-p/paseka/internal/homestate"
)

// PruneTarget selects which age-based cleanup to perform.
type PruneTarget struct {
	Runs      bool
	Worktrees bool
	Bus       bool
	OlderThan time.Duration
	Now       time.Time
}

// Any reports whether at least one target is selected.
func (t PruneTarget) Any() bool {
	return t.Runs || t.Worktrees || t.Bus
}

// PruneCandidate is one trace artifact directory older than the retention cutoff.
type PruneCandidate struct {
	TraceID  string
	Path     string
	LastUsed time.Time
	// Protected marks a candidate that must never be auto-removed (for example a
	// worktree with uncommitted changes), even when its activity looks stale.
	Protected bool
}

// PrunePlan describes artifacts that would be removed before confirmation.
type PrunePlan struct {
	Cutoff    time.Time
	Runs      []PruneCandidate
	Worktrees []PruneCandidate
	Bus       []string
}

// PruneResult reports what age-based cleanup removed.
type PruneResult struct {
	Removed []string
	Bus     []BusPurgeResult
}

// ParseRetention parses a retention period such as "14d", "2w", or any Go
// duration string ("336h", "30m"). It rejects zero and negative periods.
func ParseRetention(raw string) (time.Duration, error) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return 0, fmt.Errorf("retention period is required")
	}
	value := raw
	var unit time.Duration
	switch {
	case strings.HasSuffix(raw, "d"):
		value, unit = strings.TrimSuffix(raw, "d"), 24*time.Hour
	case strings.HasSuffix(raw, "w"):
		value, unit = strings.TrimSuffix(raw, "w"), 7*24*time.Hour
	}
	if unit != 0 {
		n, err := strconv.ParseFloat(value, 64)
		if err != nil {
			return 0, fmt.Errorf("invalid retention %q: want e.g. 14d, 2w, or 336h", raw)
		}
		d := time.Duration(n * float64(unit))
		if d <= 0 {
			return 0, fmt.Errorf("retention period must be positive")
		}
		return d, nil
	}
	d, err := time.ParseDuration(raw)
	if err != nil {
		return 0, fmt.Errorf("invalid retention %q: %w", raw, err)
	}
	if d <= 0 {
		return 0, fmt.Errorf("retention period must be positive")
	}
	return d, nil
}

func (t PruneTarget) validate() error {
	if !t.Any() {
		return fmt.Errorf("specify at least one target: --runs, --worktrees, --bus, or --all")
	}
	if t.OlderThan <= 0 {
		return fmt.Errorf("retention period must be positive")
	}
	return nil
}

// Prune lists worktree and run directories whose last activity predates the
// retention cutoff. When Bus is set it also lists correlatable traces from the
// task-ledger KV bucket and uses ledger activity to protect active traces.
func Prune(ctx colony.Context, target PruneTarget) (PrunePlan, error) {
	if err := target.validate(); err != nil {
		return PrunePlan{}, err
	}
	now := target.Now
	if now.IsZero() {
		now = time.Now().UTC()
	}
	plan := PrunePlan{Cutoff: now.Add(-target.OlderThan)}

	var ledgerLast map[string]time.Time
	var ledgerExists map[string]bool
	if target.Bus {
		purger, closer, err := connectBus(ctx)
		if err != nil {
			return plan, err
		}
		defer closer()
		activities, err := purger.ListTraceActivity()
		if err != nil {
			return plan, err
		}
		ledgerLast = make(map[string]time.Time, len(activities))
		ledgerExists = make(map[string]bool, len(activities))
		for _, a := range activities {
			ledgerLast[a.TraceID] = a.LastUsed
			ledgerExists[a.TraceID] = true
		}
	}

	fsTraces := map[string]bool{}
	if target.Runs {
		candidates, err := scanRunAges(ctx, ledgerLast)
		if err != nil {
			return plan, err
		}
		for _, c := range candidates {
			fsTraces[c.TraceID] = true
		}
		plan.Runs = olderThan(candidates, plan.Cutoff)
	}
	if target.Worktrees {
		candidates, err := scanWorktreeAges(ctx, ledgerLast)
		if err != nil {
			return plan, err
		}
		for _, c := range candidates {
			fsTraces[c.TraceID] = true
		}
		plan.Worktrees = olderThan(candidates, plan.Cutoff)
	}

	if target.Bus {
		plan.Bus = busPruneTargets(plan, fsTraces, ledgerLast, ledgerExists)
	}
	return plan, nil
}

func busPruneTargets(plan PrunePlan, fsTraces map[string]bool, ledgerLast map[string]time.Time, ledgerExists map[string]bool) []string {
	seen := map[string]bool{}
	var out []string
	add := func(traceID string) {
		if traceID == "" || seen[traceID] {
			return
		}
		seen[traceID] = true
		out = append(out, traceID)
	}

	for _, c := range plan.Runs {
		if ledgerExists[c.TraceID] {
			add(c.TraceID)
		}
	}
	for _, c := range plan.Worktrees {
		if ledgerExists[c.TraceID] {
			add(c.TraceID)
		}
	}
	for traceID, last := range ledgerLast {
		if fsTraces[traceID] || seen[traceID] {
			continue
		}
		// A zero time means the ledger has no task activity to correlate on.
		if last.IsZero() {
			continue
		}
		if last.Before(plan.Cutoff) {
			add(traceID)
		}
	}
	sort.Strings(out)
	return out
}

// PrunePlanEmpty reports whether nothing would be removed.
func PrunePlanEmpty(plan PrunePlan) bool {
	return len(plan.Runs) == 0 && len(plan.Worktrees) == 0 && len(plan.Bus) == 0
}

// FormatPrunePlan returns a human-readable summary of a prune plan.
func FormatPrunePlan(plan PrunePlan) string {
	var b strings.Builder
	if len(plan.Runs) > 0 {
		fmt.Fprintf(&b, "  runs (%d traces):\n", len(plan.Runs))
		for _, c := range plan.Runs {
			fmt.Fprintf(&b, "    - %s (last used %s)\n", c.TraceID, formatLastUsed(c.LastUsed))
		}
	}
	if len(plan.Worktrees) > 0 {
		fmt.Fprintf(&b, "  worktrees (%d):\n", len(plan.Worktrees))
		for _, c := range plan.Worktrees {
			fmt.Fprintf(&b, "    - %s (last used %s)\n", c.TraceID, formatLastUsed(c.LastUsed))
		}
	}
	if len(plan.Bus) > 0 {
		fmt.Fprintf(&b, "  bus (%d trace(s)):\n", len(plan.Bus))
		for _, traceID := range plan.Bus {
			fmt.Fprintf(&b, "    - %s\n", traceID)
		}
	}
	return strings.TrimRight(b.String(), "\n")
}

func formatLastUsed(t time.Time) string {
	if t.IsZero() {
		return "unknown"
	}
	return t.UTC().Format(time.RFC3339)
}

// ExecutePrune removes the artifacts selected by a previously computed plan.
// Working from the plan keeps the confirmed view and the deletion set identical.
func ExecutePrune(ctx colony.Context, target PruneTarget, plan PrunePlan) (PruneResult, error) {
	var res PruneResult

	for _, c := range plan.Worktrees {
		if err := removeGitWorktree(ctx.ColonyRoot, c.Path); err != nil {
			return res, err
		}
		if ctx.Slug != "" {
			if err := homestate.UnregisterWorktree(ctx.Slug, c.TraceID); err != nil {
				return res, err
			}
		}
		res.Removed = append(res.Removed, c.Path)
	}
	if len(plan.Worktrees) > 0 {
		if err := pruneWorktrees(ctx.ColonyRoot); err != nil {
			return res, err
		}
	}

	for _, c := range plan.Runs {
		if err := os.RemoveAll(c.Path); err != nil {
			return res, fmt.Errorf("prune runs %s: %w", c.Path, err)
		}
		res.Removed = append(res.Removed, c.Path)
	}

	if target.Bus && len(plan.Bus) > 0 {
		purger, closer, err := connectBus(ctx)
		if err != nil {
			return res, err
		}
		defer closer()
		for _, traceID := range plan.Bus {
			busRes, err := purger.PurgeTrace(traceID)
			if err != nil {
				return res, err
			}
			res.Bus = append(res.Bus, BusPurgeResult{
				TraceID:        traceID,
				KeysRemoved:    busRes.KeysRemoved,
				EventsRemoved:  busRes.EventsRemoved,
				ObjectsRemoved: busRes.ObjectsRemoved,
			})
		}
	}
	return res, nil
}

func scanRunAges(ctx colony.Context, ledgerLast map[string]time.Time) ([]PruneCandidate, error) {
	root := colony.PasekaPath(ctx.ColonyRoot, "runs")
	entries, err := listChildDirs(root)
	if err != nil {
		return nil, err
	}
	out := make([]PruneCandidate, 0, len(entries))
	for _, traceID := range entries {
		path := filepath.Join(root, traceID)
		last := traceDirLastUsed(path)
		if ledgerTime, ok := ledgerLast[traceID]; ok && ledgerTime.After(last) {
			last = ledgerTime
		}
		out = append(out, PruneCandidate{TraceID: traceID, Path: path, LastUsed: last})
	}
	return out, nil
}

func scanWorktreeAges(ctx colony.Context, ledgerLast map[string]time.Time) ([]PruneCandidate, error) {
	root := colony.PasekaPath(ctx.ColonyRoot, "worktrees")
	entries, err := listChildDirs(root)
	if err != nil {
		return nil, err
	}
	runsRoot := colony.PasekaPath(ctx.ColonyRoot, "runs")
	out := make([]PruneCandidate, 0, len(entries))
	for _, traceID := range entries {
		path := filepath.Join(root, traceID)
		last := worktreeDirLastUsed(path)
		if runsLast := traceDirLastUsed(filepath.Join(runsRoot, traceID)); runsLast.After(last) {
			last = runsLast
		}
		if ledgerTime, ok := ledgerLast[traceID]; ok && ledgerTime.After(last) {
			last = ledgerTime
		}
		protected := false
		if dirty, err := gitroot.Dirty(path); err == nil && dirty {
			protected = true
		}
		out = append(out, PruneCandidate{TraceID: traceID, Path: path, LastUsed: last, Protected: protected})
	}
	return out, nil
}

// traceDirLastUsed walks a trace-scoped directory and returns the newest file
// modification time. Run directories are small, so a full walk is cheap.
func traceDirLastUsed(root string) time.Time {
	var last time.Time
	_ = filepath.WalkDir(root, func(_ string, d fs.DirEntry, err error) error {
		if err != nil {
			return nil
		}
		info, infoErr := d.Info()
		if infoErr != nil {
			return nil
		}
		if info.ModTime().After(last) {
			last = info.ModTime()
		}
		return nil
	})
	return last
}

// worktreeDirLastUsed avoids walking a full checkout. It combines the worktree
// directory and its .git pointer file; callers layer run-directory and ledger
// activity on top.
func worktreeDirLastUsed(path string) time.Time {
	var last time.Time
	if info, err := os.Stat(path); err == nil {
		last = info.ModTime()
	}
	if info, err := os.Stat(filepath.Join(path, ".git")); err == nil && info.ModTime().After(last) {
		last = info.ModTime()
	}
	return last
}

func olderThan(candidates []PruneCandidate, cutoff time.Time) []PruneCandidate {
	var out []PruneCandidate
	for _, c := range candidates {
		if c.Protected {
			continue
		}
		if c.LastUsed.Before(cutoff) {
			out = append(out, c)
		}
	}
	return out
}
