package runtime

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"syscall"
	"time"

	"github.com/russ-p/paseka/internal/adapters"
	"github.com/russ-p/paseka/internal/colony"
	"github.com/russ-p/paseka/internal/logging"
	"github.com/russ-p/paseka/internal/protocol"
	"github.com/russ-p/paseka/internal/runs"
)

const (
	// checkLogMaxBytes is the hard cap on captured gate output written to disk.
	checkLogMaxBytes = 1 << 20
	// checkHeadMaxBytes is the smaller cap for the head carried into the run
	// error note and the rejection event, so rework prompts stay readable.
	checkHeadMaxBytes = 2048
	// checkLogTruncationMarker is written when captured output exceeded the cap.
	checkLogTruncationMarker = "\n…(output truncated at %d bytes)\n"
)

// cappedBuffer captures output up to max bytes and records whether it dropped
// anything. It never blocks the child: writes past the cap are accepted and
// discarded so the exit status stays authoritative.
type cappedBuffer struct {
	max       int
	buf       bytes.Buffer
	truncated bool
}

func (b *cappedBuffer) Write(p []byte) (int, error) {
	total := len(p)
	remaining := b.max - b.buf.Len()
	if remaining <= 0 {
		if total > 0 {
			b.truncated = true
		}
		return total, nil
	}
	if total > remaining {
		b.truncated = true
		total = remaining
	}
	_, _ = b.buf.Write(p[:total])
	return len(p), nil
}

func (b *cappedBuffer) String() string { return b.buf.String() }

// runCheckGate executes the bee's declared check command after the adapter has
// exited and before any deferred proposal can leave the run.
//
// The verdict is the process exit status, read by the runtime because it owns
// the process. Nothing is inferred from stdout or stderr. A gate that cannot
// run — a missing binary, a timeout, a signal kill — is a failure, never a
// pass. A bee without a gate records nothing at all.
//
// It returns the synthesized VERIFICATION/verification.failed rejection when a
// failing gate should drive rework, nil otherwise.
func (d *Dispatcher) runCheckGate(ctx context.Context, p *preparedDispatch, result *adapters.RunResult) *protocol.Event {
	gate := p.bee.CheckGate()
	if gate == nil || result == nil || result.Status != string(protocol.StatusCompleted) {
		return nil
	}

	timeout, err := gate.TimeoutDuration()
	if err != nil {
		gateResult := failedCheckResult(nil, "exec", 0, false, err.Error())
		return d.applyCheckFailure(p, result, gateResult, err.Error())
	}

	argv, err := gate.Command.RenderCommand(checkVars(p, result))
	if err != nil {
		gateResult := failedCheckResult(nil, "exec", 0, false, err.Error())
		return d.applyCheckFailure(p, result, gateResult, err.Error())
	}
	if len(argv) == 0 {
		gateResult := failedCheckResult(nil, "exec", 0, false, "checks.command rendered no argv")
		return d.applyCheckFailure(p, result, gateResult, "checks.command rendered no argv")
	}

	env := adapters.ScriptEnv(p.plan.Env, adapters.RunRequest{
		Bee:        p.bee.Role,
		TraceID:    p.req.TraceID,
		AgentID:    p.agentID,
		TaskID:     p.req.TaskID,
		Workspace:  p.workspace,
		ColonyRoot: p.colonyRoot,
		Provenance: p.req.provenance(),
	}, p.runDir)

	runCtx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	output := &cappedBuffer{max: checkLogMaxBytes}
	cmd := exec.CommandContext(runCtx, argv[0], argv[1:]...)
	cmd.Dir = p.workspace
	cmd.Env = env
	cmd.Stdout = output
	cmd.Stderr = output
	// Bound teardown even when a descendant inherited the output pipe: after the
	// context deadline Wait returns instead of waiting for a grandchild to exit.
	cmd.WaitDelay = 2 * time.Second

	started := time.Now()
	runErr := cmd.Run()
	duration := time.Since(started)

	gateResult := protocol.GateResult{
		Command:    argv,
		DurationMs: duration.Milliseconds(),
		Truncated:  output.truncated,
	}
	gateResult.ExitCode, gateResult.Reason, gateResult.Signal = classifyCheckRun(runCtx, runErr)
	gateResult.TimedOut = errors.Is(runCtx.Err(), context.DeadlineExceeded)
	if gateResult.Reason == "exec" && runErr != nil {
		// A command that could not start leaves no stderr; capture the reason
		// in the same log so the log path is never empty evidence.
		_, _ = output.Write([]byte(runErr.Error() + "\n"))
	}
	gateResult.Passed = gateResult.Reason == "" && gateResult.ExitCode == 0
	gateResult.Summary = checkSummary(gateResult)
	gateResult.LogPath = relColonyPath(p.colonyRoot, p.runDir.CheckLogPath())

	if err := writeCheckLog(p.runDir.CheckLogPath(), output.String(), gateResult.Truncated); err != nil {
		// The gate decides, but evidence it cannot write is a gate failure: a
		// verdict nobody can read is not a pass.
		gateResult.Passed = false
		gateResult.Reason = "log"
		gateResult.ExitCode = -1
		gateResult.Summary = fmt.Sprintf("checks failed (log write): %s", err.Error())
		return d.applyCheckFailure(p, result, gateResult, err.Error())
	}

	if gateResult.Passed {
		if err := p.runDir.WriteGateRecord(gateResult, protocol.StatusCompleted, "", ""); err != nil {
			runtimeLog.Warn("check gate record failed",
				logging.F("bee", p.bee.Role),
				logging.F("trace", p.req.TraceID),
				logging.F("agent", p.agentID),
				logging.F("error", err.Error()),
			)
		}
		result.Gate = &gateResult
		return nil
	}

	return d.applyCheckFailure(p, result, gateResult, gateHead(output.String()))
}

