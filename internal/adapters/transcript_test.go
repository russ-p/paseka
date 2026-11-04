package adapters

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"
)

func sampleTranscript() Transcript {
	return Transcript{Turns: []Turn{
		{Role: TurnRoleSystem, Parts: []Part{{Kind: PartText, Text: "you are a bee"}}},
		{Role: TurnRoleUser, Timestamp: time.Unix(100, 0).UTC(), Parts: []Part{
			{Kind: PartText, Text: "read it"},
		}},
		{Role: TurnRoleAssistant, Timestamp: time.Unix(101, 0).UTC(), Parts: []Part{
			{Kind: PartText, Text: "on it"},
			{Kind: PartReasoning, Reasoning: "thinking"},
			{Kind: PartTool, Tool: &ToolPart{
				Name:   "Read",
				CallID: "call-1",
				Input:  `{"path":"main.go"}`,
				Output: "package main",
			}},
		}},
	}}
}

func TestPresetsPinTheMask(t *testing.T) {
	if got := PresetToolCall(); !got.ToolInput || got.Text || got.Reasoning || got.ToolOutput || got.System {
		t.Fatalf("tool-call preset = %+v", got)
	}
	if got := PresetDialogue(); !got.Text || got.Reasoning || got.ToolInput || got.ToolOutput || got.System {
		t.Fatalf("dialogue preset = %+v", got)
	}
	if got := PresetDialogueWithReasoning(); !got.Text || !got.Reasoning || got.ToolInput || got.ToolOutput || got.System {
		t.Fatalf("dialogue-with-reasoning preset = %+v", got)
	}
	if got := PresetFull(); !got.Text || !got.Reasoning || !got.ToolInput || !got.ToolOutput || !got.System {
		t.Fatalf("full preset = %+v", got)
	}
}

func TestDefaultLimits(t *testing.T) {
	limits := DefaultTranscriptLimits()
	if limits.Text != 8000 || limits.Reasoning != 16000 || limits.ToolInput != 2000 || limits.ToolOutput != 16000 {
		t.Fatalf("defaults = %+v", limits)
	}
	if limits := (TranscriptLimits{}).withDefaults(); limits != DefaultTranscriptLimits() {
		t.Fatalf("zero limits = %+v, want defaults", limits)
	}
}

func TestNormalizeToolCallPresetHidesDialogueAndOutput(t *testing.T) {
	got := NormalizeTranscript(sampleTranscript(), PresetToolCall(), TranscriptLimits{}, TranscriptWindow{})
	if got.Omitted != "" {
		t.Fatalf("Omitted = %q", got.Omitted)
	}
	if len(got.Turns) != 1 || got.Turns[0].Role != TurnRoleAssistant {
		t.Fatalf("turns = %+v", got.Turns)
	}
	for _, turn := range got.Turns {
		for _, part := range turn.Parts {
			if part.Kind != PartTool || part.Tool == nil {
				t.Fatalf("tool-call preset leaked %+v", part)
			}
			if part.Tool.Output != "" {
				t.Fatalf("tool-call preset populated output: %+v", part.Tool)
			}
			if part.Tool.Input == "" {
				t.Fatalf("tool-call preset must keep input: %+v", part.Tool)
			}
			if part.Tool.Status != ToolStatusUnknown {
				t.Fatalf("status = %q, want %q", part.Tool.Status, ToolStatusUnknown)
			}
		}
	}
}

func TestNormalizeDialoguePresetHidesToolPayloads(t *testing.T) {
	got := NormalizeTranscript(sampleTranscript(), PresetDialogue(), TranscriptLimits{}, TranscriptWindow{})
	if len(got.Turns) != 2 {
		t.Fatalf("turns = %+v", got.Turns)
	}
	for _, turn := range got.Turns {
		for _, part := range turn.Parts {
			if part.Kind == PartReasoning {
				t.Fatalf("dialogue preset leaked reasoning: %+v", part)
			}
			if part.Tool == nil {
				continue
			}
			if part.Tool.Name != "Read" || part.Tool.CallID != "call-1" || part.Tool.Status == "" {
				t.Fatalf("tool identity missing: %+v", part.Tool)
			}
			if part.Tool.Input != "" || part.Tool.Output != "" {
				t.Fatalf("dialogue preset populated payload: %+v", part.Tool)
			}
		}
	}
}

func TestNormalizeFullPresetKeepsEverything(t *testing.T) {
	got := NormalizeTranscript(sampleTranscript(), PresetFull(), TranscriptLimits{}, TranscriptWindow{})
	if len(got.Turns) != 3 || got.Turns[0].Role != TurnRoleSystem {
		t.Fatalf("turns = %+v", got.Turns)
	}
	parts := got.Turns[2].Parts
	if len(parts) != 3 {
		t.Fatalf("parts = %+v", parts)
	}
	if parts[1].Kind != PartReasoning || parts[1].Reasoning != "thinking" {
		t.Fatalf("reasoning = %+v", parts[1])
	}
	if parts[2].Tool == nil || parts[2].Tool.Input == "" || parts[2].Tool.Output != "package main" {
		t.Fatalf("tool = %+v", parts[2])
	}
}

func TestNormalizePreservesProviderOrder(t *testing.T) {
	got := NormalizeTranscript(sampleTranscript(), PresetDialogue(), TranscriptLimits{}, TranscriptWindow{})
	if got.Turns[0].Role != TurnRoleUser || got.Turns[1].Role != TurnRoleAssistant {
		t.Fatalf("order = %+v", got.Turns)
	}
}

