package launch_test

import (
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/russ-p/paseka/internal/adapters"
	"github.com/russ-p/paseka/internal/colony"
	"github.com/russ-p/paseka/internal/launch"
	"github.com/russ-p/paseka/internal/runs"
	"gopkg.in/yaml.v3"
)

func parseCommand(t *testing.T, doc string) colony.Command {
	t.Helper()
	var c colony.Command
	if err := yaml.Unmarshal([]byte(doc), &c); err != nil {
		t.Fatalf("parse command %q: %v", doc, err)
	}
	return c
}

func testRunDir() runs.Dir {
	return runs.Dir{ColonyRoot: "/colony", TraceID: "trace-1", AgentID: "agent-1"}
}

func testBee(adapter string) colony.Bee {
	return colony.Bee{
		Role:    adapter,
		Adapter: adapter,
		Params:  map[string]any{"model": "m1"},
	}
}

func testExtra() adapters.RunParamsLayer {
	return adapters.RunParamsLayer{
		Params:   adapters.RunParams{Binary: "/opt/agent", APIKey: "secret"},
		Declared: map[string]bool{"binary": true, "api_key": true},
	}
}

func baseRequest(t *testing.T, adapter string, mode launch.Mode) launch.Request {
	t.Helper()
	bee := testBee(adapter)
	if adapter == "script" {
		bee.Command = parseCommand(t, "[/bin/true]")
	}
	return launch.Request{
		Mode:         mode,
		Adapter:      adapter,
		Bee:          bee,
		Extra:        testExtra(),
		Aliases:      nil,
		Workspace:    "/ws",
		TaskID:       "task-1",
		Prompt:       "do work",
		SystemPrompt: "You are a bee.",
		RunDir:       testRunDir(),
		EnvBase:      []string{"PATH=/bin"},
	}
}

// TestResolveAFKInteractiveParity asserts both launch shapes resolve identical
// binary, params, form, prompt, and environment, so AFK dispatch and interactive
// sessions cannot drift. The only sanctioned difference is cursor's AFK cwd
// inheritance.
func TestResolveAFKInteractiveParity(t *testing.T) {
	for _, adapter := range []string{"cursor", "pi", "claude", "opencode", "opencodev1", "opencodev2", "script"} {
		adapter := adapter
		t.Run(adapter, func(t *testing.T) {
			afk, err := launch.ResolveLaunch(baseRequest(t, adapter, launch.AFK))
			if err != nil {
				t.Fatalf("afk resolve: %v", err)
			}
			interactive, err := launch.ResolveLaunch(baseRequest(t, adapter, launch.Interactive))
			if err != nil {
				t.Fatalf("interactive resolve: %v", err)
			}

			if afk.Adapter != interactive.Adapter {
				t.Fatalf("adapter differs: %q vs %q", afk.Adapter, interactive.Adapter)
			}
			if afk.Binary != interactive.Binary {
				t.Fatalf("binary differs: %q vs %q", afk.Binary, interactive.Binary)
			}
			if afk.Binary != "/opt/agent" {
				t.Fatalf("binary = %q, want merged /opt/agent", afk.Binary)
			}
			if !reflect.DeepEqual(afk.Params, interactive.Params) {
				t.Fatalf("params differ: %+v vs %+v", afk.Params, interactive.Params)
			}
			if afk.Form != interactive.Form {
				t.Fatalf("form differs: %q vs %q", afk.Form, interactive.Form)
			}
			if afk.CommandPrompt != interactive.CommandPrompt {
				t.Fatalf("command prompt differs: %q vs %q", afk.CommandPrompt, interactive.CommandPrompt)
			}
			if !reflect.DeepEqual(afk.Env, interactive.Env) {
				t.Fatalf("env differs:\n afk=%v\n int=%v", afk.Env, interactive.Env)
			}

			wantInteractiveDir := "/ws"
			if interactive.Dir != wantInteractiveDir {
				t.Fatalf("interactive dir = %q, want %q", interactive.Dir, wantInteractiveDir)
			}
			wantAFKDir := "/ws"
			if adapter == "cursor" {
				wantAFKDir = ""
			}
			if afk.Dir != wantAFKDir {
				t.Fatalf("afk dir = %q, want %q", afk.Dir, wantAFKDir)
			}
		})
	}
}