// applyCheckFailure marks the run failed with reason checks_failed, records the
// gate verdict, and decides whether a platform-authored rejection should drive
// rework.
func (d *Dispatcher) applyCheckFailure(p *preparedDispatch, result *adapters.RunResult, gateResult protocol.GateResult, outputHead string) *protocol.Event {
	result.Status = string(protocol.StatusFailed)
	result.Reason = protocol.ReasonChecksFailed
	result.Gate = &gateResult
	if result.Err == nil {
		result.Err = fmt.Errorf("%s", gateResult.Summary)
	}

	note := gateResult.Summary
	if head := strings.TrimSpace(outputHead); head != "" {
		note += "\n" + head
	}
	if err := p.runDir.WriteGateRecord(gateResult, protocol.StatusFailed, protocol.ReasonChecksFailed, note); err != nil {
		runtimeLog.Warn("check gate record failed",
			logging.F("bee", p.bee.Role),
			logging.F("trace", p.req.TraceID),
			logging.F("agent", p.agentID),
			logging.F("error", err.Error()),
		)
	}

	runtimeLog.Warn("check gate failed",
		logging.F("bee", p.bee.Role),
		logging.F("trace", p.req.TraceID),
		logging.F("agent", p.agentID),
		logging.F("reason", gateResult.Reason),
		logging.F("exit_code", fmt.Sprintf("%d", gateResult.ExitCode)),
		logging.F("log", gateResult.LogPath),
	)

	// A taskless run fails quietly: signalling a taskless builder would loop it
	// against itself.
	if strings.TrimSpace(p.req.TaskID) == "" {
		return nil
	}

	// The cap is derived from recorded gate results, not an in-memory counter,
	// so a restarted runtime counts the same way. The current run's verdict was
	// just written, so it is included in the count.
	if cap := p.bee.CheckGate().ResolvedRejectionCap(); cap > 0 {
		if consecutiveCheckRejections(p.colonyRoot, p.req.TraceID, p.req.TaskID, p.req.AttemptID) > cap {
			result.GateReworkSuppressed = true
			runtimeLog.Warn("check gate rejection cap reached",
				logging.F("bee", p.bee.Role),
				logging.F("trace", p.req.TraceID),
				logging.F("task", p.req.TaskID),
				logging.F("cap", fmt.Sprintf("%d", cap)),
			)
			return nil
		}
	}

	ev, err := protocol.NewEvent(p.req.TraceID, protocol.RuntimeAgentID, 0, protocol.EventVerification, protocol.VerificationPayload{
		Kind:    protocol.VerificationFailed,
		TaskID:  p.req.TaskID,
		Summary: gateResult.Summary,
		Reason:  protocol.ReasonChecksFailed,
		Details: gateResult.LogPath,
	})
	if err != nil {
		runtimeLog.Warn("check gate rejection event failed", logging.F("error", err.Error()))
		return nil
	}
	if err := p.runDir.AppendEvent(ev); err != nil {
		runtimeLog.Warn("check gate rejection record failed", logging.F("error", err.Error()))
	}
	return &ev
}