func TestNormalizeTruncatesAndFlags(t *testing.T) {
	got := NormalizeTranscript(sampleTranscript(), PresetFull(), TranscriptLimits{Text: 2, Reasoning: 2, ToolInput: 2, ToolOutput: 2}, TranscriptWindow{})
	if !got.Truncated {
		t.Fatal("expected Truncated")
	}
	if got.Turns[1].Parts[0].Text != "re…" {
		t.Fatalf("text = %q", got.Turns[1].Parts[0].Text)
	}
	if got.Turns[2].Parts[2].Tool.Input != `{"…` {
		t.Fatalf("tool input = %q", got.Turns[2].Parts[2].Tool.Input)
	}
}

func TestNormalizeWindowPages(t *testing.T) {
	transcript := sampleTranscript()
	first := NormalizeTranscript(transcript, PresetDialogue(), TranscriptLimits{}, TranscriptWindow{Limit: 1})
	if len(first.Turns) != 1 || !first.HasMore || first.NextCursor == "" {
		t.Fatalf("first page = %+v", first)
	}
	second := NormalizeTranscript(transcript, PresetDialogue(), TranscriptLimits{}, TranscriptWindow{Limit: 1, Cursor: first.NextCursor})
	if len(second.Turns) != 1 || second.HasMore {
		t.Fatalf("second page = %+v", second)
	}
	past := NormalizeTranscript(transcript, PresetDialogue(), TranscriptLimits{}, TranscriptWindow{Limit: 5, Cursor: "turn:9"})
	if len(past.Turns) != 0 || past.Omitted != "" || past.HasMore {
		t.Fatalf("past end = %+v", past)
	}
}

func TestProjectToolCallsSummarizesArguments(t *testing.T) {
	calls := ProjectToolCalls(sampleTranscript())
	if len(calls) != 1 {
		t.Fatalf("calls = %+v", calls)
	}
	if calls[0].Name != "Read" || calls[0].CallID != "call-1" || calls[0].Args != "main.go" {
		t.Fatalf("call = %+v", calls[0])
	}
	if !calls[0].Timestamp.Equal(time.Unix(101, 0).UTC()) {
		t.Fatalf("timestamp = %v", calls[0].Timestamp)
	}
	if got := ProjectToolCalls(Transcript{}); got == nil || len(got) != 0 {
		t.Fatalf("empty projection = %+v", got)
	}
}

func TestSummarizeToolInputFallsBackToJSON(t *testing.T) {
	if got := summarizeToolInput(`{"pattern":"foo","glob":"*.go"}`); !strings.Contains(got, `"pattern":"foo"`) {
		t.Fatalf("args = %q", got)
	}
	if got := summarizeToolInput("main.go"); got != "main.go" {
		t.Fatalf("plain args = %q", got)
	}
	// A capped JSON prefix still projects to the first summary key.
	if got := summarizeToolInput(`{"path":"main.go","contents":"abc`); got != "main.go" {
		t.Fatalf("truncated args = %q", got)
	}
}

func TestAttachToolResultsFoldsByCallID(t *testing.T) {
	turns := sampleTranscript().Turns
	AttachToolResults(turns, []ToolResult{
		{CallID: "call-1", Output: "filled", Status: ToolStatusError},
		{CallID: "missing", Output: "dropped"},
	})
	tool := turns[2].Parts[2].Tool
	if tool.Output != "filled" || tool.Status != ToolStatusError {
		t.Fatalf("tool = %+v", tool)
	}
	if len(turns) != 3 {
		t.Fatalf("results must not become turns: %+v", turns)
	}
}

type blockingResolver struct {
	release <-chan struct{}
}

func (b blockingResolver) ResolveTranscript(context.Context, TranscriptRequest) (Transcript, error) {
	<-b.release
	return Transcript{Turns: []Turn{{Role: TurnRoleUser}}}, nil
}

type errorResolver struct{}

func (errorResolver) ResolveTranscript(context.Context, TranscriptRequest) (Transcript, error) {
	return Transcript{}, errors.New("boom")
}

func TestResolveTranscriptCutsOffBlockedResolver(t *testing.T) {
	release := make(chan struct{})
	defer close(release)

	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Millisecond)
	defer cancel()

	start := time.Now()
	got := ResolveTranscript(ctx, blockingResolver{release: release}, TranscriptRequest{})
	if got.Omitted != SessionLogOmittedTimeout {
		t.Fatalf("Omitted = %q", got.Omitted)
	}
	if elapsed := time.Since(start); elapsed > time.Second {
		t.Fatalf("resolver was not cut off: %v", elapsed)
	}
}

func TestResolveTranscriptMapsErrors(t *testing.T) {
	got := ResolveTranscript(context.Background(), errorResolver{}, TranscriptRequest{})
	if got.Omitted != SessionLogOmittedResolveError {
		t.Fatalf("Omitted = %q", got.Omitted)
	}
	if got := ResolveTranscript(context.Background(), nil, TranscriptRequest{}); got.Omitted != SessionLogOmittedUnsupported {
		t.Fatalf("nil resolver Omitted = %q", got.Omitted)
	}
}

func TestResolveTranscriptPassesRequestThrough(t *testing.T) {
	got := ResolveTranscript(context.Background(), staticResolver{}, TranscriptRequest{ProviderSessionID: "sess"})
	if got.Omitted != "" || len(got.Turns) != 1 {
		t.Fatalf("result = %+v", got)
	}
}

type staticResolver struct{}

func (staticResolver) ResolveTranscript(_ context.Context, req TranscriptRequest) (Transcript, error) {
	return NormalizeTranscript(Transcript{Turns: []Turn{{
		Role:  TurnRoleUser,
		Parts: []Part{{Kind: PartText, Text: req.ProviderSessionID}},
	}}}, PresetDialogue(), TranscriptLimits{}, TranscriptWindow{}), nil
}
