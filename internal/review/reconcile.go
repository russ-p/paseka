package review

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/russ-p/paseka/internal/bus"
	"github.com/russ-p/paseka/internal/colony"
	"github.com/russ-p/paseka/internal/forge"
	"github.com/russ-p/paseka/internal/gitroot"
	"github.com/russ-p/paseka/internal/homestate"
	"github.com/russ-p/paseka/internal/protocol"
	"github.com/russ-p/paseka/internal/taskledger"
	"github.com/russ-p/paseka/internal/worktree"
)

// ReconcileResult reports one published-trail forge poll.
type ReconcileResult struct {
	TraceID   string
	State     string
	URL       string
	Completed bool
}

// ReconcilePublished polls the forge for one trail and completes the final gate when merged.
func ReconcilePublished(ctx context.Context, colonyCtx colony.Context, pub bus.Publisher, ledger taskledger.Ledger, traceID string, opts WriteOptions) (ReconcileResult, error) {
	traceID = strings.TrimSpace(traceID)
	if traceID == "" || colonyCtx.Slug == "" {
		return ReconcileResult{}, nil
	}
	entry, ok, err := homestate.FindPullRequest(colonyCtx.Slug, traceID)
	if err != nil || !ok {
		return ReconcileResult{}, err
	}
	if len(colonyCtx.Home.Forge.Command) == 0 {
		return ReconcileResult{TraceID: traceID, State: entry.State, URL: entry.URL}, nil
	}

	head := strings.TrimSpace(entry.Head)
	if head == "" {
		resolved, err := worktree.ResolvedBranch(colonyCtx.ColonyRoot, traceID, colonyCtx.Slug)
		if err != nil {
			return ReconcileResult{}, err
		}
		head = resolved
	}
	origin, _ := gitroot.OriginURL(colonyCtx.ColonyRoot)
	base := strings.TrimSpace(entry.Base)
	if base == "" {
		base, _ = gitroot.ResolvedDefaultBranch(colonyCtx.ColonyRoot)
	}

	resp, err := forge.Invoke(ctx, forge.InvokeOpts{
		Command:    colonyCtx.Home.Forge.Command,
		ColonyRoot: colonyCtx.ColonyRoot,
		TraceID:    traceID,
	}, forge.Request{
		Op:      forge.OpGet,
		Head:    head,
		Base:    base,
		TraceID: traceID,
		Origin:  origin,
	})
	if err != nil {
		return ReconcileResult{}, fmt.Errorf("review: forge get failed: %w", err)
	}
	if resp.Found {
		entry.URL = resp.URL
		entry.Number = resp.Number
		entry.Head = resp.Head
		entry.Base = resp.Base
		entry.State = resp.State
		entry.Draft = resp.Draft
		if err := homestate.UpsertPullRequest(colonyCtx.Slug, entry); err != nil {
			return ReconcileResult{}, err
		}
	}

	result := ReconcileResult{TraceID: traceID, State: entry.State, URL: entry.URL}
	if entry.State != forge.StateMerged {
		return result, nil
	}
	completed, err := completePublishedTrail(ctx, colonyCtx, pub, ledger, traceID, entry, opts)
	if err != nil {
		return result, err
	}
	result.Completed = completed
	return result, nil
}

// ReconcileAllPublished polls every machine-local published PR.
func ReconcileAllPublished(ctx context.Context, colonyCtx colony.Context, pub bus.Publisher, ledger taskledger.Ledger, opts WriteOptions) error {
	if colonyCtx.Slug == "" {
		return nil
	}
	entries, err := homestate.ListPullRequests(colonyCtx.Slug)
	if err != nil {
		return err
	}
	var errs []error
	for _, entry := range entries {
		if ctx.Err() != nil {
			return ctx.Err()
		}
		if _, err := ReconcilePublished(ctx, colonyCtx, pub, ledger, entry.TraceID, opts); err != nil {
			errs = append(errs, fmt.Errorf("%s: %w", entry.TraceID, err))
		}
	}
	return errors.Join(errs...)
}

func completePublishedTrail(ctx context.Context, colonyCtx colony.Context, pub bus.Publisher, ledger taskledger.Ledger, traceID string, entry homestate.PullRequestEntry, opts WriteOptions) (bool, error) {
	completed := false
	if ledger != nil {
		snap, err := ledger.Snapshot(traceID)
		if err != nil {
			return false, err
		}
		task, ok := taskledger.FindFinalReviewTask(snap)
		if ok && task.Status == protocol.TaskStatusWaitingReview {
			summary := "Pull request merged"
			if u := strings.TrimSpace(entry.URL); u != "" {
				summary = "Pull request merged: " + u
			}
			ev, err := protocol.NewEvent(traceID, "runtime", 0, protocol.EventVerification, protocol.TaskCompletedPayload{
				Kind:        protocol.TaskEventCompleted,
				TaskID:      task.TaskID,
				Status:      protocol.TaskStatusCompleted,
				Summary:     summary,
				CompletedAt: time.Now().UTC(),
			})
			if err != nil {
				return false, err
			}
			if err := WriteEvent(ctx, pub, ledger, ev, opts); err != nil {
				return false, err
			}
			completed = true
		}
	}
	if err := worktree.Remove(colonyCtx.ColonyRoot, colonyCtx.Slug, traceID); err != nil {
		return completed, err
	}
	if head := strings.TrimSpace(entry.Head); head != "" {
		_ = gitroot.DeleteBranch(colonyCtx.ColonyRoot, head)
	}
	_ = homestate.RemovePullRequest(colonyCtx.Slug, traceID)
	return completed, nil
}
