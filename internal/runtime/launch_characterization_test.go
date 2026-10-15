package runtime_test

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/russ-p/paseka/internal/adapters"
	"github.com/russ-p/paseka/internal/adapters/claude"
	"github.com/russ-p/paseka/internal/adapters/cursor"
	"github.com/russ-p/paseka/internal/adapters/opencode"
	"github.com/russ-p/paseka/internal/adapters/pi"
	"github.com/russ-p/paseka/internal/colony"
	"github.com/russ-p/paseka/internal/console"
	"github.com/russ-p/paseka/internal/runtime"
	"github.com/russ-p/paseka/internal/sessions"
)

// Phase 0 characterization baseline for adapter launch resolution
// (docs/specs/038-adapter-launch-resolution.md). These tests pin the observable
// launch contract - resolved binary, full argv, cwd, and child environment - for
// every adapter across every launch mode. They drive the *real* adapters with a
// fake agent binary that records its own process facts, so a behavior-preserving
// refactor of the resolve/merge path fails loudly here.

const (
	launchSlug  = "launch-test"
	launchModel = "test-model"
	launchTask  = "do work"
	// launchAnyArg matches exactly one argv token in a subsequence assertion.
	launchAnyArg = "\x00wildcard"
)

// fakeAgentScript records cwd, argv, and environment to $PASEKA_CAPTURE_FILE and
// stubs the provider-specific probe subcommands (cursor create-chat, opencode
// serve) that interactive session adapters run before building their command.
// The capture is written to a sibling temp file and renamed into place so a
// reader never observes a half-written capture (the test polls for the file
// while the agent may still be appending to it).
const fakeAgentScript = `#!/bin/sh
case "$1" in
  create-chat) echo 11111111-1111-1111-1111-111111111111; exit 0 ;;
  serve) exit 1 ;;
esac
cap="${PASEKA_CAPTURE_FILE:-}"
if [ -n "$cap" ]; then
  tmp="$cap.tmp"
  {
    printf 'cwd=%s\n' "$PWD"
    printf 'bin=%s\n' "$0"
    for a in "$@"; do printf 'arg=%s\n' "$a"; done
    printenv | sort | sed 's/^/env=/'
  } > "$tmp"
  mv "$tmp" "$cap"
fi
printf 'ok\n'
exit 0
`

type procCapture struct {
	Cwd  string
	Bin  string
	Args []string
	Env  map[string]string
}

func parseProcCapture(data string) procCapture {
	pc := procCapture{Env: map[string]string{}}
	for _, line := range strings.Split(data, "\n") {
		switch {
		case strings.HasPrefix(line, "cwd="):
			pc.Cwd = strings.TrimPrefix(line, "cwd=")
		case strings.HasPrefix(line, "bin="):
			pc.Bin = strings.TrimPrefix(line, "bin=")
		case strings.HasPrefix(line, "arg="):
			pc.Args = append(pc.Args, strings.TrimPrefix(line, "arg="))
		case strings.HasPrefix(line, "env="):
			if k, v, ok := strings.Cut(strings.TrimPrefix(line, "env="), "="); ok {
				pc.Env[k] = v
			}
		}
	}
	return pc
}

func envSliceToMap(env []string) map[string]string {
	m := make(map[string]string, len(env))
	for _, e := range env {
		if k, v, ok := strings.Cut(e, "="); ok {
			m[k] = v
		}
	}
	return m
}

// assertArgvSubsequence fails unless want appears in order inside got. The
// launchAnyArg sentinel consumes exactly one got token; extra got tokens are
// allowed so the suite pins the significant contract without freezing volatile
// values (session ids, run dirs, titles).
func assertArgvSubsequence(t *testing.T, mode, adapterName string, got, want []string) {
	t.Helper()
	i := 0
	for _, w := range want {
		if i >= len(got) {
			t.Fatalf("[%s/%s] argv %v missing %q", mode, adapterName, got, w)
		}
		if w == launchAnyArg {
			i++
			continue
		}
		for i < len(got) && got[i] != w {
			i++
		}
		if i >= len(got) {
			t.Fatalf("[%s/%s] argv %v missing %q", mode, adapterName, got, w)
		}
		i++
	}
}

