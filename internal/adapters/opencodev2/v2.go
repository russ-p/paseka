package opencodev2

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

	"github.com/russ-p/paseka/internal/adapters"
	"github.com/russ-p/paseka/internal/logging"
	"github.com/russ-p/paseka/internal/protocol"
	"github.com/russ-p/paseka/internal/runs"
)

const (
	adapterName   = "opencodev2"
	defaultBinary = "opencode"
)

// Adapter runs the OpenCode 2 CLI in non-interactive `run` mode.
type Adapter struct{}

func New() *Adapter {
	return &Adapter{}
}

func (a *Adapter) Name() string {
	return adapterName
}

// ResolveImplementation lets a direct pin report itself without detection.
func (a *Adapter) ResolveImplementation(adapters.RunParams) (string, error) {
	return adapterName, nil
}

// LaunchSpec reports the adapter's built-in binary (OpenCode takes no credential env).
func (a *Adapter) LaunchSpec() adapters.LaunchSpec {
	return adapters.LaunchSpec{DefaultBinary: defaultBinary}
}

// Run invokes `opencode run` in headless mode with the workspace as the process
// working directory. There is no --dir flag in V2.
func (a *Adapter) Run(ctx context.Context, req adapters.RunRequest) (*adapters.RunResult, error) {
	if req.Workspace == "" {
		return nil, errors.New("opencodev2: workspace is required")
	}
	if req.ColonyRoot == "" {
		return nil, errors.New("opencodev2: colony root is required")
	}
	if req.TraceID == "" || req.AgentID == "" {
		return nil, errors.New("opencodev2: traceId and agentId are required")
	}
	if req.Prompt == "" && req.SystemPrompt == "" {
		return nil, errors.New("opencodev2: prompt or system prompt is required")
	}
	if err := assertWorkspace(req.Workspace); err != nil {
		return nil, err
	}

	prompt := joinPrompt(req.SystemPrompt, req.Prompt)
	runDir := runs.Dir{
		ColonyRoot: req.ColonyRoot,
		TraceID:    req.TraceID,
		AgentID:    req.AgentID,
	}
	configuredName, implementation := adapters.AdapterIdentity(req.AdapterName, req.Implementation, adapterName)

	binary, args := adapters.ResolveExec(req.Command, func() (string, []string) {
		b := adapters.ResolveBinary(req.Binary, req.Params.Binary, defaultBinary)
		return b, buildArgs(req, prompt)
	})
	if _, err := exec.LookPath(binary); err != nil {
		return nil, fmt.Errorf("opencodev2: %q not found in PATH (install OpenCode 2: %s)", binary, installHint)
	}

	if err := runDir.Prepare(); err != nil {
		return nil, err
	}

	startedAt := time.Now().UTC()
	if err := runDir.WritePrompt(req.Prompt); err != nil {
		return nil, fmt.Errorf("opencodev2: write prompt: %w", err)
	}
	if req.SystemPrompt != "" {
		if err := runDir.WriteSystem(req.SystemPrompt); err != nil {
			return nil, fmt.Errorf("opencodev2: write system: %w", err)
		}
	}
	meta := runs.Meta{
		TraceID:        req.TraceID,
		AgentID:        req.AgentID,
		Bee:            req.Bee,
		Adapter:        configuredName,
		Implementation: implementation,
		Workspace:      req.Workspace,
		Profile:        req.Profile,
		Provenance:     req.Provenance,
		StartedAt:      startedAt,
	}
	if err := runDir.WriteMeta(meta); err != nil {
		return nil, fmt.Errorf("opencodev2: write meta: %w", err)
	}

	// Output format is runtime-owned: json by default, default (text) for a
	// --raw invocation. A custom command may still set its own --format.
	format := runFormat(req.Raw)
	if len(req.Command) > 0 {
		if v := adapters.FlagValue(args, "--format"); v != "" {
			format = v
		}
	}

	adapters.LogAgentLaunch(nil, configuredName, binary, req, args)
	cmd := exec.CommandContext(ctx, binary, args...)
	cmd.Dir = adapters.ResolveDir(req.Dir, req.Workspace)
	cmd.Env = req.Env

	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	runErr := adapters.RunCommand(cmd, func(pid int) error {
		return runDir.WriteStatusSnapshot(protocol.StatusSnapshot{
			ProtocolVersion: protocol.Version,
			State:           protocol.StatusRunning,
			PID:             pid,
			StartedAt:       startedAt,
			Provenance:      req.Provenance,
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
	var warnings []string

	if parsed.SessionID != "" {
		meta.ProviderSessionID = parsed.SessionID
		if err := runDir.WriteMeta(meta); err != nil {
			return nil, fmt.Errorf("opencodev2: write meta: %w", err)
		}
		// V2 accounts the run in the session record, so a single-step run that
		// emitted no token event still reports what it spent.
		if ctx.Err() == nil {
			if record, recErr := readSessionRecord(req, parsed.SessionID); recErr == nil {
				parsed.Usage = mergeUsageRecord(parsed.Usage, usageFromRecord(record))
				if !samePath(record.Location, req.Workspace) && strings.TrimSpace(record.Location) != "" {
					warnings = append(warnings, fmt.Sprintf(
						"opencodev2: session location %q does not match workspace %q", record.Location, req.Workspace))
				}
				// A provider-reported failure or interruption is worth surfacing
				// even when the CLI exited zero: a zero-token run must not read as
				// a healthy one.
				if failsOutcome(record.Outcome) {
					warnings = append(warnings, fmt.Sprintf(
						"opencodev2: session %s ended with outcome %q", parsed.SessionID, record.Outcome))
				}
			}
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

	status, reason, statusErr := adapters.ResolveStatus(ctx.Err(), runErr)
	finishedAt := time.Now().UTC()
	adapters.LogAgentDone(nil, configuredName, binary, req, startedAt, string(status), exitCode, runErr, adapters.AgentDoneOutput{
		Stdout: stdoutStr, Stderr: stderrStr, Summary: summary,
	})
	for _, warning := range warnings {
		logging.Component("adapter").Warn("opencodev2 warning",
			logging.F("adapter", configuredName),
			logging.F("trace", req.TraceID),
			logging.F("agent", req.AgentID),
			logging.F("warning", warning),
		)
	}

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
		Provenance:        req.Provenance,
	}
	if err := runDir.WriteResult(protoResult); err != nil {
		return nil, fmt.Errorf("opencodev2: write result: %w", err)
	}

	_ = adapters.WriteFinalStatus(runDir, req, status, reason, exitCode, startedAt, finishedAt, statusErr)

	result := &adapters.RunResult{
		Status:            string(status),
		Summary:           summary,
		Output:            adapters.PickOutput(summary, stdoutStr),
		Artifacts:         artifacts,
		Usage:             parsed.Usage,
		ProviderSessionID: parsed.SessionID,
		ExitCode:          exitCode,
		Warnings:          warnings,
	}
	if status == protocol.StatusFailed {
		result.Err = adapters.BuildRunError("opencodev2: run failed", exitCode, runErr, stderrStr, statusErr)
	}
	return result, nil
}

// assertWorkspace verifies the launch directory exists before dispatch. V2 has
// no --dir flag to scope the run, so a wrong cwd would otherwise be silent.
func assertWorkspace(workspace string) error {
	info, err := os.Stat(workspace)
	if err != nil {
		return fmt.Errorf("opencodev2: workspace %q: %w", workspace, err)
	}
	if !info.IsDir() {
		return fmt.Errorf("opencodev2: workspace %q is not a directory", workspace)
	}
	return nil
}

// readSessionRecord reads usage for a finished AFK run through a throwaway
// server, because the run process is gone by then and its store is durable.
func readSessionRecord(req adapters.RunRequest, sessionID string) (sessionRecord, error) {
	binary := adapters.ResolveBinary(req.Binary, req.Params.Binary, defaultBinary)
	var record sessionRecord
	err := withServer(binary, req.Workspace, req.Env, func(ctx context.Context, c *apiClient) error {
		rec, err := c.Session(ctx, sessionID)
		if err != nil {
			return err
		}
		record = rec
		return nil
	})
	return record, err
}

func samePath(a, b string) bool {
	if a == "" || b == "" {
		return false
	}
	absA, errA := filepath.Abs(a)
	absB, errB := filepath.Abs(b)
	if errA == nil && errB == nil {
		return absA == absB
	}
	return filepath.Clean(a) == filepath.Clean(b)
}
