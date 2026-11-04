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

// TestNormalizeRedactsEveryPatternInEveryPart pins the one shared helper: each
// pattern class is masked in text, reasoning, tool input, and tool output, and
// the prose around the secret survives.
func TestNormalizeRedactsEveryPatternInEveryPart(t *testing.T) {
	patterns := []struct {
		name    string
		content string
		leak    string
		anchor  string
	}{
		{
			name:    "authorization header",
			content: "curl https://api.example.test Authorization: Bearer abcdefghijklmnopqrstuvwxyz",
			leak:    "abcdefghijklmnopqrstuvwxyz",
			anchor:  "https://api.example.test",
		},
		{
			name:    "well-known token prefix",
			content: "run ghp_abcdefghijklmnopqrstuvwxyz0123456789 now",
			leak:    "ghp_",
			anchor:  "run ",
		},
		{
			name:    "private key block",
			content: "before -----BEGIN RSA PRIVATE KEY-----\nMIIEexample\n-----END RSA PRIVATE KEY----- after",
			leak:    "MIIEexample",
			anchor:  "before ",
		},
		{
			name:    "secret assignment",
			content: "DB_PASSWORD=hunter2 then docs/guide/cli.md",
			leak:    "hunter2",
			anchor:  "DB_PASSWORD=",
		},
	}

	type probe struct {
		name  string
		part  Part
		value func(Part) string
	}
	probes := []probe{
		{name: "text", part: Part{Kind: PartText}, value: func(p Part) string { return p.Text }},
		{name: "reasoning", part: Part{Kind: PartReasoning}, value: func(p Part) string { return p.Reasoning }},
		{name: "tool input", part: Part{Kind: PartTool, Tool: &ToolPart{Name: "Bash"}}, value: func(p Part) string { return p.Tool.Input }},
		{name: "tool output", part: Part{Kind: PartTool, Tool: &ToolPart{Name: "Bash"}}, value: func(p Part) string { return p.Tool.Output }},
	}

	for _, pattern := range patterns {
		for _, p := range probes {
			t.Run(pattern.name+"/"+p.name, func(t *testing.T) {
				part := p.part
				switch part.Kind {
				case PartText:
					part.Text = pattern.content
				case PartReasoning:
					part.Reasoning = pattern.content
				case PartTool:
					switch p.name {
					case "tool input":
						part.Tool.Input = pattern.content
					default:
						part.Tool.Output = pattern.content
					}
				}
				got := NormalizeTranscript(Transcript{Turns: []Turn{{
					Role:  TurnRoleAssistant,
					Parts: []Part{part},
				}}}, PresetFull(), TranscriptLimits{}, TranscriptWindow{})
				value := p.value(got.Turns[0].Parts[0])
				if strings.Contains(value, pattern.leak) {
					t.Fatalf("secret leaked: %q", value)
				}
				if !strings.Contains(value, "<redacted>") {
					t.Fatalf("secret not marked redacted: %q", value)
				}
				if !strings.Contains(value, pattern.anchor) {
					t.Fatalf("non-secret content changed: %q, want %q", value, pattern.anchor)
				}
			})
		}
	}
}

// TestNormalizeRedactsBeforeCapping pins the ordering: a secret that sits across
// a cap boundary is replaced first, so truncation can never emit its prefix.
func TestNormalizeRedactsBeforeCapping(t *testing.T) {
	const secret = "ghp_abcdefghijklmnopqrstuvwxyz0123456789"
	padding := strings.Repeat("a", 9)
	content := padding + " " + secret

	tests := []struct {
		name  string
		part  Part
		limit func(*TranscriptLimits)
		value func(Part) string
	}{
		{
			name:  "text",
			part:  Part{Kind: PartText, Text: content},
			limit: func(l *TranscriptLimits) { l.Text = 15 },
			value: func(p Part) string { return p.Text },
		},
		{
			name:  "reasoning",
			part:  Part{Kind: PartReasoning, Reasoning: content},
			limit: func(l *TranscriptLimits) { l.Reasoning = 15 },
			value: func(p Part) string { return p.Reasoning },
		},
		{
			name:  "tool input",
			part:  Part{Kind: PartTool, Tool: &ToolPart{Name: "Bash", Input: content}},
			limit: func(l *TranscriptLimits) { l.ToolInput = 15 },
			value: func(p Part) string { return p.Tool.Input },
		},
		{
			name:  "tool output",
			part:  Part{Kind: PartTool, Tool: &ToolPart{Name: "Bash", Output: content}},
			limit: func(l *TranscriptLimits) { l.ToolOutput = 15 },
			value: func(p Part) string { return p.Tool.Output },
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			var limits TranscriptLimits
			tc.limit(&limits)
			got := NormalizeTranscript(Transcript{Turns: []Turn{{
				Role:  TurnRoleAssistant,
				Parts: []Part{tc.part},
			}}}, PresetFull(), limits, TranscriptWindow{})
			value := tc.value(got.Turns[0].Parts[0])
			if strings.Contains(value, "ghp") {
				t.Fatalf("partial secret emitted at the cap boundary: %q", value)
			}
			if !strings.HasPrefix(value, padding) {
				t.Fatalf("ordinary prefix changed: %q", value)
			}
			if !strings.Contains(value, "<red") || !strings.Contains(value, "…") {
				t.Fatalf("expected a capped redaction marker: %q", value)
			}
			if !got.Truncated {
				t.Fatal("expected Truncated")
			}
		})
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
