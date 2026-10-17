package proposal

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/russ-p/paseka/internal/patches"
	"github.com/russ-p/paseka/internal/protocol"
)

type ackPublisher struct{ events []protocol.Event }

func (p *ackPublisher) PublishEvent(_ context.Context, ev protocol.Event) error {
	p.events = append(p.events, ev)
	return nil
}

// The event reaches the broker, so the record must stay; only the confirmation note
// failed, and that failure is reported rather than hidden behind a silent success. On disk
// the record remains conservatively unconfirmed.
func TestPublishReportsAConfirmationWriteFailureAndKeepsTheRecord(t *testing.T) {
	store := patches.NewStore(t.TempDir(), "trace-1")
	if _, err := store.Write(patches.Record{AgentID: "agent-1"}, []byte("body")); err != nil {
		t.Fatal(err)
	}
	ev, err := protocol.NewEvent("trace-1", "agent-1", 0, protocol.EventMutation, protocol.MutationPayload{
		Kind: protocol.MutationCodeProposalIsolated,
		Ref:  store.Ref("agent-1"),
	})
	if err != nil {
		t.Fatal(err)
	}

	pub := &ackPublisher{}
	var calls int
	mark := func(agentID, state string, at time.Time, errMsg string) error {
		calls++
		if state == patches.PublicationPublished {
			return errors.New("disk full")
		}
		return store.MarkPublication(agentID, state, at, errMsg)
	}

	err = publish(pub, store, "agent-1", ev, mark)
	if err == nil {
		t.Fatal("a failed confirmation write must be reported")
	}
	if len(pub.events) != 1 {
		t.Fatalf("the event should have been published first; events = %d", len(pub.events))
	}
	if calls != 2 {
		t.Fatalf("mark calls = %d, want unconfirmed then published", calls)
	}
	rec, ok, readErr := store.Read("agent-1")
	if readErr != nil || !ok {
		t.Fatalf("record was lost: ok=%v err=%v", ok, readErr)
	}
	if rec.Publication != patches.PublicationUnconfirmed {
		t.Fatalf("publication = %q, want conservatively unconfirmed", rec.Publication)
	}
}

// A failure to record the attempt itself happens before any event is sent, so no ref is
// published against a record that is not readable.
func TestPublishRefusesToPublishWhenTheAttemptCannotBeRecorded(t *testing.T) {
	store := patches.NewStore(t.TempDir(), "trace-1")
	ev, err := protocol.NewEvent("trace-1", "agent-1", 0, protocol.EventMutation, protocol.MutationPayload{
		Kind: protocol.MutationCodeProposalIsolated,
		Ref:  store.Ref("agent-1"),
	})
	if err != nil {
		t.Fatal(err)
	}
	pub := &ackPublisher{}
	mark := func(string, string, time.Time, string) error { return errors.New("no store") }

	if err := publish(pub, store, "agent-1", ev, mark); err == nil {
		t.Fatal("expected an error")
	}
	if len(pub.events) != 0 {
		t.Fatal("no event may be published when the record could not be prepared")
	}
}
