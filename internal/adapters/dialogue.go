package adapters

import "time"

// DialogueBlockKind identifies what one render-ready dialogue block carries.
type DialogueBlockKind string

const (
	DialogueBlockText DialogueBlockKind = "text"
	DialogueBlockTool DialogueBlockKind = "tool"
)

// DialogueTool is a tool call collapsed to the identity a conversation needs:
// its name and status. Input and output stay in the Agent log projection.
type DialogueTool struct {
	Name   string
	Status string
}

// DialogueBlock is one render-ready line of a dialogue turn. It is either a
// text block (possibly capped) or a one-line tool call.
type DialogueBlock struct {
	Kind      DialogueBlockKind
	Text      string
	Truncated bool
	Tool      *DialogueTool
}

// DialogueTurn is one user or assistant turn, in provider order. Timestamp is
// the provider's and is zero when the provider records none.
type DialogueTurn struct {
	Role      string
	Timestamp time.Time
	Blocks    []DialogueBlock
}

// ProjectDialogue flattens a transcript into render-ready conversation turns. It
// keeps user and assistant turns in provider order, collapses each tool call to
// a single name/status block, and drops reasoning and system turns. It performs
// no IO and knows nothing about any provider.
func ProjectDialogue(t Transcript) []DialogueTurn {
	turns := make([]DialogueTurn, 0, len(t.Turns))
	for _, turn := range t.Turns {
		role := turn.Role
		if role != TurnRoleUser && role != TurnRoleAssistant {
			continue
		}
		next := DialogueTurn{Role: role, Timestamp: turn.Timestamp}
		for _, part := range turn.Parts {
			switch part.Kind {
			case PartText:
				if part.Text == "" {
					continue
				}
				next.Blocks = append(next.Blocks, DialogueBlock{
					Kind:      DialogueBlockText,
					Text:      part.Text,
					Truncated: part.Truncated,
				})
			case PartTool:
				if part.Tool == nil {
					continue
				}
				status := part.Tool.Status
				if status == "" {
					status = ToolStatusUnknown
				}
				next.Blocks = append(next.Blocks, DialogueBlock{
					Kind: DialogueBlockTool,
					Tool: &DialogueTool{Name: part.Tool.Name, Status: status},
				})
			}
		}
		if len(next.Blocks) == 0 {
			continue
		}
		turns = append(turns, next)
	}
	return turns
}
