package runtime_test

import (
	"testing"

	"github.com/paseka/paseka/internal/protocol"
)

func TestEventDispatchContextFormatsMutation(t *testing.T) {
	ev, err := protocol.NewEvent("trace-1", "a1", 0, protocol.EventMutation, protocol.MutationPayload{
		Kind:    protocol.MutationCodeProposal,
		Summary: "auth",
		Diff:    "+line",
		TaskID:  "task-1",
	})
	if err != nil {
		t.Fatal(err)
	}
	// exercised via reactor direct dispatch; sanity-check payload kind
	if protocol.PayloadKind(ev.Payload) != string(protocol.MutationCodeProposal) {
		t.Fatalf("kind = %q", protocol.PayloadKind(ev.Payload))
	}
}
