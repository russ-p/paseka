package export

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/russ-p/paseka/internal/adapters"
	"github.com/russ-p/paseka/internal/hiveview"
)

// toolCallTranscript builds the resolver result a provider would return for the
// Agent log: tool parts in provider order, capped input only.
func toolCallTranscript(calls ...adapters.ToolCall) adapters.Transcript {
	turns := make([]adapters.Turn, 0, len(calls))
	for _, call := range calls {
		turns = append(turns, adapters.Turn{
			Role:      adapters.TurnRoleAssistant,
			Timestamp: call.Timestamp,
			Parts: []adapters.Part{{Kind: adapters.PartTool, Tool: &adapters.ToolPart{
				Name:   call.Name,
				CallID: call.CallID,
				Status: adapters.ToolStatusUnknown,
				Input:  call.Args,
			}}},
		})
	}
	return adapters.Transcript{Turns: turns}
}

func TestResolveAgentLogTruncatesArgsAndCapsRows(t *testing.T) {
	long := strings.Repeat("a", maxAgentLogArgsRunes+20)
	calls := make([]adapters.ToolCall, maxAgentLogToolCalls+3)
	for i := range calls {
		calls[i] = adapters.ToolCall{Name: "Read", Args: long, CallID: "c"}
	}
	got := resolveAgentLog(hiveview.RunView{
		AgentID:           "agent-1",
		Adapter:           "cursor",
		ProviderSessionID: "sess",
	}, func(string) adapters.TranscriptResolver {
		return fakeTranscriptResolver{transcript: toolCallTranscript(calls...)}
	}, 0)
	if got.Omitted != "" {
		t.Fatalf("Omitted = %q", got.Omitted)
	}
	if !got.Truncated {
		t.Fatal("expected Truncated")
	}
	if len(got.ToolCalls) != maxAgentLogToolCalls {
		t.Fatalf("len = %d, want %d", len(got.ToolCalls), maxAgentLogToolCalls)
	}
	if got.ToolCalls[0].Args != strings.Repeat("a", maxAgentLogArgsRunes)+"…" {
		t.Fatalf("args not truncated: %q", got.ToolCalls[0].Args)
	}
}

// TestResolveAgentLogRequestsToolInputOnly pins the request the Agent log makes:
// tool input only, no dialogue, no tool output.
func TestResolveAgentLogRequestsToolInputOnly(t *testing.T) {
	var seen adapters.TranscriptRequest
	got := resolveAgentLog(hiveview.RunView{
		AgentID:           "agent-1",
		Adapter:           "cursor",
		ProviderSessionID: "sess",
		Workspace:         "/ws",
		ColonyRoot:        "/colony",
		RunDir:            "/colony/.paseka/runs/t/a",
	}, func(string) adapters.TranscriptResolver {
		return recordingResolver{seen: &seen, transcript: toolCallTranscript(adapters.ToolCall{Name: "Read", Args: "a.go", CallID: "c1"})}
	}, 0)
	if got.Omitted != "" {
		t.Fatalf("Omitted = %q", got.Omitted)
	}
	if seen.ProviderSessionID != "sess" || seen.Adapter != "cursor" || seen.Workspace != "/ws" || seen.ColonyRoot != "/colony" || seen.RunDir != "/colony/.paseka/runs/t/a" || seen.AgentID != "agent-1" {
		t.Fatalf("association request = %+v", seen)
	}
	if seen.Include != adapters.PresetToolCall() {
		t.Fatalf("include = %+v, want the tool-call preset", seen.Include)
	}
	wantLimits := adapters.DefaultTranscriptLimits()
	wantLimits.ToolInput = maxAgentLogToolInputRunes
	if seen.Limits != wantLimits {
		t.Fatalf("limits = %+v, want %+v", seen.Limits, wantLimits)
	}
	if seen.Window != adapters.WholeTranscriptWindow() {
		t.Fatalf("window = %+v, want the whole transcript", seen.Window)
	}
}

