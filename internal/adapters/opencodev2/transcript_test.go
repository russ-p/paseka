package opencodev2

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/russ-p/paseka/internal/adapters"
)

const exportFixture = `{
  "info": {"id": "ses_export2", "location": {"directory": "/workspace"}},
  "messages": [
    {"type": "system", "text": "system prompt", "time": {"created": 3000}},
    {"info": {"id": "msg_user", "role": "user", "time": {"created": 1000}}, "parts": [{"type": "text", "text": "read main.go"}]},
    {"type": "assistant", "id": "msg_ai", "time": {"created": 2000}, "content": [
      {"type": "reasoning", "text": "need the file"},
      {"type": "text", "text": "reading"},
      {"type": "tool", "id": "call_1", "name": "bash", "state": {"status": "completed", "input": {"command": "go test ./..."}, "content": [{"type": "text", "text": "ok"}]}}
    ]},
    {"type": "idle", "outcome": "succeeded", "time": {"created": 4000}},
    {"type": "user", "text": "thanks", "time": {"created": 5000}},
    {"type": "assistant", "time": {"created": 6000}, "content": [
      {"type": "tool", "id": "call_2", "name": "read", "state": {"status": "error", "input": {"path": "missing"}, "error": {"message": "not found"}}}
    ]}
  ]
}`

// installFakeOpenCode writes a fake provider binary that answers the export
// subcommand with body. When argsPath is non-empty the argv is recorded there,
// so a test can prove which route ran (and that no server was started).
func installFakeOpenCode(t *testing.T, body, argsPath string) string {
	t.Helper()
	dir := t.TempDir()
	fixture := filepath.Join(dir, "export.json")
	if err := os.WriteFile(fixture, []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
	script := "#!/bin/sh\n"
	if argsPath != "" {
		script += "printf '%s\\n' \"$@\" > '" + argsPath + "'\n"
	}
	script += "cat '" + fixture + "'\n"
	path := filepath.Join(dir, defaultBinary)
	if err := os.WriteFile(path, []byte(script), 0o755); err != nil {
		t.Fatal(err)
	}
	// Prepend the fake so the real provider is shadowed, while the script keeps
	// the shell utilities it uses.
	t.Setenv("PATH", dir+string(os.PathListSeparator)+os.Getenv("PATH"))
	return path
}

func TestResolveTranscriptDecodesExportJSON(t *testing.T) {
	argsPath := filepath.Join(t.TempDir(), "args")
	installFakeOpenCode(t, exportFixture, argsPath)

	got, err := New().ResolveTranscript(context.Background(), adapters.TranscriptRequest{
		ProviderSessionID: "ses_export2",
		Adapter:           "opencode",
		Workspace:         "/workspace",
		Include:           adapters.PresetFull(),
	})
	if err != nil {
		t.Fatalf("ResolveTranscript: %v", err)
	}
	if got.Omitted != "" {
		t.Fatalf("Omitted = %q", got.Omitted)
	}

	wantRoles := []string{adapters.TurnRoleSystem, adapters.TurnRoleUser, adapters.TurnRoleAssistant, adapters.TurnRoleUser, adapters.TurnRoleAssistant}
	if len(got.Turns) != len(wantRoles) {
		t.Fatalf("turns = %+v", got.Turns)
	}
	for i, role := range wantRoles {
		if got.Turns[i].Role != role {
			t.Fatalf("turn %d role = %q, want %q", i, got.Turns[i].Role, role)
		}
	}
	// Provider order wins over timestamp order.
	if got.Turns[0].Timestamp.UnixMilli() != 3000 || got.Turns[1].Timestamp.UnixMilli() != 1000 {
		t.Fatalf("timestamps = %v, %v", got.Turns[0].Timestamp, got.Turns[1].Timestamp)
	}

	assistant := got.Turns[2]
	if len(assistant.Parts) != 3 {
		t.Fatalf("assistant parts = %+v", assistant.Parts)
	}
	if assistant.Parts[0].Kind != adapters.PartReasoning || assistant.Parts[0].Reasoning != "need the file" {
		t.Fatalf("reasoning part = %+v", assistant.Parts[0])
	}
	if assistant.Parts[1].Kind != adapters.PartText || assistant.Parts[1].Text != "reading" {
		t.Fatalf("text part = %+v", assistant.Parts[1])
	}
	tool := assistant.Parts[2].Tool
	if tool == nil || tool.Name != "bash" || tool.CallID != "call_1" || tool.Status != adapters.ToolStatusOK ||
		tool.Input != `{"command": "go test ./..."}` || tool.Output != "ok" {
		t.Fatalf("tool part = %+v", tool)
	}

	errorTool := got.Turns[4].Parts[0].Tool
	if errorTool == nil || errorTool.Name != "read" || errorTool.Status != adapters.ToolStatusError || errorTool.Output != "not found" {
		t.Fatalf("error tool = %+v", errorTool)
	}

	args, err := os.ReadFile(argsPath)
	if err != nil {
		t.Fatal(err)
	}
	argv := strings.Fields(string(args))
	want := []string{"session", "export", "ses_export2"}
	if strings.Join(argv, " ") != strings.Join(want, " ") {
		t.Fatalf("argv = %v, want %v", argv, want)
	}
	for _, forbidden := range []string{"serve", "--standalone", "--server"} {
		for _, arg := range argv {
			if arg == forbidden {
				t.Fatalf("export started a server: %v", argv)
			}
		}
	}
}

func TestResolveTranscriptHonorsDialogueMask(t *testing.T) {
	installFakeOpenCode(t, exportFixture, "")

	got, err := New().ResolveTranscript(context.Background(), adapters.TranscriptRequest{
		ProviderSessionID: "ses_export2",
		Include:           adapters.PresetDialogue(),
	})
	if err != nil {
		t.Fatal(err)
	}
	if got.Omitted != "" {
		t.Fatalf("Omitted = %q", got.Omitted)
	}
	for _, turn := range got.Turns {
		if turn.Role == adapters.TurnRoleSystem {
			t.Fatalf("dialogue kept a system turn: %+v", got.Turns)
		}
		for _, part := range turn.Parts {
			if part.Kind == adapters.PartReasoning {
				t.Fatalf("dialogue kept reasoning without asking: %+v", part)
			}
			if part.Kind == adapters.PartTool && part.Tool != nil {
				if part.Tool.Name == "" || part.Tool.Status == "" {
					t.Fatalf("tool identity missing: %+v", part.Tool)
				}
				if part.Tool.Input != "" || part.Tool.Output != "" {
					t.Fatalf("dialogue materialized tool payload: %+v", part.Tool)
				}
			}
		}
	}
}

func TestDecodeExportAcceptsDataEnvelope(t *testing.T) {
	turns, err := decodeExport([]byte(`{"data":` + exportFixture + `}`))
	if err != nil {
		t.Fatal(err)
	}
	if len(turns) != 5 {
		t.Fatalf("turns = %+v", turns)
	}
}

func TestResolveTranscriptUsesOwnedServerAPI(t *testing.T) {
	prev := startSessionServer
	startSessionServer = func(string, string, []string) (sessionServer, error) {
		t.Fatal("transcript resolution must not start a server")
		return nil, nil
	}
	t.Cleanup(func() { startSessionServer = prev })

	var paths []string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		paths = append(paths, r.Method+" "+r.URL.Path)
		user, pass, ok := r.BasicAuth()
		if !ok || user != apiUsername || pass != "secret" {
			t.Errorf("basic auth = %q/%q ok=%v", user, pass, ok)
		}
		_, _ = io.WriteString(w, `{"data":`+exportFixture+`}`)
	}))
	defer server.Close()

	registerOwnedSession("ses_export2", testAPIClient(server))
	t.Cleanup(func() { unregisterOwnedSession("ses_export2") })
	t.Setenv("PATH", t.TempDir())

	got, err := New().ResolveTranscript(context.Background(), adapters.TranscriptRequest{
		ProviderSessionID: "ses_export2",
		Include:           adapters.PresetFull(),
	})
	if err != nil {
		t.Fatal(err)
	}
	if got.Omitted != "" {
		t.Fatalf("Omitted = %q", got.Omitted)
	}
	if len(got.Turns) != 5 {
		t.Fatalf("turns = %+v", got.Turns)
	}
	want := "GET /api/experimental/session/ses_export2/export"
	if len(paths) != 1 || paths[0] != want {
		t.Fatalf("paths = %v, want %s", paths, want)
	}
}

