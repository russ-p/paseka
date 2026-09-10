package review

import (
	"context"
	"fmt"
	"strings"

	"github.com/russ-p/paseka/internal/colony"
	"github.com/russ-p/paseka/internal/forge"
	"github.com/russ-p/paseka/internal/gitroot"
	"github.com/russ-p/paseka/internal/homestate"
	"github.com/russ-p/paseka/internal/runs"
	"github.com/russ-p/paseka/internal/worktree"
)

// PublishCopy is the title/body used for a forge upsert.
type PublishCopy struct {
	Title string
	Body  string
}

// ResolvePublishCopy builds PR title and body for publish.
// Title: HITL override, else trace.title, else merge-message-style fallback.
// Body: HITL override when non-empty, else pr.body, else trace.summary, else empty.
func ResolvePublishCopy(colonyRoot, traceID, prTitle, prBody string) (PublishCopy, error) {
	title := strings.TrimSpace(prTitle)
	if title == "" {
		resolved, err := runs.ResolveTraceTitle(colonyRoot, traceID)
		if err != nil {
			return PublishCopy{}, err
		}
		title = strings.TrimSpace(resolved)
	}
	if title == "" {
		title = defaultMergeSubject(traceID)
	}

	body := strings.TrimSpace(prBody)
	if body == "" {
		resolved, err := runs.ResolvePRBody(colonyRoot, traceID)
		if err != nil {
			return PublishCopy{}, err
		}
		body = strings.TrimSpace(resolved)
	}
	if body == "" {
		summary, err := runs.ResolveTraceSummary(colonyRoot, traceID)
		if err != nil {
			return PublishCopy{}, err
		}
		body = strings.TrimSpace(summary)
	}
	return PublishCopy{Title: title, Body: body}, nil
}

func publishPullRequest(ctx context.Context, colonyCtx colony.Context, traceID string, in ApproveInput) (ApproveResult, error) {
	if strings.TrimSpace(in.MergeMessage) != "" {
		return ApproveResult{}, fmt.Errorf("review: --merge-message is not valid when defaults.delivery is pull_request; use --pr-title / --pr-body")
	}

	origin, err := gitroot.OriginURL(colonyCtx.ColonyRoot)
	if err != nil {
		return ApproveResult{}, err
	}
	if strings.TrimSpace(origin) == "" {
		return ApproveResult{}, fmt.Errorf("review: cannot publish pull request: no origin remote")
	}
	if len(colonyCtx.Home.Forge.Command) == 0 {
		return ApproveResult{}, fmt.Errorf("review: cannot publish pull request: home forge.command is not configured")
	}
	if err := forge.CheckCommand(colonyCtx.Home.Forge.Command, colonyCtx.ColonyRoot); err != nil {
		return ApproveResult{}, fmt.Errorf("review: cannot publish pull request: %w", err)
	}

	wtPath := worktree.Path(colonyCtx.ColonyRoot, traceID)
	if !gitroot.IsInsideWorkTree(wtPath) {
		return ApproveResult{}, fmt.Errorf("review: cannot publish pull request: worktree for %s is missing", traceID)
	}

	branch, err := worktree.ResolvedBranch(colonyCtx.ColonyRoot, traceID, colonyCtx.Slug)
	if err != nil {
		return ApproveResult{}, err
	}
	if _, err := gitroot.PushWorktreeBranch(gitroot.PushBranchOpts{
		RepoRoot: colonyCtx.ColonyRoot,
		Branch:   branch,
		RunHooks: in.RunHooks,
	}); err != nil {
		return ApproveResult{}, fmt.Errorf("review: git push of %s failed: %w", branch, err)
	}

	pubCopy, err := ResolvePublishCopy(colonyCtx.ColonyRoot, traceID, in.PRTitle, in.PRBody)
	if err != nil {
		return ApproveResult{}, err
	}
	base, err := gitroot.ResolvedDefaultBranch(colonyCtx.ColonyRoot)
	if err != nil {
		return ApproveResult{}, err
	}

	resp, err := forge.Invoke(ctx, forge.InvokeOpts{
		Command:    colonyCtx.Home.Forge.Command,
		ColonyRoot: colonyCtx.ColonyRoot,
		TraceID:    traceID,
	}, forge.Request{
		Op:      forge.OpUpsert,
		Head:    branch,
		Base:    base,
		Title:   pubCopy.Title,
		Body:    pubCopy.Body,
		Draft:   in.Draft,
		TraceID: traceID,
		Origin:  origin,
	})
	if err != nil {
		return ApproveResult{}, fmt.Errorf("review: forge upsert failed: %w", err)
	}

	entry := homestate.PullRequestEntry{
		TraceID: traceID,
		URL:     resp.URL,
		Number:  resp.Number,
		Head:    resp.Head,
		Base:    resp.Base,
		State:   resp.State,
		Draft:   resp.Draft,
	}
	if colonyCtx.Slug != "" {
		if err := homestate.UpsertPullRequest(colonyCtx.Slug, entry); err != nil {
			return ApproveResult{}, err
		}
	}
	return ApproveResult{
		Published: true,
		PRURL:     resp.URL,
		PRNumber:  resp.Number,
		PRState:   resp.State,
	}, nil
}