// TestResolveAgentLogSummarizesWholeToolInput pins the shipped argument summary
// for an input whose summary key sits past the generic 2k tool-input cap: the
// Agent log still finds the path and then applies its own 200-rune cap.
func TestResolveAgentLogSummarizesWholeToolInput(t *testing.T) {
	input := `{"blob":"` + strings.Repeat("x", 3000) + `","path":"deep/file.go"}`
	got := resolveAgentLog(hiveview.RunView{
		AgentID:           "agent-1",
		Adapter:           "cursor",
		ProviderSessionID: "sess",
	}, func(string) adapters.TranscriptResolver {
		return normalizingResolver{transcript: toolCallTranscript(adapters.ToolCall{Name: "Read", Args: input, CallID: "c1"})}
	}, 0)
	if got.Omitted != "" {
		t.Fatalf("Omitted = %q", got.Omitted)
	}
	if len(got.ToolCalls) != 1 || got.ToolCalls[0].Args != "deep/file.go" {
		t.Fatalf("args = %+v, want deep/file.go", got.ToolCalls)
	}
}

func TestResolveAgentLogEmptyToolCallsFound(t *testing.T) {
	got := resolveAgentLog(hiveview.RunView{
		AgentID:           "agent-1",
		Adapter:           "cursor",
		ProviderSessionID: "sess",
	}, func(string) adapters.TranscriptResolver {
		return fakeTranscriptResolver{transcript: adapters.Transcript{Turns: []adapters.Turn{}}}
	}, 0)
	if got.Omitted != "" {
		t.Fatalf("Omitted = %q", got.Omitted)
	}
	if got.ToolCalls == nil {
		t.Fatal("expected empty slice, not nil omit")
	}
}

func TestResolveAgentLogEmptyToolCallsWithOmitted(t *testing.T) {
	got := resolveAgentLog(hiveview.RunView{
		AgentID:           "agent-1",
		Adapter:           "cursor",
		ProviderSessionID: "sess",
	}, func(string) adapters.TranscriptResolver {
		return fakeTranscriptResolver{transcript: adapters.Transcript{Omitted: adapters.SessionLogOmittedStoreNotFound}}
	}, 0)
	if got.Omitted != adapters.SessionLogOmittedStoreNotFound {
		t.Fatalf("Omitted = %q", got.Omitted)
	}
}

func TestResolveAgentLogError(t *testing.T) {
	got := resolveAgentLog(hiveview.RunView{
		AgentID:           "agent-1",
		Adapter:           "cursor",
		ProviderSessionID: "sess",
	}, func(string) adapters.TranscriptResolver {
		return fakeTranscriptResolver{err: errors.New("boom")}
	}, 0)
	if got.Omitted != adapters.SessionLogOmittedResolveError {
		t.Fatalf("Omitted = %q", got.Omitted)
	}
}

// TestResolveAgentLogDeadline pins that a blocked resolver is cut off by the
// deadline instead of hanging the export.
func TestResolveAgentLogDeadline(t *testing.T) {
	release := make(chan struct{})
	defer close(release)

	start := time.Now()
	got := resolveAgentLog(hiveview.RunView{
		AgentID:           "agent-1",
		Adapter:           "cursor",
		ProviderSessionID: "sess",
	}, func(string) adapters.TranscriptResolver {
		return blockingTranscriptResolver{release: release}
	}, 20*time.Millisecond)
	if got.Omitted != adapters.SessionLogOmittedTimeout {
		t.Fatalf("Omitted = %q", got.Omitted)
	}
	if elapsed := time.Since(start); elapsed > time.Second {
		t.Fatalf("resolver was not cut off: %v", elapsed)
	}
}

