package cursor

import (
	"context"
	"io/fs"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/russ-p/paseka/internal/adapters"
)

const testSessionID = "11111111-1111-1111-1111-111111111111"

const testTranscriptBody = `{"role":"system","message":{"content":[{"type":"text","text":"system prompt"}]}}
{"role":"user","timestamp":"2026-11-01T10:00:00Z","message":{"content":[{"type":"text","text":"hi"}]}}
{"role":"assistant","timestamp":"2026-11-01T10:00:01Z","message":{"content":[{"type":"text","text":"ok"},{"type":"tool_use","name":"Read","id":"call-1","input":{"path":"main.go"}},{"type":"tool_use","name":"Grep","input":{"pattern":"foo","path":"/tmp/x"}}]}}
{"role":"user","message":{"content":[{"type":"tool_result","tool_use_id":"call-1","content":[{"type":"text","text":"package main"}]}]}}
{"type":"turn_ended","status":"success"}
`

func TestResolveTranscriptToolCallPreset(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	writeTranscript(t, home, "tmp-colony", testSessionID, testTranscriptBody)

	got, err := New().ResolveTranscript(context.Background(), adapters.TranscriptRequest{
		ProviderSessionID: testSessionID,
		Workspace:         "/tmp/colony",
		Include:           adapters.PresetToolCall(),
	})
	if err != nil {
		t.Fatalf("ResolveTranscript: %v", err)
	}
	if got.Omitted != "" {
		t.Fatalf("Omitted = %q", got.Omitted)
	}

	// Text parts are not requested, so only the assistant tool turn survives.
	if len(got.Turns) != 1 || got.Turns[0].Role != adapters.TurnRoleAssistant {
		t.Fatalf("turns = %+v", got.Turns)
	}
	calls := adapters.ProjectToolCalls(got)
	if len(calls) != 2 {
		t.Fatalf("tool calls = %+v", calls)
	}
	if calls[0].Name != "Read" || calls[0].Args != "main.go" || calls[0].CallID != "call-1" {
		t.Fatalf("Read call = %+v", calls[0])
	}
	if calls[1].Name != "Grep" || calls[1].Args != "/tmp/x" {
		t.Fatalf("Grep call = %+v", calls[1])
	}
	for _, turn := range got.Turns {
		for _, part := range turn.Parts {
			if part.Kind == adapters.PartText {
				t.Fatalf("tool-call preset populated text: %+v", part)
			}
			if part.Tool != nil && part.Tool.Output != "" {
				t.Fatalf("tool-call preset populated tool output: %+v", part.Tool)
			}
		}
	}
}

func TestResolveTranscriptDialoguePreset(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	writeTranscript(t, home, "tmp-colony", testSessionID, testTranscriptBody)

	got, err := New().ResolveTranscript(context.Background(), adapters.TranscriptRequest{
		ProviderSessionID: testSessionID,
		Workspace:         "/tmp/colony",
		Include:           adapters.PresetDialogue(),
	})
	if err != nil {
		t.Fatal(err)
	}
	if got.Omitted != "" {
		t.Fatalf("Omitted = %q", got.Omitted)
	}
	if len(got.Turns) != 2 {
		t.Fatalf("turns = %+v", got.Turns)
	}
	if got.Turns[0].Role != adapters.TurnRoleUser || got.Turns[0].Parts[0].Text != "hi" {
		t.Fatalf("first turn = %+v", got.Turns[0])
	}
	if got.Turns[0].Timestamp.IsZero() {
		t.Fatal("expected provider timestamp on first turn")
	}
	for _, turn := range got.Turns {
		for _, part := range turn.Parts {
			if part.Tool == nil {
				continue
			}
			if part.Tool.Name == "" || part.Tool.Status == "" {
				t.Fatalf("tool identity missing: %+v", part.Tool)
			}
			if part.Tool.Input != "" || part.Tool.Output != "" {
				t.Fatalf("dialogue preset populated tool payload: %+v", part.Tool)
			}
		}
	}
	call := findTool(t, got, "Read")
	if call.CallID != "call-1" {
		t.Fatalf("call id = %q, want call-1", call.CallID)
	}
}

