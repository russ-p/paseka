package worktree

import (
	"fmt"
	"os/exec"
	"strings"

	"github.com/paseka/paseka/internal/gitroot"
)

const maxMergeDiffBytes = 1 << 20 // 1 MiB

// MergeDiffOptions configures computing the merge preview diff for a trace worktree.
type MergeDiffOptions struct {
	ColonyRoot string
	TraceID    string
	Slug       string
}

// MergeDiffResult is the accumulated diff between the default branch and the trace branch.
type MergeDiffResult struct {
	TraceID       string
	DefaultBranch string
	Branch        string
	BaseSHA       string
	HeadSHA       string
	Stat          string
	Diff          string
	Truncated     bool
	Empty         bool
	Missing       bool
}

// MergeDiff returns a three-dot diff of defaultBranch...traceBranch from the colony root.
func MergeDiff(opts MergeDiffOptions) (MergeDiffResult, error) {
	res := MergeDiffResult{TraceID: opts.TraceID}
	if opts.ColonyRoot == "" || opts.TraceID == "" {
		return res, fmt.Errorf("worktree: colony root and traceId are required")
	}
	colonyRoot, err := absPath(opts.ColonyRoot)
	if err != nil {
		return res, err
	}

	branch, err := ResolvedBranch(colonyRoot, opts.TraceID, opts.Slug)
	if err != nil {
		return res, err
	}
	res.Branch = branch

	defaultBranch, err := gitroot.DefaultBranch(colonyRoot)
	if err != nil {
		return res, fmt.Errorf("worktree: resolve default branch: %w", err)
	}
	if defaultBranch == "" || defaultBranch == "HEAD" {
		defaultBranch = "main"
	}
	res.DefaultBranch = defaultBranch

	if !gitroot.LocalBranchExists(colonyRoot, branch) {
		res.Missing = true
		return res, nil
	}

	baseSHA, err := revParse(colonyRoot, defaultBranch)
	if err != nil {
		return res, fmt.Errorf("worktree: resolve %s: %w", defaultBranch, err)
	}
	headSHA, err := revParse(colonyRoot, "refs/heads/"+branch)
	if err != nil {
		return res, fmt.Errorf("worktree: resolve %s: %w", branch, err)
	}
	res.BaseSHA = baseSHA
	res.HeadSHA = headSHA

	rangeSpec := defaultBranch + "...refs/heads/" + branch
	stat, err := gitOutput(colonyRoot, "diff", "--stat", rangeSpec)
	if err != nil {
		return res, err
	}
	res.Stat = stat

	diff, truncated, err := gitOutputTruncated(colonyRoot, maxMergeDiffBytes, "diff", rangeSpec)
	if err != nil {
		return res, err
	}
	res.Diff = diff
	res.Truncated = truncated
	res.Empty = strings.TrimSpace(stat) == "" && strings.TrimSpace(diff) == ""
	return res, nil
}

func gitOutput(dir string, args ...string) (string, error) {
	full := append([]string{"-C", dir}, args...)
	cmd := exec.Command("git", full...)
	out, err := cmd.Output()
	if err != nil {
		if ee, ok := err.(*exec.ExitError); ok {
			return "", fmt.Errorf("git %s: %w: %s", strings.Join(args, " "), err, strings.TrimSpace(string(ee.Stderr)))
		}
		return "", fmt.Errorf("git %s: %w", strings.Join(args, " "), err)
	}
	return string(out), nil
}

func gitOutputTruncated(dir string, maxBytes int, args ...string) (string, bool, error) {
	full := append([]string{"-C", dir}, args...)
	cmd := exec.Command("git", full...)
	out, err := cmd.Output()
	if err != nil {
		if ee, ok := err.(*exec.ExitError); ok {
			return "", false, fmt.Errorf("git %s: %w: %s", strings.Join(args, " "), err, strings.TrimSpace(string(ee.Stderr)))
		}
		return "", false, fmt.Errorf("git %s: %w", strings.Join(args, " "), err)
	}
	if len(out) <= maxBytes {
		return string(out), false, nil
	}
	return string(out[:maxBytes]), true, nil
}
