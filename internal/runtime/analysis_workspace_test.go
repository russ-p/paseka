package runtime

import (
	"os"
	"os/exec"
	"path/filepath"
	"testing"

	"github.com/russ-p/paseka/internal/colony"
	"github.com/russ-p/paseka/internal/protocol"
)

// TestAnalysisWorkspaceDoesNotRecreateRemovedWorktree pins the delivery
// independence of an analysis run: ordinary work uses the trace worktree, while
// analysis keeps the colony root so it can still start after a merge or PR
// reconcile removed that worktree.
func TestAnalysisWorkspaceDoesNotRecreateRemovedWorktree(t *testing.T) {
	root := t.TempDir()
	base := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", base)
	initAnalysisRepo(t, root)
	homeDir, err := colony.HomeDir("test")
	if err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(homeDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Join(root, ".paseka"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, ".paseka", "colony.yaml"), []byte("defaults: {}\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	manifest, err := colony.LoadColony(root)
	if err != nil {
		t.Fatal(err)
	}
	ctxColony := colony.Context{ColonyRoot: root, Slug: "test"}
	bee := colony.Bee{Role: "builder", Worktree: true}
	traceID := "trace-analysis-workspace"

	workWorkspace, _, err := workspaceForDispatchPurpose(ctxColony, manifest, bee, traceID, "", "", protocol.PurposeWork)
	if err != nil {
		t.Fatal(err)
	}
	worktreePath := filepath.Join(root, ".paseka", "worktrees", traceID)
	if workWorkspace != worktreePath {
		t.Fatalf("work workspace = %q, want %q", workWorkspace, worktreePath)
	}
	if err := os.RemoveAll(worktreePath); err != nil {
		t.Fatal(err)
	}

	analysisWorkspace, _, err := workspaceForDispatchPurpose(ctxColony, manifest, bee, traceID, "", "", protocol.PurposeAnalysis)
	if err != nil {
		t.Fatal(err)
	}
	if analysisWorkspace != root {
		t.Fatalf("analysis workspace = %q, want the colony root %q", analysisWorkspace, root)
	}
	if _, err := os.Stat(worktreePath); !os.IsNotExist(err) {
		t.Fatalf("analysis recreated the removable worktree: %v", err)
	}
}

func initAnalysisRepo(t *testing.T, root string) {
	t.Helper()
	run := func(args ...string) {
		cmd := exec.Command("git", append([]string{"-C", root}, args...)...)
		if out, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("git %v: %v\n%s", args, err, out)
		}
	}
	run("init")
	run("config", "user.email", "test@test.com")
	run("config", "user.name", "test")
	if err := os.WriteFile(filepath.Join(root, "README.md"), []byte("# test\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	run("add", "README.md")
	run("commit", "-m", "init")
}
