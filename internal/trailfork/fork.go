// Package trailfork previews and creates a control fork of one source trail.
//
// A fork is a new flight trail in the same colony. It carries one recorded
// source task input and the source attempt's captured configuration snapshot,
// starts a clean worktree at the source's initial code commit (or an explicit
// recorded alternative), seeds its own capped honey reserve, and publishes an
// ordinary task plan and ready signal to the colony's existing Reactor.
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
	"github.com/russ-p/paseka/internal/gitroot"
	"github.com/russ-p/paseka/internal/patches"
	"github.com/russ-p/paseka/internal/protocol"
	"github.com/russ-p/paseka/internal/runs"
	"github.com/russ-p/paseka/internal/taskledger"
	"github.com/russ-p/paseka/internal/tasks"
	"github.com/russ-p/paseka/internal/trailconfig"
	"github.com/russ-p/paseka/internal/worktree"
)

const (
	// LineageFileName is the fork lineage record under a trail's run root.
	LineageFileName = "fork.json"
	// DeltaNone identifies a no-delta control arm.
	DeltaNone = "none"
	// DeltaPatch identifies a fork whose candidate configuration was derived
	// from a recorded proposal patch.
	DeltaPatch = "patch"
	// StateRunnable marks an admitted fork whose task has been published.
	StateRunnable = "runnable"
	// StateDropped marks a fork whose dispatch was stopped and whose execution
	// resources were removed. Configuration evidence, lineage, attempts, and run
	// records stay readable and keep the trail inspectable.
	StateDropped = "dropped"

	lineageVersion = 1
	// DeltaFileName is the retained patch body under the fork's run root.
	DeltaFileName = "delta.patch"
)

// Delta records which configuration delta produced the fork and retains the
// evidence needed to explain it after the source proposal changes or disappears.
type Delta struct {
	Form string `json:"form"`
	// Patch is the source proposal archive ref.
	Patch string `json:"patch,omitempty"`
	// PatchTrace and PatchAgentID identify the recorded proposal.
	PatchTrace   string `json:"patchTrace,omitempty"`
	PatchAgentID string `json:"patchAgentId,omitempty"`
	// ProposalID is the stable proposal identity (agent run) and Kind its
	// normalized proposal kind.
	ProposalID   string `json:"proposalId,omitempty"`
	ProposalKind string `json:"proposalKind,omitempty"`
	// Publication is the source proposal's publication state at capture time.
	Publication string `json:"publication,omitempty"`
	// CapturedAt is when the source proposal record was captured.
	CapturedAt time.Time `json:"capturedAt,omitempty"`
	// SHA256 and Bytes identify the immutable retained patch body.
	SHA256 string `json:"sha256,omitempty"`
	Bytes  int64  `json:"bytes,omitempty"`
	// Path is the retained patch body inside the fork's own run directory.
	Path string `json:"path,omitempty"`
	// RawDiff is the file-level delta between the source snapshot and the
	// candidate; EffectiveDiff is the resolved behaviour that changes.
	RawDiff       []string `json:"rawDiff,omitempty"`
	EffectiveDiff []string `json:"effectiveDiff,omitempty"`
	// ShadowedDiff names changes a frozen higher-priority layer keeps inert.
	ShadowedDiff []string `json:"shadowedDiff,omitempty"`
}

// TaskLineage records the carried task input and the identity minted for it.
type TaskLineage struct {
	TaskID           string `json:"taskId"`
	SourceTaskID     string `json:"sourceTaskId"`
	Title            string `json:"title,omitempty"`
	Body             string `json:"body,omitempty"`
	Bee              string `json:"bee,omitempty"`
	Sector           string `json:"sector,omitempty"`
	Intent           string `json:"intent,omitempty"`
	CueID            string `json:"cueId,omitempty"`
	Review           string `json:"review,omitempty"`
	SourceTaskDigest string `json:"sourceTaskDigest"`
	TaskInputDigest  string `json:"taskInputDigest,omitempty"`
	AttemptID        string `json:"attemptId,omitempty"`
}

// Lineage is the durable fork record stored beside the fork's tasks.
type Lineage struct {
	Version              int    `json:"version"`
	RequestID            string `json:"requestId,omitempty"`
	ForkID               string `json:"forkId"`
	TraceID              string `json:"traceId"`
	SourceTrace          string `json:"sourceTrace"`
	SourceTask           string `json:"sourceTask"`
	SourceAttempt        string `json:"sourceAttempt"`
	SourceRun            string `json:"sourceRun,omitempty"`
	SourceBaseSHA        string `json:"sourceBaseSha,omitempty"`
	BaseSHA              string `json:"baseSha"`
	BaseOverridden       bool   `json:"baseOverridden,omitempty"`
	SourceConfigRef      string `json:"sourceConfigRef"`
	SourceConfigRevision int    `json:"sourceRevision"`
	ConfigRef            string `json:"configRef"`
	Policy               string `json:"policy"`
	Delta                Delta  `json:"delta"`
	Profile              string `json:"profile,omitempty"`
	Budget               int    `json:"budget"`
	State                string `json:"state"`
	Workspace            string `json:"workspace"`
	Branch               string `json:"branch"`
	Title                string `json:"title,omitempty"`
	// DroppedAt, DroppedBy, and DropReason record an explicit drop. They are
	// retained with the lineage so a removed worktree cannot erase the reason an
	// experiment stopped.
	DroppedAt   *time.Time  `json:"droppedAt,omitempty"`
	DroppedBy   string      `json:"droppedBy,omitempty"`
	DropReason  string      `json:"dropReason,omitempty"`
	Task        TaskLineage `json:"task"`
	Limitations []string    `json:"limitations,omitempty"`
	CreatedAt   time.Time   `json:"createdAt"`
}

