package opencodev2

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/russ-p/paseka/internal/adapters"
)

type stubSessionServer struct {
	url    string
	api    *apiClient
	closed atomic.Int32
	output string
}

func (s *stubSessionServer) URL() string        { return s.url }
func (s *stubSessionServer) client() *apiClient { return s.api }
func (s *stubSessionServer) Close()             { s.closed.Add(1) }
func (s *stubSessionServer) outputText() string { return s.output }

func stubStartSessionServer(t *testing.T, srv *stubSessionServer) {
	t.Helper()
	prev := startSessionServer
	startSessionServer = func(string, string, []string) (sessionServer, error) { return srv, nil }
	t.Cleanup(func() { startSessionServer = prev })
}

func fakeOpenCode(t *testing.T) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "opencode")
	if err := os.WriteFile(path, []byte("#!/bin/sh\nexit 0\n"), 0o755); err != nil {
		t.Fatal(err)
	}
	return path
}

func TestSessionCommandCreatesProviderSession(t *testing.T) {
	var created createSessionBody
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost || r.URL.Path != "/api/session" {
			t.Errorf("unexpected request %s %s", r.Method, r.URL.Path)
		}
		if err := json.NewDecoder(r.Body).Decode(&created); err != nil {
			t.Errorf("decode body: %v", err)
		}
		_, _ = io.WriteString(w, `{"data":{"id":"`+created.ID+`","location":"`+created.Location+`"}}`)
	}))
	defer server.Close()

	stub := &stubSessionServer{url: "http://127.0.0.1:9999", api: testAPIClient(server)}
	stubStartSessionServer(t, stub)
	binary := fakeOpenCode(t)

	cmd, err := NewSession().SessionCommand(adapters.SessionRequest{
		AdapterName:    "opencode",
		Implementation: adapterName,
		Bee:            "scout",
		AgentID:        "agent-1",
		Workspace:      t.TempDir(),
		Binary:         binary,
		Params: adapters.RunParams{
			Model:     "anthropic/claude-sonnet-4",
			Reasoning: "high",
			Plan:      true,
		},
		InitialPrompt: "do work",
	})
	if err != nil {
		t.Fatal(err)
	}
	if !strings.HasPrefix(created.ID, sessionIDPrefix) || len(created.ID) != len(sessionIDPrefix)+sessionIDLength {
		t.Fatalf("caller session id = %q", created.ID)
	}
	if created.Location != cmd.Dir || created.Title != "agent-1" || created.Agent != "plan" {
		t.Fatalf("create body = %+v", created)
	}
	if created.Model == nil || created.Model.ProviderID != "anthropic" || created.Model.ModelID != "claude-sonnet-4" || created.Model.Variant != "high" {
		t.Fatalf("model = %+v", created.Model)
	}
	if cmd.ProviderSessionID != created.ID {
		t.Fatalf("provider session = %q want %q", cmd.ProviderSessionID, created.ID)
	}
	if cmd.Implementation != adapterName {
		t.Fatalf("implementation = %q", cmd.Implementation)
	}
	want := []string{"--server", stub.url, "--session", created.ID, "--auto", "--prompt", "do work"}
	if strings.Join(cmd.Args, " ") != strings.Join(want, " ") {
		t.Fatalf("argv = %v, want %v", cmd.Args, want)
	}
	for _, forbidden := range []string{"--model", "--agent", "--variant", "--port", "--hostname"} {
		if containsArg(cmd.Args, forbidden) {
			t.Fatalf("argv contains forbidden %q: %v", forbidden, cmd.Args)
		}
	}
	if cmd.Cleanup == nil {
		t.Fatal("cleanup missing")
	}
	cmd.Cleanup()
	if stub.closed.Load() != 1 {
		t.Fatalf("server close count = %d", stub.closed.Load())
	}
	if !envHas(cmd.Env, "OPENCODE_SERVER_PASSWORD=secret") || !envHas(cmd.Env, "OPENCODE_SERVER_USERNAME=opencode") {
		t.Fatalf("server credentials missing from env: %v", cmd.Env)
	}
}

func TestSessionCommandResumeSkipsCreate(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		t.Errorf("resume must not create a session: %s %s", r.Method, r.URL.Path)
	}))
	defer server.Close()

	stub := &stubSessionServer{url: "http://127.0.0.1:9999", api: testAPIClient(server)}
	stubStartSessionServer(t, stub)

	cmd, err := NewSession().SessionCommand(adapters.SessionRequest{
		Workspace:       t.TempDir(),
		Binary:          fakeOpenCode(t),
		ResumeSessionID: "ses_existing",
		InitialPrompt:   "continue",
	})
	if err != nil {
		t.Fatal(err)
	}
	if cmd.ProviderSessionID != "ses_existing" {
		t.Fatalf("provider session = %q", cmd.ProviderSessionID)
	}
	want := []string{"--server", stub.url, "--session", "ses_existing", "--auto", "--prompt", "continue"}
	if strings.Join(cmd.Args, " ") != strings.Join(want, " ") {
		t.Fatalf("argv = %v, want %v", cmd.Args, want)
	}
}

func TestSessionCommandResumeEmptyContinueOmitsPrompt(t *testing.T) {
	stub := &stubSessionServer{url: "http://127.0.0.1:9999", api: newAPIClient(1, "x")}
	stubStartSessionServer(t, stub)

	cmd, err := NewSession().SessionCommand(adapters.SessionRequest{
		Workspace:       t.TempDir(),
		Binary:          fakeOpenCode(t),
		ResumeSessionID: "ses_existing",
	})
	if err != nil {
		t.Fatal(err)
	}
	if containsArg(cmd.Args, "--prompt") {
		t.Fatalf("argv = %v, want no prompt", cmd.Args)
	}
}

func TestSessionCommandOverrideRecordsExplicitSession(t *testing.T) {
	prev := startSessionServer
	startSessionServer = func(string, string, []string) (sessionServer, error) {
		t.Fatal("command override must not start a server")
		return nil, nil
	}
	t.Cleanup(func() { startSessionServer = prev })

	binary := fakeOpenCode(t)
	cmd, err := NewSession().SessionCommand(adapters.SessionRequest{
		Workspace:     t.TempDir(),
		InitialPrompt: "hi",
		Command:       []string{binary, "--session", "ses_argv", "--prompt", "hi"},
	})
	if err != nil {
		t.Fatal(err)
	}
	if cmd.ProviderSessionID != "ses_argv" {
		t.Fatalf("provider session = %q", cmd.ProviderSessionID)
	}
	if strings.Join(cmd.Args, " ") != "--session ses_argv --prompt hi" {
		t.Fatalf("argv = %v", cmd.Args)
	}
	if cmd.Cleanup != nil {
		t.Fatal("command override should not install cleanup")
	}
}

func TestSessionCommandRequiresPrompt(t *testing.T) {
	_, err := NewSession().SessionCommand(adapters.SessionRequest{Workspace: t.TempDir()})
	if err == nil || !strings.Contains(err.Error(), "prompt") {
		t.Fatalf("error = %v", err)
	}
}

func TestSessionCommandRequiresWorkspace(t *testing.T) {
	_, err := NewSession().SessionCommand(adapters.SessionRequest{InitialPrompt: "hi"})
	if err == nil || !strings.Contains(err.Error(), "workspace") {
		t.Fatalf("error = %v", err)
	}
}

func envHas(env []string, want string) bool {
	for _, entry := range env {
		if entry == want {
			return true
		}
	}
	return false
}
