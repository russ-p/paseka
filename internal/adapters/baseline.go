package adapters

import (
	"context"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/russ-p/paseka/internal/gitroot"
)

// deletedFileHash marks a tracked path that was already absent from the working
// tree when the baseline was captured. Real git blob hashes are hex strings, so
// the sentinel cannot collide with one.
const deletedFileHash = "<deleted>"

// WorkspaceBaseline captures tracked dirty state before an adapter run.
type WorkspaceBaseline struct {
	BaseSHA    string
	FileHashes map[string]string // repo-relative path -> git blob hash or deletedFileHash
}

// CaptureWorkspaceBaseline records HEAD and hashes of files dirty vs HEAD.
// workspace may be the git root or a sector subdirectory; paths are always
// resolved against the repository toplevel.
func CaptureWorkspaceBaseline(ctx context.Context, workspace string) (WorkspaceBaseline, error) {
	root, err := gitroot.Find(workspace)
	if err != nil {
		return WorkspaceBaseline{}, err
	}
	baseSHA, err := gitRevParse(ctx, root, "HEAD")
	if err != nil {
		return WorkspaceBaseline{}, err
	}
	files, err := gitDiffNameOnly(ctx, root)
	if err != nil {
		return WorkspaceBaseline{}, err
	}
	hashes := make(map[string]string, len(files))
	for _, file := range files {
		hash, err := workspaceFileHash(ctx, root, file)
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
// workspace may be the git root or a sector subdirectory.
func AttributableDiff(ctx context.Context, workspace string, baseline WorkspaceBaseline) (string, error) {
	root, err := gitroot.Find(workspace)
	if err != nil {
		return "", err
	}
	files, err := gitDiffNameOnly(ctx, root)
	if err != nil {
		return "", err
	}
	var attributable []string
	for _, file := range files {
		hash, err := workspaceFileHash(ctx, root, file)
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
	return gitDiffFiles(ctx, root, attributable)
}

func gitRevParse(ctx context.Context, repoRoot, ref string) (string, error) {
	out, err := gitOutput(exec.CommandContext(ctx, "git", "rev-parse", ref), repoRoot)
	if err != nil {
		return "", err
	}
	return strings.TrimSpace(string(out)), nil
}

func gitDiffNameOnly(ctx context.Context, repoRoot string) ([]string, error) {
	out, err := gitOutput(exec.CommandContext(ctx, "git", "diff", "HEAD", "--name-only", "-z"), repoRoot)
	if err != nil {
		out, err = gitOutput(exec.CommandContext(ctx, "git", "diff", "--name-only", "-z"), repoRoot)
		if err != nil {
			return nil, err
		}
	}
	return splitPathList(out), nil
}

// splitPathList parses git's -z output: NUL-separated paths, never quoted.
func splitPathList(out []byte) []string {
	parts := strings.Split(string(out), "\x00")
	files := make([]string, 0, len(parts))
	for _, part := range parts {
		if part == "" {
			continue
		}
		files = append(files, filepath.ToSlash(part))
	}
	return files
}

// workspaceFileHash hashes the working-tree content of file. A path that git
// lists as changed but that no longer exists (deletion, rename source) hashes to
// deletedFileHash instead of failing hash-object.
func workspaceFileHash(ctx context.Context, repoRoot, file string) (string, error) {
	if _, err := os.Lstat(filepath.Join(repoRoot, file)); err != nil {
		if errors.Is(err, fs.ErrNotExist) {
			return deletedFileHash, nil
		}
		return "", err
	}
	return gitHashObject(ctx, repoRoot, file)
}

func gitHashObject(ctx context.Context, repoRoot, file string) (string, error) {
	out, err := gitOutput(exec.CommandContext(ctx, "git", "hash-object", file), repoRoot)
	if err != nil {
		return "", err
	}
	return strings.TrimSpace(string(out)), nil
}

func gitDiffFiles(ctx context.Context, repoRoot string, files []string) (string, error) {
	args := append([]string{"diff", "HEAD", "--"}, files...)
	out, err := gitOutput(exec.CommandContext(ctx, "git", args...), repoRoot)
	if err != nil {
		args = append([]string{"diff", "--"}, files...)
		out, err = gitOutput(exec.CommandContext(ctx, "git", args...), repoRoot)
		if err != nil {
			return "", err
		}
	}
	return string(out), nil
}

func gitOutput(cmd *exec.Cmd, dir string) ([]byte, error) {
	cmd.Dir = dir
	out, err := cmd.Output()
	if err != nil {
		if ee, ok := err.(*exec.ExitError); ok && len(ee.Stderr) > 0 {
			return nil, fmt.Errorf("%w: %s", err, strings.TrimSpace(string(ee.Stderr)))
		}
		return nil, err
	}
	return out, nil
}
