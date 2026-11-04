package export

import (
	"context"
	"time"

	"github.com/russ-p/paseka/internal/adapters"
	"github.com/russ-p/paseka/internal/hiveview"
)

// ConversationExport is the per-run Conversation view for HTML/Markdown. It is
// the dialogue projection over the resolved transcript, not a second reader.
type ConversationExport struct {
	AgentID string
	Omitted string
	Turns   []adapters.DialogueTurn
}

func resolveConversations(runs []hiveview.RunView, lookup TranscriptLookup, timeout time.Duration) []ConversationExport {
	if lookup == nil {
		lookup = defaultTranscriptLookup
	}
	out := make([]ConversationExport, 0, len(runs))
	for _, run := range runs {
		out = append(out, resolveConversation(run, lookup, timeout))
	}
	return out
}

func resolveConversation(run hiveview.RunView, lookup TranscriptLookup, timeout time.Duration) ConversationExport {
	entry := ConversationExport{AgentID: run.AgentID}
	if run.ProviderSessionID == "" {
		entry.Omitted = adapters.SessionLogOmittedNoProviderSessionID
		return entry
	}
	if lookup == nil {
		lookup = defaultTranscriptLookup
	}
	resolver := lookup(run.Adapter)
	if resolver == nil {
		entry.Omitted = adapters.SessionLogOmittedUnsupported
		return entry
	}
	if timeout <= 0 {
		timeout = defaultTranscriptTimeout
	}
	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()

	transcript := adapters.ResolveTranscript(ctx, resolver, adapters.TranscriptRequest{
		ProviderSessionID: run.ProviderSessionID,
		Adapter:           run.Adapter,
		Workspace:         run.Workspace,
		ColonyRoot:        run.ColonyRoot,
		RunDir:            run.RunDir,
		AgentID:           run.AgentID,
		Include:           adapters.PresetDialogue(),
		Limits:            adapters.DefaultTranscriptLimits(),
	})
	if transcript.Omitted != "" {
		entry.Omitted = transcript.Omitted
		return entry
	}
	entry.Turns = adapters.ProjectDialogue(transcript)
	return entry
}

func conversationByAgentID(conversations []ConversationExport, agentID string) ConversationExport {
	for _, conversation := range conversations {
		if conversation.AgentID == agentID {
			return conversation
		}
	}
	return ConversationExport{AgentID: agentID}
}
