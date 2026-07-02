package cursor

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"github.com/paseka/paseka/internal/adapters"
	"github.com/paseka/paseka/internal/runs"
)

const (
	adapterName   = "cursor"
	defaultBinary = "agent"
)

// Adapter runs the Cursor Agent CLI (agent).
type Adapter struct{}

func New() *Adapter {
	return &Adapter{}
}

func (a *Adapter) Name() string {
	return adapterName
}

// Run invokes `agent` in non-interactive mode.
// Port of fizman-parent/scripts/ai-tasks-run.sh without tmux.
func (a *Adapter) Run(ctx context.Context, req adapters.RunRequest) (*adapters.RunResult, error) {
	if req.Workspace == "" {
		return nil, errors.New("cursor: workspace is required")
	}
	if req.ColonyRoot == "" {
		return nil, errors.New("cursor: colony root is required")
	}
	if req.TraceID == "" || req.AgentID == "" {
		return nil, errors.New("cursor: traceId and agentId are required")
	}
	if req.Prompt == "" {
		return nil, errors.New("cursor: prompt is required")
	}

	binary := req.Params.Binary
	if binary == "" {
		binary = defaultBinary
	}
	if _, err := exec.LookPath(binary); err != nil {
		return nil, fmt.Errorf("cursor: %q not found in PATH (install Cursor CLI)", binary)
	}

	runDir := runs.Dir{
		ColonyRoot: req.ColonyRoot,
		TraceID:    req.TraceID,
		AgentID:    req.AgentID,
	}
	if err := runDir.Prepare(); err != nil {
		return nil, err
	}

	prompt := augmentPrompt(req.Prompt, runDir.ResultPath())
	if err := runDir.WritePrompt(prompt); err != nil {
		return nil, fmt.Errorf("cursor: write prompt: %w", err)
	}
	if err := runDir.WriteMeta(runs.Meta{
		TraceID:   req.TraceID,
		AgentID:   req.AgentID,
		Bee:       req.Bee,
		Adapter:   adapterName,
		Workspace: req.Workspace,
		StartedAt: time.Now().UTC(),
	}); err != nil {
		return nil, fmt.Errorf("cursor: write meta: %w", err)
	}

	args := buildArgs(req, prompt)
	cmd := exec.CommandContext(ctx, binary, args...)
	cmd.Dir = req.Workspace
	cmd.Env = os.Environ()
	if req.Params.APIKey != "" {
		cmd.Env = append(cmd.Env, "CURSOR_API_KEY="+req.Params.APIKey)
	}

	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	runErr := cmd.Run()
	exitCode := 0
	if runErr != nil {
		var exitErr *exec.ExitError
		if errors.As(runErr, &exitErr) {
			exitCode = exitErr.ExitCode()
		}
	}

	summary, _ := runDir.ReadResult()
	summary = strings.TrimSpace(summary)

	artifacts := []adapters.Artifact{
		{Kind: "stdout", Content: stdout.String()},
	}
	if stderr.Len() > 0 {
		artifacts = append(artifacts, adapters.Artifact{Kind: "stderr", Content: stderr.String()})
	}
	if summary != "" {
		artifacts = append(artifacts, adapters.Artifact{
			Kind: "result", Path: runDir.ResultPath(), Content: summary,
		})
	}

	diff, diffErr := gitDiff(ctx, req.Workspace)
	if diffErr == nil && diff != "" {
		artifacts = append(artifacts, adapters.Artifact{Kind: "diff", Content: diff})
	}

	status := "completed"
	var statusErr string
	if runErr != nil && summary == "" {
		status = "failed"
		statusErr = runErr.Error()
	}

	_ = runDir.WriteStatus(runs.Status{
		State:      status,
		ExitCode:   exitCode,
		FinishedAt: time.Now().UTC(),
		Error:      statusErr,
	})

	result := &adapters.RunResult{
		Status:    status,
		Output:    pickOutput(summary, stdout.String()),
		Artifacts: artifacts,
		ExitCode:  exitCode,
	}
	if status == "failed" {
		result.Err = fmt.Errorf("cursor: agent exited with code %d: %w", exitCode, runErr)
		if stderr.Len() > 0 {
			result.Err = fmt.Errorf("%w\nstderr: %s", result.Err, stderr.String())
		}
	}
	return result, nil
}

func buildArgs(req adapters.RunRequest, prompt string) []string {
	p := req.Params
	args := []string{
		"-p",
		"--workspace", req.Workspace,
	}

	outputFormat := p.OutputFormat
	if outputFormat == "" {
		outputFormat = "stream-json"
	}
	args = append(args, "--output-format", outputFormat)

	if p.Trust {
		args = append(args, "--trust")
	}
	if p.Force {
		args = append(args, "--force")
	}
	if p.Plan {
		args = append(args, "--plan")
	}
	if p.Model != "" {
		args = append(args, "--model", p.Model)
	}
	if p.APIKey != "" {
		args = append(args, "--api-key", p.APIKey)
	}

	args = append(args, prompt)
	return args
}

// augmentPrompt adds a result-file contract. Uses absolute path so agents in
// worktrees still write results under colony .paseka/runs/.
func augmentPrompt(base, resultPath string) string {
	abs, err := filepath.Abs(resultPath)
	if err != nil {
		abs = resultPath
	}
	if strings.Contains(base, abs) || strings.Contains(base, runs.ResultFileName) {
		return base
	}
	return base + fmt.Sprintf(" Write your final summary to file %s.", abs)
}

func gitDiff(ctx context.Context, workspace string) (string, error) {
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

func pickOutput(summary, stdout string) string {
	if summary != "" {
		return summary
	}
	return stdout
}
