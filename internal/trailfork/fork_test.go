package trailfork_test

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/russ-p/paseka/internal/colony"
	"github.com/russ-p/paseka/internal/patches"
	"github.com/russ-p/paseka/internal/protocol"
	"github.com/russ-p/paseka/internal/runs"
	"github.com/russ-p/paseka/internal/taskledger"
	"github.com/russ-p/paseka/internal/trailconfig"
	"github.com/russ-p/paseka/internal/trailfork"
)

type capturePublisher struct {
	events []protocol.Event
}

func (p *capturePublisher) PublishEvent(_ context.Context, ev protocol.Event) error {
	p.events = append(p.events, ev)
	return nil
}

func TestPreviewReportsControlForkWithoutDurableChanges(t *testing.T) {
	root, base := setupGitRepo(t)
	ctx := testContext(root)
	writeSource(t, ctx, "trace-source", "task-1", nil)

	before := listDir(t, filepath.Join(root, ".paseka", "runs"))
	result, err := trailfork.Preview(trailfork.Options{Context: ctx, SourceTraceID: "trace-source"})
	if err != nil {
		t.Fatal(err)
	}
	plan := result.Plan
	if plan.SourceTrace != "trace-source" || plan.SourceTask != "task-1" {
		t.Fatalf("source evidence = %+v", plan)
	}
	if plan.SourceAttempt == "" || plan.SourceConfigRef == "" || plan.SourceConfigRevision != 1 {
		t.Fatalf("attempt/config evidence = %+v", plan)
	}
	if plan.Task.SourceTaskDigest == "" {
		t.Fatal("missing source task digest")
	}
	if plan.BaseSHA != base || plan.BaseOverridden {
		t.Fatalf("base = %s overridden=%v, want %s", plan.BaseSHA, plan.BaseOverridden, base)
	}
	if plan.Workspace != filepath.Join(root, ".paseka", "worktrees", plan.TraceID) {
		t.Fatalf("workspace = %q", plan.Workspace)
	}
	if plan.Branch != "paseka/"+plan.TraceID {
		t.Fatalf("branch = %q", plan.Branch)
	}
	if plan.Budget != 8 || plan.Policy != trailconfig.PolicyPinned || plan.Delta.Form != trailfork.DeltaNone {
		t.Fatalf("fork policy = %+v", plan)
	}
	if len(plan.Limitations) == 0 {
		t.Fatal("missing comparison limitations")
	}
	if result.Created {
		t.Fatal("preview reported creation")
	}
	if got := listDir(t, filepath.Join(root, ".paseka", "runs")); strings.Join(got, ",") != strings.Join(before, ",") {
		t.Fatalf("preview changed run records: %v -> %v", before, got)
	}
	if _, err := os.Stat(plan.Workspace); !os.IsNotExist(err) {
		t.Fatalf("preview created worktree: %v", err)
	}
	if _, err := os.Stat(filepath.Join(root, ".paseka", "runs", plan.TraceID)); !os.IsNotExist(err) {
		t.Fatalf("preview created trail records: %v", err)
	}
}

