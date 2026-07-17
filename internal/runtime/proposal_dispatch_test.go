package runtime

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"testing"

	"github.com/paseka/paseka/internal/adapters"
	"github.com/paseka/paseka/internal/colony"
	"github.com/paseka/paseka/internal/gitroot"
	"github.com/paseka/paseka/internal/protocol"
	"github.com/paseka/paseka/internal/taskledger"
	"github.com/paseka/paseka/internal/worktree"
)

func TestWorkspaceForDispatchRootProposalSkipsWorktree(t *testing.T) {
	repo := initProposalDispatchRepo(t)
	ctxColony := colony.Context{ColonyRoot: repo, Slug: "test-colony"}
	manifest, err := colony.LoadColony(repo)
	if err != nil {
		t.Fatal(err)
	}
	// worktree: true on reviewer must not matter for root proposal affinity.
	bee := colony.Bee{Role: "main-guard", Worktree: true}

	workspace, _, err := workspaceForDispatch(ctxColony, manifest, bee, "trace-root", "", string(protocol.MutationCodeProposalRoot))
	if err != nil {
		t.Fatal(err)
	}
	if workspace != repo {
		t.Fatalf("workspace = %q, want colony root %q", workspace, repo)
	}

	wtPath := worktree.Path(repo, "trace-root")
	if gitroot.IsInsideWorkTree(wtPath) {
		t.Fatalf("root proposal dispatch must not create worktree at %q", wtPath)
	}
}

