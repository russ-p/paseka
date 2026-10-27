package trailfork

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/russ-p/paseka/internal/bus"
	"github.com/russ-p/paseka/internal/colony"
	"github.com/russ-p/paseka/internal/filelock"
	"github.com/russ-p/paseka/internal/gitroot"
	"github.com/russ-p/paseka/internal/ids"
	"github.com/russ-p/paseka/internal/protocol"
	"github.com/russ-p/paseka/internal/runs"
	"github.com/russ-p/paseka/internal/taskledger"
	"github.com/russ-p/paseka/internal/trailconfig"
	"github.com/russ-p/paseka/internal/worktree"
)

const (
	// ForkRequestsDirName is the reserved per-source directory holding pending
	// and decided fork requests. It is not a trail and holds no runnable state.
	ForkRequestsDirName = "fork-requests"

	// DefaultPendingQuota bounds how many undecided requests a colony retains.
	DefaultPendingQuota = 20
	// DefaultRequestTTL bounds how long an undecided request stays admissible.
	DefaultRequestTTL = 24 * time.Hour

	requestVersion = 1

	// Request states. Pending and creating hold no runnable resources; approved
	// corresponds to a runnable fork trail.
	StatePending = "pending"
	// StateCreating marks a request, or the fork lineage it is creating, whose
	// on-disk creation has started but not finished. Such a fork has no
	// published initial task and must not dispatch.
	StateCreating  = "creating"
	StateApproved  = "approved"
	StateRefused   = "refused"
	StateRejected  = "rejected"
	StateExpired   = "expired"
	StateCancelled = "cancelled"

	requestOriginAgent = "agent"
	requestOriginCLI   = "cli"
)

// Sender identifies the recorded bee run that asked for a fork.
type Sender struct {
	TraceID string `json:"traceId,omitempty"`
	AgentID string `json:"agentId,omitempty"`
	Bee     string `json:"bee,omitempty"`
}

// Request is one durable fork request and its admission outcome.
//
// Evidence is the resolved source plan captured at request time. It carries no
// fork identity: forkId and traceId are minted only when an operator approves.
type Request struct {
	Version       int       `json:"version"`
	RequestID     string    `json:"requestId"`
	State         string    `json:"state"`
	Origin        string    `json:"origin"`
	Sender        Sender    `json:"sender,omitempty"`
	Evidence      Lineage   `json:"evidence"`
	ContentDigest string    `json:"contentDigest"`
	Validation    string    `json:"validation,omitempty"`
	Reason        string    `json:"reason,omitempty"`
	LastError     string    `json:"lastError,omitempty"`
	CreatedAt     time.Time `json:"createdAt"`
	UpdatedAt     time.Time `json:"updatedAt"`
	ExpiresAt     time.Time `json:"expiresAt,omitempty"`
	DecidedAt     time.Time `json:"decidedAt,omitempty"`
	DecidedBy     string    `json:"decidedBy,omitempty"`
	ForkID        string    `json:"forkId,omitempty"`
	TraceID       string    `json:"traceId,omitempty"`
	TaskID        string    `json:"taskId,omitempty"`
	Lineage       *Lineage  `json:"lineage,omitempty"`
}

// RequestOptions describes one agent-originated fork request.
type RequestOptions struct {
	Context colony.Context
	Sender  Sender
	Payload protocol.ForkRequestPayload
	// Now, Quota, and TTL are optional test/operator overrides.
	Now   time.Time
	Quota int
	TTL   time.Duration
}

// DecisionOptions describes an approve, reject, or cancel action.
type DecisionOptions struct {
	Context   colony.Context
	RequestID string
	Actor     string
	Reason    string
	Now       time.Time

	// Approve performs on-disk creation and therefore needs the colony's
	// existing publisher and task ledger.
	Publisher bus.Publisher
	Ledger    taskledger.Ledger
}

