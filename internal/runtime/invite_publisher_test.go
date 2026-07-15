package runtime

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	"github.com/paseka/paseka/internal/colony"
	"github.com/paseka/paseka/internal/protocol"
)

func TestHandleAutoInviteIgnoresNonMatching(t *testing.T) {
	r := &Reactor{}
	ev, err := protocol.NewEvent("trace-1", "scout", 0, protocol.EventInsight, protocol.NarrativeInsightPayload{
		Kind:    protocol.InsightRunSummary,
		Summary: "ok",
	})
	if err != nil {
		t.Fatal(err)
	}
	if err := r.handleAutoInvite(context.Background(), ev); err != nil {
		t.Fatal(err)
	}
}

func TestHandleAutoInviteEmptyRules(t *testing.T) {
	r := &Reactor{autoInvites: nil}
	classifiedRaw, _ := json.Marshal(map[string]any{
		"kind":     "review.needed",
		"decision": "session",
	})
	ev := protocol.Event{
		TraceID:   "trace-auto",
		Type:      protocol.EventSignal,
		CreatedAt: time.Now().UTC(),
		Payload:   classifiedRaw,
	}
	if err := r.handleAutoInvite(context.Background(), ev); err != nil {
		t.Fatal(err)
	}
}

func TestHandleAutoInviteMatch(t *testing.T) {
	repo := initInviteTestRepo(t)
	res, err := colony.Init(colony.InitOptions{StartDir: repo})
	if err != nil {
		t.Fatal(err)
	}
	stub := &inviteStubPublisher{}
	r := &Reactor{
		colony:          colony.Context{Slug: res.Slug, ColonyRoot: res.ColonyRoot},
		invitePublisher: stub,
		recentLocal:     make(map[string]time.Time),
		autoInvites:     colony.SampleAutoInviteRules(),
	}
	classifiedRaw, _ := json.Marshal(map[string]any{
		"kind":      "review.needed",
		"decision":  "session",
		"rationale": "needs session",
	})
	ev := protocol.Event{
		TraceID:   "trace-auto",
		Type:      protocol.EventSignal,
		CreatedAt: time.Now().UTC(),
		Payload:   classifiedRaw,
	}
	if err := r.handleAutoInvite(context.Background(), ev); err != nil {
		t.Fatal(err)
	}
	if len(stub.published) != 1 {
		t.Fatalf("published = %d", len(stub.published))
	}
	if protocol.PayloadKind(stub.published[0].Payload) != string(protocol.SignalSessionInvite) {
		t.Fatalf("kind = %q", protocol.PayloadKind(stub.published[0].Payload))
	}
	pending, err := colony.ListInvites(res.Slug, colony.InviteStatusPending, "trace-auto")
	if err != nil {
		t.Fatal(err)
	}
	if len(pending) != 1 {
		t.Fatalf("invites = %#v", pending)
	}
	if !r.takeLocalEcho(stub.published[0]) {
		t.Fatal("expected published invite to be remembered for echo skip")
	}
}

type inviteStubPublisher struct {
	published []protocol.Event
}

func (s *inviteStubPublisher) PublishEvent(_ context.Context, ev protocol.Event) error {
	s.published = append(s.published, ev)
	return nil
}