// TestResolveCarriesRawOnlyForAFK pins that the runtime-owned --raw flag is
// carried into the plan for the AFK shape and stays off otherwise.
func TestResolveCarriesRaw(t *testing.T) {
	req := baseRequest(t, "cursor", launch.AFK)
	req.Raw = true
	plan, err := launch.ResolveLaunch(req)
	if err != nil {
		t.Fatal(err)
	}
	if !plan.Raw {
		t.Fatal("plan.Raw = false, want true")
	}

	req.Raw = false
	plan, err = launch.ResolveLaunch(req)
	if err != nil {
		t.Fatal(err)
	}
	if plan.Raw {
		t.Fatal("plan.Raw = true, want false by default")
	}
}

func TestResolveJoinsProviderPrompt(t *testing.T) {
	for _, adapter := range []string{"cursor", "opencode", "opencodev1", "opencodev2"} {
		plan, err := launch.ResolveLaunch(baseRequest(t, adapter, launch.AFK))
		if err != nil {
			t.Fatal(err)
		}
		if plan.CommandPrompt != "You are a bee.\ndo work" {
			t.Fatalf("%s command prompt = %q", adapter, plan.CommandPrompt)
		}
	}
	for _, adapter := range []string{"pi", "claude", "script"} {
		plan, err := launch.ResolveLaunch(baseRequest(t, adapter, launch.AFK))
		if err != nil {
			t.Fatal(err)
		}
		if plan.CommandPrompt != "do work" {
			t.Fatalf("%s command prompt = %q, want task only", adapter, plan.CommandPrompt)
		}
	}
}

func TestResolveResumeSuppressesJoinAndCommand(t *testing.T) {
	req := baseRequest(t, "cursor", launch.Interactive)
	req.Resume = true
	req.Prompt = "keep going"
	req.Bee.Command = parseCommand(t, "[agent, --resume, x]")

	plan, err := launch.ResolveLaunch(req)
	if err != nil {
		t.Fatal(err)
	}
	if plan.CommandPrompt != "keep going" {
		t.Fatalf("resume command prompt = %q", plan.CommandPrompt)
	}
	if plan.Form != launch.FormParams {
		t.Fatalf("resume form = %q, want params", plan.Form)
	}
	if plan.Command != nil {
		t.Fatalf("resume command = %v, want nil", plan.Command)
	}
}

func TestResolveRendersCommandTemplate(t *testing.T) {
	req := baseRequest(t, "cursor", launch.AFK)
	req.Bee.Role = "builder"
	req.Bee.Command = parseCommand(t, "- echo\n- ${TRACE_ID}\n- ${AGENT_ID}\n- ${TASK_ID}\n- ${PROMPT}\n")

	plan, err := launch.ResolveLaunch(req)
	if err != nil {
		t.Fatal(err)
	}
	if plan.Form != launch.FormCommand {
		t.Fatalf("form = %q, want command", plan.Form)
	}
	want := []string{"echo", "trace-1", "agent-1", "task-1", "You are a bee.\ndo work"}
	if !reflect.DeepEqual(plan.Command, want) {
		t.Fatalf("command = %v, want %v", plan.Command, want)
	}
}

// TestResolveCommandTemplateReceivesPlanValues pins that a command-mode bee
// renders its argv from the plan's values — the same result-file path and plan
// env that params mode uses — so custom argv and mapped argv share conventions.
func TestResolveCommandTemplateReceivesPlanValues(t *testing.T) {
	req := baseRequest(t, "script", launch.AFK)
	req.Bee.Command = parseCommand(t, "- /bin/run\n- ${RESULT_FILE}\n- ${PASEKA_RESULT_FILE}\n- ${PASEKA_TRACE_ID}\n")

	plan, err := launch.ResolveLaunch(req)
	if err != nil {
		t.Fatal(err)
	}
	if plan.Form != launch.FormScript {
		t.Fatalf("form = %q, want script", plan.Form)
	}
	wantResult := req.RunDir.ResultPath()
	if !filepath.IsAbs(wantResult) {
		t.Fatalf("test result path not absolute: %q", wantResult)
	}
	want := []string{"/bin/run", wantResult, wantResult, "trace-1"}
	if !reflect.DeepEqual(plan.Command, want) {
		t.Fatalf("command = %v, want %v", plan.Command, want)
	}
}

