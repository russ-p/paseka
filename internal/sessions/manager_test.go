package sessions_test

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"testing"
	"time"

	"github.com/paseka/paseka/internal/adapters"
	"github.com/paseka/paseka/internal/colony"
	"github.com/paseka/paseka/internal/runs"
	"github.com/paseka/paseka/internal/sessions"
)

func TestManagerLaunchWritesSessionArtifacts(t *testing.T) {
	repo := initSessionRepo(t)
	slug := setupSessionHome(t, repo)

	fast := &instantSessionAdapter{}
	mgr := sessions.NewManager()
	mgr.RegisterSessionAdapter("cursor", fast)

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	// Run launch in goroutine, don't attach - we can't easily without exporting launch.
	// Use RunInteractive with piped stdin - attach will block until PTY closes.
	done := make(chan error, 1)
	go func() {
		_, err := mgr.RunInteractive(ctx, sessions.RunRequest{
			StartDir:     repo,
			Bee:          "scout",
			Task:         "hello session",
			InlinePrompt: "",
		})
		done <- err
	}()

	select {
	case err := <-done:
		if err != nil {
			t.Fatal(err)
		}
	case <-time.After(3 * time.Second):
		t.Fatal("timeout waiting for session")
	}

	if fast.lastReq.Bee != "scout" || fast.lastReq.Workspace != repo {
		t.Fatalf("session req = bee %q workspace %q", fast.lastReq.Bee, fast.lastReq.Workspace)
	}

	runDirs, _ := filepath.Glob(filepath.Join(repo, ".paseka", "runs", "*", "*"))
	if len(runDirs) == 0 {
		t.Fatal("expected run dir")
	}

	d := runs.Dir{
		ColonyRoot: repo,
		TraceID:    filepath.Base(filepath.Dir(runDirs[0])),
		AgentID:    filepath.Base(runDirs[0]),
	}
	sess, err := d.ReadSession()
	if err != nil {
		t.Fatal(err)
	}
	if sess.Bee != "scout" {
		t.Fatalf("bee = %q", sess.Bee)
	}
	if sess.State != string(adapters.SessionCompleted) && sess.State != string(adapters.SessionActive) {
		t.Fatalf("state = %q", sess.State)
	}

	st, err := colony.LoadState(slug)
	if err != nil {
		t.Fatal(err)
	}
	// session should be unregistered after completion
	if len(st.Sessions) != 0 {
		t.Fatalf("expected no active sessions after exit, got %+v", st.Sessions)
	}
}

type instantSessionAdapter struct {
	lastReq adapters.SessionRequest
}

func (f *instantSessionAdapter) Name() string { return "cursor" }

func (f *instantSessionAdapter) SessionCommand(req adapters.SessionRequest) (adapters.SessionCommand, error) {
	f.lastReq = req
	shell, err := exec.LookPath("sh")
	if err != nil {
		return adapters.SessionCommand{}, err
	}
	return adapters.SessionCommand{
		Binary: shell,
		Args:   []string{"-c", "exit 0"},
		Env:    os.Environ(),
		Dir:    req.Workspace,
	}, nil
}

func TestResolveTerminalKind(t *testing.T) {
	if sessions.ResolveTerminalKind("ghostty") != sessions.TerminalGhostty {
		t.Fatal("expected ghostty")
	}
	if sessions.ResolveTerminalKind("default") != sessions.TerminalDefault {
		t.Fatal("expected default")
	}
}

func TestColonySessionRegistry(t *testing.T) {
	slug := "test-slug-" + fmt.Sprintf("%d", time.Now().UnixNano())
	home := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", home)

	if err := os.MkdirAll(filepath.Join(home, "paseka", slug), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(home, "paseka", slug, "state.json"), []byte("{}\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	entry := colony.SessionEntry{
		SessionID: "sess-1",
		TraceID:   "trace-1",
		AgentID:   "sess-1",
		Bee:       "scout",
		PID:       9999,
		StartedAt: time.Now().UTC(),
	}
	if err := colony.RegisterSession(slug, entry); err != nil {
		t.Fatal(err)
	}
	list, err := colony.ListSessions(slug)
	if err != nil || len(list) != 1 {
		t.Fatalf("list = %+v, %v", list, err)
	}
	if err := colony.UnregisterSession(slug, "sess-1"); err != nil {
		t.Fatal(err)
	}
	list, _ = colony.ListSessions(slug)
	if len(list) != 0 {
		t.Fatalf("expected empty list, got %+v", list)
	}
}

func initSessionRepo(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	runGit(t, dir, "init")
	runGit(t, dir, "config", "user.email", "test@test.com")
	runGit(t, dir, "config", "user.name", "test")
	runGit(t, dir, "commit", "--allow-empty", "-m", "init")

	paseka := filepath.Join(dir, ".paseka")
	for _, sub := range []string{"bees", "prompts"} {
		if err := os.MkdirAll(filepath.Join(paseka, sub), 0o755); err != nil {
			t.Fatal(err)
		}
	}
	slug := "session-test"
	if err := os.WriteFile(filepath.Join(paseka, "colony.yaml"), []byte("slug: "+slug+"\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	scoutBee := `role: scout
adapter: cursor
prompt_template: scout.md
`
	if err := os.WriteFile(filepath.Join(paseka, "bees", "scout.yaml"), []byte(scoutBee), 0o644); err != nil {
		t.Fatal(err)
	}
	scoutPrompt := "You are scout for {{.Bee}}.\n\nTask:\n{{.Task}}\n"
	if err := os.WriteFile(filepath.Join(paseka, "prompts", "scout.md"), []byte(scoutPrompt), 0o644); err != nil {
		t.Fatal(err)
	}
	return dir
}

func setupSessionHome(t *testing.T, repo string) string {
	t.Helper()
	slug := "session-test"
	home := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", home)
	homeDir := filepath.Join(home, "paseka", slug)
	if err := os.MkdirAll(filepath.Join(homeDir, "adapters"), 0o755); err != nil {
		t.Fatal(err)
	}
	cfg := fmt.Sprintf("colony_root: %s\nslug: %s\n", repo, slug)
	if err := os.WriteFile(filepath.Join(homeDir, "config.yaml"), []byte(cfg), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(homeDir, "state.json"), []byte("{}\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(homeDir, "adapters", "cursor.yaml"), []byte("binary: agent\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	return slug
}

func runGit(t *testing.T, dir string, args ...string) {
	t.Helper()
	cmd := exec.Command("git", args...)
	cmd.Dir = dir
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("git %v: %v\n%s", args, err, out)
	}
}
