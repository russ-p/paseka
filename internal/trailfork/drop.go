package trailfork

import (
	"context"
	"errors"
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/russ-p/paseka/internal/colony"
	"github.com/russ-p/paseka/internal/gitroot"
	"github.com/russ-p/paseka/internal/tasks"
	"github.com/russ-p/paseka/internal/trailconfig"
	"github.com/russ-p/paseka/internal/worktree"
)

// DropOptions describes one trail drop.
type DropOptions struct {
	Context colony.Context
	TraceID string
	Reason  string
	Actor   string
	// Session records the kill that stops dispatch. It is required unless the
	// fork was already dropped: an active fork's execution resources must never
	// be removed before its kill is durable.
	Session *tasks.LedgerSession
	Now     time.Time
	DryRun  bool
}

// DropResult reports what dropping a fork stopped and removed, and which
// evidence stayed behind.
type DropResult struct {
	TraceID string
	ForkOf  string
	// AlreadyDropped is true when a previous drop had already recorded the
	// stopped state; the call still cleans up leftover execution resources.
	AlreadyDropped  bool
	Stopped         bool
	Killed          bool
	Workspace       string
	Branch          string
	RemovedWorktree string
	RemovedBranch   string
	// RetainedConfigDir and RetainedLineagePath point at the evidence a drop
	// deliberately keeps readable.
	RetainedConfigDir   string
	RetainedLineagePath string
	ConfigRefs          []string
	Lineage             Lineage
}

// Drop stops a fork before removing its execution resources.
//
// The recorded fork lineage is loaded first: only an experimental trail with a
// retained configuration binding can be dropped this way. Dispatch is stopped by
// recording the trace kill and waiting for the ledger to report it; the worktree
// and branch are removed only after that. Configuration snapshots, lineage,
// attempt ownership, run records, and the comb are retained, so the fork stays
// inspectable and a later source deletion cannot invalidate its evidence.
func Drop(ctx context.Context, opts DropOptions) (DropResult, error) {
	root := strings.TrimSpace(opts.Context.ColonyRoot)
	if root == "" {
		return DropResult{}, fmt.Errorf("trailfork: colony root is required")
	}
	traceID := strings.TrimSpace(opts.TraceID)
	if traceID == "" {
		return DropResult{}, fmt.Errorf("trailfork: trace id is required")
	}
	lineage, err := LoadLineage(root, traceID)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return DropResult{}, fmt.Errorf("trailfork: trail %s is not an experimental fork; ordinary trail cleanup uses paseka purge", traceID)
		}
		return DropResult{}, err
	}
	manifest, err := trailconfig.Load(root, traceID)
	if err != nil {
		if errors.Is(err, trailconfig.ErrNoBinding) {
			return DropResult{}, fmt.Errorf("trailfork: fork %s has no configuration binding to retain", traceID)
		}
		return DropResult{}, err
	}

	now := opts.Now
	if now.IsZero() {
		now = time.Now().UTC()
	}
	actor := strings.TrimSpace(opts.Actor)
	if actor == "" {
		actor = "cli"
	}

	result := DropResult{
		TraceID:             traceID,
		ForkOf:              lineage.SourceTrace,
		RetainedConfigDir:   trailconfig.ConfigDir(root, traceID),
		RetainedLineagePath: LineagePath(root, traceID),
		Lineage:             lineage,
	}
	for _, snapshot := range manifest.Snapshots {
		result.ConfigRefs = append(result.ConfigRefs, snapshot.ConfigRef)
	}
	workspace := lineage.Workspace
	if workspace == "" {
		workspace = worktree.Path(root, traceID)
	}
	branch := lineage.Branch
	if branch == "" {
		branch = worktree.BranchName(traceID)
	}
	result.Workspace = workspace
	result.Branch = branch

	if lineage.State == StateDropped {
		result.AlreadyDropped = true
		result.Killed = true
	} else if opts.DryRun {
		return result, nil
	} else {
		if opts.Session == nil || opts.Session.Ledger == nil || opts.Session.Publisher == nil {
			return DropResult{}, fmt.Errorf("trailfork: dropping %s requires the task ledger so dispatch is stopped before resources are removed", traceID)
		}
		snap, err := opts.Session.Ledger.Snapshot(traceID)
		if err != nil {
			return DropResult{}, err
		}
		if !snap.Killed {
			snap, err = tasks.KillTrace(ctx, opts.Session, tasks.KillTraceInput{
				TraceID: traceID,
				Reason:  killReason(opts.Reason),
				AgentID: "cli",
			})
			if err != nil {
				return DropResult{}, fmt.Errorf("trailfork: stop fork %s: %w", traceID, err)
			}
		}
		// The kill must be durable before any execution resource is removed.
		if !snap.Killed {
			return DropResult{}, fmt.Errorf("trailfork: fork %s is not recorded as stopped; refusing to remove its execution resources", traceID)
		}
		result.Stopped = true
		result.Killed = true
		if err := trailconfig.CloseTraceAttempts(root, traceID, "trail dropped"); err != nil {
			return DropResult{}, err
		}
	}

	if opts.DryRun {
		return result, nil
	}

	if err := worktree.Remove(root, opts.Context.Slug, traceID); err != nil {
		return result, fmt.Errorf("trailfork: remove fork worktree: %w", err)
	}
	result.RemovedWorktree = workspace
	if gitroot.LocalBranchExists(root, branch) {
		if _, err := gitroot.Run(gitroot.RunOpts{
			Dir:  root,
			Args: []string{"branch", "-D", branch},
		}); err != nil {
			return result, fmt.Errorf("trailfork: remove fork branch %q: %w", branch, err)
		}
		result.RemovedBranch = branch
	}

	if !result.AlreadyDropped {
		lineage.State = StateDropped
		lineage.DropReason = strings.TrimSpace(opts.Reason)
		lineage.DroppedBy = actor
		droppedAt := now
		lineage.DroppedAt = &droppedAt
		if err := writeLineage(root, lineage); err != nil {
			return result, err
		}
		result.Lineage = lineage
	}
	return result, nil
}

func killReason(reason string) string {
	reason = strings.TrimSpace(reason)
	if reason == "" {
		return "trail dropped"
	}
	return "trail dropped: " + reason
}
