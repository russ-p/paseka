package script

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"os/exec"
	"strings"
	"time"

	"github.com/paseka/paseka/internal/adapters"
	"github.com/paseka/paseka/internal/protocol"
	"github.com/paseka/paseka/internal/runs"
)

const adapterName = "script"

// Adapter runs a bee command as an external script process.
type Adapter struct{}

func New() *Adapter {
	return &Adapter{}
}

func (a *Adapter) Name() string {
	return adapterName
}

// Run executes the configured command argv in the bee workspace.
func (a *Adapter) Run(ctx context.Context, req adapters.RunRequest) (*adapters.RunResult, error) {
	if req.Workspace == "" {
		return nil, errors.New("script: workspace is required")
	}
	if req.ColonyRoot == "" {
		return nil, errors.New("script: colony root is required")
	}
	if req.TraceID == "" || req.AgentID == "" {
		return nil, errors.New("script: traceId and agentId are required")
	}
	if len(req.Command) == 0 {
		return nil, errors.New("script: command is required")
	}

	binary := req.Command[0]
	args := req.Command[1:]

	runDir := runs.Dir{
		ColonyRoot: req.ColonyRoot,
		TraceID:    req.TraceID,
		AgentID:    req.AgentID,
	}
	if err := runDir.Prepare(); err != nil {
		return nil, err
	}

	startedAt := time.Now().UTC()
	if prompt := strings.TrimSpace(req.Prompt); prompt != "" {
		if err := runDir.WritePrompt(prompt); err != nil {
			return nil, fmt.Errorf("script: write prompt: %w", err)
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
		return nil, fmt.Errorf("script: write meta: %w", err)
	}

	adapters.LogAgentLaunch(nil, adapterName, binary, req, req.Command)
	cmd := exec.CommandContext(ctx, binary, args...)
	cmd.Dir = req.Workspace
	cmd.Env = adapters.ScriptEnv(req, runDir)

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
	summary := strings.TrimSpace(stdoutStr)

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

	diff, diffErr := adapters.GitDiff(ctx, req.Workspace)
	if diffErr == nil && diff != "" {
		artifacts = append(artifacts, adapters.Artifact{Kind: "diff", Content: diff})
	}

	status, statusErr := resolveStatus(ctx.Err(), runErr)
	finishedAt := time.Now().UTC()
	adapters.LogAgentDone(nil, adapterName, binary, req, startedAt, string(status), exitCode, runErr, adapters.AgentDoneOutput{
		Stdout: stdoutStr, Stderr: stderrStr, Summary: summary,
	})

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
		return nil, fmt.Errorf("script: write result: %w", err)
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
	err := fmt.Errorf("script: command failed (exit %d): %s", exitCode, msg)
	if stderr != "" {
		err = fmt.Errorf("%w\nstderr: %s", err, stderr)
	}
	return err
}

func pickOutput(summary, stdout string) string {
	if summary != "" {
		return summary
	}
	return stdout
}
