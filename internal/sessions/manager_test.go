package sessions_test

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/paseka/paseka/internal/adapters"
	"github.com/paseka/paseka/internal/colony"
	"github.com/paseka/paseka/internal/runs"
	"github.com/paseka/paseka/internal/sessions"
)

func TestManagerLaunchRoutesAdapterLocalConfig(t *testing.T) {
	repo := initMixedSessionRepo(t)
	setupMixedSessionHome(t, repo)
	t.Setenv("CURSOR_API_KEY", "cursor-secret")
	t.Setenv("GEMINI_API_KEY", "pi-secret")

	cursorSess := &instantSessionAdapter{name: "cursor"}
	piSess := &instantSessionAdapter{name: "pi"}
	mgr := sessions.NewManager()
	mgr.RegisterSessionAdapter("cursor", cursorSess)
	mgr.RegisterSessionAdapter("pi", piSess)

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	runSession := func(bee string) error {
		done := make(chan error, 1)
		go func() {
			_, err := mgr.RunInteractive(ctx, sessions.RunRequest{
				StartDir: repo,
				Bee:      bee,
				Task:     "hello " + bee,
			})
			done <- err
		}()
		select {
		case err := <-done:
			return err
		case <-time.After(3 * time.Second):
			return fmt.Errorf("timeout waiting for %s session", bee)
		}
	}

	if err := runSession("scout"); err != nil {
		t.Fatal(err)
	}
	if cursorSess.lastReq.Params.Binary != "cursor-agent" {
		t.Fatalf("cursor binary = %q, want cursor-agent", cursorSess.lastReq.Params.Binary)
	}
	if cursorSess.lastReq.Params.APIKey != "cursor-secret" {
		t.Fatalf("cursor api key = %q, want cursor-secret", cursorSess.lastReq.Params.APIKey)
	}

	if err := runSession("analyst"); err != nil {
		t.Fatal(err)
	}
	if piSess.lastReq.Params.Binary != "custom-pi" {
		t.Fatalf("pi binary = %q, want custom-pi", piSess.lastReq.Params.Binary)
	}
	if piSess.lastReq.Params.APIKey != "pi-secret" {
		t.Fatalf("pi api key = %q, want pi-secret", piSess.lastReq.Params.APIKey)
	}
}

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
	name    string
	lastReq adapters.SessionRequest
}

func (f *instantSessionAdapter) Name() string {
	if f.name != "" {
		return f.name
	}
	return "cursor"
}

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

func TestManagerStartDetachedCapturesOutput(t *testing.T) {
	repo := initSessionRepo(t)
	slug := setupSessionHome(t, repo)

	output := &outputSessionAdapter{}
	mgr := sessions.NewManager()
	mgr.RegisterSessionAdapter("cursor", output)

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	res, err := mgr.StartDetached(ctx, sessions.RunRequest{
		StartDir: repo,
		Bee:      "scout",
		Task:     "hello detached",
	})
	if err != nil {
		t.Fatal(err)
	}
	if res.SessionID == "" {
		t.Fatalf("result = %+v", res)
	}
	if output.lastReq.Detached {
		t.Fatal("StartDetached must not mark SessionRequest as headless Detached")
	}

	deadline := time.Now().Add(3 * time.Second)
	for time.Now().Before(deadline) {
		if len(mgr.ListActive()) == 0 {
			break
		}
		time.Sleep(20 * time.Millisecond)
	}

	d := runs.Dir{
		ColonyRoot: repo,
		TraceID:    res.TraceID,
		AgentID:    res.AgentID,
	}
	meta, err := d.ReadSession()
	if err != nil {
		t.Fatal(err)
	}
	if meta.State != string(adapters.SessionCompleted) {
		t.Fatalf("state = %q", meta.State)
	}

	entries, err := d.ReadTranscript()
	if err != nil {
		t.Fatal(err)
	}
	foundAgent := false
	for _, e := range entries {
		if e.Role == "agent" && strings.Contains(e.Content, "hello-detached") {
			foundAgent = true
		}
	}
	if !foundAgent {
		t.Fatalf("expected agent transcript output, got %+v", entries)
	}

	result, err := d.ReadResult()
	if err != nil || result == "" {
		t.Fatalf("result.txt = %q err=%v", result, err)
	}

	st, err := colony.LoadState(slug)
	if err != nil {
		t.Fatal(err)
	}
	if len(st.Sessions) != 0 {
		t.Fatalf("expected no active sessions, got %+v", st.Sessions)
	}
}

