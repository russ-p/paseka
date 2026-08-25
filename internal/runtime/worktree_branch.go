package runtime

import (
	"context"
	"encoding/json"
	"strings"

	"github.com/paseka/paseka/internal/protocol"
	"github.com/paseka/paseka/internal/worktree"
)

func (r *Reactor) handleWorktreeBranch(ctx context.Context, ev protocol.Event) error {
	if ev.Type != protocol.EventInsight || protocol.PayloadKind(ev.Payload) != string(protocol.InsightWorktreeBranch) {
		return nil
	}
	var payload protocol.WorktreeBranchPayload
	if err := json.Unmarshal(ev.Payload, &payload); err != nil {
		return nil
	}
	branch := strings.TrimSpace(payload.Branch)
	if branch == "" {
		return nil
	}
	return worktree.ApplyBranchInsight(worktree.ApplyBranchOptions{
		ColonyRoot: r.colony.ColonyRoot,
		TraceID:    ev.TraceID,
		Slug:       r.colony.Slug,
		Branch:     branch,
	})
}
