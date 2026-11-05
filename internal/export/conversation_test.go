package export

import (
	"context"
	"errors"
	"os"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/russ-p/paseka/internal/adapters"
	"github.com/russ-p/paseka/internal/colony"
	"github.com/russ-p/paseka/internal/hiveview"
)

// dialogueTranscript builds the normalized transcript a provider would return
// for a conversation: user and assistant text, reasoning and system turns that
// the dialogue view must drop, and tool payloads it must never carry.
func dialogueTranscript() adapters.Transcript {
	return adapters.Transcript{Turns: []adapters.Turn{
		{Role: adapters.TurnRoleSystem, Timestamp: time.Unix(90, 0).UTC(), Parts: []adapters.Part{
			{Kind: adapters.PartText, Text: "system prompt must stay out"},
		}},
		{Role: adapters.TurnRoleUser, Timestamp: time.Unix(100, 0).UTC(), Parts: []adapters.Part{
			{Kind: adapters.PartText, Text: "please read the file"},
		}},
		{Role: adapters.TurnRoleAssistant, Timestamp: time.Unix(101, 0).UTC(), Parts: []adapters.Part{
			{Kind: adapters.PartReasoning, Reasoning: "private thinking must stay out"},
			{Kind: adapters.PartText, Text: "I will read it."},
			{Kind: adapters.PartTool, Tool: &adapters.ToolPart{
				Name:   "Read",
				CallID: "call-1",
				Status: adapters.ToolStatusOK,
				Input:  `{"path":"secret/lockfile.json"}`,
				Output: "TOOL OUTPUT MUST STAY OUT",
			}},
			{Kind: adapters.PartText, Text: "It is small."},
		}},
	}}
}

func exportWithIncludes(t *testing.T, ctx colony.Context, traceID string, format Format, lookup TranscriptLookup, includes ...string) string {
	t.Helper()
	path, err := ExportTrace(ctx, Options{
		TraceID:     traceID,
		OutputDir:   t.TempDir(),
		Format:      format,
		Include:     mustInclude(t, includes...),
		Transcripts: lookup,
	})
	if err != nil {
		t.Fatalf("ExportTrace: %v", err)
	}
	body, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	return string(body)
}

// conversationSection returns the Markdown conversation section, stopping at the
// next heading of the same or a higher level.
func conversationSection(markdown string) string {
	start := strings.Index(markdown, "#### Conversation")
	if start < 0 {
		return ""
	}
	rest := markdown[start+len("#### Conversation"):]
	if next := strings.Index(rest, "\n#### "); next >= 0 {
		rest = rest[:next]
	}
	if next := strings.Index(rest, "\n### "); next >= 0 {
		rest = rest[:next]
	}
	if next := strings.Index(rest, "\n## "); next >= 0 {
		rest = rest[:next]
	}
	return rest
}

func TestResolveConversationRequestsDialoguePreset(t *testing.T) {
	var seen adapters.TranscriptRequest
	got := resolveConversation(hiveview.RunView{
		AgentID:           "agent-1",
		Adapter:           "cursor",
		ProviderSessionID: "sess",
		Workspace:         "/ws",
		ColonyRoot:        "/colony",
		RunDir:            "/colony/.paseka/runs/t/a",
	}, func(string) adapters.TranscriptResolver {
		return recordingResolver{seen: &seen, transcript: dialogueTranscript()}
	}, 0, false)
	if got.Omitted != "" {
		t.Fatalf("Omitted = %q", got.Omitted)
	}
	if seen.ProviderSessionID != "sess" || seen.Adapter != "cursor" || seen.Workspace != "/ws" || seen.ColonyRoot != "/colony" || seen.RunDir != "/colony/.paseka/runs/t/a" || seen.AgentID != "agent-1" {
		t.Fatalf("association request = %+v", seen)
	}
	if seen.Include != adapters.PresetDialogue() {
		t.Fatalf("include = %+v, want the dialogue preset", seen.Include)
	}
	if seen.Limits != adapters.DefaultTranscriptLimits() {
		t.Fatalf("limits = %+v, want the defaults", seen.Limits)
	}
	if seen.Window != adapters.WholeTranscriptWindow() {
		t.Fatalf("window = %+v, want the whole transcript", seen.Window)
	}
	if len(got.Turns) != 2 {
		t.Fatalf("turns = %+v", got.Turns)
	}
}

