package proposal_test

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/russ-p/paseka/internal/bus"
	"github.com/russ-p/paseka/internal/patches"
	"github.com/russ-p/paseka/internal/proposal"
	"github.com/russ-p/paseka/internal/protocol"
)

// brokerPublisher stands in for a bus-backed publisher: it acknowledges, and it can be
// told to fail. It is deliberately not `LocalPublisher` or `NopPublisher`, because those
// two are the publishers that cannot confirm anything.
type brokerPublisher struct {
	err    error
	events []protocol.Event
}

func (p *brokerPublisher) PublishEvent(_ context.Context, ev protocol.Event) error {
	if p.err != nil {
		return p.err
	}
	p.events = append(p.events, ev)
	return nil
}

func installRecord(t *testing.T, agentID string) (patches.Store, protocol.Event) {
	t.Helper()
	store := patches.NewStore(t.TempDir(), "trace-1")
	ev, err := protocol.NewEvent("trace-1", agentID, 0, protocol.EventMutation, protocol.MutationPayload{
		Kind: protocol.MutationCodeProposalIsolated,
		Ref:  store.Ref(agentID),
	})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := store.Write(patches.Record{AgentID: agentID, Publication: patches.PublicationUnpublished}, []byte("diff --git a/x b/x\n")); err != nil {
		t.Fatal(err)
	}
	return store, ev
}

// localRecordingPublisher behaves like bus.LocalPublisher: it records the event but
// cannot confirm a broker.
type localRecordingPublisher struct {
	events []protocol.Event
}

func (p *localRecordingPublisher) PublishEvent(_ context.Context, ev protocol.Event) error {
	p.events = append(p.events, ev)
	return nil
}

func (*localRecordingPublisher) ConfirmsPublication() bool { return false }

// A publisher that cannot confirm still receives the event: its job is the local trail
// record, which is what makes the patch and its timeline row appear together. Only the
// publication state stays unpublished.
func TestPublishInvokesAPublisherThatCannotConfirm(t *testing.T) {
	store, ev := installRecord(t, "agent-1")
	pub := &localRecordingPublisher{}

	if err := proposal.Publish(pub, store, "agent-1", ev); err != nil {
		t.Fatal(err)
	}
	if len(pub.events) != 1 || pub.events[0].ID != ev.ID {
		t.Fatalf("the event was not delivered to the file-only publisher: %+v", pub.events)
	}
	if got := readPublication(t, store, "agent-1").Publication; got != patches.PublicationUnpublished {
		t.Fatalf("publication = %q, want unpublished", got)
	}
}

func readPublication(t *testing.T, store patches.Store, agentID string) patches.Record {
	t.Helper()
	rec, ok, err := store.Read(agentID)
	if err != nil || !ok {
		t.Fatalf("read record: ok=%v err=%v", ok, err)
	}
	return rec
}

func TestPublishMarksPublishedAfterAcknowledgement(t *testing.T) {
	store, ev := installRecord(t, "agent-1")
	pub := &brokerPublisher{}

	if err := proposal.Publish(pub, store, "agent-1", ev); err != nil {
		t.Fatal(err)
	}
	rec := readPublication(t, store, "agent-1")
	if rec.Publication != patches.PublicationPublished {
		t.Fatalf("publication = %q, want published", rec.Publication)
	}
	if rec.PublishedAt == nil {
		t.Fatal("a confirmed record must carry a confirmation time")
	}
	if len(pub.events) != 1 {
		t.Fatalf("events = %d, want 1", len(pub.events))
	}
}

// A nil, file-only, or no-op publisher cannot confirm anything, so the record stays
// unpublished rather than claiming a bus event that never left the machine.
func TestPublishLeavesUnpublishedWhenThePublisherCannotConfirm(t *testing.T) {
	for name, pub := range map[string]bus.Publisher{
		"nil":   nil,
		"nop":   bus.NopPublisher{},
		"local": bus.LocalPublisher{ColonyRoot: t.TempDir()},
	} {
		t.Run(name, func(t *testing.T) {
			store, ev := installRecord(t, "agent-1")
			if err := proposal.Publish(pub, store, "agent-1", ev); err != nil {
				t.Fatal(err)
			}
			if got := readPublication(t, store, "agent-1").Publication; got != patches.PublicationUnpublished {
				t.Fatalf("publication = %q, want unpublished", got)
			}
		})
	}
}

// An error after the attempt is uncertain, not a rejection: a published event may already
// reference these bytes, so the record stays and says only that the outcome is unknown.
func TestPublishErrorLeavesAnUnconfirmedRecord(t *testing.T) {
	store, ev := installRecord(t, "agent-1")
	pub := &brokerPublisher{err: errors.New("connection reset")}

	err := proposal.Publish(pub, store, "agent-1", ev)
	if err == nil {
		t.Fatal("a publish error must be reported")
	}
	rec := readPublication(t, store, "agent-1")
	if rec.Publication != patches.PublicationUnconfirmed {
		t.Fatalf("publication = %q, want unconfirmed", rec.Publication)
	}
	if rec.PublishError == "" {
		t.Fatal("the last error must ride with the retained record")
	}
	// The bytes are still there: deleting them would destroy an event's referent.
	if _, err := store.ReadContent("agent-1", patches.MaxPreviewBytes); err != nil {
		t.Fatalf("body was lost: %v", err)
	}
}

func TestPublicationStateMeansWhatWasConfirmedAtCaptureTime(t *testing.T) {
	// A state recorded now is not a claim about the bus later; this test pins the field
	// so a future surface cannot quietly reinterpret it as liveness.
	store, ev := installRecord(t, "agent-1")
	pub := &brokerPublisher{}
	if err := proposal.Publish(pub, store, "agent-1", ev); err != nil {
		t.Fatal(err)
	}
	rec := readPublication(t, store, "agent-1")
	if rec.Publication != patches.PublicationPublished {
		t.Fatalf("publication = %q", rec.Publication)
	}
	if !rec.CapturedAt.Before(time.Now().Add(time.Second)) {
		t.Fatalf("capturedAt = %v", rec.CapturedAt)
	}
}
