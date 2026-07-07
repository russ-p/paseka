package pi

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"strings"
	"time"

	"github.com/paseka/paseka/internal/adapters"
	"github.com/paseka/paseka/internal/protocol"
	"github.com/paseka/paseka/internal/runs"
)

const (
	adapterName   = "pi"
	defaultBinary = "pi"
)

// Adapter runs the Pi CLI in non-interactive print mode.
type Adapter struct{}

func New() *Adapter {
	return &Adapter{}
}

func (a *Adapter) Name() string {
	return adapterName
}

// Run invokes `pi -p` in non-interactive mode.
func (a *Adapter) Run(ctx context.Context, req adapters.RunRequest) (*adapters.RunResult, error) {
	if req.Workspace == "" {
		return nil, errors.New("pi: workspace is required")
	}
	if req.ColonyRoot == "" {
		return nil, errors.New("pi: colony root is required")
	}
	if req.TraceID == "" || req.AgentID == "" {
		return nil, errors.New("pi: traceId and agentId are required")
	}
	if req.Prompt == "" {
		return nil, errors.New("pi: prompt is required")
	}

	binary := req.Params.Binary
	if binary == "" {
		binary = defaultBinary
	}
	if _, err := exec.LookPath(binary); err != nil {
		return nil, fmt.Errorf("pi: %q not found in PATH (install Pi CLI)", binary)
	}

	runDir := runs.Dir{
		ColonyRoot: req.ColonyRoot,
		TraceID:    req.TraceID,
		AgentID:    req.AgentID,
	}
	if err := runDir.Prepare(); err != nil {
		return nil, err
	}

	startedAt := time.Now().UTC()
	prompt := req.Prompt
	if err := runDir.WritePrompt(prompt); err != nil {
		return nil, fmt.Errorf("pi: write prompt: %w", err)
	}
	if err := runDir.WriteMeta(runs.Meta{
		TraceID:   req.TraceID,
		AgentID:   req.AgentID,
		Bee:       req.Bee,
		Adapter:   adapterName,
		Workspace: req.Workspace,
		StartedAt: startedAt,
	}); err != nil {
		return nil, fmt.Errorf("pi: write meta: %w", err)
	}
	if err := runDir.WriteStatusSnapshot(protocol.StatusSnapshot{
		ProtocolVersion: protocol.Version,
		State:           protocol.StatusRunning,
		StartedAt:       startedAt,
	}); err != nil {
		return nil, fmt.Errorf("pi: write status: %w", err)
	}

	mode := piMode(req.Params.OutputFormat)
	args := buildArgs(req, prompt, mode)
	adapters.LogAgentLaunch(nil, adapterName, binary, req, args)
	cmd := exec.CommandContext(ctx, binary, args...)
	cmd.Dir = req.Workspace
	cmd.Env = os.Environ()

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

	stdoutStr := stdout.String()
	stderrStr := stderr.String()

	fileSummary, _ := runDir.ReadResult()
	fileSummary = strings.TrimSpace(fileSummary)
	summary := pickSummary(fileSummary, extractSummary(stdoutStr, mode))

	artifacts := []adapters.Artifact{
		{Kind: "stdout", Content: stdoutStr},
	}
	if stderrStr != "" {
		artifacts = append(artifacts, adapters.Artifact{Kind: "stderr", Content: stderrStr})
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

	status, statusErr := resolveStatus(ctx.Err(), runErr)
	finishedAt := time.Now().UTC()
	adapters.LogAgentDone(nil, adapterName, binary, req, startedAt, string(status), exitCode, runErr)

	artifactRefs := make([]protocol.ArtifactRef, 0, len(artifacts))
	for _, art := range artifacts {
		artifactRefs = append(artifactRefs, protocol.ArtifactRef{Kind: art.Kind, Path: art.Path})
	}

	protoResult := protocol.Result{
		ProtocolVersion: protocol.Version,
		TraceID:         req.TraceID,
		AgentID:         req.AgentID,
		Status:          status,
		Summary:         summary,
		Artifacts:       artifactRefs,
		Diagnostics: protocol.Diagnostics{
			ExitCode: exitCode,
			Error:    statusErr,
			Stderr:   stderrStr,
		},
		FinishedAt: finishedAt,
	}
	if err := runDir.WriteResult(protoResult); err != nil {
		return nil, fmt.Errorf("pi: write result: %w", err)
	}

	_ = runDir.WriteStatus(status, exitCode, startedAt, finishedAt, statusErr)

	result := &adapters.RunResult{
		Status:    string(status),
		Summary:   summary,
		Output:    pickOutput(summary, stdoutStr),
		Artifacts: artifacts,
		ExitCode:  exitCode,
	}
	if status == protocol.StatusFailed {
		result.Err = buildRunError(exitCode, runErr, stderrStr, statusErr)
	}
	return result, nil
}

func resolveStatus(ctxErr, runErr error) (protocol.RunStatus, string) {
	if ctxErr != nil {
		if errors.Is(ctxErr, context.Canceled) {
			return protocol.StatusCancelled, ctxErr.Error()
		}
		return protocol.StatusFailed, ctxErr.Error()
	}
	if runErr != nil {
		return protocol.StatusFailed, runErr.Error()
	}
	return protocol.StatusCompleted, ""
}

func buildRunError(exitCode int, runErr error, stderr, statusErr string) error {
	msg := statusErr
	if msg == "" && runErr != nil {
		msg = runErr.Error()
	}
	err := fmt.Errorf("pi: run failed (exit %d): %s", exitCode, msg)
	if stderr != "" {
		err = fmt.Errorf("%w\nstderr: %s", err, stderr)
	}
	return err
}

func pickSummary(fileSummary, parsedSummary string) string {
	if fileSummary != "" {
		return fileSummary
	}
	return parsedSummary
}

func buildArgs(req adapters.RunRequest, prompt, mode string) []string {
	p := req.Params
	args := []string{
		"-p",
		"--mode", mode,
	}
	if p.Model != "" {
		args = append(args, "--model", p.Model)
	}
	if p.Provider != "" {
		args = append(args, "--provider", p.Provider)
	}
	if p.Thinking != "" {
		args = append(args, "--thinking", p.Thinking)
	}
	if p.Plan {
		args = append(args, "--plan")
	}
	if p.APIKey != "" {
		args = append(args, "--api-key", p.APIKey)
	}
	args = append(args, prompt)
	return args
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