// SubmitAgentRequest validates one agent-originated request and records it as a
// durable pending request. It creates no trail, snapshot, worktree, honey, or
// task plan.
func SubmitAgentRequest(opts RequestOptions) (Request, error) {
	root := strings.TrimSpace(opts.Context.ColonyRoot)
	if root == "" {
		return Request{}, fmt.Errorf("trailfork: colony root is required")
	}
	payload := opts.Payload
	if strings.TrimSpace(payload.RequestID) == "" {
		return Request{}, fmt.Errorf("trailfork: requestId is required")
	}
	if err := validateRequestID(payload.RequestID); err != nil {
		return Request{}, err
	}
	if strings.TrimSpace(payload.SourceTrace) == "" {
		return Request{}, fmt.Errorf("trailfork: sourceTrace is required")
	}
	form := strings.TrimSpace(payload.Delta)
	switch form {
	case DeltaNone:
		if strings.TrimSpace(payload.Patch) != "" {
			return Request{}, fmt.Errorf("trailfork: patch is not allowed with delta %q", DeltaNone)
		}
	case DeltaPatch:
		if strings.TrimSpace(payload.Patch) == "" {
			return Request{}, fmt.Errorf("trailfork: patch is required with delta %q", DeltaPatch)
		}
	default:
		return Request{}, fmt.Errorf("trailfork: unknown delta form %q", form)
	}

	sender, err := authorizeSender(root, opts.Sender)
	if err != nil {
		return Request{}, err
	}

	now := opts.Now
	if now.IsZero() {
		now = time.Now().UTC()
	}
	forkOpts := Options{
		Context:       opts.Context,
		SourceTraceID: payload.SourceTrace,
		TaskID:        payload.SourceTask,
		SourceRun:     payload.SourceRun,
		BaseSHA:       payload.BaseSHA,
		DeltaForm:     form,
		DeltaPatch:    payload.Patch,
		Budget:        payload.Budget,
		Title:         payload.Title,
		RequestID:     payload.RequestID,
		Origin:        requestOriginAgent,
		Actor:         requestOriginAgent,
	}
	plan, _, err := resolveEvidence(forkOpts)
	if err != nil {
		return Request{}, err
	}
	digest, err := requestDigest(sender, payload)
	if err != nil {
		return Request{}, err
	}

	lock, err := acquireRequestLock(filepath.Join(root, ".paseka", "runs", ".fork-requests.lock"))
	if err != nil {
		return Request{}, err
	}
	defer lock()

	if existing, found, err := findRequest(root, payload.RequestID); err != nil {
		return Request{}, err
	} else if found {
		if existing.ContentDigest != digest {
			return Request{}, fmt.Errorf("trailfork: request id %s was already used with different contents", payload.RequestID)
		}
		return existing, nil
	}

	pending, err := loadAllRequests(root, now)
	if err != nil {
		return Request{}, err
	}
	quota := opts.Quota
	if quota <= 0 {
		quota = DefaultPendingQuota
	}
	count := 0
	for _, item := range pending {
		if item.request.State == StatePending {
			count++
		}
	}
	if count >= quota {
		return Request{}, fmt.Errorf("trailfork: pending fork request quota reached (%d)", quota)
	}

	ttl := opts.TTL
	if ttl == 0 {
		ttl = DefaultRequestTTL
	}
	evidence := pendingEvidence(plan)
	evidence.CreatedAt = now
	req := Request{
		Version:       requestVersion,
		RequestID:     payload.RequestID,
		State:         StatePending,
		Origin:        requestOriginAgent,
		Sender:        sender,
		Evidence:      evidence,
		ContentDigest: digest,
		Validation:    "ok",
		CreatedAt:     now,
		UpdatedAt:     now,
		ExpiresAt:     now.Add(ttl),
	}
	if err := saveRequest(RequestPath(root, plan.SourceTrace, req.RequestID), req); err != nil {
		return Request{}, err
	}
	return req, nil
}

// ListPending returns every unexpired pending request, oldest first. Requests
// whose expiry has passed are transitioned to expired and left out.
func ListPending(colonyRoot string) ([]Request, error) {
	items, err := loadAllRequests(colonyRoot, time.Now().UTC())
	if err != nil {
		return nil, err
	}
	var out []Request
	for _, item := range items {
		if item.request.State == StatePending {
			out = append(out, item.request)
		}
	}
	return out, nil
}

// ListOpen returns fork requests awaiting a decision or still being created,
// oldest first. A creating request may be an interrupted creation and stays
// visible until it is approved, rejected, or cancelled. ListPending keeps its
// decision and quota semantics and returns pending requests only.
func ListOpen(colonyRoot string) ([]Request, error) {
	items, err := loadAllRequests(colonyRoot, time.Now().UTC())
	if err != nil {
		return nil, err
	}
	var out []Request
	for _, item := range items {
		switch item.request.State {
		case StatePending, StateCreating:
			out = append(out, item.request)
		}
	}
	return out, nil
}

// ListRequests returns all retained requests, pending or decided, oldest first.
func ListRequests(colonyRoot string) ([]Request, error) {
	items, err := loadAllRequests(colonyRoot, time.Now().UTC())
	if err != nil {
		return nil, err
	}
	out := make([]Request, 0, len(items))
	for _, item := range items {
		out = append(out, item.request)
	}
	return out, nil
}

// LoadRequest finds one retained request by identity.
func LoadRequest(colonyRoot, requestID string) (Request, error) {
	req, found, err := findRequest(colonyRoot, requestID)
	if err != nil {
		return Request{}, err
	}
	if !found {
		return Request{}, fmt.Errorf("trailfork: fork request %s was not found", requestID)
	}
	return req, nil
}