// Options describes one preview or creation request.
type Options struct {
	Context       colony.Context
	SourceTraceID string
	TaskID        string
	SourceRun     string
	// SourceAttempt is the recorded source attempt identity. When set, it pins
	// selection to exactly that attempt for an already-recorded decision and
	// never falls back to the latest attempt or to the source run id.
	SourceAttempt string
	BaseSHA       string
	// DeltaForm is DeltaNone (the default) or DeltaPatch.
	DeltaForm string
	// DeltaPatch names a recorded proposal patch: either its colony-root-relative
	// archive ref or an agent id in the source trail's proposal store.
	DeltaPatch string
	Budget     int
	Title      string
	RequestID  string
	Actor      string
	// Origin distinguishes a direct operator creation ("cli") from an approved
	// agent request ("agent").
	Origin string
	// ApprovedBy records the operator control path that admitted the fork.
	ApprovedBy string
	// ForkTraceID and ForkTaskID let approval resume an interrupted creation
	// under the identities minted for the same request.
	ForkTraceID string
	ForkTaskID  string
	DryRun      bool
}

// Result is the previewed plan plus the resources created for it.
type Result struct {
	Plan        Lineage
	Created     bool
	Binding     trailconfig.Manifest
	Worktree    worktree.Entry
	LineagePath string
	Events      []protocol.Event
}

type sourceEvidence struct {
	forkColony colony.Colony
	candidate  *trailconfig.ForkDelta
	patchBytes []byte
}

// Preview resolves the source evidence and reports the fork that creation would
// produce without writing any trail, snapshot, worktree, lineage, or honey.
func Preview(opts Options) (Result, error) {
	plan, _, err := resolve(opts)
	if err != nil {
		return Result{}, err
	}
	return Result{Plan: plan}, nil
}

// Create performs the on-disk fork creation and publishes its initial task
// plan and ready signal to the colony's existing Reactor.
//
// A non-empty request identity makes creation idempotent: a repeated delivery
// with the same contents returns the recorded fork, while reusing the identity
// with different contents is refused.
func Create(ctx context.Context, publisher bus.Publisher, ledger taskledger.Ledger, opts Options) (Result, error) {
	opts = withDirectDefaults(opts)
	if opts.DryRun {
		plan, _, err := resolve(opts)
		if err != nil {
			return Result{}, err
		}
		return Result{Plan: plan}, nil
	}
	root := strings.TrimSpace(opts.Context.ColonyRoot)
	if root == "" {
		return Result{}, fmt.Errorf("trailfork: colony root is required")
	}
	requestID := strings.TrimSpace(opts.RequestID)
	if requestID == "" {
		return Result{}, fmt.Errorf("trailfork: request id is required")
	}
	digest := directDigest(opts)

	if existing, found, err := findRequestFile(root, requestID); err != nil {
		return Result{}, err
	} else if found {
		return resumeDirectRequest(ctx, publisher, ledger, opts, existing, digest)
	}

	plan, _, err := resolve(opts)
	if err != nil {
		return Result{}, err
	}
	if !bus.PublisherAvailable(publisher) {
		return Result{}, fmt.Errorf("trailfork: nats url not configured (fork creation requires NATS)")
	}
	if ledger == nil {
		return Result{}, fmt.Errorf("trailfork: task ledger is required (fork creation requires NATS)")
	}

	now := time.Now().UTC()
	actor := directActor(opts)
	req := Request{
		Version:       requestVersion,
		RequestID:     requestID,
		State:         StateCreating,
		Origin:        requestOriginCLI,
		Evidence:      pendingEvidence(plan),
		ContentDigest: digest,
		Validation:    "ok",
		CreatedAt:     now,
		UpdatedAt:     now,
		DecidedBy:     actor,
		ForkID:        plan.ForkID,
		TraceID:       plan.TraceID,
		TaskID:        plan.Task.TaskID,
	}
	path := RequestPath(root, plan.SourceTrace, requestID)

	// Serialize the identity lookup and pre-commit so two deliveries cannot both
	// observe "not found" and mint separate forks. Creation itself runs under
	// the per-request lock after this one is released.
	globalLock, err := acquireRequestLock(filepath.Join(root, ".paseka", "runs", ".fork-requests.lock"))
	if err != nil {
		return Result{}, err
	}
	existing, found, findErr := findRequestFile(root, requestID)
	if findErr != nil {
		globalLock()
		return Result{}, findErr
	}
	if found {
		globalLock()
		return resumeDirectRequest(ctx, publisher, ledger, opts, existing, digest)
	}
	saveErr := saveRequest(path, req)
	globalLock()
	if saveErr != nil {
		return Result{}, saveErr
	}
	return resumeDirectRequest(ctx, publisher, ledger, opts, requestFile{path: path, request: req}, digest)
}

func directActor(opts Options) string {
	actor := strings.TrimSpace(opts.ApprovedBy)
	if actor == "" {
		actor = strings.TrimSpace(opts.Actor)
	}
	if actor == "" {
		actor = requestOriginCLI
	}
	return actor
}

func resumeDirectRequest(ctx context.Context, publisher bus.Publisher, ledger taskledger.Ledger, opts Options, item requestFile, digest string) (Result, error) {
	if item.request.ContentDigest != digest {
		return Result{}, fmt.Errorf("trailfork: request id %s was already used with different contents", opts.RequestID)
	}
	if item.request.State == StatePending {
		return Result{}, fmt.Errorf("trailfork: request %s is pending operator approval", opts.RequestID)
	}
	lock, err := acquireRequestLock(item.path + ".lock")
	if err != nil {
		return Result{}, err
	}
	defer lock()

	req, err := loadRequestFile(item.path)
	if err != nil {
		return Result{}, err
	}
	if req.ContentDigest != digest {
		return Result{}, fmt.Errorf("trailfork: request id %s was already used with different contents", opts.RequestID)
	}
	if req.State == StatePending {
		return Result{}, fmt.Errorf("trailfork: request %s is pending operator approval", opts.RequestID)
	}
	return approveRequestLocked(ctx, DecisionOptions{
		Context:   opts.Context,
		RequestID: req.RequestID,
		Actor:     directActor(opts),
		Publisher: publisher,
		Ledger:    ledger,
	}, req, item.path)
}

