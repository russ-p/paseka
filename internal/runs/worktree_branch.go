package runs

import (
	"encoding/json"
	"strings"

	"github.com/paseka/paseka/internal/gitroot"
	"github.com/paseka/paseka/internal/protocol"
)

// LatestWorktreeBranch returns the latest INSIGHT/worktree.branch name for a flight trail.
// Last-write-wins by createdAt, then seq. Returns empty when none is present.
func LatestWorktreeBranch(colonyRoot, traceID string) (string, error) {
	if colonyRoot == "" || traceID == "" {
		return "", nil
	}
	events, err := ReadTraceEvents(colonyRoot, traceID)
	if err != nil {
		return "", err
	}

	for i := len(events) - 1; i >= 0; i-- {
		ev := events[i]
		if ev.Type != protocol.EventInsight {
			continue
		}
		if protocol.PayloadKind(ev.Payload) != string(protocol.InsightWorktreeBranch) {
			continue
		}
		var p protocol.WorktreeBranchPayload
		if err := json.Unmarshal(ev.Payload, &p); err != nil {
			continue
		}
		if branch := strings.TrimSpace(p.Branch); branch != "" {
			return branch, nil
		}
	}
	return "", nil
}

// ResolveWorktreeBranch returns the resolved git branch name for prompt context:
// latest INSIGHT/worktree.branch, else default paseka/<traceId>.
func ResolveWorktreeBranch(colonyRoot, traceID string) (string, error) {
	if colonyRoot == "" || traceID == "" {
		return "", nil
	}
	insight, err := LatestWorktreeBranch(colonyRoot, traceID)
	if err != nil {
		return "", err
	}
	if b := strings.TrimSpace(insight); b != "" {
		return b, nil
	}
	return gitroot.IsolatedWorktreeBranch(traceID), nil
}
