package console

import (
	"path/filepath"
	"strings"

	"github.com/paseka/paseka/internal/colony"
	"github.com/paseka/paseka/internal/gitroot"
	"github.com/paseka/paseka/internal/hiveview"
	"github.com/paseka/paseka/internal/runs"
	"github.com/paseka/paseka/internal/sessions"
)

func refusePullIfRootBees(ctx colony.Context, mgr *sessions.Manager) error {
	view, err := hiveview.GetAgents(ctx, mgr)
	if err != nil {
		return err
	}
	if view.Count == 0 {
		return nil
	}
	worktreesRoot := filepath.Join(ctx.ColonyRoot, ".paseka", "worktrees")
	for _, item := range view.Items {
		ws, ok := workspaceForLiveAgent(ctx, item)
		if !ok {
			return gitroot.Refused("live bee cwd could not be classified; refuse pull")
		}
		block, classified := liveBeeCheckoutKind(ctx.ColonyRoot, worktreesRoot, ws)
		if !classified {
			return gitroot.Refused("live bee cwd could not be classified; refuse pull")
		}
		if block {
			return gitroot.Refused("live bee is using the colony root checkout; refuse pull")
		}
	}
	return nil
}

func workspaceForLiveAgent(ctx colony.Context, item hiveview.AgentItem) (string, bool) {
	d := runs.Dir{ColonyRoot: ctx.ColonyRoot, TraceID: item.TraceID, AgentID: item.AgentID}
	if req, err := d.ReadRequest(); err == nil && strings.TrimSpace(req.Workspace) != "" {
		return req.Workspace, true
	}
	if meta, err := d.ReadSession(); err == nil && strings.TrimSpace(meta.Workspace) != "" {
		return meta.Workspace, true
	}
	return "", false
}

// liveBeeCheckoutKind reports whether Pull must wait (colony-root checkout).
// classified is false when the path is not a git work tree we can identify.
func liveBeeCheckoutKind(colonyRoot, worktreesRoot, workspace string) (block, classified bool) {
	abs, err := filepath.Abs(workspace)
	if err != nil {
		return false, false
	}
	top, err := gitroot.Find(abs)
	if err != nil {
		return false, false
	}
	wtRoot := filepath.Clean(worktreesRoot)
	if top == wtRoot || strings.HasPrefix(top, wtRoot+string(filepath.Separator)) {
		return false, true
	}
	if filepath.Clean(top) == filepath.Clean(colonyRoot) {
		return true, true
	}
	return false, true
}
