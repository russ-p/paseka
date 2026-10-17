package runtime

import (
	"strings"
	"testing"

	"github.com/russ-p/paseka/internal/protocol"
)

// A new runtime-generated archive ref hands the Guard the summary and the workspace
// instruction, never the patch bytes and never the archive pointer. Guard reviews the
// working tree with git; a pasted diff neither improves nor scales that review, and
// implying a correlation between the stored bytes and the reviewed state would be a claim
// this archive does not make.
func TestFormatMutationTaskOmitsArchiveRefAndDiff(t *testing.T) {
	payload := protocol.MutationPayload{
		Kind:    protocol.MutationCodeProposalIsolated,
		Ref:     ".paseka/runs/trace-1/proposals/agent-1.patch",
		Summary: "add the export flag",
	}
	body := formatMutationTask(payload)

	if !strings.Contains(body, "add the export flag") {
		t.Fatalf("summary missing from the task: %q", body)
	}
	if strings.Contains(body, ".patch") || strings.Contains(body, "Artifact ref") {
		t.Fatalf("archive pointer leaked into the task: %q", body)
	}
	if !strings.Contains(body, "working tree") {
		t.Fatalf("workspace review guidance missing: %q", body)
	}
}

// An explicit inline diff and a legacy opaque ref keep their previous formatting: neither
// is a runtime archive ref, and dropping third-party proposal input is not a consequence
// of changing where the runtime stores its own.
func TestFormatMutationTaskKeepsExplicitDiffsAndLegacyRefs(t *testing.T) {
	inline := formatMutationTask(protocol.MutationPayload{
		Kind: protocol.MutationCodeProposalIsolated,
		Diff: "diff --git a/x b/x\n+added\n",
	})
	if !strings.Contains(inline, "```diff") || !strings.Contains(inline, "+added") {
		t.Fatalf("explicit inline diff was dropped: %q", inline)
	}

	legacy := formatMutationTask(protocol.MutationPayload{
		Kind: protocol.MutationCodeProposalIsolated,
		Ref:  "trace-1-agent-1.diff",
	})
	if !strings.Contains(legacy, "Artifact ref: trace-1-agent-1.diff") {
		t.Fatalf("legacy ref was reinterpreted: %q", legacy)
	}
}

func TestRunOpenedCodeProposalEventDetectsAnyProposalKind(t *testing.T) {
	events := []protocol.Event{
		{Type: protocol.EventInsight, Payload: []byte(`{"kind":"task.plan"}`)},
		{Type: protocol.EventMutation, Payload: []byte(`{"kind":"code.proposal.root"}`)},
	}
	if !runOpenedCodeProposalEvent(events) {
		t.Fatal("a root proposal emitted by the run must suppress synthesis")
	}
	if runOpenedCodeProposalEvent([]protocol.Event{{Type: protocol.EventInsight, Payload: []byte(`{}`)}}) {
		t.Fatal("a non-proposal event must not suppress synthesis")
	}
}
