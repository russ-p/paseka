package colony

import (
	"bytes"
	"context"
	"fmt"
	"os"
	"os/exec"
	"strings"
)

// ExecArgv runs argv[0] with argv[1:] as a child process. When workspace is non-empty it
// becomes the process working directory.
func ExecArgv(ctx context.Context, argv []string, workspace string) error {
	if len(argv) == 0 {
		return fmt.Errorf("colony: empty command")
	}
	cmd := exec.CommandContext(ctx, argv[0], argv[1:]...)
	if workspace != "" {
		cmd.Dir = workspace
	}
	cmd.Env = os.Environ()
	var stderr bytes.Buffer
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		msg := strings.TrimSpace(stderr.String())
		if msg != "" {
			return fmt.Errorf("colony: %s: %w: %s", argv[0], err, msg)
		}
		return fmt.Errorf("colony: %s: %w", argv[0], err)
	}
	return nil
}

// RunPostExec renders and runs a bee post_exec hook when configured.
func RunPostExec(ctx context.Context, hook Command, vars CommandVars, workspace string) error {
	if !hook.IsSet() {
		return nil
	}
	argv, err := hook.RenderCommand(vars)
	if err != nil {
		return fmt.Errorf("colony: render post_exec: %w", err)
	}
	return ExecArgv(ctx, argv, workspace)
}