// ApproveRequest performs the on-disk creation for a pending request exactly
// once. Repeated approval returns the recorded fork.
func ApproveRequest(ctx context.Context, opts DecisionOptions) (Result, error) {
	root := strings.TrimSpace(opts.Context.ColonyRoot)
	_, path, err := loadRequestForDecision(root, opts.RequestID, opts.Now)
	if err != nil {
		return Result{}, err
	}
	lock, err := acquireDecisionLock(ctx, path+".lock")
	if err != nil {
		return Result{}, err
	}
	defer lock()

	req, err := loadRequestFile(path)
	if err != nil {
		return Result{}, err
	}
	return approveRequestLocked(ctx, opts, req, path)
}

// approveRequestLocked advances one request while its per-request lock is held.
//
// A creating request that already materialized its lineage is resumed from that
// retained record, not by re-resolving the live source. The source may have been
// purged after the fork copied its own binding and snapshots; its lineage is the
// authoritative evidence for finishing publication.
func approveRequestLocked(ctx context.Context, opts DecisionOptions, req Request, path string) (Result, error) {
	root := strings.TrimSpace(opts.Context.ColonyRoot)
	now := decisionTime(opts.Now)
	req = expireIfNeeded(req, now)
	if req.State == StateExpired {
		_ = saveRequest(path, req)
		return Result{}, fmt.Errorf("trailfork: fork request %s expired", req.RequestID)
	}
	if req.State == StateApproved {
		lineage := req.Lineage
		if lineage == nil {
			loaded, loadErr := LoadLineage(root, req.TraceID)
			if loadErr != nil {
				return Result{}, fmt.Errorf("trailfork: approved request %s has no readable lineage: %w", req.RequestID, loadErr)
			}
			lineage = &loaded
		}
		return resultFromLineage(opts.Context, *lineage)
	}
	if req.State != StatePending && req.State != StateCreating {
		return Result{}, fmt.Errorf("trailfork: fork request %s is %s and cannot be approved", req.RequestID, req.State)
	}
	if !bus.PublisherAvailable(opts.Publisher) {
		return Result{}, fmt.Errorf("trailfork: nats url not configured (fork approval requires NATS)")
	}
	if opts.Ledger == nil {
		return Result{}, fmt.Errorf("trailfork: task ledger is required (fork approval requires NATS)")
	}

	actorName := strings.TrimSpace(opts.Actor)
	if actorName == "" {
		actorName = requestOriginCLI
	}
	if req.State == StateCreating && strings.TrimSpace(req.TraceID) != "" {
		if lineage, loadErr := LoadLineage(root, req.TraceID); loadErr == nil && lineage.RequestID == req.RequestID {
			defaults, defaultsErr := forkDefaults(opts.Context, req.TraceID)
			if defaultsErr != nil {
				return recordCreationFailure(path, req, defaultsErr, opts.Now)
			}
			promoted, finishErr := finishCreation(ctx, opts.Publisher, opts.Ledger, root, lineage, defaults)
			if finishErr != nil {
				return recordCreationFailure(path, req, finishErr, opts.Now)
			}
			result, err := resultFromLineage(opts.Context, promoted)
			if err != nil {
				return recordCreationFailure(path, req, err, opts.Now)
			}
			return recordApprovedRequest(path, req, result, actorName, opts.Reason, now, opts.Now)
		}
	}

	forkOpts := forkOptionsFromRequest(req, opts.Context, actorName)
	plan, evidence, err := resolve(forkOpts)
	if err != nil {
		return refuseRequest(path, req, actorName, now, err)
	}
	if err := verifyEvidence(req.Evidence, plan); err != nil {
		return refuseRequest(path, req, actorName, now, err)
	}

	req.State = StateCreating
	req.TraceID = plan.TraceID
	req.TaskID = plan.Task.TaskID
	req.ForkID = plan.ForkID
	req.DecidedBy = actorName
	req.UpdatedAt = now
	if err := saveRequest(path, req); err != nil {
		return Result{}, err
	}

	result, err := createResolved(ctx, opts.Publisher, opts.Ledger, forkOpts, plan, evidence)
	if err != nil {
		return recordCreationFailure(path, req, err, opts.Now)
	}
	return recordApprovedRequest(path, req, result, actorName, opts.Reason, now, opts.Now)
}

func refuseRequest(path string, req Request, actorName string, now time.Time, cause error) (Result, error) {
	req.State = StateRefused
	req.Reason = cause.Error()
	req.DecidedAt = now
	req.DecidedBy = actorName
	req.UpdatedAt = now
	_ = saveRequest(path, req)
	return Result{}, fmt.Errorf("trailfork: approve request %s: %w", req.RequestID, cause)
}

