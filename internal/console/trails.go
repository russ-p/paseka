package console

import (
	"context"
	"fmt"
	"strings"

	"github.com/russ-p/paseka/internal/colony"
	"github.com/russ-p/paseka/internal/ids"
	"github.com/russ-p/paseka/internal/tasks"
	"github.com/russ-p/paseka/internal/trailconfig"
	"github.com/russ-p/paseka/internal/trailfork"
	"github.com/russ-p/paseka/internal/trailview"
)

// TrailConfigOverride is one validated trail-local behavioural setting. It is
// the JSON shape of a CLI `--set bee.<role>.<param>=<value>`.
type TrailConfigOverride struct {
	Role  string `json:"role"`
	Key   string `json:"key"`
	Value string `json:"value"`
}

// TrailConfigApplyRequest is the JSON body for a Console configuration apply.
// The Console uses the same service as the CLI, including its revision and
// pinned-policy checks; ExpectedRevision is optional and defaults to the
// revision the trail holds when the request arrives.
type TrailConfigApplyRequest struct {
	Source           string                `json:"source"`
	SnapshotRef      string                `json:"snapshotRef,omitempty"`
	Overrides        []TrailConfigOverride `json:"overrides,omitempty"`
	Reason           string                `json:"reason,omitempty"`
	RequestID        string                `json:"requestId,omitempty"`
	ExpectedRevision *int                  `json:"expectedRevision,omitempty"`
	DryRun           bool                  `json:"dryRun,omitempty"`
}

// TrailConfigApplyResponse is the preview or recorded outcome of one apply.
type TrailConfigApplyResponse struct {
	Manifest      trailconfig.Manifest             `json:"manifest"`
	Change        trailconfig.Change               `json:"change"`
	NewConfigRef  string                           `json:"newConfigRef,omitempty"`
	Diff          []string                         `json:"diff,omitempty"`
	EffectiveDiff []string                         `json:"effectiveDiff,omitempty"`
	ShadowedDiff  []string                         `json:"shadowedDiff,omitempty"`
	RemainingWork trailconfig.RemainingWorkPreview `json:"remainingWork"`
	DryRun        bool                             `json:"dryRun"`
}

// TrailConfigCancelRequest is the JSON body for cancelling a pending change.
type TrailConfigCancelRequest struct {
	ChangeID string `json:"changeId,omitempty"`
	Reason   string `json:"reason,omitempty"`
}

// ForkDecision is the Console answer to approving, rejecting, or cancelling a
// pending fork request.
type ForkDecision struct {
	Request trailfork.Request  `json:"request"`
	Lineage *trailfork.Lineage `json:"lineage,omitempty"`
	Created bool               `json:"created,omitempty"`
}

// ListTrails returns the whole shared trail collection. Runnable forks are
// trails; pending fork requests are returned separately so a request is never
// mistaken for a trail that can be inspected or killed.
func ListTrails(_ context.Context, colonyCtx colony.Context) (trailview.Collection, error) {
	return trailview.Load(colonyCtx)
}

// GetTrailConfig returns one trail's complete configuration binding.
func GetTrailConfig(_ context.Context, colonyCtx colony.Context, traceID string) (trailconfig.Manifest, error) {
	if strings.TrimSpace(traceID) == "" {
		return trailconfig.Manifest{}, fmt.Errorf("invalid trace id")
	}
	return trailconfig.Load(colonyCtx.ColonyRoot, traceID)
}

// ApplyTrailConfig previews or applies one configuration version through the
// same trailconfig service the CLI uses.
func ApplyTrailConfig(_ context.Context, colonyCtx colony.Context, traceID string, req TrailConfigApplyRequest) (TrailConfigApplyResponse, error) {
	if strings.TrimSpace(traceID) == "" {
		return TrailConfigApplyResponse{}, fmt.Errorf("invalid trace id")
	}
	overrides := make([]trailconfig.Override, 0, len(req.Overrides))
	for _, item := range req.Overrides {
		role := strings.TrimSpace(item.Role)
		key := strings.TrimSpace(item.Key)
		if role == "" || key == "" {
			return TrailConfigApplyResponse{}, fmt.Errorf("invalid override: role and key are required")
		}
		overrides = append(overrides, trailconfig.Override{
			Role:  role,
			Key:   key,
			Value: item.Value,
		})
	}
	source := strings.TrimSpace(req.Source)
	switch {
	case source == "" && strings.TrimSpace(req.SnapshotRef) != "":
		source = trailconfig.SourceSnapshot
	case source == "" && len(overrides) > 0:
		source = trailconfig.SourceOverrides
	case source == "":
		return TrailConfigApplyResponse{}, fmt.Errorf("invalid apply source: choose colony, snapshot, or overrides")
	}

	expected := req.ExpectedRevision
	if expected == nil {
		binding, err := trailconfig.Read(colonyCtx.ColonyRoot, traceID)
		if err != nil {
			return TrailConfigApplyResponse{}, err
		}
		current := binding.ConfigRevision
		expected = &current
	}
	requestID := strings.TrimSpace(req.RequestID)
	if requestID == "" {
		id, err := ids.Prefixed("req-")
		if err == nil {
			requestID = id
		}
	}

	result, err := trailconfig.Apply(trailconfig.ApplyOptions{
		Context:          colonyCtx,
		TraceID:          traceID,
		Source:           source,
		SnapshotRef:      strings.TrimSpace(req.SnapshotRef),
		Overrides:        overrides,
		Actor:            "console",
		Reason:           strings.TrimSpace(req.Reason),
		RequestID:        requestID,
		ExpectedRevision: expected,
		DryRun:           req.DryRun,
	})
	if err != nil {
		return TrailConfigApplyResponse{}, err
	}
	return TrailConfigApplyResponse{
		Manifest:      result.Manifest,
		Change:        result.Change,
		NewConfigRef:  result.NewRef,
		Diff:          result.Diff,
		EffectiveDiff: result.EffectiveDiff,
		ShadowedDiff:  result.ShadowedDiff,
		RemainingWork: result.RemainingWork,
		DryRun:        result.DryRun,
	}, nil
}

