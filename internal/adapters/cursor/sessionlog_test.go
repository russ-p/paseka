package cursor

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/paseka/paseka/internal/adapters"
)

const testSessionID = "11111111-1111-1111-1111-111111111111"

func TestResolveSessionLogToolCalls(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	ws := "/tmp/colony"
	body := `{"role":"user","message":{"content":[{"type":"text","text":"hi"}]}}
{"role":"assistant","message":{"content":[{"type":"text","text":"ok"},{"type":"tool_use","name":"Read","id":"call-1","input":{"path":"main.go"}},{"type":"tool_use","name":"Grep","input":{"pattern":"foo","path":"/tmp/x"}}]}}
{"type":"turn_ended","status":"success"}
`
	writeTranscript(t, home, "tmp-colony", testSessionID, body)

	got, err := New().ResolveSessionLog(context.Background(), adapters.SessionLogRequest{
		ProviderSessionID: testSessionID,
		Workspace:         ws,
	})
	if err != nil {
		t.Fatalf("ResolveSessionLog: %v", err)
	}
	if got.Omitted != "" {
		t.Fatalf("Omitted = %q", got.Omitted)
	}
	if len(got.ToolCalls) != 2 {
		t.Fatalf("tool calls = %+v", got.ToolCalls)
	}
	if got.ToolCalls[0].Name != "Read" || got.ToolCalls[0].Args != "main.go" || got.ToolCalls[0].CallID != "call-1" {
		t.Fatalf("Read call = %+v", got.ToolCalls[0])
	}
	if got.ToolCalls[1].Name != "Grep" || got.ToolCalls[1].Args != "/tmp/x" {
		t.Fatalf("Grep call = %+v", got.ToolCalls[1])
	}
}

func TestResolveSessionLogMissing(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	got, err := New().ResolveSessionLog(context.Background(), adapters.SessionLogRequest{
		ProviderSessionID: testSessionID,
		Workspace:         "/tmp/colony",
	})
	if err != nil {
		t.Fatal(err)
	}
	if got.Omitted != adapters.SessionLogOmittedStoreNotFound {
		t.Fatalf("Omitted = %q", got.Omitted)
	}
}

func TestResolveSessionLogUnsafeID(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	secret := filepath.Join(home, ".cursor", "projects", "tmp-colony", "agent-transcripts", "etc", "passwd.jsonl")
	if err := os.MkdirAll(filepath.Dir(secret), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(secret, []byte("{}\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	got, err := New().ResolveSessionLog(context.Background(), adapters.SessionLogRequest{
		ProviderSessionID: "../etc",
		Workspace:         "/tmp/colony",
	})
	if err != nil {
		t.Fatal(err)
	}
	if got.Omitted != adapters.SessionLogOmittedStoreNotFound {
		t.Fatalf("Omitted = %q", got.Omitted)
	}
}

func TestResolveSessionLogGlobsWhenWorkspaceMisses(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	writeTranscript(t, home, "other-project", testSessionID, `{"role":"assistant","message":{"content":[{"type":"tool_use","name":"Read","input":{"path":"glob.go"}}]}}`+"\n")
	got, err := New().ResolveSessionLog(context.Background(), adapters.SessionLogRequest{
		ProviderSessionID: testSessionID,
		Workspace:         "/does/not/exist",
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(got.ToolCalls) != 1 || got.ToolCalls[0].Args != "glob.go" {
		t.Fatalf("glob fallback failed: %+v omitted=%q", got.ToolCalls, got.Omitted)
	}
}

func TestResolveSessionLogPrefersWorkspaceSlug(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	writeTranscript(t, home, "other-project", testSessionID, `{"role":"assistant","message":{"content":[{"type":"tool_use","name":"Other","input":{"path":"other.go"}}]}}`+"\n")
	writeTranscript(t, home, "tmp-colony", testSessionID, `{"role":"assistant","message":{"content":[{"type":"tool_use","name":"Read","input":{"path":"wanted.go"}}]}}`+"\n")

	got, err := New().ResolveSessionLog(context.Background(), adapters.SessionLogRequest{
		ProviderSessionID: testSessionID,
		Workspace:         "/tmp/colony",
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(got.ToolCalls) != 1 || got.ToolCalls[0].Args != "wanted.go" {
		t.Fatalf("preferred slug not used: %+v", got.ToolCalls)
	}
}

func TestResolveSessionLogMalformed(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	writeTranscript(t, home, "tmp-colony", testSessionID, "{not json\n")
	got, err := New().ResolveSessionLog(context.Background(), adapters.SessionLogRequest{
		ProviderSessionID: testSessionID,
		Workspace:         "/tmp/colony",
	})
	if err != nil {
		t.Fatal(err)
	}
	if got.Omitted != adapters.SessionLogOmittedParseError {
		t.Fatalf("Omitted = %q", got.Omitted)
	}
}

func TestResolveSessionLogEmptyConversation(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	writeTranscript(t, home, "tmp-colony", testSessionID, `{"role":"user","message":{"content":[{"type":"text","text":"hi"}]}}`+"\n")
	got, err := New().ResolveSessionLog(context.Background(), adapters.SessionLogRequest{
		ProviderSessionID: testSessionID,
		Workspace:         "/tmp/colony",
	})
	if err != nil {
		t.Fatal(err)
	}
	if got.Omitted != "" {
		t.Fatalf("Omitted = %q", got.Omitted)
	}
	if len(got.ToolCalls) != 0 {
		t.Fatalf("ToolCalls = %+v", got.ToolCalls)
	}
}

func TestProjectSlug(t *testing.T) {
	if got := projectSlug("/home/ruslan/Projects/paseka"); got != "home-ruslan-Projects-paseka" {
		t.Fatalf("slug = %q", got)
	}
}

func writeTranscript(t *testing.T, home, slug, id, body string) {
	t.Helper()
	dir := filepath.Join(home, ".cursor", "projects", slug, "agent-transcripts", id)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, id+".jsonl"), []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
}

func TestSummarizeToolInputJSONFallback(t *testing.T) {
	got := summarizeToolInput([]byte(`{"pattern":"foo","glob":"*.go"}`))
	if !strings.Contains(got, `"pattern":"foo"`) {
		t.Fatalf("args = %q", got)
	}
}