func TestCreateMintsIndependentForkWithEvidenceAndReserve(t *testing.T) {
	root, base := setupGitRepo(t)
	ctx := testContext(root)
	writeSource(t, ctx, "trace-source", "task-1", nil)

	// Move HEAD beyond the source's recorded base to prove the worktree starts
	// at the initial commit rather than current HEAD.
	if err := os.WriteFile(filepath.Join(root, "later.txt"), []byte("later\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	runGit(t, root, "add", "later.txt")
	runGit(t, root, "commit", "-q", "-m", "later")

	ledger := taskledger.NewMemoryLedger()
	pub := &capturePublisher{}
	result, err := trailfork.Create(context.Background(), pub, ledger, trailfork.Options{
		Context: ctx, SourceTraceID: "trace-source",
	})
	if err != nil {
		t.Fatal(err)
	}
	if !result.Created {
		t.Fatal("fork was not created")
	}
	plan := result.Plan
	if plan.TraceID == "trace-source" || plan.Task.TaskID == "task-1" {
		t.Fatalf("identities were not minted: %+v", plan)
	}
	if plan.BaseSHA != base || plan.BaseOverridden {
		t.Fatalf("base = %s overridden=%v, want %s", plan.BaseSHA, plan.BaseOverridden, base)
	}

	binding, err := trailconfig.Load(root, plan.TraceID)
	if err != nil {
		t.Fatal(err)
	}
	if binding.Policy != trailconfig.PolicyPinned || binding.ActiveConfigRef != plan.SourceConfigRef || binding.ConfigRevision != 1 {
		t.Fatalf("binding = %+v", binding)
	}
	if binding.InitialCodeBaseSHA != base {
		t.Fatalf("binding base = %q, want %q", binding.InitialCodeBaseSHA, base)
	}
	if binding.WorkspaceRoot != result.Worktree.Path {
		t.Fatalf("binding workspace root = %q, want %q", binding.WorkspaceRoot, result.Worktree.Path)
	}
	bound, err := trailconfig.BindRef(ctx, plan.TraceID, plan.ConfigRef, 1)
	if err != nil {
		t.Fatal(err)
	}
	if bound.WorkspaceRoot != result.Worktree.Path {
		t.Fatalf("bound workspace root = %q, want %q", bound.WorkspaceRoot, result.Worktree.Path)
	}
	snapshot := filepath.Join(root, ".paseka", "runs", plan.TraceID, "config", "snapshots", plan.ConfigRef, "tree", ".paseka", "colony.yaml")
	if _, err := os.Stat(snapshot); err != nil {
		t.Fatalf("fork snapshot missing: %v", err)
	}

	if got := gitOutput(t, result.Worktree.Path, "rev-parse", "HEAD"); got != base {
		t.Fatalf("worktree HEAD = %q, want %q", got, base)
	}
	if got := gitOutput(t, result.Worktree.Path, "rev-parse", "--abbrev-ref", "HEAD"); got != plan.Branch {
		t.Fatalf("worktree branch = %q, want %q", got, plan.Branch)
	}
	if _, err := os.Stat(filepath.Join(result.Worktree.Path, "later.txt")); !os.IsNotExist(err) {
		t.Fatalf("later commit leaked into fork worktree: %v", err)
	}

	lineage, err := trailfork.LoadLineage(root, plan.TraceID)
	if err != nil {
		t.Fatal(err)
	}
	if lineage.SourceTrace != "trace-source" || lineage.SourceTask != "task-1" ||
		lineage.SourceAttempt != plan.SourceAttempt || lineage.SourceConfigRef != plan.SourceConfigRef ||
		lineage.Task.SourceTaskDigest != plan.Task.SourceTaskDigest || lineage.ConfigRef != plan.ConfigRef {
		t.Fatalf("lineage = %+v", lineage)
	}
	if lineage.Delta.Form != trailfork.DeltaNone || lineage.Policy != trailconfig.PolicyPinned {
		t.Fatalf("lineage policy/delta = %+v", lineage)
	}

	snap, err := ledger.Snapshot(plan.TraceID)
	if err != nil {
		t.Fatal(err)
	}
	if snap.EnergyBudget != 8 || snap.EnergyRemaining != 8 {
		t.Fatalf("fork honey = budget %d remaining %d", snap.EnergyBudget, snap.EnergyRemaining)
	}
	sourceSnap, err := ledger.Snapshot("trace-source")
	if err != nil {
		t.Fatal(err)
	}
	if sourceSnap.EnergyBudget != 0 || sourceSnap.EnergyRemaining != 0 {
		t.Fatalf("source honey was touched: %+v", sourceSnap)
	}

	attempt, ok, err := trailconfig.LatestTaskAttempt(root, plan.TraceID, plan.Task.TaskID)
	if err != nil || !ok {
		t.Fatalf("fork attempt: %v %v", ok, err)
	}
	if attempt.ConfigRef != plan.ConfigRef || attempt.TaskInput == nil || attempt.TaskInput.Digest == "" {
		t.Fatalf("fork attempt evidence = %+v", attempt)
	}

	if len(pub.events) != 2 {
		t.Fatalf("published %d events, want plan+ready", len(pub.events))
	}
	if kind := protocol.PayloadKind(pub.events[0].Payload); kind != string(protocol.TaskEventPlan) {
		t.Fatalf("first event kind = %q", kind)
	}
	if kind := protocol.PayloadKind(pub.events[1].Payload); kind != string(protocol.TaskEventReady) {
		t.Fatalf("second event kind = %q", kind)
	}
	var planPayload protocol.TaskPlanPayload
	if err := json.Unmarshal(pub.events[0].Payload, &planPayload); err != nil {
		t.Fatal(err)
	}
	if len(planPayload.Tasks) != 1 || planPayload.Tasks[0].TaskID != plan.Task.TaskID {
		t.Fatalf("plan payload = %+v", planPayload)
	}
	if event := pub.events[1]; event.TraceID != plan.TraceID || protocol.PayloadTaskID(event.Payload) != plan.Task.TaskID {
		t.Fatalf("ready event routed to %s/%s", event.TraceID, protocol.PayloadTaskID(event.Payload))
	}
	if pub.events[0].TraceID != plan.TraceID {
		t.Fatalf("plan event routed to %s", pub.events[0].TraceID)
	}

	// The fork owns its snapshot copy: deleting the source trail records leaves
	// the fork's configuration binding readable and executable.
	if err := os.RemoveAll(filepath.Join(root, ".paseka", "runs", "trace-source", "config")); err != nil {
		t.Fatal(err)
	}
	if _, err := trailconfig.BindRef(ctx, plan.TraceID, plan.ConfigRef, 1); err != nil {
		t.Fatalf("fork snapshot did not survive source removal: %v", err)
	}
}

func TestCreateUsesRecordedAlternativeBase(t *testing.T) {
	root, first := setupGitRepo(t)
	if err := os.WriteFile(filepath.Join(root, "second.txt"), []byte("second\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	runGit(t, root, "add", "second.txt")
	runGit(t, root, "commit", "-q", "-m", "second")
	ctx := testContext(root)
	writeSource(t, ctx, "trace-source", "task-1", nil)

	result, err := trailfork.Create(context.Background(), &capturePublisher{}, taskledger.NewMemoryLedger(), trailfork.Options{
		Context: ctx, SourceTraceID: "trace-source", BaseSHA: first,
	})
	if err != nil {
		t.Fatal(err)
	}
	if !result.Plan.BaseOverridden || result.Plan.BaseSHA != first {
		t.Fatalf("base = %s overridden=%v, want %s", result.Plan.BaseSHA, result.Plan.BaseOverridden, first)
	}
	if got := gitOutput(t, result.Worktree.Path, "rev-parse", "HEAD"); got != first {
		t.Fatalf("worktree HEAD = %q, want %q", got, first)
	}
	lineage, err := trailfork.LoadLineage(root, result.Plan.TraceID)
	if err != nil {
		t.Fatal(err)
	}
	if !lineage.BaseOverridden || lineage.BaseSHA != first || lineage.SourceBaseSHA == first {
		t.Fatalf("lineage base = %+v", lineage)
	}
}

func TestForkRefusesUnsupportedSources(t *testing.T) {
	t.Run("ambiguous roots", func(t *testing.T) {
		root, _ := setupGitRepo(t)
		ctx := testContext(root)
		manifest, err := trailconfig.Ensure(ctx, "trace-source")
		if err != nil {
			t.Fatal(err)
		}
		_ = manifest
		writeTask(t, root, "trace-source", taskledger.TaskSnapshot{TaskID: "task-a", Title: "a", Bee: "builder", Status: protocol.TaskStatusCompleted})
		writeTask(t, root, "trace-source", taskledger.TaskSnapshot{TaskID: "task-b", Title: "b", Bee: "builder", Status: protocol.TaskStatusCompleted})
		addAttempt(t, ctx, "trace-source", "task-a", "evt-a")
		addAttempt(t, ctx, "trace-source", "task-b", "evt-b")

		_, err = trailfork.Preview(trailfork.Options{Context: ctx, SourceTraceID: "trace-source"})
		if err == nil || !strings.Contains(err.Error(), "ambiguous root") {
			t.Fatalf("error = %v", err)
		}
	})

	t.Run("dependent task", func(t *testing.T) {
		root, _ := setupGitRepo(t)
		ctx := testContext(root)
		writeSource(t, ctx, "trace-source", "task-2", []string{"task-1"})
		_, err := trailfork.Preview(trailfork.Options{Context: ctx, SourceTraceID: "trace-source", TaskID: "task-2"})
		if err == nil || !strings.Contains(err.Error(), "depends on") {
			t.Fatalf("error = %v", err)
		}
	})

	t.Run("final review task", func(t *testing.T) {
		root, _ := setupGitRepo(t)
		ctx := testContext(root)
		writeSource(t, ctx, "trace-source", "task-1", nil)
		writeTask(t, root, "trace-source", taskledger.TaskSnapshot{
			TaskID: "task-review", Title: "review", Bee: "builder", Status: protocol.TaskStatusPlanned, Review: protocol.TaskReviewFinal,
		})
		_, err := trailfork.Preview(trailfork.Options{Context: ctx, SourceTraceID: "trace-source", TaskID: "task-review"})
		if err == nil || !strings.Contains(err.Error(), "final-review") {
			t.Fatalf("error = %v", err)
		}
	})

	t.Run("missing task input evidence", func(t *testing.T) {
		root, _ := setupGitRepo(t)
		ctx := testContext(root)
		if _, err := trailconfig.Ensure(ctx, "trace-source"); err != nil {
			t.Fatal(err)
		}
		writeTask(t, root, "trace-source", taskledger.TaskSnapshot{TaskID: "task-1", Title: "one", Bee: "builder", Status: protocol.TaskStatusPlanned})
		_, err := trailfork.Preview(trailfork.Options{Context: ctx, SourceTraceID: "trace-source"})
		if err == nil || !strings.Contains(err.Error(), "no recorded attempt") {
			t.Fatalf("error = %v", err)
		}
	})

	t.Run("missing snapshot", func(t *testing.T) {
		root, _ := setupGitRepo(t)
		ctx := testContext(root)
		writeSource(t, ctx, "trace-source", "task-1", nil)
		manifest, err := trailconfig.Load(root, "trace-source")
		if err != nil {
			t.Fatal(err)
		}
		if err := os.RemoveAll(filepath.Join(root, ".paseka", "runs", "trace-source", "config", "snapshots", manifest.ActiveConfigRef)); err != nil {
			t.Fatal(err)
		}
		_, err = trailfork.Preview(trailfork.Options{Context: ctx, SourceTraceID: "trace-source"})
		if err == nil || !strings.Contains(err.Error(), "unavailable") {
			t.Fatalf("error = %v", err)
		}
	})

	t.Run("dirty source state", func(t *testing.T) {
		root, _ := setupGitRepo(t)
		ctx := testContext(root)
		if err := os.WriteFile(filepath.Join(root, "uncommitted.txt"), []byte("dirty\n"), 0o644); err != nil {
			t.Fatal(err)
		}
		writeSource(t, ctx, "trace-source", "task-1", nil)
		_, err := trailfork.Preview(trailfork.Options{Context: ctx, SourceTraceID: "trace-source"})
		if err == nil || !strings.Contains(err.Error(), "dirty") {
			t.Fatalf("error = %v", err)
		}
	})

	t.Run("missing initial code base", func(t *testing.T) {
		root := t.TempDir()
		runGit(t, root, "init", "-q", "-b", "main")
		runGit(t, root, "config", "user.email", "test@example.com")
		runGit(t, root, "config", "user.name", "Test")
		writeColonyFiles(t, root)
		ctx := testContext(root)
		writeSource(t, ctx, "trace-source", "task-1", nil)
		_, err := trailfork.Preview(trailfork.Options{Context: ctx, SourceTraceID: "trace-source"})
		if err == nil || !strings.Contains(err.Error(), "no recorded initial code base") {
			t.Fatalf("error = %v", err)
		}
	})

	t.Run("fork of a fork", func(t *testing.T) {
		root, _ := setupGitRepo(t)
		ctx := testContext(root)
		writeSource(t, ctx, "trace-source", "task-1", nil)
		created, err := trailfork.Create(context.Background(), &capturePublisher{}, taskledger.NewMemoryLedger(), trailfork.Options{
			Context: ctx, SourceTraceID: "trace-source",
		})
		if err != nil {
			t.Fatal(err)
		}
		_, err = trailfork.Preview(trailfork.Options{Context: ctx, SourceTraceID: created.Plan.TraceID})
		if err == nil || !strings.Contains(err.Error(), "pinned") {
			t.Fatalf("error = %v", err)
		}
	})

	t.Run("source run selects one attempt", func(t *testing.T) {
		root, _ := setupGitRepo(t)
		ctx := testContext(root)
		writeSource(t, ctx, "trace-source", "task-1", nil)
		if err := trailconfig.CloseTaskAttempts(root, "trace-source", "task-1", "first closed"); err != nil {
			t.Fatal(err)
		}
		second := addAttempt(t, ctx, "trace-source", "task-1", "evt-2")
		if err := trailconfig.RecordAttemptAgent(root, "trace-source", second.AttemptID, "agent-second"); err != nil {
			t.Fatal(err)
		}
		_, err := trailfork.Preview(trailfork.Options{Context: ctx, SourceTraceID: "trace-source"})
		if err == nil || !strings.Contains(err.Error(), "multiple attempts") {
			t.Fatalf("error = %v", err)
		}
		result, err := trailfork.Preview(trailfork.Options{Context: ctx, SourceTraceID: "trace-source", SourceRun: "agent-second"})
		if err != nil {
			t.Fatal(err)
		}
		if result.Plan.SourceAttempt != second.AttemptID || result.Plan.SourceRun != "agent-second" {
			t.Fatalf("selected attempt = %+v", result.Plan)
		}
	})
}

func TestForkRejectsBudgetAboveColonyCeiling(t *testing.T) {
	root, _ := setupGitRepo(t)
	ctx := testContext(root)
	writeSource(t, ctx, "trace-source", "task-1", nil)
	_, err := trailfork.Preview(trailfork.Options{Context: ctx, SourceTraceID: "trace-source", Budget: 9})
	if err == nil || !strings.Contains(err.Error(), "exceeds the colony ceiling") {
		t.Fatalf("error = %v", err)
	}
}

func setupGitRepo(t *testing.T) (string, string) {
	t.Helper()
	root := t.TempDir()
	runGit(t, root, "init", "-q", "-b", "main")
	runGit(t, root, "config", "user.email", "test@example.com")
	runGit(t, root, "config", "user.name", "Test")
	writeColonyFiles(t, root)
	runGit(t, root, "add", ".")
	runGit(t, root, "commit", "-q", "-m", "initial")
	return root, gitOutput(t, root, "rev-parse", "HEAD")
}

func writeColonyFiles(t *testing.T, root string) {
	t.Helper()
	files := map[string]string{
		".gitignore":                      ".paseka/runs/\n.paseka/worktrees/\n",
		".paseka/colony.yaml":             "slug: test\ndefaults:\n  energy_budget: 8\n",
		".paseka/bees/builder.yaml":       "role: builder\nadapter: script\nprompt_template: builder.md\ncommand: [echo]\n",
		".paseka/prompts/builder.md":      "build the thing\n",
		".paseka/cues/feature.yaml":       "emit: task\ntitle: Feature\nbody: Do it\nbee: builder\n",
		".agents/skills/example/SKILL.md": "---\nname: example\ndescription: example skill\n---\nbody\n",
	}
	for rel, content := range files {
		path := filepath.Join(root, filepath.FromSlash(rel))
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
			t.Fatal(err)
		}
	}
}

func testContext(root string) colony.Context {
	return colony.Context{ColonyRoot: root}
}

func writeSource(t *testing.T, ctx colony.Context, traceID, taskID string, dependsOn []string) {
	t.Helper()
	if _, err := trailconfig.Ensure(ctx, traceID); err != nil {
		t.Fatal(err)
	}
	writeTask(t, ctx.ColonyRoot, traceID, taskledger.TaskSnapshot{
		TaskID:    taskID,
		Title:     "implement " + taskID,
		Bee:       "builder",
		Status:    protocol.TaskStatusCompleted,
		DependsOn: dependsOn,
	})
	addAttempt(t, ctx, traceID, taskID, "evt-"+taskID)
}

func writeTask(t *testing.T, root, traceID string, task taskledger.TaskSnapshot) {
	t.Helper()
	if err := runs.WriteTaskSnapshot(root, traceID, task); err != nil {
		t.Fatal(err)
	}
}

func addAttempt(t *testing.T, ctx colony.Context, traceID, taskID, eventID string) trailconfig.Attempt {
	t.Helper()
	input := trailconfig.NewTaskInputEvidence(taskID, "implement "+taskID, "body for "+taskID, "builder", "", "feature", string(protocol.TaskReviewNone))
	attempt, err := trailconfig.EnsureTaskAttempt(ctx, traceID, taskID, eventID, input)
	if err != nil {
		t.Fatal(err)
	}
	return attempt
}

func listDir(t *testing.T, dir string) []string {
	t.Helper()
	entries, err := os.ReadDir(dir)
	if err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		t.Fatal(err)
	}
	var out []string
	for _, entry := range entries {
		out = append(out, entry.Name())
	}
	return out
}

func runGit(t *testing.T, dir string, args ...string) {
	t.Helper()
	cmd := exec.Command("git", append([]string{"-C", dir}, args...)...)
	cmd.Env = append(os.Environ(), "GIT_AUTHOR_DATE=2026-01-01T00:00:00Z", "GIT_COMMITTER_DATE=2026-01-01T00:00:00Z")
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("git %s: %v: %s", strings.Join(args, " "), err, out)
	}
}

func gitOutput(t *testing.T, dir string, args ...string) string {
	t.Helper()
	cmd := exec.Command("git", append([]string{"-C", dir}, args...)...)
	out, err := cmd.Output()
	if err != nil {
		t.Fatalf("git %s: %v", strings.Join(args, " "), err)
	}
	return strings.TrimSpace(string(out))
}

func TestPatchDeltaProducesValidatedCandidateAndOwnEvidence(t *testing.T) {
	root, _ := setupGitRepo(t)
	writeFile(t, root, ".paseka/prompts/obsolete.md", "obsolete\n")
	runGit(t, root, "add", ".")
	runGit(t, root, "commit", "-q", "-m", "add obsolete prompt")

	ctx := testContext(root)
	writeSource(t, ctx, "trace-source", "task-1", nil)
	sourceManifest, err := trailconfig.Load(root, "trace-source")
	if err != nil {
		t.Fatal(err)
	}
	sourceRef := sourceManifest.ActiveConfigRef
	sourceTree := sourceSnapshotTree(t, root, "trace-source")

	patch := makePatch(t, sourceTree, func(dir string) {
		writeFile(t, dir, ".paseka/prompts/builder.md", "build it differently\n")
		if err := os.Remove(filepath.Join(dir, ".paseka", "prompts", "obsolete.md")); err != nil {
			t.Fatal(err)
		}
		writeFile(t, dir, ".paseka/prompts/added.md", "added prompt\n")
		writeFile(t, dir, ".paseka/skills/new-skill/SKILL.md", "---\nname: new-skill\ndescription: new\n---\nbody\n")
	})
	writePatchRecord(t, root, "trace-source", "agent-delta", patch)

	preview, err := trailfork.Preview(trailfork.Options{
		Context:       ctx,
		SourceTraceID: "trace-source",
		DeltaForm:     trailfork.DeltaPatch,
		DeltaPatch:    "agent-delta",
	})
	if err != nil {
		t.Fatal(err)
	}
	plan := preview.Plan
	if plan.Delta.Form != trailfork.DeltaPatch || plan.Delta.PatchAgentID != "agent-delta" {
		t.Fatalf("delta = %+v", plan.Delta)
	}
	if plan.Delta.SHA256 == "" || plan.Delta.Bytes != int64(len(patch)) {
		t.Fatalf("patch identity = %+v", plan.Delta)
	}
	if plan.ConfigRef == sourceRef {
		t.Fatal("candidate configuration ref did not change")
	}
	if !containsLine(plan.Delta.RawDiff, "changed .paseka/prompts/builder.md") ||
		!containsLine(plan.Delta.RawDiff, "removed .paseka/prompts/obsolete.md") ||
		!containsLine(plan.Delta.RawDiff, "added .paseka/prompts/added.md") {
		t.Fatalf("raw diff = %v", plan.Delta.RawDiff)
	}
	if !containsLine(plan.Delta.EffectiveDiff, "changed .paseka/prompts/builder.md") {
		t.Fatalf("effective diff = %v", plan.Delta.EffectiveDiff)
	}
	if _, err := os.Stat(filepath.Join(root, ".paseka", "runs", plan.TraceID)); !os.IsNotExist(err) {
		t.Fatalf("preview created trail records: %v", err)
	}
	if _, err := os.Stat(plan.Workspace); !os.IsNotExist(err) {
		t.Fatalf("preview created worktree: %v", err)
	}

	ledger := taskledger.NewMemoryLedger()
	result, err := trailfork.Create(context.Background(), &capturePublisher{}, ledger, trailfork.Options{
		Context:       ctx,
		SourceTraceID: "trace-source",
		DeltaForm:     trailfork.DeltaPatch,
		DeltaPatch:    "agent-delta",
	})
	if err != nil {
		t.Fatal(err)
	}
	created := result.Plan
	if created.ConfigRef == sourceRef || created.SourceConfigRef != sourceRef {
		t.Fatalf("created refs = source %s active %s", created.SourceConfigRef, created.ConfigRef)
	}

	binding, err := trailconfig.Load(root, created.TraceID)
	if err != nil {
		t.Fatal(err)
	}
	if binding.InitialConfigRef != sourceRef || binding.ActiveConfigRef != created.ConfigRef || binding.ConfigRevision != 1 {
		t.Fatalf("binding = %+v", binding)
	}
	if len(binding.Snapshots) != 2 {
		t.Fatalf("binding snapshots = %+v", binding.Snapshots)
	}

	candidateTree := filepath.Join(trailconfig.SnapshotDir(root, created.TraceID), created.ConfigRef, trailconfig.SnapshotTreeDirName)
	if data := readFile(t, candidateTree, ".paseka/prompts/builder.md"); data != "build it differently\n" {
		t.Fatalf("candidate prompt = %q", data)
	}
	if data := readFile(t, candidateTree, ".paseka/prompts/added.md"); data != "added prompt\n" {
		t.Fatalf("added prompt = %q", data)
	}
	if _, err := os.Stat(filepath.Join(candidateTree, ".paseka", "prompts", "obsolete.md")); !os.IsNotExist(err) {
		t.Fatalf("deleted prompt survived in candidate: %v", err)
	}
	baseTree := filepath.Join(trailconfig.SnapshotDir(root, created.TraceID), sourceRef, trailconfig.SnapshotTreeDirName)
	if data := readFile(t, baseTree, ".paseka/prompts/builder.md"); data != "build the thing\n" {
		t.Fatalf("base prompt was rewritten = %q", data)
	}
	if data := readFile(t, baseTree, ".paseka/prompts/obsolete.md"); data != "obsolete\n" {
		t.Fatalf("base deleted prompt = %q", data)
	}

	if created.Delta.Path == "" {
		t.Fatal("delta retained path is missing")
	}
	retained, err := os.ReadFile(created.Delta.Path)
	if err != nil {
		t.Fatal(err)
	}
	if string(retained) != string(patch) || created.Delta.SHA256 != digestHex(patch) {
		t.Fatalf("retained patch does not match evidence")
	}

	lineage, err := trailfork.LoadLineage(root, created.TraceID)
	if err != nil {
		t.Fatal(err)
	}
	if lineage.Delta.PatchAgentID != "agent-delta" || lineage.Delta.Publication == "" || lineage.Delta.ProposalID != "agent-delta" {
		t.Fatalf("lineage delta = %+v", lineage.Delta)
	}
	attempt, ok, err := trailconfig.LatestTaskAttempt(root, created.TraceID, created.Task.TaskID)
	if err != nil || !ok || attempt.ConfigRef != created.ConfigRef {
		t.Fatalf("attempt config = %+v (%v %v)", attempt, ok, err)
	}

	// Later loss of the source proposal and source binding does not rewrite the
	// fork's retained evidence or invalidate its candidate snapshot.
	if err := os.RemoveAll(patches.NewStore(root, "trace-source").Root()); err != nil {
		t.Fatal(err)
	}
	if err := os.RemoveAll(filepath.Join(root, ".paseka", "runs", "trace-source", "config")); err != nil {
		t.Fatal(err)
	}
	if data, err := os.ReadFile(created.Delta.Path); err != nil || string(data) != string(patch) {
		t.Fatalf("retained delta lost with source proposal: %v", err)
	}
	if _, err := trailconfig.BindRef(ctx, created.TraceID, created.ConfigRef, 1); err != nil {
		t.Fatalf("candidate snapshot unavailable: %v", err)
	}
}

func TestPatchDeltaPreviewReportsShadowedFrozenResolution(t *testing.T) {
	root, _ := setupGitRepo(t)
	writeFile(t, root, ".paseka/colony.yaml", "slug: test\ndefaults:\n  energy_budget: 8\nmodel_aliases:\n  fast: vendor/original\n")
	runGit(t, root, "add", ".")
	runGit(t, root, "commit", "-q", "-m", "aliases")

	ctx := colony.Context{ColonyRoot: root, ModelAliases: map[string]string{"fast": "vendor/frozen"}}
	writeSource(t, ctx, "trace-source", "task-1", nil)
	sourceTree := sourceSnapshotTree(t, root, "trace-source")
	patch := makePatch(t, sourceTree, func(dir string) {
		writeFile(t, dir, ".paseka/colony.yaml", "slug: test\ndefaults:\n  energy_budget: 8\nmodel_aliases:\n  fast: vendor/patched\n")
	})
	writePatchRecord(t, root, "trace-source", "agent-alias", patch)

	result, err := trailfork.Preview(trailfork.Options{
		Context:       ctx,
		SourceTraceID: "trace-source",
		DeltaForm:     trailfork.DeltaPatch,
		DeltaPatch:    "agent-alias",
	})
	if err != nil {
		t.Fatal(err)
	}
	plan := result.Plan
	if !containsSubstring(plan.Delta.ShadowedDiff, "model alias fast") {
		t.Fatalf("shadowed diff = %v", plan.Delta.ShadowedDiff)
	}
	for _, line := range plan.Delta.EffectiveDiff {
		if strings.Contains(line, "model alias") {
			t.Fatalf("frozen alias leaked into effective diff: %v", plan.Delta.EffectiveDiff)
		}
	}
	if !containsLine(plan.Delta.RawDiff, "changed .paseka/colony.yaml") {
		t.Fatalf("raw diff = %v", plan.Delta.RawDiff)
	}
}

func TestPatchDeltaRefusesUnsafeChanges(t *testing.T) {
	cases := []struct {
		name   string
		patch  func(t *testing.T, sourceTree string) []byte
		errSub string
	}{
		{
			name: "application source",
			patch: func(t *testing.T, sourceTree string) []byte {
				return makePatch(t, sourceTree, func(dir string) { writeFile(t, dir, "README.md", "source\n") })
			},
			errSub: "may not change README.md",
		},
		{
			name: "runtime data",
			patch: func(t *testing.T, sourceTree string) []byte {
				return makePatch(t, sourceTree, func(dir string) { writeFile(t, dir, ".paseka/runs/evil.txt", "x\n") })
			},
			errSub: "may not change .paseka/runs/evil.txt",
		},
		{
			name: "identity",
			patch: func(t *testing.T, sourceTree string) []byte {
				return makePatch(t, sourceTree, func(dir string) {
					writeFile(t, dir, ".paseka/colony.yaml", "slug: evil\ndefaults:\n  energy_budget: 8\n")
				})
			},
			errSub: "cannot change colony identity",
		},
		{
			name: "energy budget",
			patch: func(t *testing.T, sourceTree string) []byte {
				return makePatch(t, sourceTree, func(dir string) {
					writeFile(t, dir, ".paseka/colony.yaml", "slug: test\ndefaults:\n  energy_budget: 100\n")
				})
			},
			errSub: "cannot change the colony energy budget",
		},
		{
			name: "path escape",
			patch: func(t *testing.T, sourceTree string) []byte {
				return []byte("diff --git a/../escape.txt b/../escape.txt\nnew file mode 100644\n--- /dev/null\n+++ b/../escape.txt\n@@ -0,0 +1 @@\n+escape\n")
			},
			errSub: "escapes the configuration tree",
		},
		{
			name: "conflict",
			patch: func(t *testing.T, sourceTree string) []byte {
				return []byte("diff --git a/.paseka/prompts/builder.md b/.paseka/prompts/builder.md\n--- a/.paseka/prompts/builder.md\n+++ b/.paseka/prompts/builder.md\n@@ -1,1 +1,1 @@\n-does not exist\n+replacement\n")
			},
			errSub: "does not apply",
		},
		{
			name: "broken template reference",
			patch: func(t *testing.T, sourceTree string) []byte {
				return makePatch(t, sourceTree, func(dir string) {
					if err := os.Remove(filepath.Join(dir, ".paseka", "prompts", "builder.md")); err != nil {
						t.Fatal(err)
					}
				})
			},
			errSub: "prompt template \"builder.md\" is missing",
		},
		{
			name: "broken bee reference",
			patch: func(t *testing.T, sourceTree string) []byte {
				return makePatch(t, sourceTree, func(dir string) {
					if err := os.Remove(filepath.Join(dir, ".paseka", "bees", "builder.yaml")); err != nil {
						t.Fatal(err)
					}
				})
			},
			errSub: "candidate has no bee \"builder\"",
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			root, _ := setupGitRepo(t)
			ctx := testContext(root)
			writeSource(t, ctx, "trace-source", "task-1", nil)
			patch := tc.patch(t, sourceSnapshotTree(t, root, "trace-source"))
			writePatchRecord(t, root, "trace-source", "agent-unsafe", patch)

			_, err := trailfork.Preview(trailfork.Options{
				Context:       ctx,
				SourceTraceID: "trace-source",
				DeltaForm:     trailfork.DeltaPatch,
				DeltaPatch:    "agent-unsafe",
			})
			if err == nil || !strings.Contains(err.Error(), tc.errSub) {
				t.Fatalf("error = %v, want %q", err, tc.errSub)
			}
		})
	}
}