func TestManagerStartDetachedIgnoresParentContextCancel(t *testing.T) {
	repo := initSessionRepo(t)
	setupSessionHome(t, repo)

	output := &outputSessionAdapter{script: `sleep 0.1; printf 'parent-cancel\n'; exit 0`}
	mgr := sessions.NewManager()
	mgr.RegisterSessionAdapter("cursor", output)

	ctx, cancel := context.WithCancel(context.Background())
	res, err := mgr.StartDetached(ctx, sessions.RunRequest{
		StartDir: repo,
		Bee:      "scout",
		Task:     "hello detached",
	})
	if err != nil {
		t.Fatal(err)
	}
	cancel()

	deadline := time.Now().Add(3 * time.Second)
	for time.Now().Before(deadline) {
		if len(mgr.ListActive()) == 0 {
			break
		}
		time.Sleep(20 * time.Millisecond)
	}

	d := runs.Dir{
		ColonyRoot: repo,
		TraceID:    res.TraceID,
		AgentID:    res.AgentID,
	}
	meta, err := d.ReadSession()
	if err != nil {
		t.Fatal(err)
	}
	if meta.State != string(adapters.SessionCompleted) {
		t.Fatalf("state = %q", meta.State)
	}
	entries, err := d.ReadTranscript()
	if err != nil {
		t.Fatal(err)
	}
	for _, e := range entries {
		if strings.Contains(e.Content, "pty read error") || strings.Contains(e.Content, "session cancelled") {
			t.Fatalf("unexpected transcript entry after clean exit: %+v", e)
		}
	}
}

type outputSessionAdapter struct {
	lastReq adapters.SessionRequest
	script  string
}

func (o *outputSessionAdapter) Name() string { return "cursor" }

func (o *outputSessionAdapter) SessionCommand(req adapters.SessionRequest) (adapters.SessionCommand, error) {
	o.lastReq = req
	shell, err := exec.LookPath("sh")
	if err != nil {
		return adapters.SessionCommand{}, err
	}
	script := o.script
	if script == "" {
		script = `printf '\033[31mhello-detached\033[0m\n'; sleep 0.05; exit 0`
	}
	return adapters.SessionCommand{
		Binary: shell,
		Args:   []string{"-c", script},
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

func initMixedSessionRepo(t *testing.T) string {
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
	bees := map[string]string{
		"scout.yaml": `role: scout
adapter: cursor
prompt_template: scout.md
`,
		"analyst.yaml": `role: analyst
adapter: pi
prompt_template: analyst.md
`,
	}
	for name, content := range bees {
		if err := os.WriteFile(filepath.Join(paseka, "bees", name), []byte(content), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	for _, prompt := range []struct {
		file, body string
	}{
		{"scout.md", "Scout {{.Task}}\n"},
		{"analyst.md", "Analyst {{.Task}}\n"},
	} {
		if err := os.WriteFile(filepath.Join(paseka, "prompts", prompt.file), []byte(prompt.body), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	return dir
}

func setupMixedSessionHome(t *testing.T, repo string) {
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
	cursorYAML := "binary: cursor-agent\napi_key_env: CURSOR_API_KEY\n"
	if err := os.WriteFile(filepath.Join(homeDir, "adapters", "cursor.yaml"), []byte(cursorYAML), 0o644); err != nil {
		t.Fatal(err)
	}
	piYAML := "binary: custom-pi\napi_key_env: GEMINI_API_KEY\n"
	if err := os.WriteFile(filepath.Join(homeDir, "adapters", "pi.yaml"), []byte(piYAML), 0o644); err != nil {
		t.Fatal(err)
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