func recordCreationFailure(path string, req Request, cause error, now time.Time) (Result, error) {
	req.LastError = cause.Error()
	req.UpdatedAt = commitmentTime(now)
	_ = saveRequest(path, req)
	return Result{}, cause
}

func recordApprovedRequest(path string, req Request, result Result, actorName, reason string, decidedAt, now time.Time) (Result, error) {
	req.State = StateApproved
	req.Lineage = &result.Plan
	req.TraceID = result.Plan.TraceID
	req.TaskID = result.Plan.Task.TaskID
	req.ForkID = result.Plan.ForkID
	req.DecidedAt = decidedAt
	req.DecidedBy = actorName
	req.Reason = strings.TrimSpace(reason)
	req.UpdatedAt = commitmentTime(now)
	if err := saveRequest(path, req); err != nil {
		return Result{}, err
	}
	return result, nil
}

func forkDefaults(ctxColony colony.Context, traceID string) (colony.Defaults, error) {
	bound, _, err := trailconfig.Bind(ctxColony, traceID)
	if err != nil {
		return colony.Defaults{}, fmt.Errorf("trailfork: read fork configuration: %w", err)
	}
	manifest, err := colony.LoadColony(bound.ConfigRootPath())
	if err != nil {
		return colony.Defaults{}, fmt.Errorf("trailfork: read fork configuration: %w", err)
	}
	return manifest.Defaults, nil
}

// RecoverCreating resumes fork requests whose approved creation was interrupted
// by a restart.
//
// Creation is idempotent under the request identity: an interrupted approval
// either finishes the missing steps or returns the fork it already created. It
// never mints another trail, reseeds honey, or republishes an initial plan that a
// completed attempt already published.
func RecoverCreating(ctx context.Context, opts DecisionOptions) ([]Result, error) {
	if strings.TrimSpace(opts.Context.ColonyRoot) == "" {
		return nil, fmt.Errorf("trailfork: colony root is required")
	}
	requests, err := ListRequests(opts.Context.ColonyRoot)
	if err != nil {
		return nil, err
	}
	var recovered []Result
	var failures []error
	for _, req := range requests {
		// A recorded failure is terminal until an operator retries or cancels it.
		// Retrying it on every boot would repeat the same failing worktree or
		// publish attempt without a new decision.
		if req.State != StateCreating || strings.TrimSpace(req.LastError) != "" {
			continue
		}
		// A fork dropped while its creation was incomplete must not be
		// republished on every restart.
		if req.TraceID != "" {
			if lineage, err := LoadLineage(opts.Context.ColonyRoot, req.TraceID); err == nil && lineage.State == StateDropped {
				req.State = StateCancelled
				req.Reason = "fork was dropped before its creation completed"
				req.UpdatedAt = time.Now().UTC()
				if err := saveRequest(RequestPath(opts.Context.ColonyRoot, req.Evidence.SourceTrace, req.RequestID), req); err != nil {
					failures = append(failures, fmt.Errorf("trailfork: cancel dropped request %s: %w", req.RequestID, err))
				}
				continue
			}
		}
		decision := opts
		decision.RequestID = req.RequestID
		if strings.TrimSpace(decision.Actor) == "" {
			decision.Actor = "runtime"
		}
		if strings.TrimSpace(decision.Reason) == "" {
			decision.Reason = "recovered interrupted creation after restart"
		}
		result, err := ApproveRequest(ctx, decision)
		if err != nil {
			failures = append(failures, fmt.Errorf("trailfork: recover request %s: %w", req.RequestID, err))
			continue
		}
		recovered = append(recovered, result)
	}
	if len(failures) > 0 {
		return recovered, errors.Join(failures...)
	}
	return recovered, nil
}

// discardCreatingResources removes local resources from a creation that never
// reached approval. The request record itself stays under its source trace so the
// operator keeps the audit trail of what was attempted.
func discardCreatingResources(ctx colony.Context, req Request) error {
	root := strings.TrimSpace(ctx.ColonyRoot)
	traceID := strings.TrimSpace(req.TraceID)
	if root == "" || traceID == "" || traceID == strings.TrimSpace(req.Evidence.SourceTrace) {
		return nil
	}
	branch := ""
	if lineage, err := LoadLineage(root, traceID); err == nil {
		branch = strings.TrimSpace(lineage.Branch)
	}
	if branch == "" {
		branch = worktree.BranchName(traceID)
	}
	if gitroot.IsCheckoutRoot(root) {
		if err := worktree.Remove(root, ctx.Slug, traceID); err != nil {
			return fmt.Errorf("remove worktree: %w", err)
		}
		if gitroot.LocalBranchExists(root, branch) {
			if _, err := gitroot.Run(gitroot.RunOpts{
				Dir:  root,
				Args: []string{"branch", "-D", branch},
			}); err != nil {
				return fmt.Errorf("remove branch %q: %w", branch, err)
			}
		}
	}
	if err := os.RemoveAll(filepath.Join(root, ".paseka", "runs", traceID)); err != nil {
		return fmt.Errorf("remove trail records: %w", err)
	}
	return nil
}

