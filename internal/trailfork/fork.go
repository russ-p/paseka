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
	// StateRunnable marks an admitted fork whose task has been published.
	StateRunnable = "runnable"

	lineageVersion = 1
)

// Delta records which configuration delta produced the fork. This version only
// admits control arms.
type Delta struct {
	Form       string `json:"form"`
	Patch      string `json:"patch,omitempty"`
	ProposalID string `json:"proposalId,omitempty"`
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
	Review           string `json:"review,omitempty"`
	SourceTaskDigest string `json:"sourceTaskDigest"`
	TaskInputDigest  string `json:"taskInputDigest,omitempty"`
	AttemptID        string `json:"attemptId,omitempty"`
}

// Lineage is the durable fork record stored beside the fork's tasks.
type Lineage struct {
	Version              int         `json:"version"`
	RequestID            string      `json:"requestId,omitempty"`
	ForkID               string      `json:"forkId"`
	TraceID              string      `json:"traceId"`
	SourceTrace          string      `json:"sourceTrace"`
	SourceTask           string      `json:"sourceTask"`
	SourceAttempt        string      `json:"sourceAttempt"`
	SourceRun            string      `json:"sourceRun,omitempty"`
	SourceBaseSHA        string      `json:"sourceBaseSha,omitempty"`
	BaseSHA              string      `json:"baseSha"`
	BaseOverridden       bool        `json:"baseOverridden,omitempty"`
	SourceConfigRef      string      `json:"sourceConfigRef"`
	SourceConfigRevision int         `json:"sourceRevision"`
	ConfigRef            string      `json:"configRef"`
	Policy               string      `json:"policy"`
	Delta                Delta       `json:"delta"`
	Profile              string      `json:"profile,omitempty"`
	Budget               int         `json:"budget"`
	State                string      `json:"state"`
	Workspace            string      `json:"workspace"`
	Branch               string      `json:"branch"`
	Title                string      `json:"title,omitempty"`
	Task                 TaskLineage `json:"task"`
	Limitations          []string    `json:"limitations,omitempty"`
	CreatedAt            time.Time   `json:"createdAt"`
}

