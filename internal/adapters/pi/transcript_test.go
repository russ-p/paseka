package pi

import (
	"context"
	"io/fs"
	"os"
	"path/filepath"
	"reflect"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/russ-p/paseka/internal/adapters"
)

const piConversationBody = `{"type":"session","version":3,"id":"agent-1","timestamp":"2026-11-01T10:00:00Z","cwd":"/repo"}
{"type":"model_change","id":"m1","parentId":null,"timestamp":"2026-11-01T10:00:00.100Z","provider":"anthropic","modelId":"claude"}
{"type":"message","id":"m2","parentId":"m1","timestamp":"2026-11-01T10:00:01Z","message":{"role":"system","content":[{"type":"text","text":"system prompt"}],"timestamp":1762077601000}}
{"type":"message","id":"m3","parentId":"m2","timestamp":"2026-11-01T10:00:02Z","message":{"role":"user","content":"please read main.go","timestamp":1762077602000}}
{"type":"message","id":"m4","parentId":"m3","timestamp":"2026-11-01T10:00:03Z","message":{"role":"assistant","content":[{"type":"thinking","thinking":"weighing the file"},{"type":"text","text":"reading it now"},{"type":"toolCall","id":"call-1","name":"read","arguments":{"path":"main.go"}}],"timestamp":1762077603000}}
{"type":"message","id":"m5","parentId":"m4","timestamp":"2026-11-01T10:00:04Z","message":{"role":"toolResult","toolCallId":"call-1","toolName":"read","content":[{"type":"text","text":"package main"}],"isError":false,"timestamp":1762077604000}}
{"type":"usage","id":"m6","parentId":"m5","timestamp":"2026-11-01T10:00:05Z","kind":"cache_warm","usage":{"input":10,"output":0,"cacheRead":0,"cacheWrite":0,"totalTokens":10,"cost":{"total":0.01}}}
{"type":"message","id":"m7","parentId":"m6","timestamp":"2026-11-01T10:00:06Z","message":{"role":"assistant","content":[{"type":"toolCall","id":"call-2","name":"bash","arguments":{"command":"false"}}],"timestamp":1762077606000}}
{"type":"message","id":"m8","parentId":"m7","timestamp":"2026-11-01T10:00:07Z","message":{"role":"toolResult","toolCallId":"call-2","toolName":"bash","content":[{"type":"text","text":"boom"}],"isError":true,"timestamp":1762077607000}}
`

func TestResolveTranscriptOrdersTurnsAndFoldsToolResults(t *testing.T) {
	runDir := t.TempDir()
	writePiSessionFile(t, runDir, "20260101_100000_agent-1.jsonl", piConversationBody)

	got, err := New().ResolveTranscript(context.Background(), adapters.TranscriptRequest{
		ProviderSessionID: "agent-1",
		RunDir:            runDir,
		Include:           adapters.PresetFull(),
	})
	if err != nil {
		t.Fatalf("ResolveTranscript: %v", err)
	}
	if got.Omitted != "" {
		t.Fatalf("Omitted = %q", got.Omitted)
	}
	wantRoles := []string{
		adapters.TurnRoleSystem,
		adapters.TurnRoleUser,
		adapters.TurnRoleAssistant,
		adapters.TurnRoleAssistant,
	}
	if len(got.Turns) != len(wantRoles) {
		t.Fatalf("turns = %+v, want roles %v", got.Turns, wantRoles)
	}
	for i, role := range wantRoles {
		if got.Turns[i].Role != role {
			t.Fatalf("turn %d role = %q, want %q", i, got.Turns[i].Role, role)
		}
	}
	if got.Turns[0].Parts[0].Text != "system prompt" {
		t.Fatalf("system turn = %+v", got.Turns[0])
	}
	if got.Turns[1].Parts[0].Text != "please read main.go" {
		t.Fatalf("user turn = %+v", got.Turns[1])
	}
	if got.Turns[0].Timestamp.IsZero() || got.Turns[1].Timestamp.IsZero() {
		t.Fatalf("provider timestamps missing: %+v", got.Turns)
	}

	assistant := got.Turns[2].Parts
	if len(assistant) != 3 || assistant[0].Kind != adapters.PartReasoning || assistant[1].Kind != adapters.PartText || assistant[2].Kind != adapters.PartTool {
		t.Fatalf("assistant parts = %+v", assistant)
	}
	call := piTool(t, got, "read")
	if call.CallID != "call-1" || call.Output != "package main" || call.Status != adapters.ToolStatusOK {
		t.Fatalf("folded tool call = %+v", call)
	}
	if call.Input != `{"path":"main.go"}` {
		t.Fatalf("tool input = %q", call.Input)
	}
	failed := piTool(t, got, "bash")
	if failed.Output != "boom" || failed.Status != adapters.ToolStatusError {
		t.Fatalf("error tool call = %+v", failed)
	}

	calls := adapters.ProjectToolCalls(got)
	if len(calls) != 2 || calls[0].Args != "main.go" || calls[1].Args != "false" {
		t.Fatalf("tool call order = %+v", calls)
	}
}