// TestResolveAgentLogsKeepsRunsSeparate pins that each run resolves with its own
// providerSessionId and never merges turns across runs or agents.
func TestResolveAgentLogsKeepsRunsSeparate(t *testing.T) {
	var ids []string
	logs := resolveAgentLogs([]hiveview.RunView{
		{AgentID: "agent-1", Adapter: "cursor", ProviderSessionID: "sess-1"},
		{AgentID: "agent-2", Adapter: "cursor", ProviderSessionID: "sess-2"},
	}, func(string) adapters.TranscriptResolver {
		return recordingIDsResolver{ids: &ids}
	}, 0)
	if len(logs) != 2 {
		t.Fatalf("logs = %+v", logs)
	}
	if len(ids) != 2 || ids[0] != "sess-1" || ids[1] != "sess-2" {
		t.Fatalf("resolved ids = %v", ids)
	}
	if logs[0].AgentID != "agent-1" || logs[1].AgentID != "agent-2" {
		t.Fatalf("logs = %+v", logs)
	}
	if len(logs[0].ToolCalls) != 1 || logs[0].ToolCalls[0].CallID != "call-sess-1" {
		t.Fatalf("first log = %+v", logs[0])
	}
	if len(logs[1].ToolCalls) != 1 || logs[1].ToolCalls[0].CallID != "call-sess-2" {
		t.Fatalf("second log = %+v", logs[1])
	}
}

func TestDefaultTranscriptLookup(t *testing.T) {
	if defaultTranscriptLookup("cursor") == nil || defaultTranscriptLookup("pi") == nil || defaultTranscriptLookup("opencode") == nil {
		t.Fatal("cursor, pi, and opencode must resolve")
	}
	if defaultTranscriptLookup("claude") != nil || defaultTranscriptLookup("script") != nil ||
		defaultTranscriptLookup("opencodev1") != nil || defaultTranscriptLookup("opencodev2") != nil {
		t.Fatal("claude, script, and the generation-split implementation names must be unsupported")
	}
	got, err := defaultTranscriptLookup("cursor").ResolveTranscript(context.Background(), adapters.TranscriptRequest{})
	if err != nil {
		t.Fatal(err)
	}
	if got.Omitted != adapters.SessionLogOmittedStoreNotFound {
		t.Fatalf("cursor missing id Omitted = %q", got.Omitted)
	}
	piLog, err := defaultTranscriptLookup("pi").ResolveTranscript(context.Background(), adapters.TranscriptRequest{ProviderSessionID: "agent-1"})
	if err != nil {
		t.Fatal(err)
	}
	if piLog.Omitted != adapters.SessionLogOmittedStoreNotFound {
		t.Fatalf("pi missing run dir Omitted = %q", piLog.Omitted)
	}
	openCodeLog, err := defaultTranscriptLookup("opencode").ResolveTranscript(context.Background(), adapters.TranscriptRequest{})
	if err != nil {
		t.Fatal(err)
	}
	if openCodeLog.Omitted != adapters.SessionLogOmittedStoreNotFound {
		t.Fatalf("opencode missing id Omitted = %q", openCodeLog.Omitted)
	}
}