// CancelTrailConfig cancels the pending change without touching the active
// configuration. A change identity, when supplied, must match the pending one.
func CancelTrailConfig(_ context.Context, colonyCtx colony.Context, traceID string, req TrailConfigCancelRequest) (trailconfig.Manifest, error) {
	if strings.TrimSpace(traceID) == "" {
		return trailconfig.Manifest{}, fmt.Errorf("invalid trace id")
	}
	return trailconfig.Cancel(colonyCtx.ColonyRoot, traceID, strings.TrimSpace(req.ChangeID), "console", strings.TrimSpace(req.Reason))
}

// ListForkRequests returns pending requests by default, or every retained
// request with includeDecided. A non-empty source narrows the answer.
func ListForkRequests(_ context.Context, colonyCtx colony.Context, source string, includeDecided bool) ([]trailfork.Request, error) {
	var (
		requests []trailfork.Request
		err      error
	)
	if includeDecided {
		requests, err = trailfork.ListRequests(colonyCtx.ColonyRoot)
	} else {
		requests, err = trailfork.ListPending(colonyCtx.ColonyRoot)
	}
	if err != nil {
		return nil, err
	}
	source = strings.TrimSpace(source)
	out := make([]trailfork.Request, 0, len(requests))
	for _, req := range requests {
		if source == "" || req.Evidence.SourceTrace == source {
			out = append(out, req)
		}
	}
	return out, nil
}

// ApproveForkRequest admits one pending request through the existing trailfork
// creation path. Approval needs the same publisher and ledger the CLI uses.
func ApproveForkRequest(ctx context.Context, colonyCtx colony.Context, requestID, reason string) (ForkDecision, error) {
	requestID = strings.TrimSpace(requestID)
	session, err := tasks.OpenLedger(colonyCtx)
	if err != nil {
		return ForkDecision{}, err
	}
	defer session.Close()
	if session.Publisher == nil || session.Ledger == nil {
		return ForkDecision{}, fmt.Errorf("nats url not configured (fork approval requires NATS)")
	}
	res, err := trailfork.ApproveRequest(ctx, trailfork.DecisionOptions{
		Context:   colonyCtx,
		RequestID: requestID,
		Actor:     "console",
		Reason:    strings.TrimSpace(reason),
		Publisher: session.Publisher,
		Ledger:    session.Ledger,
	})
	if err != nil {
		return ForkDecision{}, err
	}
	req, err := trailfork.LoadRequest(colonyCtx.ColonyRoot, requestID)
	if err != nil {
		return ForkDecision{}, err
	}
	lineage := res.Plan
	return ForkDecision{Request: req, Lineage: &lineage, Created: res.Created}, nil
}

// RejectForkRequest refuses a pending request without creating resources.
func RejectForkRequest(_ context.Context, colonyCtx colony.Context, requestID, reason string) (ForkDecision, error) {
	req, err := trailfork.RejectRequest(trailfork.DecisionOptions{
		Context:   colonyCtx,
		RequestID: requestID,
		Actor:     "console",
		Reason:    strings.TrimSpace(reason),
	})
	if err != nil {
		return ForkDecision{}, err
	}
	return ForkDecision{Request: req}, nil
}

// CancelForkRequest withdraws a pending request without creating resources.
func CancelForkRequest(_ context.Context, colonyCtx colony.Context, requestID, reason string) (ForkDecision, error) {
	req, err := trailfork.CancelRequest(trailfork.DecisionOptions{
		Context:   colonyCtx,
		RequestID: requestID,
		Actor:     "console",
		Reason:    strings.TrimSpace(reason),
	})
	if err != nil {
		return ForkDecision{}, err
	}
	return ForkDecision{Request: req}, nil
}

// ListForksOf returns the runnable forks whose source is sourceTrace.
func ListForksOf(ctx context.Context, colonyCtx colony.Context, sourceTrace string) ([]trailview.Trail, error) {
	collection, err := ListTrails(ctx, colonyCtx)
	if err != nil {
		return nil, err
	}
	return collection.ForksOf(sourceTrace), nil
}