func createResolved(ctx context.Context, publisher bus.Publisher, ledger taskledger.Ledger, opts Options, plan Lineage, evidence sourceEvidence) (Result, error) {
	if plan.RequestID != "" {
		if existing, err := LoadLineage(opts.Context.ColonyRoot, plan.TraceID); err == nil && existing.RequestID == plan.RequestID {
			// An interrupted approval may have written the lineage and binding
			// before its initial plan reached the ledger. Finish that publication
			// idempotently instead of leaving a created-but-unrunnable fork.
			if err := finishInitialPublication(ctx, publisher, ledger, existing, evidence.forkColony.Defaults); err != nil {
				return Result{}, err
			}
			return resultFromLineage(opts.Context, existing)
		}
	}

	binding, err := trailconfig.CreateForkBinding(trailconfig.ForkBindingOptions{
		Context:              opts.Context,
		SourceTraceID:        plan.SourceTrace,
		TraceID:              plan.TraceID,
		ConfigRef:            plan.ConfigRef,
		BaseConfigRef:        plan.SourceConfigRef,
		Candidate:            evidence.candidate,
		InitialCodeBaseSHA:   plan.BaseSHA,
		InitialCodeBaseDirty: false,
		WorkspaceRoot:        plan.Workspace,
		Actor:                actor(opts),
		RequestID:            plan.RequestID,
	})
	if err != nil {
		return Result{}, err
	}

	if len(evidence.patchBytes) > 0 {
		if err := writeDelta(opts.Context.ColonyRoot, plan.TraceID, evidence.patchBytes); err != nil {
			return Result{}, err
		}
	}

	entry, err := worktree.Ensure(worktree.EnsureOptions{
		ColonyRoot: opts.Context.ColonyRoot,
		TraceID:    plan.TraceID,
		Slug:       opts.Context.Slug,
		Branch:     plan.Branch,
		BaseSHA:    plan.BaseSHA,
	})
	if err != nil {
		return Result{}, fmt.Errorf("trailfork: create worktree: %w", err)
	}
	plan.Workspace = entry.Path
	plan.Branch = entry.Branch

	if err := runs.WriteTaskSnapshot(opts.Context.ColonyRoot, plan.TraceID, taskledger.TaskSnapshot{
		TaskID: plan.Task.TaskID,
		Title:  plan.Task.Title,
		Body:   plan.Task.Body,
		Bee:    plan.Task.Bee,
		Sector: plan.Task.Sector,
		Intent: plan.Task.Intent,
		Review: protocol.TaskReviewPolicy(plan.Task.Review),
		Status: protocol.TaskStatusPlanned,
	}); err != nil {
		return Result{}, fmt.Errorf("trailfork: write task record: %w", err)
	}

	inputEvidence := trailconfig.NewTaskInputEvidence(
		plan.Task.TaskID,
		plan.Task.Title,
		plan.Task.Body,
		plan.Task.Bee,
		plan.Task.Sector,
		plan.Task.Intent,
		plan.Task.Review,
		plan.Task.CueID,
	)
	attempt, err := trailconfig.EnsureTaskAttempt(opts.Context, plan.TraceID, plan.Task.TaskID, "", inputEvidence)
	if err != nil {
		return Result{}, fmt.Errorf("trailfork: record task attempt: %w", err)
	}
	plan.Task.AttemptID = attempt.AttemptID
	plan.Task.TaskInputDigest = inputEvidence.Digest

	if err := writeLineage(opts.Context.ColonyRoot, plan); err != nil {
		return Result{}, err
	}
	if err := ledger.SeedEnergy(plan.TraceID, plan.Budget); err != nil {
		return Result{}, fmt.Errorf("trailfork: seed honey reserve: %w", err)
	}

	agentID := "cli"
	spec := protocol.TaskSpec{
		TaskID: plan.Task.TaskID,
		Title:  plan.Task.Title,
		Body:   plan.Task.Body,
		Bee:    plan.Task.Bee,
		Sector: plan.Task.Sector,
		Intent: plan.Task.Intent,
		CueID:  plan.Task.CueID,
		Review: protocol.TaskReviewPolicy(plan.Task.Review),
	}
	planEv, err := tasks.PlanEvent(plan.TraceID, agentID, spec)
	if err != nil {
		return Result{}, err
	}
	readyEv, err := tasks.ReadyEvent(plan.TraceID, agentID, taskledger.TaskSnapshot{
		TaskID: plan.Task.TaskID,
		Title:  plan.Task.Title,
		Body:   plan.Task.Body,
		Bee:    plan.Task.Bee,
		Sector: plan.Task.Sector,
		Intent: plan.Task.Intent,
		CueID:  plan.Task.CueID,
		Review: protocol.TaskReviewPolicy(plan.Task.Review),
	}, evidence.forkColony.Defaults)
	if err != nil {
		return Result{}, err
	}
	for _, ev := range []protocol.Event{planEv, readyEv} {
		if err := publisher.PublishEvent(ctx, ev); err != nil {
			return Result{}, fmt.Errorf("trailfork: publish %s: %w", protocol.PayloadKind(ev.Payload), err)
		}
	}

	return Result{
		Plan:        plan,
		Created:     true,
		Binding:     binding,
		Worktree:    entry,
		LineagePath: LineagePath(opts.Context.ColonyRoot, plan.TraceID),
		Events:      []protocol.Event{planEv, readyEv},
	}, nil
}

