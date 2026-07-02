package gitroot

import (
	"bytes"
	"fmt"
	"os/exec"
	"path/filepath"
	"strings"
)

// Find walks from dir upward is not needed — git resolves from any path inside repo.
func Find(dir string) (string, error) {
	if dir == "" {
		return "", fmt.Errorf("git: directory is required")
	}
	abs, err := filepath.Abs(dir)
	if err != nil {
		return "", err
	}
	cmd := exec.Command("git", "-C", abs, "rev-parse", "--show-toplevel")
	out, err := cmd.Output()
	if err != nil {
		return "", fmt.Errorf("git: not a repository (run from inside a git repo): %w", err)
	}
	root := strings.TrimSpace(string(out))
	if root == "" {
		return "", fmt.Errorf("git: empty repository root")
	}
	return filepath.Abs(root)
}

// OriginURL returns the origin remote URL, or empty if unset.
func OriginURL(repoRoot string) (string, error) {
	cmd := exec.Command("git", "-C", repoRoot, "remote", "get-url", "origin")
	out, err := cmd.Output()
	if err != nil {
		// no origin is fine
		return "", nil
	}
	return strings.TrimSpace(string(out)), nil
}

// DefaultBranch returns the current default branch name (best effort).
func DefaultBranch(repoRoot string) (string, error) {
	cmd := exec.Command("git", "-C", repoRoot, "rev-parse", "--abbrev-ref", "HEAD")
	out, err := cmd.Output()
	if err != nil {
		return "", err
	}
	return strings.TrimSpace(string(out)), nil
}

// IsInsideWorkTree reports whether dir is inside a git work tree.
func IsInsideWorkTree(dir string) bool {
	cmd := exec.Command("git", "-C", dir, "rev-parse", "--is-inside-work-tree")
	out, err := cmd.Output()
	return err == nil && bytes.Equal(bytes.TrimSpace(out), []byte("true"))
}