// RejectRequest refuses a pending request, or discards an interrupted creation,
// without leaving runnable resources.
func RejectRequest(opts DecisionOptions) (Request, error) {
	return decidePending(opts, StateRejected)
}

// CancelRequest cancels a pending request, or discards an interrupted creation,
// without leaving runnable resources.
func CancelRequest(opts DecisionOptions) (Request, error) {
	return decidePending(opts, StateCancelled)
}

func decidePending(opts DecisionOptions, state string) (Request, error) {
	root := strings.TrimSpace(opts.Context.ColonyRoot)
	_, path, err := loadRequestForDecision(root, opts.RequestID, opts.Now)
	if err != nil {
		return Request{}, err
	}
	lock, err := acquireRequestLock(path + ".lock")
	if err != nil {
		return Request{}, err
	}
	defer lock()

	req, err := loadRequestFile(path)
	if err != nil {
		return Request{}, err
	}
	now := decisionTime(opts.Now)
	req = expireIfNeeded(req, now)
	if req.State == StateExpired {
		_ = saveRequest(path, req)
		return req, fmt.Errorf("trailfork: fork request %s expired", req.RequestID)
	}
	if req.State == state {
		return req, nil
	}
	if req.State != StatePending && req.State != StateCreating {
		return req, fmt.Errorf("trailfork: fork request %s is %s and cannot be %s", req.RequestID, req.State, state)
	}
	if req.State == StateCreating {
		if err := discardCreatingResources(opts.Context, req); err != nil {
			return Request{}, fmt.Errorf("trailfork: discard creating request %s: %w", req.RequestID, err)
		}
	}
	actorName := strings.TrimSpace(opts.Actor)
	if actorName == "" {
		actorName = requestOriginCLI
	}
	req.State = state
	req.Reason = strings.TrimSpace(opts.Reason)
	req.DecidedAt = now
	req.DecidedBy = actorName
	req.UpdatedAt = now
	if err := saveRequest(path, req); err != nil {
		return Request{}, err
	}
	return req, nil
}

// RequestsDir returns .paseka/runs/<sourceTrace>/fork-requests.
func RequestsDir(colonyRoot, sourceTrace string) string {
	return filepath.Join(colonyRoot, ".paseka", "runs", sourceTrace, ForkRequestsDirName)
}

// RequestPath returns .paseka/runs/<sourceTrace>/fork-requests/<requestId>.json.
func RequestPath(colonyRoot, sourceTrace, requestID string) string {
	return filepath.Join(RequestsDir(colonyRoot, sourceTrace), requestID+".json")
}

func authorizeSender(root string, sender Sender) (Sender, error) {
	sender.TraceID = strings.TrimSpace(sender.TraceID)
	sender.AgentID = strings.TrimSpace(sender.AgentID)
	if sender.TraceID == "" || sender.AgentID == "" {
		return Sender{}, fmt.Errorf("trailfork: fork request sender is not a recorded bee run")
	}
	meta, ok, err := runs.FindRun(root, sender.TraceID, sender.AgentID)
	if err != nil {
		return Sender{}, fmt.Errorf("trailfork: read sender run: %w", err)
	}
	if !ok {
		return Sender{}, fmt.Errorf("trailfork: fork request sender %s/%s is not a recorded bee run", sender.TraceID, sender.AgentID)
	}
	bees, err := colony.LoadAllBees(root)
	if err != nil {
		return Sender{}, fmt.Errorf("trailfork: load canonical colony grants: %w", err)
	}
	bee, ok := bees[meta.Bee]
	if !ok || !bee.ExplicitlyDeclaresPublish(protocol.EventSignal, string(protocol.SignalTrailFork)) {
		return Sender{}, fmt.Errorf("trailfork: colony policy does not grant bee %q publication of %s", meta.Bee, protocol.SignalTrailFork)
	}
	sender.Bee = meta.Bee
	return sender, nil
}

func withDirectDefaults(opts Options) Options {
	opts.RequestID = strings.TrimSpace(opts.RequestID)
	if opts.RequestID == "" {
		if id, err := ids.Prefixed("req-"); err == nil {
			opts.RequestID = id
		}
	}
	if strings.TrimSpace(opts.Origin) == "" {
		opts.Origin = requestOriginCLI
	}
	if strings.TrimSpace(opts.ApprovedBy) == "" {
		opts.ApprovedBy = requestOriginCLI
	}
	if strings.TrimSpace(opts.Actor) == "" {
		opts.Actor = opts.ApprovedBy
	}
	return opts
}