func TestResolveOpenCodeTranscriptUsesSharedProjections(t *testing.T) {
	xdg := t.TempDir()
	t.Setenv("XDG_DATA_HOME", xdg)
	writeOpenCodeExportFixture(t, filepath.Join(xdg, "opencode", "storage"))

	run := hiveview.RunView{
		AgentID:           "agent-1",
		Adapter:           "opencode",
		ProviderSessionID: "ses_export",
		Workspace:         "/tmp/colony",
	}

	// Agent logs resolve through the default lookup, so the caller still asks
	// for adapter `opencode` and gets the same tool-call projection as Cursor.
	log := resolveAgentLog(run, defaultTranscriptLookup, 0)
	if log.Omitted != "" {
		t.Fatalf("Agent log Omitted = %q", log.Omitted)
	}
	if len(log.ToolCalls) != 1 || log.ToolCalls[0].Name != "bash" || log.ToolCalls[0].Args != "go test ./..." {
		t.Fatalf("Agent log calls = %+v", log.ToolCalls)
	}

	// The same lookup feeds the transcript include; the dialogue projection
	// keeps the tool identity but never its input.
	conversation := resolveConversation(run, defaultTranscriptLookup, 0, false)
	if conversation.Omitted != "" {
		t.Fatalf("conversation Omitted = %q", conversation.Omitted)
	}
	if len(conversation.Turns) != 2 {
		t.Fatalf("conversation turns = %+v", conversation.Turns)
	}
	if conversation.Turns[0].Blocks[0].Text != "read main.go" || conversation.Turns[1].Blocks[0].Text != "on it" {
		t.Fatalf("conversation text = %+v", conversation.Turns)
	}
	tools := 0
	for _, turn := range conversation.Turns {
		for _, block := range turn.Blocks {
			if block.Kind != adapters.DialogueBlockTool || block.Tool == nil {
				continue
			}
			tools++
			if block.Tool.Name == "" || block.Tool.Status == "" {
				t.Fatalf("tool identity missing: %+v", block.Tool)
			}
		}
	}
	if tools != 1 {
		t.Fatalf("dialogue tool blocks = %d, want 1", tools)
	}
}

func writeOpenCodeExportFixture(t *testing.T, root string) {
	t.Helper()
	files := map[string]string{
		filepath.Join(root, "session", "project-hash", "ses_export.json"):  `{"id":"ses_export","directory":"/tmp/colony"}`,
		filepath.Join(root, "message", "ses_export", "msg_user.json"):      `{"id":"msg_user","sessionID":"ses_export","role":"user","time":{"created":1000}}`,
		filepath.Join(root, "part", "msg_user", "prt_0001_text.json"):      `{"type":"text","text":"read main.go"}`,
		filepath.Join(root, "message", "ses_export", "msg_assistant.json"): `{"id":"msg_assistant","sessionID":"ses_export","role":"assistant","time":{"created":2000}}`,
		filepath.Join(root, "part", "msg_assistant", "prt_0001_text.json"): `{"type":"text","text":"on it"}`,
		filepath.Join(root, "part", "msg_assistant", "prt_0002_tool.json"): `{"type":"tool","callID":"call-1","tool":"bash","state":{"status":"completed","input":{"command":"go test ./..."},"output":"ok"}}`,
	}
	for path, body := range files {
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, []byte(body), 0o644); err != nil {
			t.Fatal(err)
		}
	}
}

type blockingTranscriptResolver struct {
	release <-chan struct{}
}

func (b blockingTranscriptResolver) ResolveTranscript(context.Context, adapters.TranscriptRequest) (adapters.Transcript, error) {
	<-b.release
	return adapters.Transcript{}, nil
}

type recordingResolver struct {
	seen       *adapters.TranscriptRequest
	transcript adapters.Transcript
}

func (r recordingResolver) ResolveTranscript(_ context.Context, req adapters.TranscriptRequest) (adapters.Transcript, error) {
	*r.seen = req
	return r.transcript, nil
}

// normalizingResolver behaves like a real adapter: it applies the request mask,
// caps, and window before returning.
type normalizingResolver struct {
	transcript adapters.Transcript
}

func (r normalizingResolver) ResolveTranscript(_ context.Context, req adapters.TranscriptRequest) (adapters.Transcript, error) {
	return adapters.NormalizeTranscript(r.transcript, req.Include, req.Limits, req.Window), nil
}

type recordingIDsResolver struct {
	ids *[]string
}

func (r recordingIDsResolver) ResolveTranscript(_ context.Context, req adapters.TranscriptRequest) (adapters.Transcript, error) {
	*r.ids = append(*r.ids, req.ProviderSessionID)
	return toolCallTranscript(adapters.ToolCall{Name: "Read", Args: "a.go", CallID: "call-" + req.ProviderSessionID}), nil
}