// TestResolveConversationRendersPastThePageDefault pins that both transcript
// includes ask for the whole conversation: a dialogue longer than the Console's
// first page still renders every turn.
func TestResolveConversationRendersPastThePageDefault(t *testing.T) {
	count := adapters.DefaultTranscriptPageTurns + 20
	turns := make([]adapters.Turn, 0, count)
	for i := 0; i < count; i++ {
		turns = append(turns, adapters.Turn{
			Role:  adapters.TurnRoleUser,
			Parts: []adapters.Part{{Kind: adapters.PartText, Text: "turn " + strconv.Itoa(i)}},
		})
	}

	for _, reasoning := range []bool{false, true} {
		got := resolveConversation(hiveview.RunView{
			AgentID:           "agent-1",
			Adapter:           "cursor",
			ProviderSessionID: "sess",
		}, func(string) adapters.TranscriptResolver {
			return normalizingResolver{transcript: adapters.Transcript{Turns: turns}}
		}, 0, reasoning)
		if got.Omitted != "" {
			t.Fatalf("reasoning=%v Omitted = %q", reasoning, got.Omitted)
		}
		if len(got.Turns) != count {
			t.Fatalf("reasoning=%v turns = %d, want %d", reasoning, len(got.Turns), count)
		}
	}
}

func TestResolveConversationOmitReasons(t *testing.T) {
	tests := []struct {
		name   string
		run    hiveview.RunView
		lookup TranscriptLookup
		want   string
	}{
		{
			name: "no provider session id",
			run:  hiveview.RunView{AgentID: "agent-1", Adapter: "cursor"},
			want: adapters.SessionLogOmittedNoProviderSessionID,
		},
		{
			name: "unsupported adapter",
			run:  hiveview.RunView{AgentID: "agent-1", Adapter: "script", ProviderSessionID: "sess"},
			want: adapters.SessionLogOmittedUnsupported,
		},
		{
			name: "store not found",
			run:  hiveview.RunView{AgentID: "agent-1", Adapter: "cursor", ProviderSessionID: "sess"},
			lookup: func(string) adapters.TranscriptResolver {
				return fakeTranscriptResolver{transcript: adapters.Transcript{Omitted: adapters.SessionLogOmittedStoreNotFound}}
			},
			want: adapters.SessionLogOmittedStoreNotFound,
		},
		{
			name: "resolver error",
			run:  hiveview.RunView{AgentID: "agent-1", Adapter: "cursor", ProviderSessionID: "sess"},
			lookup: func(string) adapters.TranscriptResolver {
				return fakeTranscriptResolver{err: errors.New("boom")}
			},
			want: adapters.SessionLogOmittedResolveError,
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := resolveConversation(tc.run, tc.lookup, 0, false)
			if got.Omitted != tc.want {
				t.Fatalf("Omitted = %q, want %q", got.Omitted, tc.want)
			}
		})
	}
}

