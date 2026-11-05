package opencode

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/russ-p/paseka/internal/adapters"
)

// TestResolveTranscriptReadsFrozenV1StoreOnV2Binary pins the generation split:
// a session recorded on the 1.x line stays readable when the installed binary
// reports 2.x, and the caller keeps asking for adapter `opencode`.
func TestResolveTranscriptReadsFrozenV1StoreOnV2Binary(t *testing.T) {
	ResetVersionCache()
	t.Cleanup(ResetVersionCache)

	binary := writeProxyFake(t, "2.1.4", filepath.Join(t.TempDir(), "args"))
	implementation, err := New().ResolveImplementation(adapters.RunParams{Binary: binary})
	if err != nil {
		t.Fatal(err)
	}
	if implementation != implementationV2 {
		t.Fatalf("implementation = %q, want %q", implementation, implementationV2)
	}

	xdg := t.TempDir()
	t.Setenv("XDG_DATA_HOME", xdg)
	writeProxyTranscriptFixture(t, filepath.Join(xdg, "opencode", "storage"))

	got, err := New().ResolveTranscript(context.Background(), adapters.TranscriptRequest{
		ProviderSessionID: "ses_frozen",
		Adapter:           "opencode",
		Workspace:         "/tmp/colony",
		Include:           adapters.PresetDialogue(),
	})
	if err != nil {
		t.Fatalf("ResolveTranscript: %v", err)
	}
	if got.Omitted != "" {
		t.Fatalf("Omitted = %q", got.Omitted)
	}
	if len(got.Turns) != 1 || got.Turns[0].Role != adapters.TurnRoleUser || got.Turns[0].Parts[0].Text != "still readable" {
		t.Fatalf("turns = %+v", got.Turns)
	}
}

// TestResolveTranscriptV2ExportThroughProxy keeps one adapter identity: the
// caller asks for `opencode`, the 1.x store has nothing, and the 2.x export
// subcommand answers.
func TestResolveTranscriptV2ExportThroughProxy(t *testing.T) {
	t.Setenv("XDG_DATA_HOME", t.TempDir())
	argsPath := filepath.Join(t.TempDir(), "args")
	installProxyExport(t, `{
	  "info": {"id": "ses_v2", "location": {"directory": "/tmp/colony"}},
	  "messages": [
	    {"type": "user", "text": "read main.go", "time": {"created": 1000}},
	    {"type": "assistant", "time": {"created": 2000}, "content": [
	      {"type": "text", "text": "reading"},
	      {"type": "tool", "id": "call-1", "name": "bash", "state": {"status": "completed", "input": {"command": "go test ./..."}, "content": [{"type": "text", "text": "ok"}]}}
	    ]}
	  ]
	}`, argsPath)

	got, err := New().ResolveTranscript(context.Background(), adapters.TranscriptRequest{
		ProviderSessionID: "ses_v2",
		Adapter:           "opencode",
		Workspace:         "/tmp/colony",
		Include:           adapters.PresetDialogue(),
	})
	if err != nil {
		t.Fatalf("ResolveTranscript: %v", err)
	}
	if got.Omitted != "" {
		t.Fatalf("Omitted = %q", got.Omitted)
	}
	if len(got.Turns) != 2 || got.Turns[0].Role != adapters.TurnRoleUser || got.Turns[1].Role != adapters.TurnRoleAssistant {
		t.Fatalf("turns = %+v", got.Turns)
	}
	if got.Turns[0].Parts[0].Text != "read main.go" || got.Turns[1].Parts[0].Text != "reading" {
		t.Fatalf("turns = %+v", got.Turns)
	}

	args, err := os.ReadFile(argsPath)
	if err != nil {
		t.Fatal(err)
	}
	if argv := strings.Fields(string(args)); strings.Join(argv, " ") != "session export ses_v2" {
		t.Fatalf("argv = %v", argv)
	}
}

// TestResolveTranscriptMissingV1StoreIsStableOmission keeps the missing-store
// answer stable. PATH is isolated so the 2.x route cannot invoke a real
// provider binary from the test host.
func TestResolveTranscriptMissingV1StoreIsStableOmission(t *testing.T) {
	t.Setenv("XDG_DATA_HOME", t.TempDir())
	t.Setenv("PATH", t.TempDir())

	got, err := New().ResolveTranscript(context.Background(), adapters.TranscriptRequest{
		ProviderSessionID: "ses_missing",
		Adapter:           "opencode",
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got.Omitted != adapters.SessionLogOmittedStoreNotFound {
		t.Fatalf("Omitted = %q", got.Omitted)
	}
	if len(got.Turns) != 0 {
		t.Fatalf("turns = %+v", got.Turns)
	}
}

// TestResolveTranscriptSQLiteOnlyStoreIsOmission pins the no-database rule: a
// session whose only local artefact is the provider's SQLite store is not
// opened. With no export and no owned server, the result is a stable omission
// rather than a database read.
func TestResolveTranscriptSQLiteOnlyStoreIsOmission(t *testing.T) {
	xdg := t.TempDir()
	t.Setenv("XDG_DATA_HOME", xdg)
	t.Setenv("PATH", t.TempDir())

	dataDir := filepath.Join(xdg, "opencode")
	if err := os.MkdirAll(dataDir, 0o755); err != nil {
		t.Fatal(err)
	}
	// Invalid SQLite content: a reader that opened and parsed this file would
	// report a parse/resolve failure, not a missing store.
	if err := os.WriteFile(filepath.Join(dataDir, "opencode.db"), []byte("not a database"), 0o644); err != nil {
		t.Fatal(err)
	}

	got, err := New().ResolveTranscript(context.Background(), adapters.TranscriptRequest{
		ProviderSessionID: "ses_sqlite_only",
		Adapter:           "opencode",
		Workspace:         t.TempDir(),
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got.Omitted != adapters.SessionLogOmittedStoreNotFound {
		t.Fatalf("Omitted = %q", got.Omitted)
	}
	if len(got.Turns) != 0 {
		t.Fatalf("turns = %+v", got.Turns)
	}
}

func installProxyExport(t *testing.T, body, argsPath string) {
	t.Helper()
	dir := t.TempDir()
	fixture := filepath.Join(dir, "export.json")
	if err := os.WriteFile(fixture, []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
	script := "#!/bin/sh\n" +
		"printf '%s\\n' \"$@\" > '" + argsPath + "'\n" +
		"cat '" + fixture + "'\n"
	if err := os.WriteFile(filepath.Join(dir, "opencode"), []byte(script), 0o755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", dir+string(os.PathListSeparator)+os.Getenv("PATH"))
}

func writeProxyTranscriptFixture(t *testing.T, root string) {
	t.Helper()
	writeProxyTranscriptFile(t, filepath.Join(root, "session", "project-hash", "ses_frozen.json"),
		`{"id":"ses_frozen","directory":"/tmp/colony"}`)
	writeProxyTranscriptFile(t, filepath.Join(root, "message", "ses_frozen", "msg_user.json"),
		`{"id":"msg_user","sessionID":"ses_frozen","role":"user","time":{"created":1000}}`)
	writeProxyTranscriptFile(t, filepath.Join(root, "part", "msg_user", "prt_0001_text.json"),
		`{"id":"prt_0001_text","type":"text","text":"still readable"}`)
}

func writeProxyTranscriptFile(t *testing.T, path, body string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
}
