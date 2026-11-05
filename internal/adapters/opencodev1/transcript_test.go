package opencodev1

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/russ-p/paseka/internal/adapters"
)

const (
	openCodeSessionID  = "ses_open1"
	openCodeProjectID  = "c52c8ee2fd194d313516e260377f2f17a023c4e7"
	openCodeWorkspace  = "/tmp/colony"
	openCodeUserID     = "msg_z_user"
	openCodeAssistID   = "msg_a_assistant"
	openCodeErrorMsgID = "msg_b_error"
)

func TestResolveTranscriptOrdersTurnsAndTypedParts(t *testing.T) {
	root := setupOpenCodeStore(t)
	writeOpenCodeFixture(t, root)

	got, err := New().ResolveTranscript(context.Background(), adapters.TranscriptRequest{
		ProviderSessionID: openCodeSessionID,
		Workspace:         openCodeWorkspace,
		Include:           adapters.PresetFull(),
	})
	if err != nil {
		t.Fatalf("ResolveTranscript: %v", err)
	}
	if got.Omitted != "" {
		t.Fatalf("Omitted = %q", got.Omitted)
	}

	wantRoles := []string{adapters.TurnRoleUser, adapters.TurnRoleAssistant, adapters.TurnRoleAssistant}
	if len(got.Turns) != len(wantRoles) {
		t.Fatalf("turns = %+v, want roles %v", got.Turns, wantRoles)
	}
	for i, role := range wantRoles {
		if got.Turns[i].Role != role {
			t.Fatalf("turn %d role = %q, want %q", i, got.Turns[i].Role, role)
		}
	}
	// Files sort with the assistant first, but the provider's creation time
	// puts the user turn first.
	if got.Turns[0].Parts[0].Text != "hi" {
		t.Fatalf("first turn = %+v", got.Turns[0])
	}
	if got.Turns[0].Timestamp != time.UnixMilli(1000).UTC() {
		t.Fatalf("user timestamp = %v", got.Turns[0].Timestamp)
	}

	parts := got.Turns[1].Parts
	if len(parts) != 3 {
		t.Fatalf("assistant parts = %+v", parts)
	}
	if parts[0].Kind != adapters.PartReasoning || parts[0].Reasoning != "thinking" {
		t.Fatalf("reasoning part = %+v", parts[0])
	}
	if parts[1].Kind != adapters.PartText || parts[1].Text != "working" {
		t.Fatalf("text part = %+v", parts[1])
	}
	tool := parts[2].Tool
	if tool == nil || tool.Name != "read" || tool.CallID != "call-1" || tool.Status != adapters.ToolStatusOK {
		t.Fatalf("read tool = %+v", tool)
	}
	if tool.Input != `{"filePath":"main.go"}` || tool.Output != "package main" {
		t.Fatalf("read payload = %+v", tool)
	}

	failed := got.Turns[2].Parts[0].Tool
	if failed == nil || failed.Name != "bash" || failed.Status != adapters.ToolStatusError {
		t.Fatalf("error tool = %+v", failed)
	}
	if failed.Input != `{"command":"go test ./..."}` || failed.Output != "boom" {
		t.Fatalf("error tool payload = %+v", failed)
	}

	// The Agent log is the shared projection: a command input summarizes the
	// same way a Cursor tool input does.
	calls := adapters.ProjectToolCalls(got)
	if len(calls) != 2 {
		t.Fatalf("tool calls = %+v", calls)
	}
	if calls[0].Name != "read" || calls[0].Args != `{"filePath":"main.go"}` || calls[0].CallID != "call-1" {
		t.Fatalf("read call = %+v", calls[0])
	}
	if calls[1].Name != "bash" || calls[1].Args != "go test ./..." || calls[1].CallID != "call-2" {
		t.Fatalf("bash call = %+v", calls[1])
	}
}