func TestResolveConversationsKeepsRunsSeparate(t *testing.T) {
	var ids []string
	conversations := resolveConversations([]hiveview.RunView{
		{AgentID: "agent-1", Adapter: "cursor", ProviderSessionID: "sess-1"},
		{AgentID: "agent-2", Adapter: "cursor", ProviderSessionID: "sess-2"},
	}, func(string) adapters.TranscriptResolver {
		return recordingDialogueResolver{ids: &ids}
	}, 0, false)
	if len(conversations) != 2 {
		t.Fatalf("conversations = %+v", conversations)
	}
	if len(ids) != 2 || ids[0] != "sess-1" || ids[1] != "sess-2" {
		t.Fatalf("resolved ids = %v", ids)
	}
	if conversations[0].AgentID != "agent-1" || conversations[1].AgentID != "agent-2" {
		t.Fatalf("conversations = %+v", conversations)
	}
	if got := conversations[0].Turns[0].Blocks[0].Text; got != "turn sess-1" {
		t.Fatalf("first conversation text = %q", got)
	}
	if got := conversations[1].Turns[0].Blocks[0].Text; got != "turn sess-2" {
		t.Fatalf("second conversation text = %q", got)
	}
}

func TestExportTraceIncludeTranscriptMarkdown(t *testing.T) {
	ctx, repo := setupExportColony(t)
	traceID := "trace-transcript-md"
	writeCompletedRunCfg(t, repo, traceID, time.Now().UTC().Add(-time.Minute), completedRunCfg{
		ProviderSessionID: "cursor-uuid",
	})

	md := exportWithIncludes(t, ctx, traceID, FormatMarkdown, func(string) adapters.TranscriptResolver {
		return normalizingResolver{transcript: dialogueTranscript()}
	}, "transcript")

	section := conversationSection(md)
	if section == "" {
		t.Fatalf("missing Conversation section:\n%s", md)
	}
	for _, want := range []string{
		"**user**",
		"please read the file",
		"**assistant**",
		"I will read it.",
		"`Read` · ok",
		"It is small.",
	} {
		if !strings.Contains(section, want) {
			t.Fatalf("conversation missing %q:\n%s", want, section)
		}
	}
	for _, unwanted := range []string{
		"system prompt must stay out",
		"private thinking must stay out",
		"secret/lockfile.json",
		"TOOL OUTPUT MUST STAY OUT",
	} {
		if strings.Contains(section, unwanted) {
			t.Fatalf("conversation leaked %q:\n%s", unwanted, section)
		}
	}
}

func TestExportTraceIncludeTranscriptHTML(t *testing.T) {
	ctx, repo := setupExportColony(t)
	traceID := "trace-transcript-html"
	writeCompletedRunCfg(t, repo, traceID, time.Now().UTC().Add(-time.Minute), completedRunCfg{
		ProviderSessionID: "cursor-uuid",
	})

	html := exportWithIncludes(t, ctx, traceID, FormatHTML, func(string) adapters.TranscriptResolver {
		return normalizingResolver{transcript: dialogueTranscript()}
	}, "transcript")

	for _, want := range []string{
		"<h3>Conversation</h3>",
		"please read the file",
		"I will read it.",
		"<code>Read</code>",
		">ok<",
	} {
		if !strings.Contains(html, want) {
			t.Fatalf("HTML missing %q", want)
		}
	}
	for _, unwanted := range []string{
		"system prompt must stay out",
		"private thinking must stay out",
		"secret/lockfile.json",
		"TOOL OUTPUT MUST STAY OUT",
	} {
		if strings.Contains(html, unwanted) {
			t.Fatalf("HTML leaked %q", unwanted)
		}
	}
}

func TestExportTraceWithoutTranscriptIncludeHasNoConversation(t *testing.T) {
	ctx, repo := setupExportColony(t)
	traceID := "trace-transcript-absent"
	writeCompletedRunCfg(t, repo, traceID, time.Now().UTC().Add(-time.Minute), completedRunCfg{
		ProviderSessionID: "cursor-uuid",
	})
	lookup := func(string) adapters.TranscriptResolver {
		return normalizingResolver{transcript: dialogueTranscript()}
	}

	for _, format := range []Format{FormatMarkdown, FormatHTML} {
		body := exportWithIncludes(t, ctx, traceID, format, lookup, "usage")
		marker := "#### Conversation"
		if format == FormatHTML {
			marker = "<h3>Conversation</h3>"
		}
		if strings.Contains(body, marker) {
			t.Fatalf("format %s rendered a Conversation without the include:\n%s", format, body)
		}
		if strings.Contains(body, "please read the file") {
			t.Fatalf("format %s rendered transcript content without the include", format)
		}
	}
}

