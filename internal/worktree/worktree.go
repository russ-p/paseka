package worktree

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"github.com/paseka/paseka/internal/gitroot"
	"github.com/paseka/paseka/internal/homestate"
	"github.com/paseka/paseka/internal/protocol"
	"github.com/paseka/paseka/internal/runs"
)

// Entry describes a colony-managed worktree.
type Entry struct {
	TraceID   string
	Path      string
	BaseSHA   string
	Branch    string
	CreatedAt time.Time
}

// EnsureOptions configures worktree creation or reuse.
type EnsureOptions struct {
	ColonyRoot string
	TraceID    string
	Slug       string // home config slug for state.json
	Branch     string // optional resolved branch override (else from latest insight)
}

// ApplyBranchOptions renames an existing trace worktree to the given branch name.
type ApplyBranchOptions struct {
	ColonyRoot string
	TraceID    string
	Slug       string
	Branch     string
}

// BranchName returns the default anonymous branch for a trace: paseka/<safeTraceId>.
func BranchName(traceID string) string {
	return gitroot.IsolatedWorktreeBranch(traceID)
}

// Resolve returns the git branch name for isolated work: insight when set, else default.
func Resolve(traceID, insightBranch string) string {
	if b := strings.TrimSpace(insightBranch); b != "" {
		return b
	}
	return BranchName(traceID)
}

// Ensure creates or reuses .paseka/worktrees/<traceId>/ under the colony root.
func Ensure(opts EnsureOptions) (Entry, error) {
	if opts.ColonyRoot == "" || opts.TraceID == "" {
		return Entry{}, fmt.Errorf("worktree: colony root and traceId are required")
	}
	colonyRoot, err := filepath.Abs(opts.ColonyRoot)
	if err != nil {
		return Entry{}, err
	}

	insight, resolved, err := resolveBranchForEnsure(colonyRoot, opts)
	if err != nil {
		return Entry{}, err
	}
	if details := protocol.ValidateBranchRef(resolved, colonyRoot); len(details) > 0 {
		return Entry{}, fmt.Errorf("worktree: invalid branch %q: %s", resolved, details[0].Message)
	}

	path := Path(colonyRoot, opts.TraceID)
	if gitroot.IsInsideWorkTree(path) {
		if strings.TrimSpace(insight) == "" {
			return keepExistingBranch(colonyRoot, opts.Slug, opts.TraceID, path)
		}
		return reconcileBranch(colonyRoot, opts.Slug, opts.TraceID, path, resolved)
	}
	if _, err := os.Stat(path); err == nil {
		return Entry{}, fmt.Errorf("worktree: %s exists but is not a git worktree", path)
	}

	baseSHA, err := revParse(colonyRoot, "HEAD")
	if err != nil {
		return Entry{}, fmt.Errorf("worktree: resolve HEAD: %w", err)
	}

	if gitroot.LocalBranchExists(colonyRoot, resolved) {
		return Entry{}, fmt.Errorf("worktree: branch %q already exists", resolved)
	}

	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return Entry{}, err
	}

	if err := addWorktree(colonyRoot, resolved, path, "HEAD"); err != nil {
		return Entry{}, err
	}

	entry := Entry{
		TraceID:   opts.TraceID,
		Path:      path,
		BaseSHA:   baseSHA,
		Branch:    resolved,
		CreatedAt: time.Now().UTC(),
	}

	if opts.Slug != "" {
		if err := homestate.RegisterWorktree(opts.Slug, homestate.WorktreeEntry{
			TraceID:   entry.TraceID,
			Path:      entry.Path,
			BaseSHA:   entry.BaseSHA,
			Branch:    entry.Branch,
			CreatedAt: entry.CreatedAt,
		}); err != nil {
			return Entry{}, err
		}
		if err := homestate.UpdateWorktreeBranch(opts.Slug, entry.TraceID, entry.Branch); err != nil {
			return Entry{}, err
		}
	}
	return entry, nil
}

// ApplyBranchInsight renames the trace worktree branch when a worktree.branch insight lands.
// No-op when no worktree exists yet for the trace.
func ApplyBranchInsight(opts ApplyBranchOptions) error {
	if opts.ColonyRoot == "" || opts.TraceID == "" {
		return fmt.Errorf("worktree: colony root and traceId are required")
	}
	branch := strings.TrimSpace(opts.Branch)
	if branch == "" {
		return nil
	}
	colonyRoot, err := filepath.Abs(opts.ColonyRoot)
	if err != nil {
		return err
	}
	if details := protocol.ValidateBranchRef(branch, colonyRoot); len(details) > 0 {
		return fmt.Errorf("worktree: invalid branch %q: %s", branch, details[0].Message)
	}
	path := Path(colonyRoot, opts.TraceID)
	if !gitroot.IsInsideWorkTree(path) {
		return nil
	}
	_, err = reconcileBranch(colonyRoot, opts.Slug, opts.TraceID, path, branch)
	return err
}

// Path returns the absolute worktree directory for a trace.
func Path(colonyRoot, traceID string) string {
	return filepath.Join(colonyRoot, ".paseka", "worktrees", traceID)
}

// ResolvedBranch returns the branch name used for merge and merge-diff.
// Preference: live worktree HEAD, registry Branch, latest insight or default.
func ResolvedBranch(colonyRoot, traceID, slug string) (string, error) {
	colonyRoot, err := absPath(colonyRoot)
	if err != nil {
		return "", err
	}
	path := Path(colonyRoot, traceID)
	if gitroot.IsInsideWorkTree(path) {
		branch, err := currentBranch(path)
		if err == nil && branch != "" && branch != "HEAD" {
			return branch, nil
		}
	}
	entry, ok, err := findWorktreeEntry(slug, traceID)
	if err != nil {
		return "", err
	}
	if ok && strings.TrimSpace(entry.Branch) != "" {
		return entry.Branch, nil
	}
	insight, err := runs.LatestWorktreeBranch(colonyRoot, traceID)
	if err != nil {
		return "", err
	}
	return Resolve(traceID, insight), nil
}