func directDigest(opts Options) string {
	content := struct {
		SourceTrace string `json:"sourceTrace"`
		SourceTask  string `json:"sourceTask"`
		SourceRun   string `json:"sourceRun"`
		BaseSHA     string `json:"baseSha"`
		DeltaForm   string `json:"deltaForm"`
		DeltaPatch  string `json:"deltaPatch"`
		Budget      int    `json:"budget"`
		Title       string `json:"title"`
	}{
		SourceTrace: strings.TrimSpace(opts.SourceTraceID),
		SourceTask:  strings.TrimSpace(opts.TaskID),
		SourceRun:   strings.TrimSpace(opts.SourceRun),
		BaseSHA:     strings.TrimSpace(opts.BaseSHA),
		DeltaForm:   strings.TrimSpace(opts.DeltaForm),
		DeltaPatch:  strings.TrimSpace(opts.DeltaPatch),
		Budget:      opts.Budget,
		Title:       strings.TrimSpace(opts.Title),
	}
	data, _ := json.Marshal(content)
	return digestHex(data)
}

func requestDigest(sender Sender, payload protocol.ForkRequestPayload) (string, error) {
	content := struct {
		Sender      Sender `json:"sender"`
		RequestID   string `json:"requestId"`
		SourceTrace string `json:"sourceTrace"`
		SourceTask  string `json:"sourceTask"`
		SourceRun   string `json:"sourceRun"`
		BaseSHA     string `json:"baseSha"`
		Delta       string `json:"delta"`
		Patch       string `json:"patch"`
		Budget      int    `json:"budget"`
		Title       string `json:"title"`
	}{
		Sender:      sender,
		RequestID:   strings.TrimSpace(payload.RequestID),
		SourceTrace: strings.TrimSpace(payload.SourceTrace),
		SourceTask:  strings.TrimSpace(payload.SourceTask),
		SourceRun:   strings.TrimSpace(payload.SourceRun),
		BaseSHA:     strings.TrimSpace(payload.BaseSHA),
		Delta:       strings.TrimSpace(payload.Delta),
		Patch:       strings.TrimSpace(payload.Patch),
		Budget:      payload.Budget,
		Title:       strings.TrimSpace(payload.Title),
	}
	data, err := json.Marshal(content)
	if err != nil {
		return "", err
	}
	sum := sha256.Sum256(data)
	return hex.EncodeToString(sum[:]), nil
}

func pendingEvidence(lineage Lineage) Lineage {
	lineage.RequestID = ""
	lineage.ForkID = ""
	lineage.TraceID = ""
	lineage.Workspace = ""
	lineage.Branch = ""
	lineage.State = ""
	lineage.Delta.Path = ""
	lineage.Task.TaskID = ""
	lineage.Task.AttemptID = ""
	lineage.Task.TaskInputDigest = ""
	return lineage
}

func verifyEvidence(expected, actual Lineage) error {
	type pair struct {
		name     string
		expected string
		actual   string
	}
	pairs := []pair{
		{"source trace", expected.SourceTrace, actual.SourceTrace},
		{"source task", expected.SourceTask, actual.SourceTask},
		{"source attempt", expected.SourceAttempt, actual.SourceAttempt},
		{"source run", expected.SourceRun, actual.SourceRun},
		{"source base", expected.SourceBaseSHA, actual.SourceBaseSHA},
		{"base", expected.BaseSHA, actual.BaseSHA},
		{"source config", expected.SourceConfigRef, actual.SourceConfigRef},
		{"candidate config", expected.ConfigRef, actual.ConfigRef},
		{"profile", expected.Profile, actual.Profile},
		{"delta form", expected.Delta.Form, actual.Delta.Form},
		{"delta patch", expected.Delta.Patch, actual.Delta.Patch},
		{"delta sha256", expected.Delta.SHA256, actual.Delta.SHA256},
		{"task title", expected.Task.Title, actual.Task.Title},
		{"task body", expected.Task.Body, actual.Task.Body},
		{"task bee", expected.Task.Bee, actual.Task.Bee},
		{"task sector", expected.Task.Sector, actual.Task.Sector},
		{"task intent", expected.Task.Intent, actual.Task.Intent},
		{"task cue", expected.Task.CueID, actual.Task.CueID},
		{"task review", expected.Task.Review, actual.Task.Review},
		{"task digest", expected.Task.SourceTaskDigest, actual.Task.SourceTaskDigest},
	}
	for _, item := range pairs {
		if item.expected != item.actual {
			return fmt.Errorf("retained evidence changed: %s was %q, now %q", item.name, item.expected, item.actual)
		}
	}
	if expected.SourceConfigRevision != actual.SourceConfigRevision {
		return fmt.Errorf("retained evidence changed: source revision was %d, now %d", expected.SourceConfigRevision, actual.SourceConfigRevision)
	}
	if expected.Budget != actual.Budget {
		return fmt.Errorf("retained evidence changed: budget was %d, now %d", expected.Budget, actual.Budget)
	}
	if expected.Delta.Bytes != actual.Delta.Bytes {
		return fmt.Errorf("retained evidence changed: patch size was %d, now %d", expected.Delta.Bytes, actual.Delta.Bytes)
	}
	return nil
}