// finishInitialPublication completes the initial plan and ready signal for a
// fork whose lineage already exists. A normal repeat approval publishes
// nothing: once the carried task is past planned, the publication is done.
//
// The ledger can record the task as planned without it ever becoming ready when
// a crash lands between task.plan and task.ready. That gap is finished here by
// publishing only the missing event: task.plan is skipped when the ledger
// already applied it, and task.ready only promotes a still-planned task.
func finishInitialPublication(ctx context.Context, publisher bus.Publisher, ledger taskledger.Ledger, lineage Lineage, defaults colony.Defaults) error {
	if ledger == nil {
		return fmt.Errorf("trailfork: task ledger is required to finish fork creation")
	}
	if strings.TrimSpace(lineage.Task.TaskID) == "" {
		return fmt.Errorf("trailfork: fork %s has no recorded initial task", lineage.TraceID)
	}
	snap, err := ledger.Snapshot(lineage.TraceID)
	if err != nil {
		return fmt.Errorf("trailfork: inspect fork ledger: %w", err)
	}
	planned, known := snap.Tasks[lineage.Task.TaskID]
	// A carried task past planned (ready, running, or terminal) already had its
	// ready published. Republishing would reset a live task, so leave it alone.
	applied := known && planned.Status != ""
	if applied && planned.Status != protocol.TaskStatusPlanned {
		return nil
	}
	if snap.Killed {
		return fmt.Errorf("trailfork: fork %s was stopped before its initial task was published", lineage.TraceID)
	}
	if err := ledger.SeedEnergy(lineage.TraceID, lineage.Budget); err != nil {
		return fmt.Errorf("trailfork: seed honey reserve: %w", err)
	}
	task := taskledger.TaskSnapshot{
		TaskID: lineage.Task.TaskID,
		Title:  lineage.Task.Title,
		Body:   lineage.Task.Body,
		Bee:    lineage.Task.Bee,
		Sector: lineage.Task.Sector,
		Intent: lineage.Task.Intent,
		CueID:  lineage.Task.CueID,
		Review: protocol.TaskReviewPolicy(lineage.Task.Review),
	}
	spec := protocol.TaskSpec{
		TaskID: task.TaskID,
		Title:  task.Title,
		Body:   task.Body,
		Bee:    task.Bee,
		Sector: task.Sector,
		Intent: task.Intent,
		CueID:  task.CueID,
		Review: task.Review,
	}
	var publication []protocol.Event
	if !applied {
		planEv, err := tasks.PlanEvent(lineage.TraceID, "cli", spec)
		if err != nil {
			return err
		}
		publication = append(publication, planEv)
	}
	readyEv, err := tasks.ReadyEvent(lineage.TraceID, "cli", task, defaults)
	if err != nil {
		return err
	}
	publication = append(publication, readyEv)
	for _, ev := range publication {
		if err := publisher.PublishEvent(ctx, ev); err != nil {
			return fmt.Errorf("trailfork: publish %s: %w", protocol.PayloadKind(ev.Payload), err)
		}
	}
	return nil
}

// resolveEvidence validates the selected source evidence without minting trail
// or task identities. Submit uses it to record a pending request; approval
// repeats it before any on-disk creation.
func resolveEvidence(opts Options) (Lineage, sourceEvidence, error) {
	if opts.Context.ColonyRoot == "" {
		return Lineage{}, sourceEvidence{}, fmt.Errorf("trailfork: colony root is required")
	}
	sourceTrace := strings.TrimSpace(opts.SourceTraceID)
	if sourceTrace == "" {
		return Lineage{}, sourceEvidence{}, fmt.Errorf("trailfork: source trail is required (--source)")
	}

	sourceManifest, err := trailconfig.Load(opts.Context.ColonyRoot, sourceTrace)
	if err != nil {
		if errors.Is(err, trailconfig.ErrNoBinding) {
			return Lineage{}, sourceEvidence{}, fmt.Errorf("trailfork: source trail %s has no configuration binding (legacy trails cannot be forked in v1)", sourceTrace)
		}
		return Lineage{}, sourceEvidence{}, err
	}
	if sourceManifest.Policy == trailconfig.PolicyPinned {
		return Lineage{}, sourceEvidence{}, fmt.Errorf("trailfork: source trail %s is a pinned experimental trail; forks of forks are not supported", sourceTrace)
	}
	if sourceManifest.Policy != trailconfig.PolicyEditable {
		return Lineage{}, sourceEvidence{}, fmt.Errorf("trailfork: source trail %s has an unknown configuration policy %q", sourceTrace, sourceManifest.Policy)
	}
	snapshot, err := runs.LoadTraceTasksFromFS(opts.Context.ColonyRoot, sourceTrace)
	if err != nil {
		return Lineage{}, sourceEvidence{}, fmt.Errorf("trailfork: load source tasks: %w", err)
	}
	task, err := selectTask(snapshot, opts.TaskID)
	if err != nil {
		return Lineage{}, sourceEvidence{}, err
	}
	attempt, err := selectAttempt(opts.Context.ColonyRoot, sourceTrace, task, opts.SourceRun, opts.SourceAttempt)
	if err != nil {
		return Lineage{}, sourceEvidence{}, err
	}
	input := attempt.TaskInput
	if input == nil || strings.TrimSpace(input.Digest) == "" {
		return Lineage{}, sourceEvidence{}, fmt.Errorf("trailfork: source attempt %s for task %s has no recorded task input evidence", attempt.AttemptID, task.TaskID)
	}
	if strings.TrimSpace(input.TaskID) == "" {
		input.TaskID = task.TaskID
	}
	if strings.TrimSpace(attempt.ConfigRef) == "" || attempt.ConfigRevision < 1 {
		return Lineage{}, sourceEvidence{}, fmt.Errorf("trailfork: source attempt %s has no recorded configuration reference; its historical configuration is unknown", attempt.AttemptID)
	}
	if !retainsSnapshot(sourceManifest, attempt.ConfigRef) {
		return Lineage{}, sourceEvidence{}, fmt.Errorf("trailfork: source trail %s no longer retains configuration %s", sourceTrace, attempt.ConfigRef)
	}
	bound, err := trailconfig.BindRef(opts.Context, sourceTrace, attempt.ConfigRef, attempt.ConfigRevision)
	if err != nil {
		return Lineage{}, sourceEvidence{}, fmt.Errorf("trailfork: source configuration evidence is unavailable: %w", err)
	}
	sourceColony, err := colony.LoadColony(bound.ConfigRootPath())
	if err != nil {
		return Lineage{}, sourceEvidence{}, fmt.Errorf("trailfork: source configuration is unreadable: %w", err)
	}
	resolvedTaskBee := colony.EffectiveTaskBee(input.Bee, sourceColony.Defaults)
	delta, patchBytes, candidate, err := resolveDelta(opts, sourceTrace, attempt.ConfigRef, task.TaskID, input, resolvedTaskBee)
	if err != nil {
		return Lineage{}, sourceEvidence{}, err
	}

	var forkColony colony.Colony
	var bee string
	if candidate != nil {
		bee = candidate.TaskBee
		if bee == "" {
			bee = resolvedTaskBee
		}
	} else {
		forkColony, bee, err = validateCandidate(bound, task, input)
		if err != nil {
			return Lineage{}, sourceEvidence{}, err
		}
	}

	baseSHA, sourceBaseSHA, overridden, err := resolveBase(opts, sourceManifest)
	if err != nil {
		return Lineage{}, sourceEvidence{}, err
	}

	manifest, err := colony.LoadColony(opts.Context.ColonyRoot)
	if err != nil {
		return Lineage{}, sourceEvidence{}, err
	}
	ceiling := manifest.ResolvedEnergyBudget()
	budget := opts.Budget
	if budget <= 0 {
		budget = ceiling
	}
	if budget > ceiling {
		return Lineage{}, sourceEvidence{}, fmt.Errorf("trailfork: budget %d exceeds the colony ceiling %d", budget, ceiling)
	}

	activeConfigRef := attempt.ConfigRef
	if candidate != nil {
		activeConfigRef = candidate.ConfigRef
	}
	review := string(protocol.NormalizeTaskReviewPolicy(protocol.TaskReviewPolicy(strings.TrimSpace(input.Review))))
	plan := Lineage{
		Version:              lineageVersion,
		RequestID:            strings.TrimSpace(opts.RequestID),
		SourceTrace:          sourceTrace,
		SourceTask:           task.TaskID,
		SourceAttempt:        attempt.AttemptID,
		SourceRun:            strings.TrimSpace(opts.SourceRun),
		SourceBaseSHA:        sourceBaseSHA,
		BaseSHA:              baseSHA,
		BaseOverridden:       overridden,
		SourceConfigRef:      attempt.ConfigRef,
		SourceConfigRevision: attempt.ConfigRevision,
		ConfigRef:            activeConfigRef,
		Policy:               trailconfig.PolicyPinned,
		Delta:                delta,
		Profile:              bound.Profile,
		Budget:               budget,
		Title:                strings.TrimSpace(opts.Title),
		Task: TaskLineage{
			SourceTaskID:     task.TaskID,
			Title:            strings.TrimSpace(input.Title),
			Body:             strings.TrimSpace(input.Body),
			Bee:              bee,
			Sector:           strings.TrimSpace(input.Sector),
			Intent:           strings.TrimSpace(input.Intent),
			CueID:            strings.TrimSpace(input.CueID),
			Review:           review,
			SourceTaskDigest: input.Digest,
		},
		Limitations: limitations(baseSHA, sourceBaseSHA, overridden, delta),
		CreatedAt:   time.Now().UTC(),
	}
	if plan.Title == "" {
		plan.Title = plan.Task.Title
	}
	return plan, sourceEvidence{forkColony: forkColony, candidate: candidate, patchBytes: patchBytes}, nil
}