func TestResolveTranscriptToolCallPresetHidesDialogueAndOutput(t *testing.T) {
	root := setupOpenCodeStore(t)
	writeOpenCodeFixture(t, root)

	got, err := New().ResolveTranscript(context.Background(), adapters.TranscriptRequest{
		ProviderSessionID: openCodeSessionID,
		Workspace:         openCodeWorkspace,
		Include:           adapters.PresetToolCall(),
	})
	if err != nil {
		t.Fatalf("ResolveTranscript: %v", err)
	}
	if got.Omitted != "" {
		t.Fatalf("Omitted = %q", got.Omitted)
	}
	if len(got.Turns) != 2 {
		t.Fatalf("turns = %+v, want only the two tool turns", got.Turns)
	}
	for _, turn := range got.Turns {
		if turn.Role != adapters.TurnRoleAssistant {
			t.Fatalf("tool-call preset leaked non-tool turn: %+v", turn)
		}
		for _, part := range turn.Parts {
			if part.Kind != adapters.PartTool || part.Tool == nil {
				t.Fatalf("tool-call preset leaked %+v", part)
			}
			if part.Tool.Input == "" {
				t.Fatalf("tool-call preset must keep input: %+v", part.Tool)
			}
			if part.Tool.Output != "" {
				t.Fatalf("tool-call preset populated output: %+v", part.Tool)
			}
			if part.Tool.Status == "" {
				t.Fatalf("tool status missing: %+v", part.Tool)
			}
		}
	}
}

func TestResolveTranscriptDialoguePresetHidesToolPayloads(t *testing.T) {
	root := setupOpenCodeStore(t)
	writeOpenCodeFixture(t, root)

	got, err := New().ResolveTranscript(context.Background(), adapters.TranscriptRequest{
		ProviderSessionID: openCodeSessionID,
		Workspace:         openCodeWorkspace,
		Include:           adapters.PresetDialogue(),
	})
	if err != nil {
		t.Fatalf("ResolveTranscript: %v", err)
	}
	if got.Omitted != "" {
		t.Fatalf("Omitted = %q", got.Omitted)
	}
	if len(got.Turns) != 3 {
		t.Fatalf("turns = %+v", got.Turns)
	}
	if got.Turns[0].Parts[0].Text != "hi" || got.Turns[1].Parts[0].Text != "working" {
		t.Fatalf("dialogue text missing: %+v", got.Turns)
	}
	for _, turn := range got.Turns {
		for _, part := range turn.Parts {
			if part.Kind == adapters.PartReasoning {
				t.Fatalf("dialogue preset leaked reasoning: %+v", part)
			}
			if part.Tool == nil {
				continue
			}
			if part.Tool.Name == "" || part.Tool.CallID == "" || part.Tool.Status == "" {
				t.Fatalf("tool identity missing: %+v", part.Tool)
			}
			if part.Tool.Input != "" || part.Tool.Output != "" {
				t.Fatalf("dialogue preset populated tool payload: %+v", part.Tool)
			}
		}
	}
}

func TestResolveTranscriptMissingOrUnsafeSessionIsStableOmission(t *testing.T) {
	setupOpenCodeStore(t)

	for _, id := range []string{"", "ses_missing", "../escape", "ses/bad"} {
		got, err := New().ResolveTranscript(context.Background(), adapters.TranscriptRequest{ProviderSessionID: id})
		if err != nil {
			t.Fatalf("%q: unexpected error: %v", id, err)
		}
		if got.Omitted != adapters.SessionLogOmittedStoreNotFound {
			t.Fatalf("%q: Omitted = %q", id, got.Omitted)
		}
		if len(got.Turns) != 0 {
			t.Fatalf("%q: turns = %+v", id, got.Turns)
		}
	}
}

func TestResolveTranscriptMalformedSessionIsParseError(t *testing.T) {
	root := setupOpenCodeStore(t)
	path := filepath.Join(root, "session", openCodeProjectID, openCodeSessionID+".json")
	writeOpenCodeFile(t, path, "{not json")

	got, err := New().ResolveTranscript(context.Background(), adapters.TranscriptRequest{ProviderSessionID: openCodeSessionID})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got.Omitted != adapters.SessionLogOmittedParseError {
		t.Fatalf("Omitted = %q", got.Omitted)
	}
}

