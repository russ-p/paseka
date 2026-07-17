package adapters

import (
	"context"
	"os/exec"
	"path/filepath"
	"strings"
)

// WorkspaceBaseline captures tracked dirty state before an adapter run.
type WorkspaceBaseline struct {
	BaseSHA    string
	FileHashes map[string]string // repo-relative path -> git blob hash
}

// CaptureWorkspaceBaseline records HEAD and hashes of files dirty vs HEAD.
func CaptureWorkspaceBaseline(ctx context.Context, workspace string) (WorkspaceBaseline, error) {
	baseSHA, err := gitRevParse(ctx, workspace, "HEAD")
	if err != nil {
		return WorkspaceBaseline{}, err
	}
	files, err := gitDiffNameOnly(ctx, workspace)
	if err != nil {
		return WorkspaceBaseline{}, err
	}
	hashes := make(map[string]string, len(files))
	for _, file := range files {
		hash, err := gitHashObject(ctx, workspace, file)
		if err != nil {
			return WorkspaceBaseline{}, err
		}
		hashes[file] = hash
	}
	return WorkspaceBaseline{
		BaseSHA:    baseSHA,
		FileHashes: hashes,
	}, nil
}

// AttributableDiff returns git diff HEAD for tracked files whose content changed since baseline.
func AttributableDiff(ctx context.Context, workspace string, baseline WorkspaceBaseline) (string, error) {
	files, err := gitDiffNameOnly(ctx, workspace)
	if err != nil {
		return "", err
	}
	var attributable []string
	for _, file := range files {
		hash, err := gitHashObject(ctx, workspace, file)
		if err != nil {
			return "", err
		}
		prev, ok := baseline.FileHashes[file]
		if !ok || prev != hash {
			attributable = append(attributable, file)
		}
	}
	if len(attributable) == 0 {
		return "", nil
	}
	return gitDiffFiles(ctx, workspace, attributable)
}

func gitRevParse(ctx context.Context, workspace, ref string) (string, error) {
	cmd := exec.CommandContext(ctx, "git", "rev-parse", ref)
	cmd.Dir = workspace
	out, err := cmd.Output()
	if err != nil {
		return "", err
	}
	return strings.TrimSpace(string(out)), nil
}

func gitDiffNameOnly(ctx context.Context, workspace string) ([]string, error) {
	cmd := exec.CommandContext(ctx, "git", "diff", "HEAD", "--name-only")
	cmd.Dir = workspace
	out, err := cmd.Output()
	if err != nil {
		cmd = exec.CommandContext(ctx, "git", "diff", "--name-only")
		cmd.Dir = workspace
		out, err = cmd.Output()
		if err != nil {
			return nil, err
		}
	}
	var files []string
	for _, line := range strings.Split(string(out), "\n") {
		line = strings.TrimSpace(line)
		if line != "" {
			files = append(files, filepath.ToSlash(line))
		}
	}
	return files, nil
}

func gitHashObject(ctx context.Context, workspace, file string) (string, error) {
	cmd := exec.CommandContext(ctx, "git", "hash-object", file)
	cmd.Dir = workspace
	out, err := cmd.Output()
	if err != nil {
		return "", err
	}
	return strings.TrimSpace(string(out)), nil
}

func gitDiffFiles(ctx context.Context, workspace string, files []string) (string, error) {
	args := append([]string{"diff", "HEAD", "--"}, files...)
	cmd := exec.CommandContext(ctx, "git", args...)
	cmd.Dir = workspace
	out, err := cmd.Output()
	if err != nil {
		args = append([]string{"diff", "--"}, files...)
		cmd = exec.CommandContext(ctx, "git", args...)
		cmd.Dir = workspace
		out, err = cmd.Output()
		if err != nil {
			return "", err
		}
	}
	return string(out), nil
}
