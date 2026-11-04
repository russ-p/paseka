package opencode

import (
	"context"
	"os"
	"path/filepath"
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

func TestResolveTranscriptMissingV1StoreIsStableOmission(t *testing.T) {
	t.Setenv("XDG_DATA_HOME", t.TempDir())

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