func TestResolveTranscriptDeadlineCutsOffSubprocess(t *testing.T) {
	dir := t.TempDir()
	script := "#!/bin/sh\nexec sleep 30\n"
	if err := os.WriteFile(filepath.Join(dir, defaultBinary), []byte(script), 0o755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", dir+string(os.PathListSeparator)+os.Getenv("PATH"))

	ctx, cancel := context.WithTimeout(context.Background(), 150*time.Millisecond)
	defer cancel()
	start := time.Now()
	got, err := New().ResolveTranscript(ctx, adapters.TranscriptRequest{
		ProviderSessionID: "ses_blocked",
	})
	if err != nil {
		t.Fatal(err)
	}
	if got.Omitted != adapters.SessionLogOmittedTimeout {
		t.Fatalf("Omitted = %q", got.Omitted)
	}
	if elapsed := time.Since(start); elapsed > 5*time.Second {
		t.Fatalf("resolution took %s; the deadline did not cut it off", elapsed)
	}
}

func TestResolveTranscriptDeadlineCutsOffHTTP(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		<-r.Context().Done()
	}))
	defer server.Close()

	registerOwnedSession("ses_blocked_http", testAPIClient(server))
	t.Cleanup(func() { unregisterOwnedSession("ses_blocked_http") })
	t.Setenv("PATH", t.TempDir())

	ctx, cancel := context.WithTimeout(context.Background(), 150*time.Millisecond)
	defer cancel()
	start := time.Now()
	got, err := New().ResolveTranscript(ctx, adapters.TranscriptRequest{
		ProviderSessionID: "ses_blocked_http",
	})
	if err != nil {
		t.Fatal(err)
	}
	if got.Omitted != adapters.SessionLogOmittedTimeout {
		t.Fatalf("Omitted = %q", got.Omitted)
	}
	if elapsed := time.Since(start); elapsed > 5*time.Second {
		t.Fatalf("resolution took %s; the deadline did not cut it off", elapsed)
	}
}

func TestResolveTranscriptMissingBinaryIsOmission(t *testing.T) {
	t.Setenv("PATH", t.TempDir())
	got, err := New().ResolveTranscript(context.Background(), adapters.TranscriptRequest{
		ProviderSessionID: "ses_missing",
	})
	if err != nil {
		t.Fatal(err)
	}
	if got.Omitted != adapters.SessionLogOmittedStoreNotFound {
		t.Fatalf("Omitted = %q", got.Omitted)
	}
}