func TestResolveTranscriptWithoutMessagesIsEmptyNotOmitted(t *testing.T) {
	root := setupOpenCodeStore(t)
	writeOpenCodeFile(t, filepath.Join(root, "session", openCodeProjectID, openCodeSessionID+".json"),
		`{"id":"`+openCodeSessionID+`","directory":"`+openCodeWorkspace+`"}`)

	got, err := New().ResolveTranscript(context.Background(), adapters.TranscriptRequest{
		ProviderSessionID: openCodeSessionID,
		Include:           adapters.PresetDialogue(),
	})
	if err != nil {
		t.Fatalf("ResolveTranscript: %v", err)
	}
	if got.Omitted != "" {
		t.Fatalf("Omitted = %q", got.Omitted)
	}
	if got.Turns == nil || len(got.Turns) != 0 {
		t.Fatalf("turns = %+v, want an empty slice", got.Turns)
	}
}

func setupOpenCodeStore(t *testing.T) string {
	t.Helper()
	xdg := t.TempDir()
	t.Setenv("XDG_DATA_HOME", xdg)
	return filepath.Join(xdg, "opencode", "storage")
}

func writeOpenCodeFixture(t *testing.T, root string) {
	t.Helper()
	writeOpenCodeFile(t, filepath.Join(root, "session", openCodeProjectID, openCodeSessionID+".json"),
		`{"id":"`+openCodeSessionID+`","directory":"`+openCodeWorkspace+`","time":{"created":500,"updated":4000}}`)

	writeOpenCodeFile(t, filepath.Join(root, "message", openCodeSessionID, openCodeUserID+".json"),
		`{"id":"`+openCodeUserID+`","sessionID":"`+openCodeSessionID+`","role":"user","time":{"created":1000}}`)
	writeOpenCodePart(t, root, openCodeUserID, "prt_0001_text",
		`{"id":"prt_0001_text","type":"text","text":"hi"}`)

	writeOpenCodeFile(t, filepath.Join(root, "message", openCodeSessionID, openCodeAssistID+".json"),
		`{"id":"`+openCodeAssistID+`","sessionID":"`+openCodeSessionID+`","role":"assistant","time":{"created":2000}}`)
	writeOpenCodePart(t, root, openCodeAssistID, "prt_0001_reasoning",
		`{"id":"prt_0001_reasoning","type":"reasoning","text":"thinking"}`)
	writeOpenCodePart(t, root, openCodeAssistID, "prt_0002_text",
		`{"id":"prt_0002_text","type":"text","text":"working"}`)
	writeOpenCodePart(t, root, openCodeAssistID, "prt_0003_tool",
		`{"id":"prt_0003_tool","type":"tool","callID":"call-1","tool":"read","state":{"status":"completed","input":{"filePath":"main.go"},"output":"package main"}}`)
	writeOpenCodePart(t, root, openCodeAssistID, "prt_0004_step",
		`{"id":"prt_0004_step","type":"step-finish","reason":"tool-calls"}`)

	writeOpenCodeFile(t, filepath.Join(root, "message", openCodeSessionID, openCodeErrorMsgID+".json"),
		`{"id":"`+openCodeErrorMsgID+`","sessionID":"`+openCodeSessionID+`","role":"assistant","time":{"created":3000}}`)
	writeOpenCodePart(t, root, openCodeErrorMsgID, "prt_0001_tool",
		`{"id":"prt_0001_tool","type":"tool","callID":"call-2","tool":"bash","state":{"status":"error","input":{"command":"go test ./..."},"error":"boom"}}`)
}

func writeOpenCodePart(t *testing.T, root, messageID, name, body string) {
	t.Helper()
	writeOpenCodeFile(t, filepath.Join(root, "part", messageID, name+".json"), body)
}

func writeOpenCodeFile(t *testing.T, path, body string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
}