func resolve(opts Options) (Lineage, sourceEvidence, error) {
	plan, evidence, err := resolveEvidence(opts)
	if err != nil {
		return Lineage{}, sourceEvidence{}, err
	}

	traceID := strings.TrimSpace(opts.ForkTraceID)
	mintedTrace := traceID == ""
	if mintedTrace {
		traceID, err = colony.NewTraceID()
		if err != nil {
			return Lineage{}, sourceEvidence{}, err
		}
	}
	taskID := strings.TrimSpace(opts.ForkTaskID)
	if taskID == "" {
		taskID, err = colony.NewTaskID()
		if err != nil {
			return Lineage{}, sourceEvidence{}, err
		}
	}

	workspace := worktree.Path(opts.Context.ColonyRoot, traceID)
	branch := worktree.BranchName(traceID)
	// Only a freshly minted identity must be collision-free. A resumed approval
	// reuses the identities recorded on its pending request and may find the
	// worktree or branch it already created.
	if mintedTrace {
		if _, statErr := os.Stat(workspace); statErr == nil {
			return Lineage{}, sourceEvidence{}, fmt.Errorf("trailfork: worktree %s already exists", workspace)
		}
		if gitroot.LocalBranchExists(opts.Context.ColonyRoot, branch) {
			return Lineage{}, sourceEvidence{}, fmt.Errorf("trailfork: branch %q already exists", branch)
		}
	}

	plan.ForkID = "fork-" + strings.TrimPrefix(traceID, "trace-")
	plan.TraceID = traceID
	plan.Task.TaskID = taskID
	plan.State = StateRunnable
	plan.Workspace = workspace
	plan.Branch = branch
	if plan.Delta.Form == DeltaPatch {
		plan.Delta.Path = DeltaPath(opts.Context.ColonyRoot, traceID)
	}
	return plan, evidence, nil
}

