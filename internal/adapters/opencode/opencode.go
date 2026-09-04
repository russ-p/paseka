package opencode

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
	adapterName   = "opencode"
	defaultBinary = "opencode"
)

// Adapter runs the OpenCode CLI in non-interactive `run` mode.
type Adapter struct{}

func New() *Adapter {
	return &Adapter{}
}

func (a *Adapter) Name() string {
	return adapterName
}

// Run invokes `opencode run` in headless mode inside the workspace.
func (a *Adapter) Run(ctx context.Context, req adapters.RunRequest) (*adapters.RunResult, error) {
	if req.Workspace == "" {
		return nil, errors.New("opencode: workspace is required")
	}
	if req.ColonyRoot == "" {
		return nil, errors.New("opencode: colony root is required")
	}
	if req.TraceID == "" || req.AgentID == "" {
		return nil, errors.New("opencode: traceId and agentId are required")
	}
	if req.Prompt == "" && req.SystemPrompt == "" {
		return nil, errors.New("opencode: prompt or system prompt is required")
	}

	prompt := joinPrompt(req.SystemPrompt, req.Prompt)
	runDir := runs.Dir{
		ColonyRoot: req.ColonyRoot,
		TraceID:    req.TraceID,
		AgentID:    req.AgentID,
	}

	binary, args := adapters.ResolveExec(req.Command, func() (string, []string) {
		b := req.Params.Binary
		if b == "" {
			b = defaultBinary
		}
		return b, buildArgs(req, prompt)
	})
	if _, err := exec.LookPath(binary); err != nil {
		return nil, fmt.Errorf("opencode: %q not found in PATH (install OpenCode CLI)", binary)
	}

	if err := runDir.Prepare(); err != nil {
		return nil, err
	}

	startedAt := time.Now().UTC()
	if err := runDir.WritePrompt(req.Prompt); err != nil {
		return nil, fmt.Errorf("opencode: write prompt: %w", err)
	}
	if req.SystemPrompt != "" {
		if err := runDir.WriteSystem(req.SystemPrompt); err != nil {
			return nil, fmt.Errorf("opencode: write system: %w", err)
		}
	}
	meta := runs.Meta{
		TraceID:   req.TraceID,
		AgentID:   req.AgentID,
		Bee:       req.Bee,
		Adapter:   adapterName,
		Workspace: req.Workspace,
		StartedAt: startedAt,
	}
	if err := runDir.WriteMeta(meta); err != nil {
		return nil, fmt.Errorf("opencode: write meta: %w", err)
	}

	format := runFormat(req.Params.OutputFormat)
	if len(req.Command) > 0 {
		if v := adapters.FlagValue(args, "--format"); v != "" {
			format = v
		}
	}

	adapters.LogAgentLaunch(nil, adapterName, binary, req, args)
	cmd := exec.CommandContext(ctx, binary, args...)
	cmd.Dir = req.Workspace
	cmd.Env = os.Environ()

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
	parsed := parseRunOutput(stdoutStr, format)

	if parsed.SessionID != "" {
		meta.ProviderSessionID = parsed.SessionID
		if err := runDir.WriteMeta(meta); err != nil {
			return nil, fmt.Errorf("opencode: write meta: %w", err)
		}
	}

	fileSummary, _ := runDir.ReadResult()
	fileSummary = strings.TrimSpace(fileSummary)
	summary := adapters.PickSummary(fileSummary, parsed.Summary)

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

	status, statusErr := adapters.ResolveStatus(ctx.Err(), runErr)
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
		Usage:             parsed.Usage,
		ProviderSessionID: parsed.SessionID,
		FinishedAt:        finishedAt,
	}
	if err := runDir.WriteResult(protoResult); err != nil {
		return nil, fmt.Errorf("opencode: write result: %w", err)
	}

	_ = runDir.WriteStatus(status, exitCode, startedAt, finishedAt, statusErr)

	result := &adapters.RunResult{
		Status:            string(status),
		Summary:           summary,
		Output:            adapters.PickOutput(summary, stdoutStr),
		Artifacts:         artifacts,
		Usage:             parsed.Usage,
		ProviderSessionID: parsed.SessionID,
		ExitCode:          exitCode,
	}
	if status == protocol.StatusFailed {
		result.Err = adapters.BuildRunError("opencode: run failed", exitCode, runErr, stderrStr, statusErr)
	}
	return result, nil
}