type launchFixture struct {
	repo  string
	fake  string
	ws    string
	trace string
	task  string
	model string
}

func (f launchFixture) promptFor(role string) string {
	return role + ": " + f.task
}

func writeFakeAgent(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	path := filepath.Join(dir, "fake-agent")
	if err := os.WriteFile(path, []byte(fakeAgentScript), 0o755); err != nil {
		t.Fatal(err)
	}
	return path
}

func launchBeeYAML(role, adapter, format string) string {
	return fmt.Sprintf("role: %s\nadapter: %s\nprompt_template: prompt.md\nworktree: false\nparams:\n  model: %s\n  output_format: %s\n",
		role, adapter, launchModel, format)
}

// newLaunchFixture provisions a colony repo plus a machine-local home whose
// adapter binaries resolve to the fake agent, then points XDG_CONFIG_HOME at it.
func newLaunchFixture(t *testing.T) launchFixture {
	t.Helper()
	t.Setenv("PASEKA_PROFILE", "")
	repo := t.TempDir()
	fake := writeFakeAgent(t)

	colonyFiles := map[string]string{
		".paseka/colony.yaml":        "slug: " + launchSlug + "\ndefaults:\n  prompt_template: default.md\n",
		".paseka/bees/cursor.yaml":   launchBeeYAML("cursor", "cursor", "stream-json"),
		".paseka/bees/pi.yaml":       launchBeeYAML("pi", "pi", "json"),
		".paseka/bees/claude.yaml":   launchBeeYAML("claude", "claude", "stream-json"),
		".paseka/bees/opencode.yaml": launchBeeYAML("opencode", "opencode", "json"),
		".paseka/bees/script.yaml":   "role: script\nadapter: script\ncommand: " + fake + " script-arg-1 script-arg-2\nrun_summary: disabled\n",
		".paseka/prompts/prompt.md":  "{{.Bee}}: {{.Task}}",
		".paseka/prompts/default.md": "{{.Task}}",
	}
	for rel, content := range colonyFiles {
		full := filepath.Join(repo, rel)
		if err := os.MkdirAll(filepath.Dir(full), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(full, []byte(content), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	if err := initTestGitRepo(repo); err != nil {
		t.Fatal(err)
	}

	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	homeDir, err := colony.HomeDir(launchSlug)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Join(homeDir, "adapters"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(homeDir, "config.yaml"),
		[]byte("colony_root: "+repo+"\nslug: "+launchSlug+"\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	// The source env vars are deliberately named differently from the vars each
	// adapter injects into the child, so an env assertion can only pass when the
	// adapter actually injects it (a plain inherited value cannot satisfy it).
	adapterFiles := map[string]string{
		"cursor.yaml":   "binary: " + fake + "\napi_key_env: LAUNCH_CURSOR_KEY\n",
		"pi.yaml":       "binary: " + fake + "\napi_key_env: LAUNCH_PI_KEY\n",
		"claude.yaml":   "binary: " + fake + "\napi_key_env: LAUNCH_CLAUDE_KEY\n",
		"opencode.yaml": "binary: " + fake + "\n",
	}
	for name, content := range adapterFiles {
		if err := os.WriteFile(filepath.Join(homeDir, "adapters", name), []byte(content), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	if err := os.WriteFile(filepath.Join(homeDir, "state.json"), []byte("{}\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	t.Setenv("LAUNCH_CURSOR_KEY", "cursor-key")
	t.Setenv("LAUNCH_PI_KEY", "pi-key")
	t.Setenv("LAUNCH_CLAUDE_KEY", "claude-key")

	return launchFixture{repo: repo, fake: fake, ws: repo, trace: "trace-launch", task: launchTask, model: launchModel}
}

func waitForCapture(t *testing.T, path string) procCapture {
	t.Helper()
	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		if data, err := os.ReadFile(path); err == nil {
			if pc := parseProcCapture(string(data)); pc.Bin != "" && pc.Cwd != "" {
				return pc
			}
		}
		time.Sleep(20 * time.Millisecond)
	}
	t.Fatalf("launch capture %s was not written", path)
	return procCapture{}
}

type launchAdapterCase struct {
	name         string
	bee          string
	env          func(f launchFixture) map[string]string
	afkArgs      func(f launchFixture) []string
	sessionArgs  func(f launchFixture) []string // nil when interactive is unsupported
	cwdInherited bool                           // true when the adapter does not chdir to the workspace
}

func launchAdapterCases() []launchAdapterCase {
	return []launchAdapterCase{
		{
			name: "cursor", bee: "cursor", cwdInherited: true,
			env: func(launchFixture) map[string]string { return map[string]string{"CURSOR_API_KEY": "cursor-key"} },
			afkArgs: func(f launchFixture) []string {
				return []string{"-p", "--workspace", f.ws, "--output-format", "stream-json",
					"--trust", "--force", "--model", f.model, "--api-key", "cursor-key", f.promptFor("cursor")}
			},
			sessionArgs: func(f launchFixture) []string {
				return []string{"--workspace", f.ws, "--force", "--model", f.model,
					"--api-key", "cursor-key", f.promptFor("cursor")}
			},
		},
		{
			name: "pi", bee: "pi",
			afkArgs: func(f launchFixture) []string {
				return []string{"-p", "--mode", "json", "--session-dir", launchAnyArg, "--session-id", launchAnyArg,
					"--model", f.model, "--api-key", "pi-key", "--", f.promptFor("pi")}
			},
			sessionArgs: func(f launchFixture) []string {
				return []string{"--session-dir", launchAnyArg, "--session-id", launchAnyArg,
					"--model", f.model, "--api-key", "pi-key", "--", f.promptFor("pi")}
			},
		},
		{
			name: "claude", bee: "claude",
			env: func(launchFixture) map[string]string { return map[string]string{"ANTHROPIC_API_KEY": "claude-key"} },
			afkArgs: func(f launchFixture) []string {
				return []string{"-p", "--output-format", "stream-json", "--verbose",
					"--permission-mode", "bypassPermissions", "--model", f.model, f.promptFor("claude")}
			},
			sessionArgs: func(f launchFixture) []string {
				return []string{"--model", f.model, f.promptFor("claude")}
			},
		},
		{
			name: "opencode", bee: "opencode",
			afkArgs: func(f launchFixture) []string {
				return []string{"run", "--format", "json", "--dir", f.ws, "--auto", "--title", launchAnyArg,
					"--model", f.model, "--", f.promptFor("opencode")}
			},
			sessionArgs: func(f launchFixture) []string {
				return []string{"--model", f.model, "--prompt", f.promptFor("opencode")}
			},
		},
		{
			name: "script", bee: "script",
			env: func(f launchFixture) map[string]string {
				return map[string]string{
					"PASEKA_BEE":         "script",
					"PASEKA_TRACE_ID":    f.trace,
					"PASEKA_WORKSPACE":   f.ws,
					"PASEKA_COLONY_ROOT": f.repo,
				}
			},
			afkArgs: func(f launchFixture) []string {
				return []string{"script-arg-1", "script-arg-2"}
			},
		},
	}
}

func assertLaunchContract(t *testing.T, mode string, ac launchAdapterCase, f launchFixture, pc procCapture, marker string) {
	t.Helper()
	if pc.Bin != f.fake {
		t.Fatalf("[%s/%s] binary = %q, want resolved %q", mode, ac.name, pc.Bin, f.fake)
	}
	want := ac.afkArgs(f)
	if mode == "interactive" {
		want = ac.sessionArgs(f)
	}
	assertArgvSubsequence(t, mode, ac.name, pc.Args, want)

	if mode == "interactive" || !ac.cwdInherited {
		if pc.Cwd != f.ws {
			t.Fatalf("[%s/%s] cwd = %q, want workspace %q", mode, ac.name, pc.Cwd, f.ws)
		}
	} else {
		wd, err := os.Getwd()
		if err != nil {
			t.Fatalf("[%s/%s] os.Getwd: %v", mode, ac.name, err)
		}
		if pc.Cwd != wd {
			t.Fatalf("[%s/%s] cwd = %q, want inherited %q", mode, ac.name, pc.Cwd, wd)
		}
	}

	if got := pc.Env["PASEKA_LAUNCH_MARKER"]; got != marker {
		t.Fatalf("[%s/%s] env PASEKA_LAUNCH_MARKER = %q, want %q", mode, ac.name, got, marker)
	}
	if ac.env != nil {
		for k, v := range ac.env(f) {
			if got := pc.Env[k]; got != v {
				t.Fatalf("[%s/%s] env %s = %q, want %q", mode, ac.name, k, got, v)
			}
		}
	}
}

func TestAdapterLaunchCharacterization(t *testing.T) {
	afkModes := []struct {
		name string
		run  func(t *testing.T, d *runtime.Dispatcher, f launchFixture, bee string)
	}{
		{"afk-dispatch", func(t *testing.T, d *runtime.Dispatcher, f launchFixture, bee string) {
			if _, err := d.BeeRun(context.Background(), runtime.BeeRunRequest{
				StartDir: f.repo, Bee: bee, TraceID: f.trace, Task: f.task, NoBus: true,
			}); err != nil {
				t.Fatalf("BeeRun: %v", err)
			}
		}},
		{"reactor-task", func(t *testing.T, d *runtime.Dispatcher, f launchFixture, bee string) {
			ctxColony, err := colony.ResolveContext(f.repo)
			if err != nil {
				t.Fatalf("resolve context: %v", err)
			}
			if _, err := d.DispatchColonyBee(context.Background(), ctxColony, runtime.ColonyDispatchRequest{
				Bee: bee, TraceID: f.trace, Task: f.task, TaskID: "task-1",
			}, runtime.DispatchModeTask); err != nil {
				t.Fatalf("DispatchColonyBee(task): %v", err)
			}
		}},
		{"reactor-direct", func(t *testing.T, d *runtime.Dispatcher, f launchFixture, bee string) {
			ctxColony, err := colony.ResolveContext(f.repo)
			if err != nil {
				t.Fatalf("resolve context: %v", err)
			}
			if _, err := d.DispatchColonyBee(context.Background(), ctxColony, runtime.ColonyDispatchRequest{
				Bee: bee, TraceID: f.trace, Task: f.task,
			}, runtime.DispatchModeDirect); err != nil {
				t.Fatalf("DispatchColonyBee(direct): %v", err)
			}
		}},
		{"console-run", func(t *testing.T, d *runtime.Dispatcher, f launchFixture, bee string) {
			ctxColony, err := colony.ResolveContext(f.repo)
			if err != nil {
				t.Fatalf("resolve context: %v", err)
			}
			// RunBee answers before the run finishes: it dispatches the adapter
			// on a detached goroutine, so without this signal the test's
			// t.TempDir teardown races the run's artifact writer.
			done := make(chan struct{})
			run := func(ctx context.Context, req runtime.BeeRunRequest) (*runtime.BeeRunResult, error) {
				defer close(done)
				req.NoBus = true
				return d.BeeRun(ctx, req)
			}
			if _, err := console.RunBee(context.Background(), ctxColony, bee,
				console.RunBeeRequest{Body: f.task, TraceID: f.trace}, run); err != nil {
				t.Fatalf("console.RunBee: %v", err)
			}
			<-done
		}},
	}

	for _, mode := range afkModes {
		for _, ac := range launchAdapterCases() {
			ac := ac
			t.Run(mode.name+"/"+ac.name, func(t *testing.T) {
				f := newLaunchFixture(t)
				marker := mode.name + "-" + ac.name
				capPath := filepath.Join(t.TempDir(), "capture.txt")
				t.Setenv("PASEKA_CAPTURE_FILE", capPath)
				t.Setenv("PASEKA_LAUNCH_MARKER", marker)

				d := runtime.NewDispatcher()
				mode.run(t, d, f, ac.bee)

				pc := waitForCapture(t, capPath)
				assertLaunchContract(t, mode.name, ac, f, pc, marker)
			})
		}
	}
}

// capturingSession wraps a real session adapter, records the command it builds,
// and returns a harmless shell command so the PTY manager can actually start and
// finish a process without executing the provider TUI.
type capturingSession struct {
	name  string
	inner adapters.SessionAdapter
	mu    sync.Mutex
	cmd   adapters.SessionCommand
	err   error
}

func (c *capturingSession) Name() string { return c.name }

func (c *capturingSession) SessionCommand(req adapters.SessionRequest) (adapters.SessionCommand, error) {
	cmd, err := c.inner.SessionCommand(req)
	c.mu.Lock()
	c.cmd = cmd
	c.err = err
	c.mu.Unlock()
	if err != nil {
		return adapters.SessionCommand{}, err
	}
	shell, lookErr := exec.LookPath("sh")
	if lookErr != nil {
		return cmd, nil
	}
	return adapters.SessionCommand{
		Binary: shell,
		Args:   []string{"-c", "exit 0"},
		Env:    os.Environ(),
		Dir:    req.Workspace,
	}, nil
}

func (c *capturingSession) recorded() adapters.SessionCommand {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.cmd
}

func realSessionAdapter(name string) adapters.SessionAdapter {
	switch name {
	case "cursor":
		return cursor.NewSession()
	case "pi":
		return pi.NewSession()
	case "claude":
		return claude.NewSession()
	case "opencode":
		return opencode.NewSession()
	}
	return nil
}

func waitSessionsDone(t *testing.T, mgr *sessions.Manager) {
	t.Helper()
	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		if len(mgr.ListActive()) == 0 {
			return
		}
		time.Sleep(20 * time.Millisecond)
	}
	t.Fatal("interactive session did not finish")
}

func TestInteractiveLaunchCharacterization(t *testing.T) {
	for _, ac := range launchAdapterCases() {
		ac := ac
		if ac.sessionArgs == nil {
			t.Run("interactive/"+ac.name, func(t *testing.T) {
				f := newLaunchFixture(t)
				ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
				defer cancel()
				mgr := sessions.NewManager()
				if _, err := mgr.StartDetached(ctx, sessions.RunRequest{
					StartDir: f.repo, Bee: ac.bee, TraceID: f.trace, Task: f.task,
				}); err == nil {
					t.Fatalf("interactive session for %s should be refused", ac.name)
				}
			})
			continue
		}

		t.Run("interactive/"+ac.name, func(t *testing.T) {
			f := newLaunchFixture(t)
			marker := "interactive-" + ac.name
			t.Setenv("PASEKA_LAUNCH_MARKER", marker)

			capturing := &capturingSession{name: ac.name, inner: realSessionAdapter(ac.name)}
			mgr := sessions.NewManager()
			mgr.RegisterSessionAdapter(ac.name, capturing)

			ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
			defer cancel()
			if _, err := mgr.StartDetached(ctx, sessions.RunRequest{
				StartDir: f.repo, Bee: ac.bee, TraceID: f.trace, Task: f.task,
			}); err != nil {
				t.Fatalf("StartDetached: %v", err)
			}

			cmd := capturing.recorded()
			pc := procCapture{Cwd: cmd.Dir, Bin: cmd.Binary, Args: cmd.Args, Env: envSliceToMap(cmd.Env)}
			assertLaunchContract(t, "interactive", ac, f, pc, marker)
			waitSessionsDone(t, mgr)
		})
	}
}
