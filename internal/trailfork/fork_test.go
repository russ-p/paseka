package trailfork_test

import (
	"context"
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/russ-p/paseka/internal/colony"
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