// TestResolveCommandModeKeepsPlanEnv pins that assembling the plan env is not
// skipped in command mode: the injected credential still reaches the child.
func TestResolveCommandModeKeepsPlanEnv(t *testing.T) {
	req := baseRequest(t, "cursor", launch.AFK)
	req.Bee.Command = parseCommand(t, "- agent\n- -p\n- $PROMPT\n")

	plan, err := launch.ResolveLaunch(req)
	if err != nil {
		t.Fatal(err)
	}
	if plan.Form != launch.FormCommand {
		t.Fatalf("form = %q, want command", plan.Form)
	}
	if env := envMap(plan.Env); env["CURSOR_API_KEY"] != "secret" {
		t.Fatalf("credential missing from command-mode plan env: %v", plan.Env)
	}
}

func TestResolveScriptRequiresCommand(t *testing.T) {
	req := baseRequest(t, "script", launch.AFK)
	req.Bee.Command = colony.Command{}
	if _, err := launch.ResolveLaunch(req); err == nil {
		t.Fatal("script without command should error")
	}
}

func TestResolveScriptEnvLayersPasekaContract(t *testing.T) {
	req := baseRequest(t, "script", launch.AFK)
	req.Bee.Command = parseCommand(t, "[/bin/true]")

	plan, err := launch.ResolveLaunch(req)
	if err != nil {
		t.Fatal(err)
	}
	if plan.Form != launch.FormScript {
		t.Fatalf("form = %q, want script", plan.Form)
	}
	env := envMap(plan.Env)
	if env["PATH"] != "/bin" {
		t.Fatalf("inherited PATH lost: %v", plan.Env)
	}
	want := map[string]string{
		"PASEKA_TRACE_ID":  "trace-1",
		"PASEKA_AGENT_ID":  "agent-1",
		"PASEKA_BEE":       "script",
		"PASEKA_WORKSPACE": "/ws",
	}
	for k, v := range want {
		if env[k] != v {
			t.Fatalf("env %s = %q, want %q", k, env[k], v)
		}
	}
}

func TestResolveCredentialEnvPerAdapter(t *testing.T) {
	cases := map[string]string{
		"cursor":   "CURSOR_API_KEY",
		"claude":   "ANTHROPIC_API_KEY",
		"pi":       "",
		"opencode": "",
	}
	for adapter, key := range cases {
		plan, err := launch.ResolveLaunch(baseRequest(t, adapter, launch.AFK))
		if err != nil {
			t.Fatal(err)
		}
		env := envMap(plan.Env)
		if key == "" {
			for k, v := range env {
				if v == "secret" {
					t.Fatalf("%s leaked credential into %s", adapter, k)
				}
			}
			continue
		}
		if env[key] != "secret" {
			t.Fatalf("%s env %s = %q, want secret", adapter, key, env[key])
		}
	}
}

// TestResolveRecordsCredentialForRedaction pins that the injected credential key
// is recorded on the plan so logs can redact it, and that pi (which carries its
// key on argv) records none.
func TestResolveRecordsCredentialForRedaction(t *testing.T) {
	plan, err := launch.ResolveLaunch(baseRequest(t, "cursor", launch.AFK))
	if err != nil {
		t.Fatal(err)
	}
	if len(plan.RedactedEnv) == 0 || plan.RedactedEnv[0] != "CURSOR_API_KEY" {
		t.Fatalf("cursor RedactedEnv = %v, want [CURSOR_API_KEY]", plan.RedactedEnv)
	}

	pi, err := launch.ResolveLaunch(baseRequest(t, "pi", launch.AFK))
	if err != nil {
		t.Fatal(err)
	}
	for _, key := range pi.RedactedEnv {
		if key == "API_KEY" {
			t.Fatalf("pi must not record an env credential: %v", pi.RedactedEnv)
		}
	}
}