func TestExportTraceTranscriptAndAgentLogsAreIndependent(t *testing.T) {
	ctx, repo := setupExportColony(t)
	traceID := "trace-transcript-family"
	writeCompletedRunCfg(t, repo, traceID, time.Now().UTC().Add(-time.Minute), completedRunCfg{
		ProviderSessionID: "cursor-uuid",
	})
	lookup := func(string) adapters.TranscriptResolver {
		return normalizingResolver{transcript: dialogueTranscript()}
	}

	tests := []struct {
		name             string
		includes         []string
		wantAgentLog     bool
		wantConversation bool
	}{
		{name: "neither", includes: []string{"usage"}},
		{name: "agent logs only", includes: []string{"agent-logs"}, wantAgentLog: true},
		{name: "transcript only", includes: []string{"transcript"}, wantConversation: true},
		{name: "both", includes: []string{"agent-logs", "transcript"}, wantAgentLog: true, wantConversation: true},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			md := exportWithIncludes(t, ctx, traceID, FormatMarkdown, lookup, tc.includes...)
			if got := strings.Contains(md, "#### Agent log"); got != tc.wantAgentLog {
				t.Fatalf("Agent log present = %v, want %v:\n%s", got, tc.wantAgentLog, md)
			}
			if got := strings.Contains(md, "#### Conversation"); got != tc.wantConversation {
				t.Fatalf("Conversation present = %v, want %v:\n%s", got, tc.wantConversation, md)
			}
		})
	}
}

func TestExportTraceTranscriptToolHeavyHasNoFileDumps(t *testing.T) {
	ctx, repo := setupExportColony(t)
	traceID := "trace-transcript-tools"
	writeCompletedRunCfg(t, repo, traceID, time.Now().UTC().Add(-time.Minute), completedRunCfg{
		ProviderSessionID: "cursor-uuid",
	})
	big := strings.Repeat("lockfile line\n", 200)
	transcript := adapters.Transcript{Turns: []adapters.Turn{
		{Role: adapters.TurnRoleAssistant, Timestamp: time.Unix(100, 0).UTC(), Parts: []adapters.Part{
			{Kind: adapters.PartTool, Tool: &adapters.ToolPart{
				Name:   "Bash",
				CallID: "call-1",
				Status: adapters.ToolStatusError,
				Input:  `{"command":"cat go.sum"}`,
				Output: big,
			}},
			{Kind: adapters.PartText, Text: "That failed."},
		}},
	}}
	md := exportWithIncludes(t, ctx, traceID, FormatMarkdown, func(string) adapters.TranscriptResolver {
		return normalizingResolver{transcript: transcript}
	}, "transcript")

	section := conversationSection(md)
	if !strings.Contains(section, "`Bash` · error") {
		t.Fatalf("tool line missing:\n%s", section)
	}
	if strings.Contains(section, big) || strings.Contains(section, "cat go.sum") {
		t.Fatalf("conversation carried tool payload:\n%s", section)
	}
	if strings.Contains(section, "lockfile line") {
		t.Fatalf("conversation carried tool output:\n%s", section)
	}
}

func TestExportTraceTranscriptMarksTruncation(t *testing.T) {
	ctx, repo := setupExportColony(t)
	traceID := "trace-transcript-truncated"
	writeCompletedRunCfg(t, repo, traceID, time.Now().UTC().Add(-time.Minute), completedRunCfg{
		ProviderSessionID: "cursor-uuid",
	})
	long := strings.Repeat("x", adapters.DefaultMaxTextRunes+50)
	transcript := adapters.Transcript{Turns: []adapters.Turn{
		{Role: adapters.TurnRoleAssistant, Parts: []adapters.Part{{Kind: adapters.PartText, Text: long}}},
	}}

	for _, format := range []Format{FormatMarkdown, FormatHTML} {
		body := exportWithIncludes(t, ctx, traceID, format, func(string) adapters.TranscriptResolver {
			return normalizingResolver{transcript: transcript}
		}, "transcript")
		if !strings.Contains(body, "truncated") {
			t.Fatalf("format %s did not mark truncation", format)
		}
		if strings.Contains(body, long) {
			t.Fatalf("format %s carried the uncapped text", format)
		}
	}
}

