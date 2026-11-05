package adapters

import (
	"testing"
	"time"
)

func TestProjectDialogueKeepsOrderAndCollapsesTools(t *testing.T) {
	transcript := Transcript{Turns: []Turn{
		{Role: TurnRoleSystem, Parts: []Part{{Kind: PartText, Text: "system prompt"}}},
		{Role: TurnRoleUser, Timestamp: time.Unix(100, 0).UTC(), Parts: []Part{
			{Kind: PartText, Text: "read the file"},
		}},
		{Role: TurnRoleAssistant, Timestamp: time.Unix(101, 0).UTC(), Parts: []Part{
			{Kind: PartReasoning, Reasoning: "hidden thinking"},
			{Kind: PartText, Text: "reading it"},
			{Kind: PartTool, Tool: &ToolPart{
				Name:   "Read",
				CallID: "call-1",
				Status: ToolStatusOK,
				Input:  `{"path":"main.go"}`,
				Output: "package main",
			}},
			{Kind: PartText, Text: "done"},
		}},
	}}

	turns := ProjectDialogue(transcript)
	if len(turns) != 2 {
		t.Fatalf("turns = %+v", turns)
	}
	if turns[0].Role != TurnRoleUser || turns[0].Timestamp != time.Unix(100, 0).UTC() {
		t.Fatalf("first turn = %+v", turns[0])
	}
	if len(turns[0].Blocks) != 1 || turns[0].Blocks[0].Text != "read the file" {
		t.Fatalf("first turn blocks = %+v", turns[0].Blocks)
	}
	if turns[1].Role != TurnRoleAssistant {
		t.Fatalf("second role = %q", turns[1].Role)
	}
	if len(turns[1].Blocks) != 3 {
		t.Fatalf("assistant blocks = %+v", turns[1].Blocks)
	}
	if turns[1].Blocks[0].Kind != DialogueBlockText || turns[1].Blocks[0].Text != "reading it" {
		t.Fatalf("block 0 = %+v", turns[1].Blocks[0])
	}
	tool := turns[1].Blocks[1]
	if tool.Kind != DialogueBlockTool || tool.Tool == nil || tool.Tool.Name != "Read" || tool.Tool.Status != ToolStatusOK {
		t.Fatalf("tool block = %+v", tool)
	}
	if turns[1].Blocks[2].Text != "done" {
		t.Fatalf("block 2 = %+v", turns[1].Blocks[2])
	}
	for _, turn := range turns {
		for _, block := range turn.Blocks {
			if block.Kind == DialogueBlockTool && block.Tool != nil {
				// The dialogue block has no fields for arguments or results.
				if block.Tool.Name == "" || block.Tool.Status == "" {
					t.Fatalf("tool identity missing: %+v", block.Tool)
				}
			}
		}
	}
}

func TestProjectDialogueMarksTruncatedText(t *testing.T) {
	transcript := NormalizeTranscript(Transcript{Turns: []Turn{
		{Role: TurnRoleAssistant, Parts: []Part{{Kind: PartText, Text: "abcdef"}}},
	}}, PresetDialogue(), TranscriptLimits{Text: 3}, TranscriptWindow{})

	turns := ProjectDialogue(transcript)
	if len(turns) != 1 || len(turns[0].Blocks) != 1 {
		t.Fatalf("turns = %+v", turns)
	}
	if !turns[0].Blocks[0].Truncated {
		t.Fatalf("truncation missing: %+v", turns[0].Blocks[0])
	}
	if turns[0].Blocks[0].Text != "abc…" {
		t.Fatalf("text = %q", turns[0].Blocks[0].Text)
	}
}

func TestProjectDialogueDefaultsUnknownToolStatus(t *testing.T) {
	turns := ProjectDialogue(Transcript{Turns: []Turn{
		{Role: TurnRoleAssistant, Parts: []Part{{Kind: PartTool, Tool: &ToolPart{Name: "Bash"}}}},
	}})
	if len(turns) != 1 || len(turns[0].Blocks) != 1 || turns[0].Blocks[0].Tool == nil {
		t.Fatalf("turns = %+v", turns)
	}
	if got := turns[0].Blocks[0].Tool.Status; got != ToolStatusUnknown {
		t.Fatalf("status = %q, want %q", got, ToolStatusUnknown)
	}
}

func TestProjectDialogueSkipsEmptyAndNonDialogueTurns(t *testing.T) {
	turns := ProjectDialogue(Transcript{Turns: []Turn{
		{Role: TurnRoleSystem, Parts: []Part{{Kind: PartText, Text: "system"}}},
		{Role: TurnRoleAssistant, Parts: []Part{{Kind: PartReasoning, Reasoning: "only thinking"}}},
		{Role: TurnRoleUser, Parts: []Part{{Kind: PartText, Text: "hi"}}},
	}})
	if len(turns) != 1 || turns[0].Role != TurnRoleUser {
		t.Fatalf("turns = %+v", turns)
	}
}

// TestProjectDialogueWithReasoningKeepsThinking pins the opt-in projection: the
// reasoning request keeps thinking blocks, and a thinking-only assistant turn
// still renders because the reasoning block is the payload the consumer asked for.
func TestProjectDialogueWithReasoningKeepsThinking(t *testing.T) {
	transcript := NormalizeTranscript(Transcript{Turns: []Turn{
		{Role: TurnRoleSystem, Parts: []Part{{Kind: PartText, Text: "system prompt"}}},
		{Role: TurnRoleAssistant, Parts: []Part{
			{Kind: PartReasoning, Reasoning: "weighing the options"},
			{Kind: PartText, Text: "decided"},
		}},
		{Role: TurnRoleAssistant, Parts: []Part{{Kind: PartReasoning, Reasoning: "only thinking"}}},
	}}, PresetDialogueWithReasoning(), TranscriptLimits{}, TranscriptWindow{})

	turns := ProjectDialogueWithReasoning(transcript)
	if len(turns) != 2 {
		t.Fatalf("turns = %+v", turns)
	}
	first := turns[0].Blocks
	if len(first) != 2 || first[0].Kind != DialogueBlockReasoning || first[0].Reasoning != "weighing the options" {
		t.Fatalf("first blocks = %+v", first)
	}
	if first[1].Kind != DialogueBlockText || first[1].Text != "decided" {
		t.Fatalf("second block = %+v", first[1])
	}
	if len(turns[1].Blocks) != 1 || turns[1].Blocks[0].Kind != DialogueBlockReasoning {
		t.Fatalf("thinking-only turn = %+v", turns[1])
	}
}

func TestProjectDialogueMarksTruncatedReasoning(t *testing.T) {
	transcript := NormalizeTranscript(Transcript{Turns: []Turn{
		{Role: TurnRoleAssistant, Parts: []Part{{Kind: PartReasoning, Reasoning: "abcdef"}}},
	}}, PresetDialogueWithReasoning(), TranscriptLimits{Reasoning: 3}, TranscriptWindow{})

	turns := ProjectDialogueWithReasoning(transcript)
	if len(turns) != 1 || len(turns[0].Blocks) != 1 {
		t.Fatalf("turns = %+v", turns)
	}
	block := turns[0].Blocks[0]
	if block.Kind != DialogueBlockReasoning || block.Reasoning != "abc…" || !block.Truncated {
		t.Fatalf("reasoning block = %+v", block)
	}
}