func sourceSnapshotTree(t *testing.T, root, traceID string) string {
	t.Helper()
	manifest, err := trailconfig.Load(root, traceID)
	if err != nil {
		t.Fatal(err)
	}
	return filepath.Join(trailconfig.SnapshotDir(root, traceID), manifest.ActiveConfigRef, trailconfig.SnapshotTreeDirName)
}

func writePatchRecord(t *testing.T, root, traceID, agentID string, body []byte) {
	t.Helper()
	store := patches.NewStore(root, traceID)
	if _, err := store.Write(patches.Record{
		AgentID:     agentID,
		Bee:         "builder",
		Kind:        "code.proposal.isolated",
		Publication: patches.PublicationPublished,
		CapturedAt:  time.Date(2026, 10, 23, 8, 0, 0, 0, time.UTC),
		Summary:     "proposal",
	}, body); err != nil {
		t.Fatal(err)
	}
}

func makePatch(t *testing.T, sourceTree string, mutate func(dir string)) []byte {
	t.Helper()
	dir := t.TempDir()
	copyDir(t, sourceTree, dir)
	runGit(t, dir, "init", "-q", "-b", "main")
	runGit(t, dir, "config", "user.email", "test@example.com")
	runGit(t, dir, "config", "user.name", "Test")
	runGit(t, dir, "add", ".")
	runGit(t, dir, "commit", "-q", "-m", "base")
	mutate(dir)
	runGit(t, dir, "add", "-A")
	cmd := exec.Command("git", "-C", dir, "diff", "--cached", "--binary", "--no-color")
	data, err := cmd.Output()
	if err != nil {
		t.Fatalf("git diff: %v", err)
	}
	return data
}

