package worktree

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"github.com/paseka/paseka/internal/colony"
	"github.com/paseka/paseka/internal/gitroot"
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

	path := Path(colonyRoot, opts.TraceID)
	if gitroot.IsInsideWorkTree(path) {
		return entryFromPath(colonyRoot, opts.TraceID, path)
	}
	if _, err := os.Stat(path); err == nil {
		return Entry{}, fmt.Errorf("worktree: %s exists but is not a git worktree", path)
	}

	baseSHA, err := revParse(colonyRoot, "HEAD")
	if err != nil {
		return Entry{}, fmt.Errorf("worktree: resolve HEAD: %w", err)
	}

	branch := branchName(opts.TraceID)
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return Entry{}, err
	}

	if err := addWorktree(colonyRoot, branch, path, "HEAD"); err != nil {
		if strings.Contains(err.Error(), "already exists") {
			if err := addWorktree(colonyRoot, "", path, "HEAD"); err != nil {
				return Entry{}, err
			}
			branch = ""
		} else {
			return Entry{}, err
		}
	}

	entry := Entry{
		TraceID:   opts.TraceID,
		Path:      path,
		BaseSHA:   baseSHA,
		Branch:    branch,
		CreatedAt: time.Now().UTC(),
	}

	if opts.Slug != "" {
		if err := colony.RegisterWorktree(opts.Slug, colony.WorktreeEntry{
			TraceID:   entry.TraceID,
			Path:      entry.Path,
			BaseSHA:   entry.BaseSHA,
			Branch:    entry.Branch,
			CreatedAt: entry.CreatedAt,
		}); err != nil {
			return Entry{}, err
		}
	}
	return entry, nil
}

// Path returns the absolute worktree directory for a trace.
func Path(colonyRoot, traceID string) string {
	return filepath.Join(colonyRoot, ".paseka", "worktrees", traceID)
}

func entryFromPath(colonyRoot, traceID, path string) (Entry, error) {
	baseSHA, err := revParse(path, "HEAD")
	if err != nil {
		return Entry{}, err
	}
	branch, _ := gitroot.DefaultBranch(path)
	return Entry{
		TraceID: traceID,
		Path:    path,
		BaseSHA: baseSHA,
		Branch:  branch,
	}, nil
}

func branchName(traceID string) string {
	safe := strings.NewReplacer("/", "-", " ", "-").Replace(traceID)
	return "paseka/" + safe
}

func addWorktree(colonyRoot, branch, path, startPoint string) error {
	args := []string{"-C", colonyRoot, "worktree", "add"}
	if branch != "" {
		args = append(args, "-b", branch)
	}
	args = append(args, path, startPoint)
	cmd := exec.Command("git", args...)
	out, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("git worktree add: %w: %s", err, strings.TrimSpace(string(out)))
	}
	return nil
}

func revParse(dir, ref string) (string, error) {
	cmd := exec.Command("git", "-C", dir, "rev-parse", ref)
	out, err := cmd.Output()
	if err != nil {
		return "", err
	}
	return strings.TrimSpace(string(out)), nil
}