// TestResolveDeclaredEnvPassThrough pins that params.env reaches the child
// environment, both as a literal and as a `$HOST_VAR` reference, and that the
// declared keys are recorded for plan-aware redaction.
func TestResolveDeclaredEnvPassThrough(t *testing.T) {
	req := baseRequest(t, "cursor", launch.AFK)
	req.EnvBase = []string{"PATH=/bin", "HOST_TOKEN=host-secret"}
	req.Bee.Params = map[string]any{
		"model": "m1",
		"env": map[string]any{
			"LITERAL_ONE": "literal-value",
			"INHERITED":   "$HOST_TOKEN",
		},
	}

	plan, err := launch.ResolveLaunch(req)
	if err != nil {
		t.Fatal(err)
	}
	env := envMap(plan.Env)
	if env["LITERAL_ONE"] != "literal-value" {
		t.Fatalf("literal declared env missing: %v", plan.Env)
	}
	if env["INHERITED"] != "host-secret" {
		t.Fatalf("host reference not expanded: %q", env["INHERITED"])
	}
	redacted := strings.Join(plan.RedactedEnv, ",")
	for _, key := range []string{"LITERAL_ONE", "INHERITED", "CURSOR_API_KEY"} {
		if !strings.Contains(redacted, key) {
			t.Fatalf("redacted keys %v missing %s", plan.RedactedEnv, key)
		}
	}
}

// TestResolveStrictEnvStripsUndeclared pins the opt-in env_mode: strict. The
// default keeps undeclared secrets flowing; strict drops them but keeps process
// essentials, declared pass-through, and the injected credential.
func TestResolveStrictEnvStripsUndeclared(t *testing.T) {
	mk := func(mode string) launch.Request {
		req := baseRequest(t, "cursor", launch.AFK)
		req.EnvBase = []string{"PATH=/bin", "HOME=/home/bee", "UNDECLARED_SECRET=s3cr3t"}
		params := map[string]any{
			"model": "m1",
			"env":   map[string]any{"DECLARED": "kept"},
		}
		if mode != "" {
			params["env_mode"] = mode
		}
		req.Bee.Params = params
		return req
	}

	def, err := launch.ResolveLaunch(mk(""))
	if err != nil {
		t.Fatal(err)
	}
	if envMap(def.Env)["UNDECLARED_SECRET"] != "s3cr3t" {
		t.Fatalf("default mode must keep undeclared env: %v", def.Env)
	}

	strict, err := launch.ResolveLaunch(mk("strict"))
	if err != nil {
		t.Fatal(err)
	}
	env := envMap(strict.Env)
	if _, ok := env["UNDECLARED_SECRET"]; ok {
		t.Fatalf("strict mode must strip undeclared secret: %v", strict.Env)
	}
	if env["PATH"] != "/bin" || env["HOME"] != "/home/bee" {
		t.Fatalf("strict mode dropped process essentials: %v", strict.Env)
	}
	if env["DECLARED"] != "kept" {
		t.Fatalf("strict mode dropped declared pass-through: %v", strict.Env)
	}
	if env["CURSOR_API_KEY"] != "secret" {
		t.Fatalf("strict mode dropped injected credential: %v", strict.Env)
	}
}

func envMap(env []string) map[string]string {
	m := make(map[string]string, len(env))
	for _, e := range env {
		if k, v, ok := splitEnv(e); ok {
			m[k] = v
		}
	}
	return m
}

func splitEnv(e string) (string, string, bool) {
	for i := 0; i < len(e); i++ {
		if e[i] == '=' {
			return e[:i], e[i+1:], true
		}
	}
	return "", "", false
}

// TestResolveTimeoutAFKOnly pins params.timeout as an AFK-only process bound: an
// AFK plan carries the duration, and an interactive plan stays unbounded because
// the human is the timeout.
func TestResolveTimeoutAFKOnly(t *testing.T) {
	req := baseRequest(t, "cursor", launch.AFK)
	req.Bee.Params = map[string]any{"model": "m1", "timeout": "2m30s"}

	afk, err := launch.ResolveLaunch(req)
	if err != nil {
		t.Fatal(err)
	}
	if afk.Timeout != 150*time.Second {
		t.Fatalf("afk timeout = %v, want 2m30s", afk.Timeout)
	}

	req.Mode = launch.Interactive
	interactive, err := launch.ResolveLaunch(req)
	if err != nil {
		t.Fatal(err)
	}
	if interactive.Timeout != 0 {
		t.Fatalf("interactive timeout = %v, want unbounded", interactive.Timeout)
	}
}

// TestResolveTimeoutDefaultUnbounded pins that an undeclared timeout leaves the
// AFK run unbounded, preserving today's behavior.
func TestResolveTimeoutDefaultUnbounded(t *testing.T) {
	plan, err := launch.ResolveLaunch(baseRequest(t, "cursor", launch.AFK))
	if err != nil {
		t.Fatal(err)
	}
	if plan.Timeout != 0 {
		t.Fatalf("default timeout = %v, want unbounded", plan.Timeout)
	}
}