func TestResolveTranscriptFullPreset(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	writeTranscript(t, home, "tmp-colony", testSessionID, testTranscriptBody)

	got, err := New().ResolveTranscript(context.Background(), adapters.TranscriptRequest{
		ProviderSessionID: testSessionID,
		Workspace:         "/tmp/colony",
		Include:           adapters.PresetFull(),
	})
	if err != nil {
		t.Fatal(err)
	}
	if got.Omitted != "" {
		t.Fatalf("full preset must not omit: %q", got.Omitted)
	}
	if len(got.Turns) != 3 || got.Turns[0].Role != adapters.TurnRoleSystem {
		t.Fatalf("full preset must include system turns: %+v", got.Turns)
	}

	// Cursor records no reasoning parts: a reasoning request answers with an
	// empty set, not an omission.
	for _, turn := range got.Turns {
		for _, part := range turn.Parts {
			if part.Kind == adapters.PartReasoning {
				t.Fatalf("cursor must not report reasoning: %+v", part)
			}
		}
	}

	call := findTool(t, got, "Read")
	if call.Input == "" || call.Output != "package main" {
		t.Fatalf("full preset tool part = %+v", call)
	}
	if call.Status != adapters.ToolStatusOK {
		t.Fatalf("attached result status = %q, want %q", call.Status, adapters.ToolStatusOK)
	}
	unanswered := findTool(t, got, "Grep")
	if unanswered.Status != adapters.ToolStatusUnknown {
		t.Fatalf("unanswered tool status = %q, want %q", unanswered.Status, adapters.ToolStatusUnknown)
	}
}

// TestResolveTranscriptReasoningPresetIsEmptyNotOmitted pins that Cursor
// answers the reasoning include with an empty reasoning set, not an omission.
func TestResolveTranscriptReasoningPresetIsEmptyNotOmitted(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	writeTranscript(t, home, "tmp-colony", testSessionID, testTranscriptBody)

	got, err := New().ResolveTranscript(context.Background(), adapters.TranscriptRequest{
		ProviderSessionID: testSessionID,
		Workspace:         "/tmp/colony",
		Include:           adapters.PresetDialogueWithReasoning(),
	})
	if err != nil {
		t.Fatal(err)
	}
	if got.Omitted != "" {
		t.Fatalf("reasoning request must not omit: %q", got.Omitted)
	}
	if len(got.Turns) == 0 {
		t.Fatal("expected dialogue turns")
	}
	for _, turn := range got.Turns {
		for _, part := range turn.Parts {
			if part.Kind == adapters.PartReasoning {
				t.Fatalf("cursor must answer reasoning with an empty set: %+v", part)
			}
		}
	}
}

// TestResolveTranscriptAttachesNestedToolResult pins the other provider shape:
// when the result is nested on the call, consumers still see one tool part.
func TestResolveTranscriptAttachesNestedToolResult(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	body := `{"role":"assistant","message":{"content":[{"type":"tool_use","name":"Read","id":"call-2","input":{"path":"b.go"},"output":"package b"}]}}` + "\n"
	writeTranscript(t, home, "tmp-colony", testSessionID, body)

	got, err := New().ResolveTranscript(context.Background(), adapters.TranscriptRequest{
		ProviderSessionID: testSessionID,
		Workspace:         "/tmp/colony",
		Include:           adapters.PresetFull(),
	})
	if err != nil {
		t.Fatal(err)
	}
	call := findTool(t, got, "Read")
	if call.Output != "package b" || call.Status != adapters.ToolStatusOK {
		t.Fatalf("nested result not attached: %+v", call)
	}
}

// TestResolveTranscriptIsReadOnly pins that resolution leaves the provider store
// untouched: it opens the transcript in place and creates no files.
func TestResolveTranscriptIsReadOnly(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	writeTranscript(t, home, "tmp-colony", testSessionID, testTranscriptBody)
	before := snapshotTree(t, home)

	got, err := New().ResolveTranscript(context.Background(), adapters.TranscriptRequest{
		ProviderSessionID: testSessionID,
		Workspace:         "/tmp/colony",
		Include:           adapters.PresetFull(),
	})
	if err != nil {
		t.Fatal(err)
	}
	if got.Omitted != "" {
		t.Fatalf("Omitted = %q", got.Omitted)
	}
	if after := snapshotTree(t, home); !reflect.DeepEqual(before, after) {
		t.Fatalf("resolution changed the provider store:\nbefore=%v\nafter=%v", before, after)
	}
}