func TestWorkspaceForDispatchIsolatedProposalReusesDirtyWorktree(t *testing.T) {
	repo := initProposalDispatchRepo(t)
	slug := setupProposalDispatchHome(t, repo)
	ctxColony := colony.Context{ColonyRoot: repo, Slug: slug}
	manifest, err := colony.LoadColony(repo)
	if err != nil {
		t.Fatal(err)
	}

	entry, err := worktree.Ensure(worktree.EnsureOptions{
		ColonyRoot: repo,
		TraceID:    "trace-iso",
		Slug:       slug,
	})
	if err != nil {
		t.Fatal(err)
	}
	marker := filepath.Join(entry.Path, "publisher-edit.txt")
	if err := os.WriteFile(marker, []byte("dirty from builder\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	// Reviewer with worktree: false must still land in the trace worktree for isolated proposals.
	bee := colony.Bee{Role: "guard", Worktree: false}
	workspace, _, err := workspaceForDispatch(ctxColony, manifest, bee, "trace-iso", "", string(protocol.MutationCodeProposalIsolated))
	if err != nil {
		t.Fatal(err)
	}
	if workspace != entry.Path {
		t.Fatalf("workspace = %q, want reused worktree %q", workspace, entry.Path)
	}
	if _, err := os.Stat(filepath.Join(workspace, "publisher-edit.txt")); err != nil {
		t.Fatalf("reviewer workspace must see publisher edits on disk: %v", err)
	}
}

func TestWorkspaceForDispatchAliasNormalizesToIsolated(t *testing.T) {
	repo := initProposalDispatchRepo(t)
	slug := setupProposalDispatchHome(t, repo)
	ctxColony := colony.Context{ColonyRoot: repo, Slug: slug}
	manifest, err := colony.LoadColony(repo)
	if err != nil {
		t.Fatal(err)
	}

	bee := colony.Bee{Role: "guard", Worktree: false}
	workspace, _, err := workspaceForDispatch(ctxColony, manifest, bee, "trace-alias", "", string(protocol.MutationCodeProposal))
	if err != nil {
		t.Fatal(err)
	}
	want := worktree.Path(repo, "trace-alias")
	if workspace != want {
		t.Fatalf("workspace = %q, want %q", workspace, want)
	}
	if !gitroot.IsInsideWorkTree(workspace) {
		t.Fatal("alias proposal must ensure isolated worktree")
	}
}

func TestReactorDirectDispatchRootProposalNoWorktree(t *testing.T) {
	repo := initProposalDispatchRepo(t)
	setupProposalDispatchHome(t, repo)

	rootEdit := filepath.Join(repo, "root-only.txt")
	if err := os.WriteFile(rootEdit, []byte("hivewright edit\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	r := newProposalDispatchReactor(t, repo, map[string]colony.Bee{
		"main-guard": {
			Role:     "main-guard",
			Worktree: true,
			Subscribes: []colony.SubscriptionRule{
				{EventRule: colony.EventRule{Type: "MUTATION", Kind: "code.proposal.root"}, Dispatch: colony.DispatchDirect},
			},
		},
	})
	rec := &proposalRecordingAdapter{}
	r.Dispatcher().RegisterAdapter("cursor", rec)

	ev, err := protocol.NewEvent("trace-root", "hivewright-1", 1, protocol.EventMutation, protocol.MutationPayload{
		Kind:      protocol.MutationCodeProposalRoot,
		Workspace: protocol.ProposalWorkspaceRoot,
		Summary:   "retune bees",
		Diff:      "+root",
		TaskID:    "task-root",
	})
	if err != nil {
		t.Fatal(err)
	}
	if err := r.ProcessEvent(context.Background(), ev); err != nil {
		t.Fatal(err)
	}
	if rec.calls != 1 {
		t.Fatalf("adapter calls = %d, want 1", rec.calls)
	}
	if rec.lastReq.Workspace != repo {
		t.Fatalf("adapter workspace = %q, want colony root %q", rec.lastReq.Workspace, repo)
	}
	if _, err := os.Stat(filepath.Join(rec.lastReq.Workspace, "root-only.txt")); err != nil {
		t.Fatalf("reviewer must see root edits on disk: %v", err)
	}
	wtPath := worktree.Path(repo, "trace-root")
	if gitroot.IsInsideWorkTree(wtPath) {
		t.Fatalf("root proposal must not create trace worktree at %q", wtPath)
	}
}

func TestReactorDirectDispatchIsolatedReusesPublisherWorktree(t *testing.T) {
	repo := initProposalDispatchRepo(t)
	slug := setupProposalDispatchHome(t, repo)

	entry, err := worktree.Ensure(worktree.EnsureOptions{
		ColonyRoot: repo,
		TraceID:    "trace-iso",
		Slug:       slug,
	})
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(entry.Path, "builder.txt"), []byte("builder change\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	r := newProposalDispatchReactor(t, repo, map[string]colony.Bee{
		"guard": {
			Role: "guard",
			Subscribes: []colony.SubscriptionRule{
				{EventRule: colony.EventRule{Type: "MUTATION", Kind: "code.proposal.isolated"}, Dispatch: colony.DispatchDirect},
			},
		},
	})
	rec := &proposalRecordingAdapter{}
	r.Dispatcher().RegisterAdapter("cursor", rec)

	ev, err := protocol.NewEvent("trace-iso", "builder-1", 1, protocol.EventMutation, protocol.MutationPayload{
		Kind:         protocol.MutationCodeProposalIsolated,
		Workspace:    protocol.ProposalWorkspaceIsolated,
		WorktreePath: ".paseka/worktrees/trace-iso",
		Summary:      "add endpoint",
		Diff:         "+builder",
		TaskID:       "task-iso",
	})
	if err != nil {
		t.Fatal(err)
	}
	if err := r.ProcessEvent(context.Background(), ev); err != nil {
		t.Fatal(err)
	}
	if rec.calls != 1 {
		t.Fatalf("adapter calls = %d, want 1", rec.calls)
	}
	if rec.lastReq.Workspace != entry.Path {
		t.Fatalf("adapter workspace = %q, want reused worktree %q", rec.lastReq.Workspace, entry.Path)
	}
	if _, err := os.Stat(filepath.Join(rec.lastReq.Workspace, "builder.txt")); err != nil {
		t.Fatalf("guard must see builder edits on disk: %v", err)
	}
}

func TestDirectDispatchPerEventAllProposalKinds(t *testing.T) {
	for _, kind := range []string{
		string(protocol.MutationCodeProposal),
		string(protocol.MutationCodeProposalIsolated),
		string(protocol.MutationCodeProposalRoot),
	} {
		if !directDispatchPerEvent(protocol.EventMutation, kind) {
			t.Fatalf("directDispatchPerEvent should key by event identity for %q", kind)
		}
	}
}

type proposalRecordingAdapter struct {
	lastReq adapters.RunRequest
	calls   int
}

func (r *proposalRecordingAdapter) Name() string { return "cursor" }

func (r *proposalRecordingAdapter) Run(_ context.Context, req adapters.RunRequest) (*adapters.RunResult, error) {
	r.calls++
	r.lastReq = req
	return &adapters.RunResult{Status: "completed", Output: "ok"}, nil
}

func initProposalDispatchRepo(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	runGitProposal(t, dir, "init")
	runGitProposal(t, dir, "config", "user.email", "test@test.com")
	runGitProposal(t, dir, "config", "user.name", "test")

	colonyFiles := map[string]string{
		".paseka/colony.yaml": `slug: test-colony
defaults:
  prompt_template: default.md
`,
		".paseka/prompts/default.md": `{{.Task}}`,
	}
	for path, content := range colonyFiles {
		full := filepath.Join(dir, path)
		if err := os.MkdirAll(filepath.Dir(full), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(full, []byte(content), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	if err := os.WriteFile(filepath.Join(dir, "README.md"), []byte("# test\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	runGitProposal(t, dir, "add", ".")
	runGitProposal(t, dir, "commit", "-m", "init")
	return dir
}

func setupProposalDispatchHome(t *testing.T, repo string) string {
	t.Helper()
	base := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", base)
	slug := "test-colony"

	homeDir, err := colony.HomeDir(slug)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Join(homeDir, "adapters"), 0o755); err != nil {
		t.Fatal(err)
	}
	cfg := []byte("colony_root: " + `"` + repo + `"` + "\nslug: " + `"` + slug + `"` + "\n")
	if err := os.WriteFile(filepath.Join(homeDir, "config.yaml"), cfg, 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(homeDir, "adapters", "cursor.yaml"), []byte("binary: agent\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(homeDir, "state.json"), []byte("{}\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	return slug
}

func newProposalDispatchReactor(t *testing.T, root string, bees map[string]colony.Bee) *Reactor {
	t.Helper()
	if err := writeProposalDispatchColony(root, bees); err != nil {
		t.Fatal(err)
	}
	d := NewDispatcher()
	reg := NewBeeRegistryFromBees(bees)
	d.SetBeeRegistry(reg)
	return NewTestReactor(TestReactorOptions{
		ColonyRoot: root,
		Dispatcher: d,
		Registry:   reg,
		Ledger:     taskledger.NewMemoryLedger(),
	})
}

func writeProposalDispatchColony(root string, bees map[string]colony.Bee) error {
	if err := os.MkdirAll(filepath.Join(root, ".paseka/bees"), 0o755); err != nil {
		return err
	}
	for role, bee := range bees {
		content := "role: " + role + "\nadapter: cursor\nprompt_template: default.md\n"
		if bee.Worktree {
			content += "worktree: true\n"
		}
		if len(bee.Subscribes) > 0 {
			content += "subscribes:\n"
			for _, sub := range bee.Subscribes {
				content += "  - type: " + sub.Type + "\n"
				if sub.Kind != "" {
					content += "    kind: " + sub.Kind + "\n"
				}
				if sub.Dispatch != "" {
					content += "    dispatch: " + string(sub.Dispatch) + "\n"
				}
			}
		}
		if err := os.WriteFile(filepath.Join(root, ".paseka/bees", role+".yaml"), []byte(content), 0o644); err != nil {
			return err
		}
	}
	return nil
}

func runGitProposal(t *testing.T, dir string, args ...string) {
	t.Helper()
	cmd := exec.Command("git", append([]string{"-C", dir}, args...)...)
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("git %v: %v\n%s", args, err, out)
	}
}