// consecutiveCheckRejections counts trailing gate rejections for one task
// attempt. A passing gate clears the count; a non-gated run says nothing and is
// skipped rather than treated as a pass.
func consecutiveCheckRejections(colonyRoot, traceID, taskID, attemptID string) int {
	runsList, err := runs.RecordedGateRuns(colonyRoot, traceID)
	if err != nil {
		return 0
	}
	count := 0
	for i := len(runsList) - 1; i >= 0; i-- {
		recorded := runsList[i]
		if recorded.TaskID != taskID || recorded.AttemptID != attemptID {
			continue
		}
		if recorded.Gate.Passed {
			break
		}
		count++
	}
	return count
}

func failedCheckGate(result *adapters.RunResult) *protocol.GateResult {
	if result == nil || result.Gate == nil || result.Gate.Passed {
		return nil
	}
	return result.Gate
}

func checkVars(p *preparedDispatch, result *adapters.RunResult) colony.CommandVars {
	return postExecVars(p.rendered, p.workspace, resultText(result), p.runDir, p.req.TraceID, p.agentID, p.req.TaskID, p.colonyRoot, p.plan.Env)
}

func classifyCheckRun(runCtx context.Context, runErr error) (exitCode int, reason, signal string) {
	if runErr == nil {
		return 0, "", ""
	}
	if errors.Is(runCtx.Err(), context.DeadlineExceeded) {
		return -1, "timeout", ""
	}
	var exitErr *exec.ExitError
	if errors.As(runErr, &exitErr) {
		exitCode = exitErr.ExitCode()
		if status, ok := exitErr.Sys().(syscall.WaitStatus); ok && status.Signaled() {
			return -1, "signal", status.Signal().String()
		}
		if exitCode == 0 {
			exitCode = -1
		}
		return exitCode, "exit", ""
	}
	return -1, "exec", ""
}

func failedCheckResult(argv []string, reason string, exitCode int, truncated bool, message string) protocol.GateResult {
	gate := protocol.GateResult{
		Command:   argv,
		ExitCode:  exitCode,
		Reason:    reason,
		Truncated: truncated,
	}
	gate.Summary = checkSummary(gate)
	if msg := oneLine(message); msg != "" {
		gate.Summary = fmt.Sprintf("%s: %s", gate.Summary, msg)
	}
	return gate
}

// checkSummary renders the one-line verdict. It names the command, never the
// log: the summary lands in task status, the Console, gateway messages, and the
// capped rework task body.
func checkSummary(gate protocol.GateResult) string {
	command := oneLine(strings.Join(gate.Command, " "))
	var verdict string
	switch {
	case gate.Reason == "timeout":
		verdict = "checks failed (timeout)"
	case gate.Reason == "exec":
		verdict = "checks failed (could not start)"
	case gate.Reason == "log":
		verdict = "checks failed (log write)"
	case gate.Reason == "signal":
		verdict = fmt.Sprintf("checks failed (signal %s)", gate.Signal)
	case gate.ExitCode != 0:
		verdict = fmt.Sprintf("checks failed (exit %d)", gate.ExitCode)
	default:
		if command == "" {
			return "checks passed"
		}
		return "checks passed: " + command
	}
	if command == "" {
		return verdict
	}
	return verdict + ": " + command
}

func writeCheckLog(path, output string, truncated bool) error {
	if truncated {
		output += fmt.Sprintf(checkLogTruncationMarker, checkLogMaxBytes)
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	return os.WriteFile(path, []byte(output), 0o644)
}

// gateHead returns a bounded head of captured output for the error note and the
// rejection summary.
func gateHead(output string) string {
	output = strings.TrimSpace(output)
	if len(output) <= checkHeadMaxBytes {
		return output
	}
	return output[:checkHeadMaxBytes] + "\n…(truncated)"
}

func relColonyPath(colonyRoot, path string) string {
	rel, err := filepath.Rel(colonyRoot, path)
	if err != nil || rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
		return path
	}
	return rel
}

func oneLine(s string) string {
	s = strings.TrimSpace(strings.ReplaceAll(s, "\n", " "))
	if len(s) > 200 {
		return s[:200] + "…"
	}
	return s
}
