package adapters

import (
	"context"
	"errors"
	"fmt"

	"github.com/paseka/paseka/internal/protocol"
)

// ResolveStatus maps process and context errors to a run status and diagnostic message.
func ResolveStatus(ctxErr, runErr error) (protocol.RunStatus, string) {
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

// BuildRunError formats a failed adapter run error. failurePrefix is the label before
// "(exit N)", e.g. "cursor: agent run failed".
func BuildRunError(failurePrefix string, exitCode int, runErr error, stderr, statusErr string) error {
	msg := statusErr
	if msg == "" && runErr != nil {
		msg = runErr.Error()
	}
	err := fmt.Errorf("%s (exit %d): %s", failurePrefix, exitCode, msg)
	if stderr != "" {
		err = fmt.Errorf("%w\nstderr: %s", err, stderr)
	}
	return err
}

// PickOutput prefers summary text over raw stdout for RunResult.Output.
func PickOutput(summary, stdout string) string {
	if summary != "" {
		return summary
	}
	return stdout
}

// PickSummary prefers an on-disk result file over parsed stream/output summary.
func PickSummary(fileSummary, streamSummary string) string {
	if fileSummary != "" {
		return fileSummary
	}
	return streamSummary
}

// IsStreamFormat reports whether adapter stdout should be parsed as stream-json.
func IsStreamFormat(format string) bool {
	if format == "" {
		return true
	}
	return format == "stream-json"
}
