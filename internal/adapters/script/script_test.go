package script_test

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/paseka/paseka/internal/adapters"
	"github.com/paseka/paseka/internal/adapters/script"
)

func initGitRepo(t *testing.T, root string) {
	t.Helper()
	runGit(t, root, "init")
	runGit(t, root, "config", "user.email", "test@example.com")
	runGit(t, root, "config", "user.name", "Test")
	if err := os.WriteFile(filepath.Join(root, "README.md"), []byte("seed\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	runGit(t, root, "add", "README.md")
	runGit(t, root, "commit", "-m", "seed")
}

func runGit(t *testing.T, dir string, args ...string) {
	t.Helper()
	cmd := exec.Command("git", args...)
	cmd.Dir = dir
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("git %s: %v\n%s", strings.Join(args, " "), err, out)
	}
}

func TestScriptAdapterRunSetsEnvAndCompletes(t *testing.T) {
	root := t.TempDir()
	initGitRepo(t, root)

	outPath := filepath.Join(root, "env.out")
	scriptPath := filepath.Join(root, "capture.sh")
	body := "#!/bin/sh\n" +
		"echo \"$PASEKA_TRACE_ID\" > \"" + outPath + "\"\n" +
		"echo \"$PASEKA_AGENT_ID\" >> \"" + outPath + "\"\n" +
		"echo \"$PASEKA_BEE\" >> \"" + outPath + "\"\n" +
		"echo \"$PASEKA_WORKSPACE\" >> \"" + outPath + "\"\n" +
		"echo \"$PASEKA_RUN_DIR\" >> \"" + outPath + "\"\n" +
		"echo done\n"
	if err := os.WriteFile(scriptPath, []byte(body), 0o755); err != nil {
		t.Fatal(err)
	}

	a := script.New()
	result, err := a.Run(context.Background(), adapters.RunRequest{
		Bee:        "oracle-guard",
		ColonyRoot: root,
		Workspace:  root,
		TraceID:    "trace-script-1",
		AgentID:    "agent-script-1",
		Command:    []string{scriptPath},
	})
	if err != nil {
		t.Fatal(err)
	}
	if result.Status != "completed" {
		t.Fatalf("status = %q, want completed", result.Status)
	}
	if result.Summary != "done" {
		t.Fatalf("summary = %q, want done", result.Summary)
	}

	data, err := os.ReadFile(outPath)
	if err != nil {
		t.Fatal(err)
	}
	lines := strings.Split(strings.TrimSpace(string(data)), "\n")
	want := []string{
		"trace-script-1",
		"agent-script-1",
		"oracle-guard",
		root,
		filepath.Join(root, ".paseka", "runs", "trace-script-1", "agent-script-1"),
	}
	if len(lines) != len(want) {
		t.Fatalf("env lines = %v, want %v", lines, want)
	}
	for i := range want {
		if lines[i] != want[i] {
			t.Fatalf("line[%d] = %q, want %q", i, lines[i], want[i])
		}
	}
}

func TestScriptAdapterFailedExit(t *testing.T) {
	root := t.TempDir()
	initGitRepo(t, root)

	scriptPath := filepath.Join(root, "fail.sh")
	if err := os.WriteFile(scriptPath, []byte("#!/bin/sh\nexit 42\n"), 0o755); err != nil {
		t.Fatal(err)
	}

	a := script.New()
	result, err := a.Run(context.Background(), adapters.RunRequest{
		Bee:        "oracle-guard",
		ColonyRoot: root,
		Workspace:  root,
		TraceID:    "trace-fail",
		AgentID:    "agent-fail",
		Command:    []string{scriptPath},
	})
	if err != nil {
		t.Fatal(err)
	}
	if result.Status != "failed" {
		t.Fatalf("status = %q, want failed", result.Status)
	}
	if result.ExitCode != 42 {
		t.Fatalf("exit code = %d, want 42", result.ExitCode)
	}
	if result.Err == nil {
		t.Fatal("expected result.Err")
	}
}

func TestScriptAdapterCapturesGitDiff(t *testing.T) {
	root := t.TempDir()
	initGitRepo(t, root)

	scriptPath := filepath.Join(root, "mutate.sh")
	if err := os.WriteFile(scriptPath, []byte("#!/bin/sh\necho changed >> README.md\n"), 0o755); err != nil {
		t.Fatal(err)
	}

	a := script.New()
	result, err := a.Run(context.Background(), adapters.RunRequest{
		Bee:        "fault-builder",
		ColonyRoot: root,
		Workspace:  root,
		TraceID:    "trace-diff",
		AgentID:    "agent-diff",
		Command:    []string{scriptPath},
	})
	if err != nil {
		t.Fatal(err)
	}
	foundDiff := false
	for _, art := range result.Artifacts {
		if art.Kind == "diff" && strings.Contains(art.Content, "changed") {
			foundDiff = true
			break
		}
	}
	if !foundDiff {
		t.Fatalf("expected diff artifact, got %#v", result.Artifacts)
	}
}

func TestScriptAdapterRequiresCommand(t *testing.T) {
	a := script.New()
	_, err := a.Run(context.Background(), adapters.RunRequest{
		Bee:        "oracle-guard",
		ColonyRoot: t.TempDir(),
		Workspace:  t.TempDir(),
		TraceID:    "trace-1",
		AgentID:    "agent-1",
	})
	if err == nil || !strings.Contains(err.Error(), "command is required") {
		t.Fatalf("err = %v, want command required", err)
	}
}