func copyDir(t *testing.T, src, dst string) {
	t.Helper()
	err := filepath.WalkDir(src, func(current string, entry os.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		rel, err := filepath.Rel(src, current)
		if err != nil {
			return err
		}
		if rel == "." {
			return nil
		}
		target := filepath.Join(dst, rel)
		if entry.IsDir() {
			return os.MkdirAll(target, 0o755)
		}
		if !entry.Type().IsRegular() {
			return nil
		}
		info, err := entry.Info()
		if err != nil {
			return err
		}
		data, err := os.ReadFile(current)
		if err != nil {
			return err
		}
		if err := os.MkdirAll(filepath.Dir(target), 0o755); err != nil {
			return err
		}
		return os.WriteFile(target, data, info.Mode().Perm())
	})
	if err != nil {
		t.Fatal(err)
	}
}

func writeFile(t *testing.T, root, rel, content string) {
	t.Helper()
	path := filepath.Join(root, filepath.FromSlash(rel))
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
}

func readFile(t *testing.T, root, rel string) string {
	t.Helper()
	data, err := os.ReadFile(filepath.Join(root, filepath.FromSlash(rel)))
	if err != nil {
		t.Fatal(err)
	}
	return string(data)
}

func containsLine(lines []string, want string) bool {
	for _, line := range lines {
		if line == want {
			return true
		}
	}
	return false
}

func containsSubstring(lines []string, want string) bool {
	for _, line := range lines {
		if strings.Contains(line, want) {
			return true
		}
	}
	return false
}

func digestHex(data []byte) string {
	sum := sha256.Sum256(data)
	return hex.EncodeToString(sum[:])
}