func resolveBranchForEnsure(colonyRoot string, opts EnsureOptions) (insight, resolved string, err error) {
	if b := strings.TrimSpace(opts.Branch); b != "" {
		return b, b, nil
	}
	insight, err = runs.LatestWorktreeBranch(colonyRoot, opts.TraceID)
	if err != nil {
		return "", "", err
	}
	return insight, Resolve(opts.TraceID, insight), nil
}

func keepExistingBranch(colonyRoot, slug, traceID, path string) (Entry, error) {
	entry, err := entryFromPath(colonyRoot, traceID, path)
	if err != nil {
		return Entry{}, err
	}
	if slug != "" && strings.TrimSpace(entry.Branch) != "" && entry.Branch != "HEAD" {
		if err := homestate.UpdateWorktreeBranch(slug, traceID, entry.Branch); err != nil {
			return Entry{}, err
		}
	}
	return entry, nil
}

func reconcileBranch(colonyRoot, slug, traceID, path, resolved string) (Entry, error) {
	entry, err := entryFromPath(colonyRoot, traceID, path)
	if err != nil {
		return Entry{}, err
	}

	current, err := currentBranch(path)
	if err != nil {
		return Entry{}, fmt.Errorf("worktree: read current branch: %w", err)
	}

	if current == resolved {
		if slug != "" {
			if err := homestate.UpdateWorktreeBranch(slug, traceID, resolved); err != nil {
				return Entry{}, err
			}
		}
		entry.Branch = resolved
		return entry, nil
	}

	if current == "HEAD" || current == "" {
		if gitroot.LocalBranchExists(colonyRoot, resolved) {
			if worktreeOwnsBranch(colonyRoot, path, resolved) {
				if err := checkoutBranch(path, resolved); err != nil {
					return Entry{}, err
				}
			} else {
				return Entry{}, fmt.Errorf("worktree: branch %q already exists", resolved)
			}
		} else {
			if err := createBranch(path, resolved); err != nil {
				return Entry{}, err
			}
		}
	} else {
		if gitroot.LocalBranchExists(colonyRoot, resolved) {
			return Entry{}, fmt.Errorf("worktree: branch %q already exists", resolved)
		}
		if err := renameBranch(path, resolved); err != nil {
			return Entry{}, err
		}
	}

	if slug != "" {
		if err := homestate.UpdateWorktreeBranch(slug, traceID, resolved); err != nil {
			return Entry{}, err
		}
	}

	entry, err = entryFromPath(colonyRoot, traceID, path)
	if err != nil {
		return Entry{}, err
	}
	entry.Branch = resolved
	return entry, nil
}

func entryFromPath(colonyRoot, traceID, path string) (Entry, error) {
	baseSHA, err := revParse(path, "HEAD")
	if err != nil {
		return Entry{}, err
	}
	branch, _ := currentBranch(path)
	return Entry{
		TraceID: traceID,
		Path:    path,
		BaseSHA: baseSHA,
		Branch:  branch,
	}, nil
}

func addWorktree(colonyRoot, branch, path, startPoint string) error {
	cmd := exec.Command("git", "-C", colonyRoot, "worktree", "add", "-b", branch, "--", path, startPoint)
	out, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("git worktree add: %w: %s", err, strings.TrimSpace(string(out)))
	}
	return nil
}

func currentBranch(dir string) (string, error) {
	cmd := exec.Command("git", "-C", dir, "rev-parse", "--abbrev-ref", "HEAD")
	out, err := cmd.Output()
	if err != nil {
		return "", err
	}
	return strings.TrimSpace(string(out)), nil
}

func renameBranch(worktreeDir, newName string) error {
	cmd := exec.Command("git", "-C", worktreeDir, "branch", "-m", "--", newName)
	out, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("git branch -m: %w: %s", err, strings.TrimSpace(string(out)))
	}
	return nil
}

func createBranch(worktreeDir, name string) error {
	cmd := exec.Command("git", "-C", worktreeDir, "switch", "-c", name)
	out, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("git switch -c: %w: %s", err, strings.TrimSpace(string(out)))
	}
	return nil
}

func checkoutBranch(worktreeDir, name string) error {
	cmd := exec.Command("git", "-C", worktreeDir, "switch", "--", name)
	out, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("git switch: %w: %s", err, strings.TrimSpace(string(out)))
	}
	return nil
}

func worktreeOwnsBranch(colonyRoot, worktreePath, branch string) bool {
	worktreePath, err := filepath.Abs(worktreePath)
	if err != nil {
		return false
	}
	cmd := exec.Command("git", "-C", colonyRoot, "worktree", "list", "--porcelain")
	out, err := cmd.Output()
	if err != nil {
		return false
	}
	var currentPath string
	for line := range strings.SplitSeq(string(out), "\n") {
		if strings.HasPrefix(line, "worktree ") {
			currentPath = strings.TrimPrefix(line, "worktree ")
			continue
		}
		if strings.HasPrefix(line, "branch ") && filepath.Clean(currentPath) == filepath.Clean(worktreePath) {
			ref := strings.TrimPrefix(line, "branch ")
			ref = strings.TrimPrefix(ref, "refs/heads/")
			return ref == branch
		}
	}
	return false
}

func revParse(dir, ref string) (string, error) {
	cmd := exec.Command("git", "-C", dir, "rev-parse", ref)
	out, err := cmd.Output()
	if err != nil {
		return "", err
	}
	return strings.TrimSpace(string(out)), nil
}
