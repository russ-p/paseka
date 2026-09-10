package console

import (
	"context"

	"github.com/russ-p/paseka/internal/colony"
	"github.com/russ-p/paseka/internal/gitroot"
	"github.com/russ-p/paseka/internal/homestate"
	"github.com/russ-p/paseka/internal/review"
	"github.com/russ-p/paseka/internal/tasks"
	"github.com/russ-p/paseka/internal/worktree"
)

// MergeDiffView is the Queen Console projection of a trace merge preview.
type MergeDiffView struct {
	TraceID           string           `json:"traceId"`
	DefaultBranch     string           `json:"defaultBranch"`
	Branch            string           `json:"branch"`
	BaseSHA           string           `json:"baseSha"`
	HeadSHA           string           `json:"headSha"`
	Stat              string           `json:"stat,omitempty"`
	Diff              string           `json:"diff,omitempty"`
	Truncated         bool             `json:"truncated,omitempty"`
	Empty             bool             `json:"empty,omitempty"`
	MissingWorktree   bool             `json:"missingWorktree,omitempty"`
	OriginBehindCount *int             `json:"originBehindCount,omitempty"`
	Delivery          string           `json:"delivery,omitempty"`
	PRTitle           string           `json:"prTitle,omitempty"`
	PRBody            string           `json:"prBody,omitempty"`
	PullRequest       *PullRequestView `json:"pullRequest,omitempty"`
}

// GetMergeDiff returns the accumulated worktree diff for a trace merge gate.
func GetMergeDiff(ctx context.Context, colonyCtx colony.Context, traceID string) (MergeDiffView, error) {
	res, err := worktree.MergeDiff(worktree.MergeDiffOptions{
		ColonyRoot: colonyCtx.ColonyRoot,
		TraceID:    traceID,
		Slug:       colonyCtx.Slug,
	})
	if err != nil {
		return MergeDiffView{}, err
	}
	view := MergeDiffView{
		TraceID:         res.TraceID,
		DefaultBranch:   res.DefaultBranch,
		Branch:          res.Branch,
		BaseSHA:         res.BaseSHA,
		HeadSHA:         res.HeadSHA,
		Stat:            res.Stat,
		Diff:            res.Diff,
		Truncated:       res.Truncated,
		Empty:           res.Empty,
		MissingWorktree: res.Missing,
	}
	if _, behind, ok, err := gitroot.AheadBehind(colonyCtx.ColonyRoot, res.DefaultBranch); err == nil && ok {
		view.OriginBehindCount = &behind
	}
	if manifest, err := colony.LoadColony(colonyCtx.ColonyRoot); err == nil {
		view.Delivery = manifest.Defaults.ResolvedDelivery()
	}
	if pubCopy, err := review.ResolvePublishCopy(colonyCtx.ColonyRoot, traceID, "", ""); err == nil {
		view.PRTitle = pubCopy.Title
		view.PRBody = pubCopy.Body
	}
	if view.Delivery == colony.DeliveryPullRequest && colonyCtx.Slug != "" {
		session, err := tasks.OpenLedger(colonyCtx)
		if err == nil {
			defer session.Close()
			_, _ = review.ReconcilePublished(ctx, colonyCtx, session.Publisher, session.Ledger, traceID, review.WriteOptions{})
		} else {
			_, _ = review.ReconcilePublished(ctx, colonyCtx, nil, nil, traceID, review.WriteOptions{})
		}
	}
	if colonyCtx.Slug != "" {
		if pr, ok, err := homestate.FindPullRequest(colonyCtx.Slug, traceID); err == nil && ok {
			view.PullRequest = &PullRequestView{
				URL:    pr.URL,
				Number: pr.Number,
				Head:   pr.Head,
				Base:   pr.Base,
				State:  pr.State,
				Draft:  pr.Draft,
			}
		}
	}
	return view, nil
}