func forkOptionsFromRequest(req Request, ctxColony colony.Context, actorName string) Options {
	return Options{
		Context:       ctxColony,
		SourceTraceID: req.Evidence.SourceTrace,
		TaskID:        req.Evidence.SourceTask,
		SourceRun:     req.Evidence.SourceRun,
		SourceAttempt: req.Evidence.SourceAttempt,
		BaseSHA:       req.Evidence.BaseSHA,
		DeltaForm:     req.Evidence.Delta.Form,
		DeltaPatch:    req.Evidence.Delta.Patch,
		Budget:        req.Evidence.Budget,
		Title:         req.Evidence.Title,
		RequestID:     req.RequestID,
		Origin:        req.Origin,
		ApprovedBy:    actorName,
		Actor:         actorName,
		ForkTraceID:   req.TraceID,
		ForkTaskID:    req.TaskID,
	}
}

func resultFromLineage(ctxColony colony.Context, lineage Lineage) (Result, error) {
	binding, err := trailconfig.Load(ctxColony.ColonyRoot, lineage.TraceID)
	if err != nil {
		return Result{}, fmt.Errorf("trailfork: load fork binding: %w", err)
	}
	return Result{
		Plan:    lineage,
		Created: true,
		Binding: binding,
		Worktree: worktree.Entry{
			TraceID:   lineage.TraceID,
			Path:      lineage.Workspace,
			BaseSHA:   lineage.BaseSHA,
			Branch:    lineage.Branch,
			CreatedAt: lineage.CreatedAt,
		},
		LineagePath: LineagePath(ctxColony.ColonyRoot, lineage.TraceID),
	}, nil
}

type requestFile struct {
	path    string
	request Request
}

func loadAllRequests(colonyRoot string, now time.Time) ([]requestFile, error) {
	if strings.TrimSpace(colonyRoot) == "" {
		return nil, fmt.Errorf("trailfork: colony root is required")
	}
	runsRoot := filepath.Join(colonyRoot, ".paseka", "runs")
	entries, err := os.ReadDir(runsRoot)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, err
	}
	var out []requestFile
	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}
		dir := filepath.Join(runsRoot, entry.Name(), ForkRequestsDirName)
		files, err := os.ReadDir(dir)
		if err != nil {
			if os.IsNotExist(err) {
				continue
			}
			return nil, err
		}
		for _, file := range files {
			if file.IsDir() || !strings.HasSuffix(file.Name(), ".json") {
				continue
			}
			path := filepath.Join(dir, file.Name())
			req, err := loadRequestFile(path)
			if err != nil {
				continue
			}
			if req.State == StatePending {
				expired := expireIfNeeded(req, now)
				if expired.State == StateExpired {
					lock, lockErr := acquireRequestLock(path + ".lock")
					if lockErr == nil {
						current, readErr := loadRequestFile(path)
						if readErr == nil {
							current = expireIfNeeded(current, now)
							_ = saveRequest(path, current)
							req = current
						}
						lock()
					}
				}
			}
			out = append(out, requestFile{path: path, request: req})
		}
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].request.CreatedAt.Equal(out[j].request.CreatedAt) {
			return out[i].request.RequestID < out[j].request.RequestID
		}
		return out[i].request.CreatedAt.Before(out[j].request.CreatedAt)
	})
	return out, nil
}

func findRequest(colonyRoot, requestID string) (Request, bool, error) {
	item, found, err := findRequestFile(colonyRoot, requestID)
	if err != nil || !found {
		return Request{}, found, err
	}
	return item.request, true, nil
}

func findRequestFile(colonyRoot, requestID string) (requestFile, bool, error) {
	requestID = strings.TrimSpace(requestID)
	if requestID == "" {
		return requestFile{}, false, nil
	}
	items, err := loadAllRequests(colonyRoot, time.Now().UTC())
	if err != nil {
		return requestFile{}, false, err
	}
	for _, item := range items {
		if item.request.RequestID == requestID {
			return item, true, nil
		}
	}
	return requestFile{}, false, nil
}

func loadRequestForDecision(colonyRoot, requestID string, now time.Time) (Request, string, error) {
	requestID = strings.TrimSpace(requestID)
	if requestID == "" {
		return Request{}, "", fmt.Errorf("trailfork: requestId is required")
	}
	items, err := loadAllRequests(colonyRoot, decisionTime(now))
	if err != nil {
		return Request{}, "", err
	}
	for _, item := range items {
		if item.request.RequestID == requestID {
			return item.request, item.path, nil
		}
	}
	return Request{}, "", fmt.Errorf("trailfork: fork request %s was not found", requestID)
}

