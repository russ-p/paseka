package console

import (
	"github.com/paseka/paseka/internal/colony"
	"github.com/paseka/paseka/internal/worktree"
)

// MergeDiffView is the Queen Console projection of a trace merge preview.
type MergeDiffView struct {
	TraceID         string `json:"traceId"`
	DefaultBranch   string `json:"defaultBranch"`
	Branch          string `json:"branch"`
	BaseSHA         string `json:"baseSha"`
	HeadSHA         string `json:"headSha"`
	Stat            string `json:"stat,omitempty"`
	Diff            string `json:"diff,omitempty"`
	Truncated       bool   `json:"truncated,omitempty"`
	Empty           bool   `json:"empty,omitempty"`
	MissingWorktree bool   `json:"missingWorktree,omitempty"`
}

// GetMergeDiff returns the accumulated worktree diff for a trace merge gate.
func GetMergeDiff(ctx colony.Context, traceID string) (MergeDiffView, error) {
	res, err := worktree.MergeDiff(worktree.MergeDiffOptions{
		ColonyRoot: ctx.ColonyRoot,
		TraceID:    traceID,
		Slug:       ctx.Slug,
	})
	if err != nil {
		return MergeDiffView{}, err
	}
	return MergeDiffView{
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
	}, nil
}