func TestExportTraceTranscriptOmittedStillSucceeds(t *testing.T) {
	ctx, repo := setupExportColony(t)
	traceID := "trace-transcript-omitted"
	writeCompletedRunCfg(t, repo, traceID, time.Now().UTC().Add(-time.Minute), completedRunCfg{
		ProviderSessionID: "cursor-uuid",
	})

	md := exportWithIncludes(t, ctx, traceID, FormatMarkdown, func(string) adapters.TranscriptResolver {
		return fakeTranscriptResolver{transcript: adapters.Transcript{Omitted: adapters.SessionLogOmittedStoreNotFound}}
	}, "transcript")
	if !strings.Contains(md, "#### Conversation") {
		t.Fatalf("missing Conversation section:\n%s", md)
	}
	if !strings.Contains(md, "_omitted: store not found_") {
		t.Fatalf("missing one-line omit reason:\n%s", md)
	}
}

func TestExportTraceTranscriptFailedRunStillShowsConversation(t *testing.T) {
	ctx, repo := setupExportColony(t)
	traceID := "trace-transcript-failed"
	writeCompletedRunCfg(t, repo, traceID, time.Now().UTC().Add(-time.Minute), completedRunCfg{
		ProviderSessionID: "cursor-uuid",
		State:             "failed",
	})

	md := exportWithIncludes(t, ctx, traceID, FormatMarkdown, func(string) adapters.TranscriptResolver {
		return normalizingResolver{transcript: dialogueTranscript()}
	}, "transcript")
	if !strings.Contains(md, "**assistant**") || !strings.Contains(md, "I will read it.") {
		t.Fatalf("failed run lost its conversation:\n%s", md)
	}
}

func TestExportTraceTranscriptCancelledRunStillShowsConversation(t *testing.T) {
	ctx, repo := setupExportColony(t)
	traceID := "trace-transcript-cancelled"
	writeCompletedRunCfg(t, repo, traceID, time.Now().UTC().Add(-time.Minute), completedRunCfg{
		ProviderSessionID: "cursor-uuid",
		State:             "cancelled",
	})

	md := exportWithIncludes(t, ctx, traceID, FormatMarkdown, func(string) adapters.TranscriptResolver {
		return normalizingResolver{transcript: dialogueTranscript()}
	}, "transcript")
	if !strings.Contains(md, "**assistant**") || !strings.Contains(md, "I will read it.") {
		t.Fatalf("cancelled run lost its conversation:\n%s", md)
	}
}

func TestExportTraceTranscriptTwoAgentsGetSeparateSections(t *testing.T) {
	ctx, repo := setupExportColony(t)
	traceID := "trace-transcript-two-agents"
	started := time.Now().UTC().Add(-2 * time.Minute)
	writeCompletedRunCfg(t, repo, traceID, started, completedRunCfg{
		AgentID:           "agent-1",
		ProviderSessionID: "sess-1",
	})
	writeCompletedRunCfg(t, repo, traceID, started.Add(time.Minute), completedRunCfg{
		AgentID:           "agent-2",
		ProviderSessionID: "sess-2",
	})

	md := exportWithIncludes(t, ctx, traceID, FormatMarkdown, func(string) adapters.TranscriptResolver {
		return recordingDialogueResolver{}
	}, "transcript")
	if strings.Count(md, "#### Conversation") != 2 {
		t.Fatalf("expected one Conversation per run:\n%s", md)
	}
	for _, want := range []string{"turn sess-1", "turn sess-2"} {
		if !strings.Contains(md, want) {
			t.Fatalf("missing %q:\n%s", want, md)
		}
	}
}

