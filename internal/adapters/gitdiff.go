package adapters

import (
	"context"
	"os/exec"
)

// GitDiff returns unstaged and staged changes in workspace relative to HEAD.
func GitDiff(ctx context.Context, workspace string) (string, error) {
	cmd := exec.CommandContext(ctx, "git", "diff", "HEAD")
	cmd.Dir = workspace
	out, err := cmd.Output()
	if err != nil {
		cmd = exec.CommandContext(ctx, "git", "diff")
		cmd.Dir = workspace
		out, err = cmd.Output()
		if err != nil {
			return "", err
		}
	}
	return string(out), nil
}
