package cursor

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
	"github.com/paseka/paseka/internal/adapters/systeminject"
	"github.com/paseka/paseka/internal/protocol"
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
	if req.Prompt == "" && req.SystemPrompt == "" {
		return nil, errors.New("cursor: prompt or system prompt is required")
	}

	prompt := req.Prompt
	runDir := runs.Dir{
		ColonyRoot: req.ColonyRoot,
		TraceID:    req.TraceID,
		AgentID:    req.AgentID,
	}
	var pluginDir string
	if req.SystemPrompt != "" {
		dir, err := systeminject.WriteCursorPlugin(runDir, req.SystemPrompt)
		if err != nil {
			return nil, fmt.Errorf("cursor: write system plugin: %w", err)
		}
		pluginDir = dir
	}

	binary, args := adapters.ResolveExec(req.Command, func() (string, []string) {
		b := req.Params.Binary
		if b == "" {
			b = defaultBinary
		}
		return b, buildArgs(req, prompt, pluginDir)
	})
	if _, err := exec.LookPath(binary); err != nil {
		return nil, fmt.Errorf("cursor: %q not found in PATH (install Cursor CLI)", binary)
	}

	if err := runDir.Prepare(); err != nil {
		return nil, err
	}

	startedAt := time.Now().UTC()
	if err := runDir.WritePrompt(prompt); err != nil {
		return nil, fmt.Errorf("cursor: write prompt: %w", err)
	}
	if req.SystemPrompt != "" {
		if err := runDir.WriteSystem(req.SystemPrompt); err != nil {
			return nil, fmt.Errorf("cursor: write system: %w", err)
		}
	}
	if err := runDir.WriteMeta(runs.Meta{
		TraceID:   req.TraceID,
		AgentID:   req.AgentID,
		Bee:       req.Bee,
		Adapter:   adapterName,
		Workspace: req.Workspace,
		StartedAt: startedAt,
	}); err != nil {
		return nil, fmt.Errorf("cursor: write meta: %w", err)
	}

	adapters.LogAgentLaunch(nil, adapterName, binary, req, args)
	cmd := exec.CommandContext(ctx, binary, args...)
	cmd.Env = os.Environ()
	if req.Params.APIKey != "" {
		cmd.Env = append(cmd.Env, "CURSOR_API_KEY="+req.Params.APIKey)
	}

	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	runErr := adapters.RunCommand(cmd, func(pid int) error {
		return runDir.WriteStatusSnapshot(protocol.StatusSnapshot{
			ProtocolVersion: protocol.Version,
			State:           protocol.StatusRunning,
			PID:             pid,
			StartedAt:       startedAt,
		})
	})
	exitCode := 0
	if runErr != nil {
		var exitErr *exec.ExitError
		if errors.As(runErr, &exitErr) {
			exitCode = exitErr.ExitCode()
		}
	}

	stdoutStr := stdout.String()
	stderrStr := stderr.String()

	var events []protocol.Event
	var streamSummary string
	outputFormat := req.Params.OutputFormat
	if len(req.Command) > 0 {
		outputFormat = adapters.FlagValue(args, "--output-format")
	}
	if isStreamFormat(outputFormat) {
		parsed := parseStreamJSON(stdoutStr, req.TraceID, req.AgentID)
		events = parsed.Events
		streamSummary = strings.TrimSpace(parsed.Summary)
		for _, ev := range events {
			_ = runDir.AppendEvent(ev)
		}
	}

	fileSummary, _ := runDir.ReadResult()
	fileSummary = strings.TrimSpace(fileSummary)
	summary := pickSummary(fileSummary, streamSummary)

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
	adapters.LogAgentDone(nil, adapterName, binary, req, startedAt, string(status), exitCode, runErr, adapters.AgentDoneOutput{
		Stdout: stdoutStr, Stderr: stderrStr, Summary: summary,
	})

	artifactRefs := make([]protocol.ArtifactRef, 0, len(artifacts))
	for _, a := range artifacts {
		artifactRefs = append(artifactRefs, protocol.ArtifactRef{Kind: a.Kind, Path: a.Path})
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
		return nil, fmt.Errorf("cursor: write result: %w", err)
	}

	_ = runDir.WriteStatus(status, exitCode, startedAt, finishedAt, statusErr)

	result := &adapters.RunResult{
		Status:    string(status),
		Summary:   summary,
		Output:    pickOutput(summary, stdoutStr),
		Events:    events,
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
	err := fmt.Errorf("cursor: agent run failed (exit %d): %s", exitCode, msg)
	if stderr != "" {
		err = fmt.Errorf("%w\nstderr: %s", err, stderr)
	}
	return err
}

func isStreamFormat(format string) bool {
	if format == "" {
		return true
	}
	return format == "stream-json"
}

func pickSummary(fileSummary, streamSummary string) string {
	if fileSummary != "" {
		return fileSummary
	}
	return streamSummary
}

func buildArgs(req adapters.RunRequest, prompt, pluginDir string) []string {
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
	if pluginDir != "" {
		args = append(args, "--plugin-dir", pluginDir)
	}
	if prompt != "" {
		args = append(args, prompt)
	}
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