func expireIfNeeded(req Request, now time.Time) Request {
	if req.State != StatePending || req.ExpiresAt.IsZero() {
		return req
	}
	if now.Before(req.ExpiresAt) {
		return req
	}
	req.State = StateExpired
	req.DecidedAt = now
	req.UpdatedAt = now
	return req
}

func decisionTime(value time.Time) time.Time {
	if value.IsZero() {
		return time.Now().UTC()
	}
	return value.UTC()
}

func commitmentTime(value time.Time) time.Time {
	return decisionTime(value)
}

func validateRequestID(value string) error {
	value = strings.TrimSpace(value)
	if value == "" {
		return fmt.Errorf("trailfork: requestId is required")
	}
	if len(value) > 128 {
		return fmt.Errorf("trailfork: requestId is too long")
	}
	for _, r := range value {
		switch {
		case r >= 'a' && r <= 'z':
		case r >= 'A' && r <= 'Z':
		case r >= '0' && r <= '9':
		case r == '-', r == '_', r == '.':
		default:
			return fmt.Errorf("trailfork: requestId %q contains unsupported characters", value)
		}
	}
	if value == "." || value == ".." {
		return fmt.Errorf("trailfork: invalid requestId %q", value)
	}
	return nil
}

func loadRequestFile(path string) (Request, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return Request{}, err
	}
	var req Request
	if err := json.Unmarshal(data, &req); err != nil {
		return Request{}, fmt.Errorf("trailfork: parse request %s: %w", filepath.Base(path), err)
	}
	return req, nil
}

func saveRequest(path string, req Request) error {
	data, err := json.MarshalIndent(req, "", "  ")
	if err != nil {
		return fmt.Errorf("trailfork: encode request: %w", err)
	}
	data = append(data, '\n')
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return fmt.Errorf("trailfork: mkdir request dir: %w", err)
	}
	tmp, err := os.CreateTemp(filepath.Dir(path), ".request-*.json")
	if err != nil {
		return fmt.Errorf("trailfork: create request temp: %w", err)
	}
	tmpName := tmp.Name()
	defer func() { _ = os.Remove(tmpName) }()
	if _, err := tmp.Write(data); err != nil {
		_ = tmp.Close()
		return fmt.Errorf("trailfork: write request: %w", err)
	}
	if err := tmp.Sync(); err != nil {
		_ = tmp.Close()
		return fmt.Errorf("trailfork: sync request: %w", err)
	}
	if err := tmp.Close(); err != nil {
		return fmt.Errorf("trailfork: close request: %w", err)
	}
	if err := os.Rename(tmpName, path); err != nil {
		return fmt.Errorf("trailfork: publish request: %w", err)
	}
	return nil
}

const (
	requestLockBudget     = 5 * time.Second
	requestLockStaleAfter = 30 * time.Second
	// requestDecisionLockBudget outlives requestLockStaleAfter: a duplicate
	// approval or direct retry may legitimately wait for a slow in-flight
	// creation and must then return its recorded outcome instead of timing out
	// mid-creation.
	requestDecisionLockBudget = 90 * time.Second
)

func acquireRequestLock(path string) (func(), error) {
	return acquireRequestLockWithBudget(path, requestLockBudget)
}

// acquireDecisionLock waits for a decision-path lock while honoring caller
// cancellation. Its budget outlives stale-lock reclamation so a legitimate
// creation is never mistaken for an abandoned lock.
func acquireDecisionLock(ctx context.Context, path string) (func(), error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	type acquired struct {
		release func()
		err     error
	}
	// filelock waits without a context, so acquire in the background and return
	// the caller's error on cancellation. A late acquisition is released.
	done := make(chan acquired, 1)
	go func() {
		release, err := acquireRequestLockWithBudget(path, requestDecisionLockBudget)
		done <- acquired{release: release, err: err}
	}()
	select {
	case <-ctx.Done():
		go func() {
			if late := <-done; late.release != nil {
				late.release()
			}
		}()
		return nil, ctx.Err()
	case result := <-done:
		if err := ctx.Err(); err != nil {
			if result.release != nil {
				result.release()
			}
			return nil, err
		}
		return result.release, result.err
	}
}

func acquireRequestLockWithBudget(path string, budget time.Duration) (func(), error) {
	lock, err := filelock.Acquire(path, filelock.Options{
		Budget:     budget,
		StaleAfter: requestLockStaleAfter,
	})
	if err != nil {
		return nil, fmt.Errorf("trailfork: %w", err)
	}
	return lock.Release, nil
}