// reasoningTranscript builds the transcript a provider that records thinking
// returns: a system turn and tool payload the dialogue must keep out, and one
// reasoning part visible only when the consumer asks for reasoning.
func reasoningTranscript() adapters.Transcript {
	return adapters.Transcript{Turns: []adapters.Turn{
		{Role: adapters.TurnRoleSystem, Timestamp: time.Unix(90, 0).UTC(), Parts: []adapters.Part{
			{Kind: adapters.PartText, Text: "system prompt must stay out"},
		}},
		{Role: adapters.TurnRoleUser, Timestamp: time.Unix(100, 0).UTC(), Parts: []adapters.Part{
			{Kind: adapters.PartText, Text: "please read the file"},
		}},
		{Role: adapters.TurnRoleAssistant, Timestamp: time.Unix(101, 0).UTC(), Parts: []adapters.Part{
			{Kind: adapters.PartReasoning, Reasoning: "weighing whether the lockfile matters"},
			{Kind: adapters.PartText, Text: "I will read it."},
			{Kind: adapters.PartTool, Tool: &adapters.ToolPart{
				Name:   "Read",
				CallID: "call-1",
				Status: adapters.ToolStatusOK,
				Input:  `{"path":"secret/lockfile.json"}`,
				Output: "TOOL OUTPUT MUST STAY OUT",
			}},
		}},
	}}
}

func TestResolveConversationReasoningRequestsDialogueWithReasoning(t *testing.T) {
	var seen adapters.TranscriptRequest
	got := resolveConversation(hiveview.RunView{
		AgentID:           "agent-1",
		Adapter:           "cursor",
		ProviderSessionID: "sess",
	}, func(string) adapters.TranscriptResolver {
		return recordingResolver{seen: &seen, transcript: reasoningTranscript()}
	}, 0, true)
	if got.Omitted != "" {
		t.Fatalf("Omitted = %q", got.Omitted)
	}
	if seen.Include != adapters.PresetDialogueWithReasoning() {
		t.Fatalf("include = %+v, want the dialogue-with-reasoning preset", seen.Include)
	}
	if len(got.Turns) != 2 {
		t.Fatalf("turns = %+v", got.Turns)
	}
	blocks := got.Turns[1].Blocks
	if len(blocks) != 3 || blocks[0].Kind != adapters.DialogueBlockReasoning {
		t.Fatalf("assistant blocks = %+v", blocks)
	}
	if blocks[0].Reasoning != "weighing whether the lockfile matters" {
		t.Fatalf("reasoning block = %+v", blocks[0])
	}
}

// cursorTranscriptWithoutReasoning is the Cursor case: a transcript with text
// and tools only, because Cursor records no reasoning parts.
func cursorTranscriptWithoutReasoning() adapters.Transcript {
	transcript := dialogueTranscript()
	for i := range transcript.Turns {
		parts := make([]adapters.Part, 0, len(transcript.Turns[i].Parts))
		for _, part := range transcript.Turns[i].Parts {
			if part.Kind == adapters.PartReasoning {
				continue
			}
			parts = append(parts, part)
		}
		transcript.Turns[i].Parts = parts
	}
	return transcript
}