// Options describes one preview or creation request.
type Options struct {
	Context       colony.Context
	SourceTraceID string
	TaskID        string
	SourceRun     string
	BaseSHA       string
	Budget        int
	Title         string
	RequestID     string
	Actor         string
	DryRun        bool
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
func Create(ctx context.Context, publisher bus.Publisher, ledger taskledger.Ledger, opts Options) (Result, error) {
	plan, evidence, err := resolve(opts)
	if err != nil {
		return Result{}, err
	}
	if opts.DryRun {
		return Result{Plan: plan}, nil
	}
	if !bus.PublisherAvailable(publisher) {
		return Result{}, fmt.Errorf("trailfork: nats url not configured (fork creation requires NATS)")
	}
	if ledger == nil {
		return Result{}, fmt.Errorf("trailfork: task ledger is required (fork creation requires NATS)")
	}

	binding, err := trailconfig.CreateForkBinding(trailconfig.ForkBindingOptions{
		Context:              opts.Context,
		SourceTraceID:        plan.SourceTrace,
		TraceID:              plan.TraceID,
		ConfigRef:            plan.ConfigRef,
		InitialCodeBaseSHA:   plan.BaseSHA,
		InitialCodeBaseDirty: false,
		WorkspaceRoot:        plan.Workspace,
		Actor:                actor(opts),
		RequestID:            plan.RequestID,
	})
	if err != nil {
		return Result{}, err
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

func resolve(opts Options) (Lineage, sourceEvidence, error) {
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
			return Lineage{}, sourceEvidence{}, fmt.Errorf("trailfork: source trail %s has no configuration snapshot", sourceTrace)
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
	attempt, err := selectAttempt(opts.Context.ColonyRoot, sourceTrace, task, opts.SourceRun)
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
		return Lineage{}, sourceEvidence{}, fmt.Errorf("trailfork: source attempt %s has no recorded configuration reference", attempt.AttemptID)
	}
	if !retainsSnapshot(sourceManifest, attempt.ConfigRef) {
		return Lineage{}, sourceEvidence{}, fmt.Errorf("trailfork: source trail %s no longer retains configuration %s", sourceTrace, attempt.ConfigRef)
	}
	bound, err := trailconfig.BindRef(opts.Context, sourceTrace, attempt.ConfigRef, attempt.ConfigRevision)
	if err != nil {
		return Lineage{}, sourceEvidence{}, fmt.Errorf("trailfork: source configuration evidence is unavailable: %w", err)
	}
	forkColony, bee, err := validateCandidate(bound, task, input)
	if err != nil {
		return Lineage{}, sourceEvidence{}, err
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

	traceID, err := colony.NewTraceID()
	if err != nil {
		return Lineage{}, sourceEvidence{}, err
	}
	taskID, err := colony.NewTaskID()
	if err != nil {
		return Lineage{}, sourceEvidence{}, err
	}
	workspace := worktree.Path(opts.Context.ColonyRoot, traceID)
	branch := worktree.BranchName(traceID)
	if _, statErr := os.Stat(workspace); statErr == nil {
		return Lineage{}, sourceEvidence{}, fmt.Errorf("trailfork: worktree %s already exists", workspace)
	}
	if gitroot.LocalBranchExists(opts.Context.ColonyRoot, branch) {
		return Lineage{}, sourceEvidence{}, fmt.Errorf("trailfork: branch %q already exists", branch)
	}

	review := string(protocol.NormalizeTaskReviewPolicy(protocol.TaskReviewPolicy(strings.TrimSpace(input.Review))))
	plan := Lineage{
		Version:              lineageVersion,
		RequestID:            strings.TrimSpace(opts.RequestID),
		ForkID:               "fork-" + strings.TrimPrefix(traceID, "trace-"),
		TraceID:              traceID,
		SourceTrace:          sourceTrace,
		SourceTask:           task.TaskID,
		SourceAttempt:        attempt.AttemptID,
		SourceRun:            strings.TrimSpace(opts.SourceRun),
		SourceBaseSHA:        sourceBaseSHA,
		BaseSHA:              baseSHA,
		BaseOverridden:       overridden,
		SourceConfigRef:      attempt.ConfigRef,
		SourceConfigRevision: attempt.ConfigRevision,
		ConfigRef:            attempt.ConfigRef,
		Policy:               trailconfig.PolicyPinned,
		Delta:                Delta{Form: DeltaNone},
		Profile:              bound.Profile,
		Budget:               budget,
		State:                StateRunnable,
		Workspace:            workspace,
		Branch:               branch,
		Title:                strings.TrimSpace(opts.Title),
		Task: TaskLineage{
			TaskID:           taskID,
			SourceTaskID:     task.TaskID,
			Title:            strings.TrimSpace(input.Title),
			Body:             strings.TrimSpace(input.Body),
			Bee:              bee,
			Sector:           strings.TrimSpace(input.Sector),
			Intent:           strings.TrimSpace(input.Intent),
			Review:           review,
			SourceTaskDigest: input.Digest,
		},
		Limitations: limitations(baseSHA, sourceBaseSHA, overridden),
		CreatedAt:   time.Now().UTC(),
	}
	if plan.Title == "" {
		plan.Title = plan.Task.Title
	}
	return plan, sourceEvidence{forkColony: forkColony}, nil
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

func selectAttempt(colonyRoot, traceID string, task taskledger.TaskSnapshot, sourceRun string) (trailconfig.Attempt, error) {
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

	run := strings.TrimSpace(sourceRun)
	if run == "" {
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

func limitations(baseSHA, sourceBaseSHA string, overridden bool) []string {
	out := []string{
		"control arm: the source attempt's configuration and task input are inherited without a delta",
		"comparison uses one recorded attempt of one source task; aggregate source-trail metrics are not a baseline",
		"model nondeterminism, external services, credentials, client-discovered instructions, and tool versions can still differ",
		"the fork runs in its own worktree; source code changes, runs, comb, and honey are not copied",
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
