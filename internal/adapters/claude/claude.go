package claude

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"os/exec"
	"strings"
	"time"

	"github.com/russ-p/paseka/internal/adapters"
	"github.com/russ-p/paseka/internal/protocol"
	"github.com/russ-p/paseka/internal/runs"
)

const (
	adapterName   = "claude"
	defaultBinary = "claude"
)

// Adapter runs the Claude Code CLI (claude) in non-interactive print mode.
type Adapter struct{}

func New() *Adapter {
	return &Adapter{}
}

func (a *Adapter) Name() string {
	return adapterName
}

// LaunchSpec reports the adapter's built-in binary and credential env var.
func (a *Adapter) LaunchSpec() adapters.LaunchSpec {
	return adapters.LaunchSpec{DefaultBinary: defaultBinary, CredentialEnv: "ANTHROPIC_API_KEY"}
}

// Run invokes `claude -p` in headless mode inside the workspace.
func (a *Adapter) Run(ctx context.Context, req adapters.RunRequest) (*adapters.RunResult, error) {
	if req.Workspace == "" {
		return nil, errors.New("claude: workspace is required")
	}
	if req.ColonyRoot == "" {
		return nil, errors.New("claude: colony root is required")
	}
	if req.TraceID == "" || req.AgentID == "" {
		return nil, errors.New("claude: traceId and agentId are required")
	}
	if req.Prompt == "" && req.SystemPrompt == "" {
		return nil, errors.New("claude: prompt or system prompt is required")
	}

	prompt := req.Prompt
	runDir := runs.Dir{
		ColonyRoot: req.ColonyRoot,
		TraceID:    req.TraceID,
		AgentID:    req.AgentID,
	}
	systemFile := ""
	if req.SystemPrompt != "" {
		systemFile = runDir.SystemPath()
	}

	binary, args := adapters.ResolveExec(req.Command, func() (string, []string) {
		b := adapters.ResolveBinary(req.Binary, req.Params.Binary, defaultBinary)
		return b, buildArgs(req, prompt, systemFile)
	})
	if _, err := exec.LookPath(binary); err != nil {
		return nil, fmt.Errorf("claude: %q not found in PATH (install Claude Code CLI)", binary)
	}

	if err := runDir.Prepare(); err != nil {
		return nil, err
	}

	startedAt := time.Now().UTC()
	if err := runDir.WritePrompt(prompt); err != nil {
		return nil, fmt.Errorf("claude: write prompt: %w", err)
	}
	if req.SystemPrompt != "" {
		if err := runDir.WriteSystem(req.SystemPrompt); err != nil {
			return nil, fmt.Errorf("claude: write system: %w", err)
		}
	}
	if err := runDir.WriteMeta(runs.Meta{
		TraceID:    req.TraceID,
		AgentID:    req.AgentID,
		Bee:        req.Bee,
		Adapter:    adapterName,
		Workspace:  req.Workspace,
		Profile:    req.Profile,
		Provenance: req.Provenance,
		StartedAt:  startedAt,
	}); err != nil {
		return nil, fmt.Errorf("claude: write meta: %w", err)
	}

	adapters.LogAgentLaunch(nil, adapterName, binary, req, args)
	cmd := exec.CommandContext(ctx, binary, args...)
	// Claude Code operates on the current working directory; there is no
	// --workspace flag, so the process is launched inside the workspace.
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

	var events []protocol.Event
	var streamSummary string
	// Output format is runtime-owned: structured by default, plain text only for
	// a --raw invocation. A custom command may still set its own format.
	outputFormat := structuredOutputFormat(req.Raw)
	if len(req.Command) > 0 {
		if v := adapters.FlagValue(args, "--output-format"); v != "" {
			outputFormat = v
		}
	}
	if adapters.IsStreamFormat(outputFormat) {
		parsed := parseStreamJSON(stdoutStr, req.TraceID, req.AgentID)
		events = parsed.Events
		streamSummary = strings.TrimSpace(parsed.Summary)
		// parsed.Events are stream diagnostics (TOOL_CALL, ASSISTANT_TEXT), not
		// domain bus publishes, so they are not appended to the run's event log.
		// That file is the audit of what was published to the colony; the same
		// tool calls are readable from the provider session log instead.
	}

	fileSummary, _ := runDir.ReadResult()
	fileSummary = strings.TrimSpace(fileSummary)
	summary := adapters.PickSummary(fileSummary, streamSummary)

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
		Provenance: req.Provenance,
	}
	if err := runDir.WriteResult(protoResult); err != nil {
		return nil, fmt.Errorf("claude: write result: %w", err)
	}

	_ = adapters.WriteFinalStatus(runDir, req, status, reason, exitCode, startedAt, finishedAt, statusErr)

	result := &adapters.RunResult{
		Status:    string(status),
		Summary:   summary,
		Output:    adapters.PickOutput(summary, stdoutStr),
		Events:    events,
		Artifacts: artifacts,
		ExitCode:  exitCode,
	}
	if status == protocol.StatusFailed {
		result.Err = adapters.BuildRunError("claude: agent run failed", exitCode, runErr, stderrStr, statusErr)
	}
	return result, nil
}

// structuredOutputFormat returns Claude's structured output mode, or the plain
// text fallback for a --raw invocation.
func structuredOutputFormat(raw bool) string {
	if raw {
		return "text"
	}
	return "stream-json"
}

// buildArgs assembles the `claude -p` headless invocation.
//
// Claude Code has no --workspace flag (it runs in cwd), and requires
// --verbose whenever stream-json output is combined with --print/-p.
// Trust/Force/Plan map onto Claude's --permission-mode.
func buildArgs(req adapters.RunRequest, prompt, systemFile string) []string {
	p := req.Params
	args := []string{"-p"}

	outputFormat := structuredOutputFormat(req.Raw)
	args = append(args, "--output-format", outputFormat)
	if outputFormat == "stream-json" {
		// stream-json in print mode is only valid alongside --verbose.
		args = append(args, "--verbose")
	}

	args = append(args, "--permission-mode", permissionMode(p))

	if p.Model != "" {
		args = append(args, "--model", p.Model)
	}
	if systemFile != "" {
		args = append(args, "--append-system-prompt-file", systemFile)
	}
	if prompt != "" {
		args = append(args, prompt)
	}
	return args
}

// permissionMode translates bee params into a Claude Code permission mode.
//   - plan   -> read-only planning
//   - force/trust -> bypass all permission prompts (autonomous headless run)
//   - default -> auto-accept file edits
func permissionMode(p adapters.RunParams) string {
	switch {
	case p.Plan:
		return "plan"
	case p.Force || p.Trust:
		return "bypassPermissions"
	default:
		return "acceptEdits"
	}
}