func TestExportTraceIncludeReasoningAloneRendersConversation(t *testing.T) {
	ctx, repo := setupExportColony(t)
	traceID := "trace-reasoning-alone"
	writeCompletedRunCfg(t, repo, traceID, time.Now().UTC().Add(-time.Minute), completedRunCfg{
		ProviderSessionID: "cursor-uuid",
	})
	lookup := func(string) adapters.TranscriptResolver {
		return normalizingResolver{transcript: reasoningTranscript()}
	}

	for _, format := range []Format{FormatMarkdown, FormatHTML} {
		body := exportWithIncludes(t, ctx, traceID, format, lookup, "reasoning")
		marker := "#### Conversation"
		if format == FormatHTML {
			marker = "<h3>Conversation</h3>"
		}
		if strings.Count(body, marker) != 1 {
			t.Fatalf("format %s rendered %d Conversation sections:\n%s", format, strings.Count(body, marker), body)
		}
		for _, want := range []string{
			"please read the file",
			"weighing whether the lockfile matters",
			"I will read it.",
		} {
			if !strings.Contains(body, want) {
				t.Fatalf("format %s missing %q", format, want)
			}
		}
		if strings.Contains(body, "system prompt must stay out") {
			t.Fatalf("format %s carried the system turn", format)
		}
	}
}

func TestExportTraceReasoningAndTranscriptReasoningProduceOneSection(t *testing.T) {
	ctx, repo := setupExportColony(t)
	traceID := "trace-reasoning-equivalent"
	writeCompletedRunCfg(t, repo, traceID, time.Now().UTC().Add(-time.Minute), completedRunCfg{
		ProviderSessionID: "cursor-uuid",
	})
	lookup := func(string) adapters.TranscriptResolver {
		return normalizingResolver{transcript: reasoningTranscript()}
	}

	alone := exportWithIncludes(t, ctx, traceID, FormatMarkdown, lookup, "reasoning")
	both := exportWithIncludes(t, ctx, traceID, FormatMarkdown, lookup, "transcript", "reasoning")
	for name, body := range map[string]string{"reasoning": alone, "transcript+reasoning": both} {
		if got := strings.Count(body, "#### Conversation"); got != 1 {
			t.Fatalf("%s rendered %d Conversation sections:\n%s", name, got, body)
		}
		if !strings.Contains(conversationSection(body), "weighing whether the lockfile matters") {
			t.Fatalf("%s lost its reasoning block:\n%s", name, body)
		}
	}
}

func TestExportTraceTranscriptAloneCarriesNoReasoning(t *testing.T) {
	ctx, repo := setupExportColony(t)
	traceID := "trace-transcript-no-reasoning"
	writeCompletedRunCfg(t, repo, traceID, time.Now().UTC().Add(-time.Minute), completedRunCfg{
		ProviderSessionID: "cursor-uuid",
	})
	lookup := func(string) adapters.TranscriptResolver {
		return normalizingResolver{transcript: reasoningTranscript()}
	}

	for _, format := range []Format{FormatMarkdown, FormatHTML} {
		body := exportWithIncludes(t, ctx, traceID, format, lookup, "transcript")
		if strings.Contains(body, "weighing whether the lockfile matters") {
			t.Fatalf("format %s carried reasoning without the include", format)
		}
		if strings.Contains(body, ">reasoning<") {
			t.Fatalf("format %s rendered a reasoning block without the include", format)
		}
		if !strings.Contains(body, "I will read it.") {
			t.Fatalf("format %s lost the dialogue text", format)
		}
	}
}

func TestExportTraceReasoningCapMarksTruncation(t *testing.T) {
	ctx, repo := setupExportColony(t)
	traceID := "trace-reasoning-truncated"
	writeCompletedRunCfg(t, repo, traceID, time.Now().UTC().Add(-time.Minute), completedRunCfg{
		ProviderSessionID: "cursor-uuid",
	})
	long := strings.Repeat("r", adapters.DefaultMaxReasoningRunes+50)
	transcript := adapters.Transcript{Turns: []adapters.Turn{
		{Role: adapters.TurnRoleAssistant, Parts: []adapters.Part{
			{Kind: adapters.PartReasoning, Reasoning: long},
			{Kind: adapters.PartText, Text: "still here"},
		}},
	}}

	for _, format := range []Format{FormatMarkdown, FormatHTML} {
		body := exportWithIncludes(t, ctx, traceID, format, func(string) adapters.TranscriptResolver {
			return normalizingResolver{transcript: transcript}
		}, "reasoning")
		if !strings.Contains(body, "truncated") {
			t.Fatalf("format %s did not mark reasoning truncation", format)
		}
		if strings.Contains(body, long) {
			t.Fatalf("format %s carried the uncapped reasoning", format)
		}
		if !strings.Contains(body, "still here") {
			t.Fatalf("format %s lost the dialogue text", format)
		}
	}
}

