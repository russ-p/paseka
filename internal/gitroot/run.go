package gitroot

import (
	"bytes"
	"context"
	"fmt"
	"os"
	"os/exec"
	"strings"
	"time"
)

const (
	// StatusTimeout bounds read-only git (status, rev-parse, for-each-ref).
	StatusTimeout = 15 * time.Second
	// NetworkTimeout bounds fetch, push, and pull.
	NetworkTimeout = 60 * time.Second
	// OutputCap is the max combined stdout/stderr returned from Run.
	OutputCap = 16 * 1024
)

// RunOpts configures a git child process.
type RunOpts struct {
	Dir       string
	Args      []string
	Timeout   time.Duration
	ExtraEnv  []string
	AllowFail bool
}

// Run runs git -C dir with empty stdin, inherited environment plus ExtraEnv, and a timeout.
// Combined output is capped. Do not set GIT_ASKPASS or GIT_TERMINAL_PROMPT=1.
func Run(opts RunOpts) (string, error) {
	if len(opts.Args) == 0 {
		return "", fmt.Errorf("git: no args")
	}
	timeout := opts.Timeout
	if timeout <= 0 {
		timeout = StatusTimeout
	}
	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()

	full := opts.Args
	if opts.Dir != "" {
		full = append([]string{"-C", opts.Dir}, opts.Args...)
	}
	cmd := exec.CommandContext(ctx, "git", full...)
	cmd.Stdin = bytes.NewReader(nil)
	if len(opts.ExtraEnv) > 0 {
		cmd.Env = append(os.Environ(), opts.ExtraEnv...)
	}
	out, err := cmd.CombinedOutput()
	text := capOutput(out)
	if err != nil {
		if ctx.Err() == context.DeadlineExceeded {
			return text, fmt.Errorf("git %s: timed out: %s", strings.Join(opts.Args, " "), text)
		}
		if opts.AllowFail {
			return text, err
		}
		return text, fmt.Errorf("git %s: %w: %s", strings.Join(opts.Args, " "), err, strings.TrimSpace(text))
	}
	return text, nil
}

func capOutput(out []byte) string {
	if len(out) <= OutputCap {
		return string(out)
	}
	return string(out[:OutputCap]) + "\n...[truncated]"
}
