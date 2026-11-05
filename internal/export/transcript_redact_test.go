package export

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

const (
	cursorRedactSessionID = "11111111-2222-3333-4444-666666666666"
	cursorPlainSessionID  = "11111111-2222-3333-4444-777777777777"

	githubTextSecret = "ghp_abcdefghijklmnopqrstuvwxyz0123456789"
	bearerTextSecret = "assistantbearersecret123"
	bearerToolSecret = "toolbearersecret1234567890"
)

// writeCursorTranscriptFixture writes a provider-native Cursor transcript under
// the temp HOME. The export resolves it through the real adapter, so the
// redaction under test is the shipped path rather than a stub.
func writeCursorTranscriptFixture(t *testing.T, home, sessionID, body string) {
	t.Helper()
	dir := filepath.Join(home, ".cursor", "projects", "fixture-project", "agent-transcripts", sessionID)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, sessionID+".jsonl"), []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
}

func TestExportTraceTranscriptAndAgentLogRedactCursorSecrets(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	ctx, repo := setupExportColony(t)
	traceID := "trace-redact-secrets"
	writeCompletedRunCfg(t, repo, traceID, time.Now().UTC().Add(-time.Minute), completedRunCfg{
		ProviderSessionID: cursorRedactSessionID,
	})

	body := `{"role":"user","timestamp":"2026-11-01T10:00:00Z","message":{"content":[{"type":"text","text":"please call it with ` + githubTextSecret + ` then read docs/guide/cli.md"}]}}
{"role":"assistant","timestamp":"2026-11-01T10:00:01Z","message":{"content":[{"type":"text","text":"sure, using Authorization: Bearer ` + bearerTextSecret + ` now"},{"type":"tool_use","name":"Bash","id":"call-abc","input":{"command":"curl -H \"Authorization: Bearer ` + bearerToolSecret + `\" https://api.example.test"}}]}}
{"role":"user","message":{"content":[{"type":"tool_result","tool_use_id":"call-abc","content":[{"type":"text","text":"ok"}]}]}}
`
	writeCursorTranscriptFixture(t, home, cursorRedactSessionID, body)

	secrets := []string{githubTextSecret, bearerTextSecret, bearerToolSecret}
	for _, format := range []Format{FormatMarkdown, FormatHTML} {
		exported := exportWithIncludes(t, ctx, traceID, format, nil, "agent-logs", "transcript")

		for _, secret := range secrets {
			if strings.Contains(exported, secret) {
				t.Fatalf("format %s leaked a secret %q:\n%s", format, secret, exported)
			}
		}
		if !strings.Contains(exported, "redacted") {
			t.Fatalf("format %s did not mark any redaction:\n%s", format, exported)
		}
		for _, want := range []string{
			"docs/guide/cli.md",
			"://api.example.test",
			"please call it with",
			"sure, using Authorization: Bearer",
			"call-abc",
		} {
			if !strings.Contains(exported, want) {
				t.Fatalf("format %s lost non-secret content %q:\n%s", format, want, exported)
			}
		}
	}
}

func TestExportTraceTranscriptAndAgentLogWithoutSecretsUnchanged(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	ctx, repo := setupExportColony(t)
	traceID := "trace-redact-clean"
	writeCompletedRunCfg(t, repo, traceID, time.Now().UTC().Add(-time.Minute), completedRunCfg{
		ProviderSessionID: cursorPlainSessionID,
	})

	body := `{"role":"user","timestamp":"2026-11-01T10:00:00Z","message":{"content":[{"type":"text","text":"please call it then read docs/guide/cli.md"}]}}
{"role":"assistant","timestamp":"2026-11-01T10:00:01Z","message":{"content":[{"type":"text","text":"sure, the endpoint is ready now"},{"type":"tool_use","name":"Bash","id":"call-abc","input":{"command":"curl -H \"Accept: application/json\" https://api.example.test"}}]}}
`
	writeCursorTranscriptFixture(t, home, cursorPlainSessionID, body)

	for _, format := range []Format{FormatMarkdown, FormatHTML} {
		exported := exportWithIncludes(t, ctx, traceID, format, nil, "agent-logs", "transcript")
		if strings.Contains(strings.ToLower(exported), "redacted") {
			t.Fatalf("format %s redacted a secret-free fixture:\n%s", format, exported)
		}
		for _, want := range []string{
			"docs/guide/cli.md",
			"https://api.example.test",
			"please call it then read",
			"sure, the endpoint is ready now",
			"call-abc",
		} {
			if !strings.Contains(exported, want) {
				t.Fatalf("format %s did not render %q unchanged:\n%s", format, want, exported)
			}
		}
	}
}