func TestResolveTranscriptMatchesSessionHeaderEvenWhenOlder(t *testing.T) {
	runDir := t.TempDir()
	wanted := writePiSessionFile(t, runDir, "20260101_000000_wanted.jsonl",
		`{"type":"session","version":3,"id":"wanted"}`+"\n"+
			`{"type":"message","message":{"role":"user","content":"wanted turn"}}`+"\n")
	newer := writePiSessionFile(t, runDir, "20260102_000000_other.jsonl",
		`{"type":"session","version":3,"id":"other"}`+"\n"+
			`{"type":"message","message":{"role":"user","content":"other turn"}}`+"\n")
	setPiModTime(t, wanted, time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC))
	setPiModTime(t, newer, time.Date(2026, 1, 2, 0, 0, 0, 0, time.UTC))

	got, err := New().ResolveTranscript(context.Background(), adapters.TranscriptRequest{
		ProviderSessionID: "wanted",
		RunDir:            runDir,
		Include:           adapters.PresetDialogue(),
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(got.Turns) != 1 || got.Turns[0].Parts[0].Text != "wanted turn" {
		t.Fatalf("matching header not used: %+v", got.Turns)
	}
}

func TestResolveTranscriptFallsBackToNewestFileWithoutMixing(t *testing.T) {
	runDir := t.TempDir()
	older := writePiSessionFile(t, runDir, "20260101_000000_older.jsonl",
		`{"type":"session","version":3,"id":"older"}`+"\n"+
			`{"type":"message","message":{"role":"user","content":"older turn"}}`+"\n")
	newer := writePiSessionFile(t, runDir, "20260102_000000_newer.jsonl",
		`{"type":"session","version":3,"id":"newer"}`+"\n"+
			`{"type":"message","message":{"role":"user","content":"newer turn"}}`+"\n")
	setPiModTime(t, older, time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC))
	setPiModTime(t, newer, time.Date(2026, 1, 2, 0, 0, 0, 0, time.UTC))

	got, err := New().ResolveTranscript(context.Background(), adapters.TranscriptRequest{
		ProviderSessionID: "missing",
		RunDir:            runDir,
		Include:           adapters.PresetDialogue(),
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(got.Turns) != 1 || got.Turns[0].Parts[0].Text != "newer turn" {
		t.Fatalf("newest file not used alone: %+v", got.Turns)
	}
}

func TestResolveTranscriptIgnoresNonConversationRecords(t *testing.T) {
	runDir := t.TempDir()
	body := `{"type":"session","version":3,"id":"agent-1"}
{"type":"model_change","provider":"anthropic","modelId":"claude"}
{"type":"thinking_level_change","thinkingLevel":"high"}
{"type":"usage","kind":"cache_warm","usage":{"input":1}}
{"type":"compaction","summary":"older context","tokensBefore":10}
{"type":"branch_summary","fromId":"x","summary":"other branch"}
{"type":"custom","customType":"state","data":{"count":1}}
{"type":"custom_message","customType":"state","content":"injected"}
{"type":"context_edit","targetId":"x","replacement":null}
{"type":"label","targetId":"x","label":"checkpoint"}
{"type":"message","message":{"role":"user","content":"only conversation record"}}
`
	writePiSessionFile(t, runDir, "20260101_100000_agent-1.jsonl", body)

	got, err := New().ResolveTranscript(context.Background(), adapters.TranscriptRequest{
		ProviderSessionID: "agent-1",
		RunDir:            runDir,
		Include:           adapters.PresetFull(),
	})
	if err != nil {
		t.Fatalf("ResolveTranscript: %v", err)
	}
	if got.Omitted != "" {
		t.Fatalf("Omitted = %q", got.Omitted)
	}
	if len(got.Turns) != 1 || got.Turns[0].Parts[0].Text != "only conversation record" {
		t.Fatalf("turns = %+v", got.Turns)
	}
}

func TestResolveTranscriptReasoningOnlyWhenRequested(t *testing.T) {
	runDir := t.TempDir()
	writePiSessionFile(t, runDir, "20260101_100000_agent-1.jsonl", piConversationBody)

	without, err := New().ResolveTranscript(context.Background(), adapters.TranscriptRequest{
		ProviderSessionID: "agent-1",
		RunDir:            runDir,
		Include:           adapters.PresetDialogue(),
	})
	if err != nil {
		t.Fatal(err)
	}
	for _, turn := range without.Turns {
		for _, part := range turn.Parts {
			if part.Kind == adapters.PartReasoning {
				t.Fatalf("dialogue preset carried reasoning: %+v", part)
			}
		}
	}

	with, err := New().ResolveTranscript(context.Background(), adapters.TranscriptRequest{
		ProviderSessionID: "agent-1",
		RunDir:            runDir,
		Include:           adapters.PresetDialogueWithReasoning(),
	})
	if err != nil {
		t.Fatal(err)
	}
	found := false
	for _, turn := range with.Turns {
		for _, part := range turn.Parts {
			if part.Kind == adapters.PartReasoning && part.Reasoning == "weighing the file" {
				found = true
			}
		}
	}
	if !found {
		t.Fatalf("reasoning preset lost thinking: %+v", with.Turns)
	}
}

func TestResolveTranscriptRedactsSharedSecrets(t *testing.T) {
	runDir := t.TempDir()
	secret := "ghp_abcdefghijklmnopqrstuvwxyz0123456789"
	body := `{"type":"session","version":3,"id":"agent-1"}
{"type":"message","message":{"role":"user","content":"token ` + secret + `"}}
{"type":"message","message":{"role":"assistant","content":[{"type":"toolCall","id":"call-1","name":"bash","arguments":{"command":"curl -H \"Authorization: Bearer ` + secret + `\""}}]}}
{"type":"message","message":{"role":"toolResult","toolCallId":"call-1","content":[{"type":"text","text":"output ` + secret + `"}],"isError":false}}
`
	writePiSessionFile(t, runDir, "20260101_100000_agent-1.jsonl", body)

	got, err := New().ResolveTranscript(context.Background(), adapters.TranscriptRequest{
		ProviderSessionID: "agent-1",
		RunDir:            runDir,
		Include:           adapters.PresetFull(),
	})
	if err != nil {
		t.Fatal(err)
	}
	for _, turn := range got.Turns {
		for _, part := range turn.Parts {
			text := part.Text + part.Reasoning
			if part.Tool != nil {
				text += part.Tool.Input + part.Tool.Output
			}
			if strings.Contains(text, secret) {
				t.Fatalf("secret leaked: %+v", part)
			}
		}
	}
	if !transcriptContains(got, "<redacted>") {
		t.Fatalf("redaction placeholder missing: %+v", got.Turns)
	}
}

func TestResolveTranscriptIsReadOnly(t *testing.T) {
	runDir := t.TempDir()
	writePiSessionFile(t, runDir, "20260101_100000_agent-1.jsonl", piConversationBody)
	before := snapshotPiTree(t, runDir)

	if _, err := New().ResolveTranscript(context.Background(), adapters.TranscriptRequest{
		ProviderSessionID: "agent-1",
		RunDir:            runDir,
		Include:           adapters.PresetFull(),
	}); err != nil {
		t.Fatal(err)
	}
	if after := snapshotPiTree(t, runDir); !reflect.DeepEqual(before, after) {
		t.Fatalf("resolution changed the run directory:\nbefore=%v\nafter=%v", before, after)
	}
}

// TestResolveTranscriptPagesLongSessions pins the default first page and the
// cursor walk on the reference line-oriented reader.
func TestResolveTranscriptPagesLongSessions(t *testing.T) {
	runDir := t.TempDir()
	var body strings.Builder
	body.WriteString(`{"type":"session","version":3,"id":"agent-1","timestamp":"2026-11-01T10:00:00Z"}` + "\n")
	for i := 0; i < 120; i++ {
		role := "user"
		if i%2 == 1 {
			role = "assistant"
		}
		body.WriteString(`{"type":"message","id":"m` + strconv.Itoa(i) + `","message":{"role":"` + role + `","content":"turn ` + strconv.Itoa(i) + `"}}` + "\n")
	}
	writePiSessionFile(t, runDir, "20260101_100000_agent-1.jsonl", body.String())

	req := adapters.TranscriptRequest{
		ProviderSessionID: "agent-1",
		RunDir:            runDir,
		Include:           adapters.PresetDialogue(),
	}
	first, err := New().ResolveTranscript(context.Background(), req)
	if err != nil {
		t.Fatal(err)
	}
	if len(first.Turns) != adapters.DefaultTranscriptPageTurns || !first.HasMore || first.NextCursor == "" {
		t.Fatalf("first page = %d turns, hasMore %v, cursor %q", len(first.Turns), first.HasMore, first.NextCursor)
	}

	req.Window = adapters.TranscriptWindow{Cursor: first.NextCursor}
	second, err := New().ResolveTranscript(context.Background(), req)
	if err != nil {
		t.Fatal(err)
	}
	if len(second.Turns) != 20 || second.HasMore || second.NextCursor != "" {
		t.Fatalf("second page = %d turns, hasMore %v, cursor %q", len(second.Turns), second.HasMore, second.NextCursor)
	}
	if last := second.Turns[len(second.Turns)-1].Parts[0].Text; last != "turn 119" {
		t.Fatalf("last turn = %q", last)
	}
}

func TestResolveTranscriptMissingRunLocalStore(t *testing.T) {
	tests := map[string]adapters.TranscriptRequest{
		"no run dir":     {ProviderSessionID: "agent-1"},
		"no session dir": {ProviderSessionID: "agent-1", RunDir: t.TempDir()},
		"empty dir":      {ProviderSessionID: "agent-1", RunDir: piSessionRunDir(t)},
	}
	for name, req := range tests {
		t.Run(name, func(t *testing.T) {
			got, err := New().ResolveTranscript(context.Background(), req)
			if err != nil {
				t.Fatal(err)
			}
			if got.Omitted != adapters.SessionLogOmittedStoreNotFound {
				t.Fatalf("Omitted = %q", got.Omitted)
			}
			if len(got.Turns) != 0 {
				t.Fatalf("turns = %+v", got.Turns)
			}
		})
	}
}

func TestResolveTranscriptNotImplementedReasonIsRetired(t *testing.T) {
	runDir := t.TempDir()
	writePiSessionFile(t, runDir, "20260101_100000_agent-1.jsonl", piConversationBody)
	got, err := New().ResolveTranscript(context.Background(), adapters.TranscriptRequest{
		ProviderSessionID: "agent-1",
		RunDir:            runDir,
	})
	if err != nil {
		t.Fatal(err)
	}
	if got.Omitted == "not implemented" {
		t.Fatal("Pi reader still reports the retired stub")
	}
}

func TestResolveTranscriptHonoursIncludeMask(t *testing.T) {
	runDir := t.TempDir()
	writePiSessionFile(t, runDir, "20260101_100000_agent-1.jsonl", piConversationBody)

	dialogue, err := New().ResolveTranscript(context.Background(), adapters.TranscriptRequest{
		ProviderSessionID: "agent-1",
		RunDir:            runDir,
		Include:           adapters.PresetDialogue(),
	})
	if err != nil {
		t.Fatal(err)
	}
	for _, turn := range dialogue.Turns {
		for _, part := range turn.Parts {
			if part.Tool == nil {
				continue
			}
			if part.Tool.Name == "" || part.Tool.CallID == "" || part.Tool.Status == "" {
				t.Fatalf("tool identity missing: %+v", part.Tool)
			}
			if part.Tool.Input != "" || part.Tool.Output != "" {
				t.Fatalf("dialogue preset carried tool payload: %+v", part.Tool)
			}
		}
	}

	toolCalls, err := New().ResolveTranscript(context.Background(), adapters.TranscriptRequest{
		ProviderSessionID: "agent-1",
		RunDir:            runDir,
		Include:           adapters.PresetToolCall(),
	})
	if err != nil {
		t.Fatal(err)
	}
	for _, turn := range toolCalls.Turns {
		for _, part := range turn.Parts {
			if part.Kind != adapters.PartTool {
				t.Fatalf("tool-call preset carried %q: %+v", part.Kind, part)
			}
			if part.Tool.Input == "" {
				t.Fatalf("tool-call preset lost tool input: %+v", part.Tool)
			}
		}
	}
}

func TestResolveTranscriptMalformed(t *testing.T) {
	runDir := t.TempDir()
	writePiSessionFile(t, runDir, "20260101_100000_agent-1.jsonl", "{not json\n")
	got, err := New().ResolveTranscript(context.Background(), adapters.TranscriptRequest{
		ProviderSessionID: "agent-1",
		RunDir:            runDir,
	})
	if err != nil {
		t.Fatal(err)
	}
	if got.Omitted != adapters.SessionLogOmittedParseError {
		t.Fatalf("Omitted = %q", got.Omitted)
	}
}

func TestResolveTranscriptHonoursDeadline(t *testing.T) {
	runDir := t.TempDir()
	writePiSessionFile(t, runDir, "20260101_100000_agent-1.jsonl", piConversationBody)

	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	got, err := New().ResolveTranscript(ctx, adapters.TranscriptRequest{
		ProviderSessionID: "agent-1",
		RunDir:            runDir,
	})
	if err != nil {
		t.Fatal(err)
	}
	if got.Omitted != adapters.SessionLogOmittedTimeout {
		t.Fatalf("Omitted = %q", got.Omitted)
	}
}

func TestResolveTranscriptLeavesUnrelatedFilesAlone(t *testing.T) {
	runDir := t.TempDir()
	writePiSessionFile(t, runDir, "20260101_100000_agent-1.jsonl", piConversationBody)
	if err := os.WriteFile(filepath.Join(runDir, piSessionsDir, "notes.txt"), []byte("ignored"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.Mkdir(filepath.Join(runDir, piSessionsDir, "nested.jsonl"), 0o755); err != nil {
		t.Fatal(err)
	}

	got, err := New().ResolveTranscript(context.Background(), adapters.TranscriptRequest{
		ProviderSessionID: "agent-1",
		RunDir:            runDir,
		Include:           adapters.PresetDialogue(),
	})
	if err != nil {
		t.Fatal(err)
	}
	if got.Omitted != "" || len(got.Turns) == 0 {
		t.Fatalf("transcript = %+v", got)
	}
}

func piTool(t *testing.T, transcript adapters.Transcript, name string) *adapters.ToolPart {
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

func transcriptContains(transcript adapters.Transcript, want string) bool {
	for _, turn := range transcript.Turns {
		for _, part := range turn.Parts {
			if strings.Contains(part.Text+part.Reasoning, want) {
				return true
			}
			if part.Tool != nil && (strings.Contains(part.Tool.Input, want) || strings.Contains(part.Tool.Output, want)) {
				return true
			}
		}
	}
	return false
}

func writePiSessionFile(t *testing.T, runDir, name, body string) string {
	t.Helper()
	dir := filepath.Join(runDir, piSessionsDir)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(dir, name)
	if err := os.WriteFile(path, []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
	return path
}

func setPiModTime(t *testing.T, path string, mod time.Time) {
	t.Helper()
	if err := os.Chtimes(path, mod, mod); err != nil {
		t.Fatal(err)
	}
}

func piSessionRunDir(t *testing.T) string {
	t.Helper()
	runDir := t.TempDir()
	if err := os.MkdirAll(filepath.Join(runDir, piSessionsDir), 0o755); err != nil {
		t.Fatal(err)
	}
	return runDir
}

func snapshotPiTree(t *testing.T, root string) map[string]string {
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
