package worktree

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/paseka/paseka/internal/colony"
	"github.com/paseka/paseka/internal/gitroot"
)

// MergeOptions configures merging a trace worktree branch into the default branch.
type MergeOptions struct {
	ColonyRoot string
	TraceID    string
	Slug       string
	Message    string
}

// MergeResult reports the merge outcome.
type MergeResult struct {
	CommitSHA string
	Branch    string
}

// Merge merges the trace worktree branch into the colony default branch with a merge commit.
func Merge(opts MergeOptions) (MergeResult, error) {
	if opts.ColonyRoot == "" || opts.TraceID == "" {
		return MergeResult{}, fmt.Errorf("worktree: colony root and traceId are required")
	}
	colonyRoot, err := absPath(opts.ColonyRoot)
	if err != nil {
		return MergeResult{}, err
	}

	entry, ok, err := findWorktreeEntry(opts.Slug, opts.TraceID)
	if err != nil {
		return MergeResult{}, err
	}
	branch := branchName(opts.TraceID)
	if ok && entry.Branch != "" {
		branch = entry.Branch
	}

	defaultBranch, err := gitroot.DefaultBranch(colonyRoot)
	if err != nil {
		return MergeResult{}, fmt.Errorf("worktree: resolve default branch: %w", err)
	}
	if defaultBranch == "" || defaultBranch == "HEAD" {
		defaultBranch = "main"
	}

	if dirty, err := hasUncommittedChanges(colonyRoot); err != nil {
		return MergeResult{}, err
	} else if dirty {
		return MergeResult{}, fmt.Errorf("worktree: colony root has uncommitted changes — commit or stash before merge")
	}

	message := strings.TrimSpace(opts.Message)
	if message == "" {
		message = fmt.Sprintf("paseka: merge trace %s", opts.TraceID)
	}

	if err := runGit(colonyRoot, "checkout", defaultBranch); err != nil {
		return MergeResult{}, err
	}
	if err := runGit(colonyRoot, "merge", "--no-ff", "-m", message, branch); err != nil {
		return MergeResult{}, err
	}
	commitSHA, err := revParse(colonyRoot, "HEAD")
	if err != nil {
		return MergeResult{}, err
	}

	if err := removeTraceWorktree(colonyRoot, opts.Slug, opts.TraceID); err != nil {
		return MergeResult{}, err
	}

	return MergeResult{CommitSHA: commitSHA, Branch: defaultBranch}, nil
}

func findWorktreeEntry(slug, traceID string) (colony.WorktreeEntry, bool, error) {
	if slug == "" {
		return colony.WorktreeEntry{}, false, nil
	}
	st, err := colony.LoadState(slug)
	if err != nil {
		return colony.WorktreeEntry{}, false, err
	}
	for _, w := range st.Worktrees {
		if w.TraceID == traceID {
			return w, true, nil
		}
	}
	return colony.WorktreeEntry{}, false, nil
}

func hasUncommittedChanges(dir string) (bool, error) {
	cmd := exec.Command("git", "-C", dir, "status", "--porcelain")
	out, err := cmd.Output()
	if err != nil {
		return false, err
	}
	return len(strings.TrimSpace(string(out))) > 0, nil
}

func removeTraceWorktree(colonyRoot, slug, traceID string) error {
	path := Path(colonyRoot, traceID)
	if gitroot.IsInsideWorkTree(path) {
		if err := runGit(colonyRoot, "worktree", "remove", "--force", path); err != nil {
			return err
		}
	} else if _, err := os.Stat(path); err == nil {
		if err := os.RemoveAll(path); err != nil {
			return err
		}
	}
	if err := runGit(colonyRoot, "worktree", "prune"); err != nil {
		return err
	}
	if slug != "" {
		return colony.UnregisterWorktree(slug, traceID)
	}
	return nil
}

func runGit(dir string, args ...string) error {
	full := append([]string{"-C", dir}, args...)
	cmd := exec.Command("git", full...)
	out, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("git %s: %w: %s", strings.Join(args, " "), err, strings.TrimSpace(string(out)))
	}
	return nil
}

func absPath(path string) (string, error) {
	if path == "" {
		return "", fmt.Errorf("worktree: path is required")
	}
	return filepath.Abs(path)
}