// resolveDelta selects the configuration delta and prepares a patch candidate.
//
// A patch delta is read from the recorded proposal store and copied into the
// candidate by PrepareForkDelta. The returned bytes are retained by the fork so
// later changes to the source proposal cannot rewrite its evidence.
func resolveDelta(opts Options, sourceTrace, sourceConfigRef, sourceTaskID string, input *trailconfig.TaskInputEvidence, resolvedTaskBee string) (Delta, []byte, *trailconfig.ForkDelta, error) {
	form := strings.TrimSpace(opts.DeltaForm)
	if form == "" {
		form = DeltaNone
	}
	switch form {
	case DeltaNone:
		if strings.TrimSpace(opts.DeltaPatch) != "" {
			return Delta{}, nil, nil, fmt.Errorf("trailfork: a patch selector requires delta form %q", DeltaPatch)
		}
		return Delta{Form: DeltaNone}, nil, nil, nil
	case DeltaPatch:
		selector := strings.TrimSpace(opts.DeltaPatch)
		if selector == "" {
			return Delta{}, nil, nil, fmt.Errorf("trailfork: --delta requires a proposal patch ref or agent id")
		}
		var store patches.Store
		agentID := selector
		if parsed, ok := patches.ParseArchiveRef(selector); ok {
			store = patches.NewStore(opts.Context.ColonyRoot, parsed.TraceID)
			agentID = parsed.AgentID
		} else {
			if strings.ContainsAny(selector, `/\`) {
				return Delta{}, nil, nil, fmt.Errorf("trailfork: delta %q is not a proposal archive ref", selector)
			}
			store = patches.NewStore(opts.Context.ColonyRoot, sourceTrace)
		}
		record, ok, err := store.Read(agentID)
		if err != nil {
			return Delta{}, nil, nil, fmt.Errorf("trailfork: read proposal patch %s: %w", agentID, err)
		}
		if !ok {
			return Delta{}, nil, nil, fmt.Errorf("trailfork: proposal patch record %s was not found", agentID)
		}
		body, err := os.ReadFile(store.PatchPath(agentID))
		if err != nil {
			return Delta{}, nil, nil, fmt.Errorf("trailfork: read proposal patch bytes %s: %w", agentID, err)
		}
		if int64(len(body)) != record.Bytes {
			return Delta{}, nil, nil, fmt.Errorf("trailfork: proposal patch %s does not match its recorded size", agentID)
		}
		candidate, err := trailconfig.PrepareForkDelta(trailconfig.ForkDeltaOptions{
			Context:         opts.Context,
			SourceTraceID:   sourceTrace,
			SourceConfigRef: sourceConfigRef,
			Patch:           body,
			PatchName:       record.Ref(opts.Context.ColonyRoot),
			TaskID:          sourceTaskID,
			TaskInput:       input,
			ResolvedTaskBee: resolvedTaskBee,
		})
		if err != nil {
			return Delta{}, nil, nil, err
		}
		delta := Delta{
			Form:          DeltaPatch,
			Patch:         record.Ref(opts.Context.ColonyRoot),
			PatchTrace:    record.TraceID,
			PatchAgentID:  record.AgentID,
			ProposalID:    record.AgentID,
			ProposalKind:  record.Kind,
			Publication:   record.Publication,
			CapturedAt:    record.CapturedAt,
			SHA256:        digestHex(body),
			Bytes:         int64(len(body)),
			RawDiff:       candidate.RawDiff,
			EffectiveDiff: candidate.EffectiveDiff,
			ShadowedDiff:  candidate.ShadowedDiff,
		}
		return delta, body, &candidate, nil
	default:
		return Delta{}, nil, nil, fmt.Errorf("trailfork: unknown delta form %q", form)
	}
}

func digestHex(data []byte) string {
	sum := sha256.Sum256(data)
	return hex.EncodeToString(sum[:])
}

func selectTask(snapshot taskledger.TraceSnapshot, requested string) (taskledger.TaskSnapshot, error) {
	requested = strings.TrimSpace(requested)
	if requested != "" {
		task, ok := snapshot.Tasks[requested]
		if !ok {
			return taskledger.TaskSnapshot{}, fmt.Errorf("trailfork: source task %s was not found", requested)
		}
		if taskledger.IsFinalReviewTask(task) {
			return taskledger.TaskSnapshot{}, fmt.Errorf("trailfork: task %s is a final-review task and cannot be forked", requested)
		}
		if len(task.DependsOn) > 0 {
			return taskledger.TaskSnapshot{}, fmt.Errorf("trailfork: task %s depends on %s; dependent tasks cannot be forked in v1", requested, strings.Join(task.DependsOn, ", "))
		}
		return task, nil
	}

	var roots []taskledger.TaskSnapshot
	for _, task := range snapshot.Tasks {
		if taskledger.IsFinalReviewTask(task) || len(task.DependsOn) > 0 {
			continue
		}
		roots = append(roots, task)
	}
	sort.Slice(roots, func(i, j int) bool { return roots[i].TaskID < roots[j].TaskID })
	switch len(roots) {
	case 1:
		return roots[0], nil
	case 0:
		for _, task := range snapshot.Tasks {
			if taskledger.IsFinalReviewTask(task) {
				return taskledger.TaskSnapshot{}, fmt.Errorf("trailfork: source has no root task; final-review tasks cannot be forked")
			}
		}
		return taskledger.TaskSnapshot{}, fmt.Errorf("trailfork: source has no root task to fork")
	default:
		ids := make([]string, 0, len(roots))
		for _, task := range roots {
			ids = append(ids, task.TaskID)
		}
		return taskledger.TaskSnapshot{}, fmt.Errorf("trailfork: source has ambiguous root tasks (%s); select one with --task", strings.Join(ids, ", "))
	}
}

func selectAttempt(colonyRoot, traceID string, task taskledger.TaskSnapshot, sourceRun, sourceAttempt string) (trailconfig.Attempt, error) {
	attempts, err := trailconfig.Attempts(colonyRoot, traceID)
	if err != nil {
		return trailconfig.Attempt{}, fmt.Errorf("trailfork: load source attempts: %w", err)
	}
	var candidates []trailconfig.Attempt
	for _, attempt := range attempts {
		if attempt.Kind == trailconfig.AttemptKindTask && attempt.TaskID == task.TaskID {
			candidates = append(candidates, attempt)
		}
	}
	sort.Slice(candidates, func(i, j int) bool {
		if candidates[i].CreatedAt.Equal(candidates[j].CreatedAt) {
			return candidates[i].AttemptID < candidates[j].AttemptID
		}
		return candidates[i].CreatedAt.Before(candidates[j].CreatedAt)
	})
	if len(candidates) == 0 {
		return trailconfig.Attempt{}, fmt.Errorf("trailfork: source task %s has no recorded attempt evidence", task.TaskID)
	}

	// A recorded attempt is a decision about one specific execution. It must
	// survive a source retry and must not follow the latest attempt or the
	// source run id.
	pinnedID := strings.TrimSpace(sourceAttempt)
	var pinned *trailconfig.Attempt
	if pinnedID != "" {
		for i := range candidates {
			if candidates[i].AttemptID == pinnedID {
				pinned = &candidates[i]
				break
			}
		}
		if pinned == nil {
			return trailconfig.Attempt{}, fmt.Errorf("trailfork: source attempt %s for task %s is not recorded", pinnedID, task.TaskID)
		}
	}

	run := strings.TrimSpace(sourceRun)
	if run == "" {
		if pinned != nil {
			return *pinned, nil
		}
		if len(candidates) > 1 {
			ids := make([]string, 0, len(candidates))
			for _, attempt := range candidates {
				ids = append(ids, attempt.AttemptID)
			}
			return trailconfig.Attempt{}, fmt.Errorf("trailfork: source task %s has multiple attempts (%s); select one with --source-run", task.TaskID, strings.Join(ids, ", "))
		}
		return candidates[0], nil
	}

	matched, err := attemptForRun(colonyRoot, traceID, task.TaskID, run, attempts)
	if err != nil {
		return trailconfig.Attempt{}, err
	}
	if pinned != nil && pinned.AttemptID != matched.AttemptID {
		return trailconfig.Attempt{}, fmt.Errorf("trailfork: source run %s selects attempt %s but the recorded source attempt for task %s is %s", run, matched.AttemptID, task.TaskID, pinned.AttemptID)
	}
	return matched, nil
}

func attemptForRun(colonyRoot, traceID, taskID, run string, attempts []trailconfig.Attempt) (trailconfig.Attempt, error) {
	for _, attempt := range attempts {
		if attempt.AttemptID == run && attempt.TaskID == taskID && attempt.Kind == trailconfig.AttemptKindTask {
			return attempt, nil
		}
	}
	taskDir, err := runs.NewTaskDir(colonyRoot, traceID, taskID)
	if err == nil {
		entries, readErr := taskDir.ReadTaskRuns()
		if readErr == nil {
			for _, entry := range entries {
				if entry.AgentID != run || strings.TrimSpace(entry.AttemptID) == "" {
					continue
				}
				attempt, ok, findErr := trailconfig.FindAttempt(colonyRoot, traceID, entry.AttemptID)
				if findErr != nil {
					return trailconfig.Attempt{}, findErr
				}
				if ok && attempt.TaskID == taskID && attempt.Kind == trailconfig.AttemptKindTask {
					return attempt, nil
				}
			}
		}
	}
	for _, attempt := range attempts {
		if attempt.TaskID != taskID || attempt.Kind != trailconfig.AttemptKindTask {
			continue
		}
		for _, agentID := range attempt.Agents {
			if agentID == run {
				return attempt, nil
			}
		}
	}
	return trailconfig.Attempt{}, fmt.Errorf("trailfork: source run %s does not belong to a recorded attempt of task %s", run, taskID)
}

func validateCandidate(bound colony.Context, task taskledger.TaskSnapshot, input *trailconfig.TaskInputEvidence) (colony.Colony, string, error) {
	manifest, err := colony.LoadColony(bound.ConfigRootPath())
	if err != nil {
		return colony.Colony{}, "", fmt.Errorf("trailfork: source configuration is unreadable: %w", err)
	}
	bees, err := colony.LoadAllBees(bound.ConfigRootPath())
	if err != nil {
		return colony.Colony{}, "", fmt.Errorf("trailfork: source bee definitions are unreadable: %w", err)
	}
	beeRole := colony.EffectiveTaskBee(input.Bee, manifest.Defaults)
	bee, ok := bees[beeRole]
	if !ok {
		return colony.Colony{}, "", fmt.Errorf("trailfork: source configuration has no bee %q for task %s", beeRole, task.TaskID)
	}
	if err := bee.ValidateAdapterRequirements(); err != nil {
		return colony.Colony{}, "", fmt.Errorf("trailfork: source configuration is invalid for task %s: %w", task.TaskID, err)
	}
	if sector := strings.TrimSpace(input.Sector); sector != "" {
		if _, err := manifest.ResolveSector(sector); err != nil {
			return colony.Colony{}, "", fmt.Errorf("trailfork: source configuration is invalid for task %s: %w", task.TaskID, err)
		}
	}
	spec := protocol.TaskSpec{
		TaskID: task.TaskID,
		Title:  input.Title,
		Body:   input.Body,
		Bee:    beeRole,
		Sector: input.Sector,
		Intent: input.Intent,
		Review: protocol.TaskReviewPolicy(input.Review),
	}
	if err := colony.ValidateTaskReviewPolicy(spec, bees, manifest.Defaults); err != nil {
		return colony.Colony{}, "", err
	}
	return manifest, beeRole, nil
}

func resolveBase(opts Options, sourceManifest trailconfig.Manifest) (baseSHA, sourceBaseSHA string, overridden bool, err error) {
	sourceBaseSHA = strings.TrimSpace(sourceManifest.InitialCodeBaseSHA)
	requested := strings.TrimSpace(opts.BaseSHA)
	if requested == "" {
		if sourceBaseSHA == "" {
			return "", "", false, fmt.Errorf("trailfork: source has no recorded initial code base")
		}
		if sourceManifest.InitialCodeBaseDirty {
			return "", "", false, fmt.Errorf("trailfork: source initial code state was dirty; an unrecorded clean base is required")
		}
		baseSHA, err = resolveCommit(opts.Context.ColonyRoot, sourceBaseSHA)
		if err != nil {
			return "", "", false, fmt.Errorf("trailfork: source initial code commit %s is unavailable: %w", sourceBaseSHA, err)
		}
		return baseSHA, sourceBaseSHA, false, nil
	}

	baseSHA, err = resolveCommit(opts.Context.ColonyRoot, requested)
	if err != nil {
		return "", "", false, fmt.Errorf("trailfork: base commit %s is unavailable: %w", requested, err)
	}
	overridden = sourceBaseSHA == "" || !strings.EqualFold(sourceBaseSHA, baseSHA)
	if !overridden && sourceManifest.InitialCodeBaseDirty {
		return "", "", false, fmt.Errorf("trailfork: source initial code state was dirty; an unrecorded clean base is required")
	}
	return baseSHA, sourceBaseSHA, overridden, nil
}

func resolveCommit(colonyRoot, ref string) (string, error) {
	out, err := gitroot.Run(gitroot.RunOpts{
		Dir:     colonyRoot,
		Timeout: gitroot.StatusTimeout,
		Args:    []string{"rev-parse", "--verify", strings.TrimSpace(ref) + "^{commit}"},
	})
	if err != nil {
		return "", err
	}
	return strings.TrimSpace(out), nil
}

func retainsSnapshot(manifest trailconfig.Manifest, ref string) bool {
	for _, snapshot := range manifest.Snapshots {
		if snapshot.ConfigRef == ref {
			return true
		}
	}
	return false
}

func limitations(baseSHA, sourceBaseSHA string, overridden bool, delta Delta) []string {
	headline := "control arm: the source attempt's configuration and task input are inherited without a delta"
	if delta.Form == DeltaPatch {
		headline = "patch delta: the candidate applies the recorded proposal patch to the source snapshot's portable tree"
	}
	out := []string{
		headline,
		"comparison uses one recorded attempt of one source task; aggregate source-trail metrics are not a baseline",
		"model nondeterminism, external services, credentials, client-discovered instructions, and tool versions can still differ",
		"the fork runs in its own worktree; source code changes, runs, comb, and honey are not copied",
	}
	if delta.Form == DeltaPatch {
		out = append(out, "the retained patch bytes and proposal provenance are fork-owned evidence; later proposal changes do not rewrite them")
	}
	if overridden {
		out = append(out, fmt.Sprintf("explicit base override %s -> %s removes the equal-base comparison guarantee", shortSHA(sourceBaseSHA), shortSHA(baseSHA)))
	}
	return out
}

func shortSHA(sha string) string {
	sha = strings.TrimSpace(sha)
	if len(sha) > 12 {
		return sha[:12]
	}
	if sha == "" {
		return "(none)"
	}
	return sha
}

func actor(opts Options) string {
	if strings.TrimSpace(opts.Actor) == "" {
		return "cli"
	}
	return strings.TrimSpace(opts.Actor)
}

// LineagePath returns .paseka/runs/<traceId>/fork.json.
func LineagePath(colonyRoot, traceID string) string {
	return filepath.Join(colonyRoot, ".paseka", "runs", traceID, LineageFileName)
}

// DeltaPath returns the colony-root-relative retained patch body for a fork:
// .paseka/runs/<traceId>/delta.patch.
func DeltaPath(colonyRoot, traceID string) string {
	return filepath.Join(colonyRoot, ".paseka", "runs", traceID, DeltaFileName)
}

func writeDelta(colonyRoot, traceID string, body []byte) error {
	path := DeltaPath(colonyRoot, traceID)
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return fmt.Errorf("trailfork: mkdir delta dir: %w", err)
	}
	tmp, err := os.CreateTemp(filepath.Dir(path), ".delta-*.patch")
	if err != nil {
		return fmt.Errorf("trailfork: create delta temp: %w", err)
	}
	tmpName := tmp.Name()
	defer func() { _ = os.Remove(tmpName) }()
	if _, err := tmp.Write(body); err != nil {
		_ = tmp.Close()
		return fmt.Errorf("trailfork: write delta: %w", err)
	}
	if err := tmp.Sync(); err != nil {
		_ = tmp.Close()
		return fmt.Errorf("trailfork: sync delta: %w", err)
	}
	if err := tmp.Close(); err != nil {
		return fmt.Errorf("trailfork: close delta: %w", err)
	}
	if err := os.Rename(tmpName, path); err != nil {
		return fmt.Errorf("trailfork: publish delta: %w", err)
	}
	return nil
}

// LoadLineage reads a fork lineage record.
func LoadLineage(colonyRoot, traceID string) (Lineage, error) {
	data, err := os.ReadFile(LineagePath(colonyRoot, traceID))
	if err != nil {
		return Lineage{}, err
	}
	var lineage Lineage
	if err := json.Unmarshal(data, &lineage); err != nil {
		return Lineage{}, fmt.Errorf("trailfork: parse lineage: %w", err)
	}
	return lineage, nil
}

func writeLineage(colonyRoot string, lineage Lineage) error {
	path := LineagePath(colonyRoot, lineage.TraceID)
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return fmt.Errorf("trailfork: mkdir lineage dir: %w", err)
	}
	data, err := json.MarshalIndent(lineage, "", "  ")
	if err != nil {
		return fmt.Errorf("trailfork: encode lineage: %w", err)
	}
	data = append(data, '\n')
	tmp, err := os.CreateTemp(filepath.Dir(path), ".fork-*.json")
	if err != nil {
		return fmt.Errorf("trailfork: create lineage temp: %w", err)
	}
	tmpName := tmp.Name()
	defer func() { _ = os.Remove(tmpName) }()
	if _, err := tmp.Write(data); err != nil {
		_ = tmp.Close()
		return fmt.Errorf("trailfork: write lineage: %w", err)
	}
	if err := tmp.Sync(); err != nil {
		_ = tmp.Close()
		return fmt.Errorf("trailfork: sync lineage: %w", err)
	}
	if err := tmp.Close(); err != nil {
		return fmt.Errorf("trailfork: close lineage: %w", err)
	}
	if err := os.Rename(tmpName, path); err != nil {
		return fmt.Errorf("trailfork: publish lineage: %w", err)
	}
	return nil
}