// TestExportTraceReasoningCursorSessionHasNoBlocksOrOmission pins the Cursor
// case: the provider records no reasoning, so the reasoning include renders an
// ordinary dialogue with no thinking blocks and no omission.
func TestExportTraceReasoningCursorSessionHasNoBlocksOrOmission(t *testing.T) {
	ctx, repo := setupExportColony(t)
	traceID := "trace-reasoning-cursor-empty"
	writeCompletedRunCfg(t, repo, traceID, time.Now().UTC().Add(-time.Minute), completedRunCfg{
		ProviderSessionID: "cursor-uuid",
	})
	lookup := func(string) adapters.TranscriptResolver {
		return normalizingResolver{transcript: cursorTranscriptWithoutReasoning()}
	}

	for _, format := range []Format{FormatMarkdown, FormatHTML} {
		body := exportWithIncludes(t, ctx, traceID, format, lookup, "reasoning")
		if strings.Contains(body, "omitted:") {
			t.Fatalf("format %s omitted a reasoning request:\n%s", format, body)
		}
		if strings.Contains(body, ">reasoning<") {
			t.Fatalf("format %s rendered a thinking block for Cursor", format)
		}
		if !strings.Contains(body, "please read the file") || !strings.Contains(body, "I will read it.") {
			t.Fatalf("format %s lost the dialogue", format)
		}
	}
}

// TestExportTraceReasoningRedactsSecrets proves the reasoning include travels
// the shared redaction path instead of bypassing it.
func TestExportTraceReasoningRedactsSecrets(t *testing.T) {
	ctx, repo := setupExportColony(t)
	traceID := "trace-reasoning-redacted"
	writeCompletedRunCfg(t, repo, traceID, time.Now().UTC().Add(-time.Minute), completedRunCfg{
		ProviderSessionID: "cursor-uuid",
	})
	secret := "ghp_abcdefghijklmnopqrstuvwxyz0123456789"
	transcript := adapters.Transcript{Turns: []adapters.Turn{
		{Role: adapters.TurnRoleAssistant, Parts: []adapters.Part{
			{Kind: adapters.PartReasoning, Reasoning: "the operator token is " + secret},
			{Kind: adapters.PartText, Text: "noted"},
		}},
	}}

	for _, format := range []Format{FormatMarkdown, FormatHTML} {
		body := exportWithIncludes(t, ctx, traceID, format, func(string) adapters.TranscriptResolver {
			return normalizingResolver{transcript: transcript}
		}, "reasoning")
		if strings.Contains(body, secret) {
			t.Fatalf("format %s leaked the reasoning secret", format)
		}
		if !strings.Contains(body, "redacted") {
			t.Fatalf("format %s did not render the redaction placeholder", format)
		}
		if !strings.Contains(body, "noted") {
			t.Fatalf("format %s lost the dialogue text", format)
		}
	}
}

type recordingDialogueResolver struct {
	ids *[]string
}

func (r recordingDialogueResolver) ResolveTranscript(_ context.Context, req adapters.TranscriptRequest) (adapters.Transcript, error) {
	if r.ids != nil {
		*r.ids = append(*r.ids, req.ProviderSessionID)
	}
	return adapters.NormalizeTranscript(adapters.Transcript{Turns: []adapters.Turn{
		{Role: adapters.TurnRoleUser, Parts: []adapters.Part{{Kind: adapters.PartText, Text: "turn " + req.ProviderSessionID}}},
	}}, req.Include, req.Limits, req.Window), nil
}