func snapshotTree(t *testing.T, root string) map[string]string {
	t.Helper()
	out := map[string]string{}
	err := filepath.WalkDir(root, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() {
			out[path] = ""
			return nil
		}
		data, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		out[path] = string(data)
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	return out
}

// TestResolveTranscriptAttachesSeparateToolResult pins that a result stored as
// its own record is folded into the call it answers by call id.
func TestResolveTranscriptAttachesSeparateToolResult(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	body := `{"role":"assistant","message":{"content":[{"type":"tool_use","name":"Read","id":"call-9","input":{"path":"a.go"}}]}}
{"role":"user","message":{"content":[{"type":"tool_result","tool_use_id":"call-9","is_error":true,"content":"boom"}]}}
`
	writeTranscript(t, home, "tmp-colony", testSessionID, body)

	got, err := New().ResolveTranscript(context.Background(), adapters.TranscriptRequest{
		ProviderSessionID: testSessionID,
		Workspace:         "/tmp/colony",
		Include:           adapters.PresetFull(),
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(got.Turns) != 1 {
		t.Fatalf("result record must not become a turn: %+v", got.Turns)
	}
	call := findTool(t, got, "Read")
	if call.Output != "boom" || call.Status != adapters.ToolStatusError {
		t.Fatalf("tool result not attached: %+v", call)
	}
}

func TestResolveTranscriptTruncatesParts(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	body := `{"role":"assistant","message":{"content":[{"type":"text","text":"` + strings.Repeat("x", 50) + `"}]}}` + "\n"
	writeTranscript(t, home, "tmp-colony", testSessionID, body)

	got, err := New().ResolveTranscript(context.Background(), adapters.TranscriptRequest{
		ProviderSessionID: testSessionID,
		Workspace:         "/tmp/colony",
		Include:           adapters.PresetDialogue(),
		Limits:            adapters.TranscriptLimits{Text: 10},
	})
	if err != nil {
		t.Fatal(err)
	}
	if !got.Truncated {
		t.Fatal("expected Truncated")
	}
	if got.Turns[0].Parts[0].Text != strings.Repeat("x", 10)+"…" {
		t.Fatalf("text = %q", got.Turns[0].Parts[0].Text)
	}
}

func TestResolveTranscriptWindow(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	writeTranscript(t, home, "tmp-colony", testSessionID, testTranscriptBody)

	req := adapters.TranscriptRequest{
		ProviderSessionID: testSessionID,
		Workspace:         "/tmp/colony",
		Include:           adapters.PresetDialogue(),
		Window:            adapters.TranscriptWindow{Limit: 1},
	}
	first, err := New().ResolveTranscript(context.Background(), req)
	if err != nil {
		t.Fatal(err)
	}
	if len(first.Turns) != 1 || !first.HasMore || first.NextCursor == "" {
		t.Fatalf("first page = %+v", first)
	}
	req.Window = adapters.TranscriptWindow{Limit: 1, Cursor: first.NextCursor}
	second, err := New().ResolveTranscript(context.Background(), req)
	if err != nil {
		t.Fatal(err)
	}
	if len(second.Turns) != 1 || second.HasMore {
		t.Fatalf("second page = %+v", second)
	}
}

func TestResolveTranscriptMissing(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	got, err := New().ResolveTranscript(context.Background(), adapters.TranscriptRequest{
		ProviderSessionID: testSessionID,
		Workspace:         "/tmp/colony",
		Include:           adapters.PresetToolCall(),
	})
	if err != nil {
		t.Fatal(err)
	}
	if got.Omitted != adapters.SessionLogOmittedStoreNotFound {
		t.Fatalf("Omitted = %q", got.Omitted)
	}
}

func TestResolveTranscriptUnsafeID(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	secret := filepath.Join(home, ".cursor", "projects", "tmp-colony", "agent-transcripts", "etc", "passwd.jsonl")
	if err := os.MkdirAll(filepath.Dir(secret), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(secret, []byte("{}\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	got, err := New().ResolveTranscript(context.Background(), adapters.TranscriptRequest{
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

func TestResolveTranscriptGlobsWhenWorkspaceMisses(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	writeTranscript(t, home, "other-project", testSessionID, `{"role":"assistant","message":{"content":[{"type":"tool_use","name":"Read","input":{"path":"glob.go"}}]}}`+"\n")
	got, err := New().ResolveTranscript(context.Background(), adapters.TranscriptRequest{
		ProviderSessionID: testSessionID,
		Workspace:         "/does/not/exist",
		Include:           adapters.PresetToolCall(),
	})
	if err != nil {
		t.Fatal(err)
	}
	calls := adapters.ProjectToolCalls(got)
	if len(calls) != 1 || calls[0].Args != "glob.go" {
		t.Fatalf("glob fallback failed: %+v omitted=%q", calls, got.Omitted)
	}
}

func TestResolveTranscriptPrefersWorkspaceSlug(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	writeTranscript(t, home, "other-project", testSessionID, `{"role":"assistant","message":{"content":[{"type":"text","text":"other"}]}}`+"\n")
	writeTranscript(t, home, "tmp-colony", testSessionID, `{"role":"assistant","message":{"content":[{"type":"text","text":"wanted"}]}}`+"\n")

	got, err := New().ResolveTranscript(context.Background(), adapters.TranscriptRequest{
		ProviderSessionID: testSessionID,
		Workspace:         "/tmp/colony",
		Include:           adapters.PresetDialogue(),
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(got.Turns) != 1 || got.Turns[0].Parts[0].Text != "wanted" {
		t.Fatalf("preferred slug not used: %+v", got.Turns)
	}
}

func TestResolveTranscriptMalformed(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	writeTranscript(t, home, "tmp-colony", testSessionID, "{not json\n")
	got, err := New().ResolveTranscript(context.Background(), adapters.TranscriptRequest{
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

func TestResolveTranscriptEmptyConversation(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	writeTranscript(t, home, "tmp-colony", testSessionID, `{"role":"user","message":{"content":[{"type":"text","text":"hi"}]}}`+"\n")
	got, err := New().ResolveTranscript(context.Background(), adapters.TranscriptRequest{
		ProviderSessionID: testSessionID,
		Workspace:         "/tmp/colony",
		Include:           adapters.PresetToolCall(),
	})
	if err != nil {
		t.Fatal(err)
	}
	if got.Omitted != "" {
		t.Fatalf("Omitted = %q", got.Omitted)
	}
	if len(got.Turns) != 0 {
		t.Fatalf("turns = %+v", got.Turns)
	}
	if calls := adapters.ProjectToolCalls(got); len(calls) != 0 || calls == nil {
		t.Fatalf("tool calls = %+v", calls)
	}
}

// TestResolveTranscriptHonoursDeadline pins that a cancelled context yields a
// stable omission rather than an error.
func TestResolveTranscriptHonoursDeadline(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	writeTranscript(t, home, "tmp-colony", testSessionID, testTranscriptBody)

	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	got, err := New().ResolveTranscript(ctx, adapters.TranscriptRequest{
		ProviderSessionID: testSessionID,
		Workspace:         "/tmp/colony",
	})
	if err != nil {
		t.Fatal(err)
	}
	if got.Omitted != adapters.SessionLogOmittedTimeout {
		t.Fatalf("Omitted = %q", got.Omitted)
	}
}

func TestProjectSlug(t *testing.T) {
	if got := projectSlug("/home/ruslan/Projects/paseka"); got != "home-ruslan-Projects-paseka" {
		t.Fatalf("slug = %q", got)
	}
}

func findTool(t *testing.T, transcript adapters.Transcript, name string) *adapters.ToolPart {
	t.Helper()
	for _, turn := range transcript.Turns {
		for _, part := range turn.Parts {
			if part.Tool != nil && part.Tool.Name == name {
				return part.Tool
			}
		}
	}
	t.Fatalf("tool %q not found in %+v", name, transcript.Turns)
	return nil
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

func TestParseTimestamp(t *testing.T) {
	if got := parseTimestamp("2026-11-01T10:00:00Z"); got.IsZero() || !got.Equal(time.Date(2026, 11, 1, 10, 0, 0, 0, time.UTC)) {
		t.Fatalf("timestamp = %v", got)
	}
	if got := parseTimestamp("not a time"); !got.IsZero() {
		t.Fatalf("invalid timestamp = %v", got)
	}
}
