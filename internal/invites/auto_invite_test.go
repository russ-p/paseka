package invites

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/paseka/paseka/internal/colony"
	"github.com/paseka/paseka/internal/protocol"
)

func sampleRules() []colony.AutoInviteRule {
	return colony.SampleAutoInviteRules()
}

func classifiedEvent(decision, rationale string, events ...protocol.Event) (protocol.Event, []protocol.Event) {
	raw, _ := json.Marshal(map[string]any{
		"kind":      "review.needed",
		"decision":  decision,
		"rationale": rationale,
	})
	ev := protocol.Event{
		TraceID: "trace-1",
		Type:    protocol.EventSignal,
		Payload: raw,
	}
	return ev, events
}

func TestMatchAutoInviteDecision(t *testing.T) {
	ev, _ := classifiedEvent("session", "")
	if !MatchAutoInvite(ev, sampleRules()[0]) {
		t.Fatal("expected session decision to match")
	}
	ev, _ = classifiedEvent("skip", "")
	if MatchAutoInvite(ev, sampleRules()[0]) {
		t.Fatal("expected skip decision to miss")
	}
}

func TestBuildInviteTaskFromTrace(t *testing.T) {
	reqRaw, _ := json.Marshal(map[string]any{
		"kind":  "review.requested",
		"title": "Live bees header",
	})
	events := []protocol.Event{{Type: protocol.EventSignal, Payload: reqRaw}}
	ev, traceEvents := classifiedEvent("session", "fallback", events...)
	payload, err := BuildInvite(ev, sampleRules()[0], traceEvents)
	if err != nil {
		t.Fatal(err)
	}
	if payload.Task != "Review: Live bees header" {
		t.Fatalf("task = %q", payload.Task)
	}
}

func TestBuildInviteTaskFallbackRationale(t *testing.T) {
	ev, _ := classifiedEvent("session", "Needs grilling before breakdown")
	payload, err := BuildInvite(ev, sampleRules()[0], nil)
	if err != nil {
		t.Fatal(err)
	}
	if payload.Task != "Needs grilling before breakdown" {
		t.Fatalf("task = %q", payload.Task)
	}
}

func TestBuildInviteTaskDefault(t *testing.T) {
	ev, _ := classifiedEvent("session", "")
	payload, err := BuildInvite(ev, sampleRules()[0], nil)
	if err != nil {
		t.Fatal(err)
	}
	if payload.Task != "Review item" {
		t.Fatalf("task = %q", payload.Task)
	}
}

func TestBuildInviteDefaultsBeeIntent(t *testing.T) {
	ev, _ := classifiedEvent("session", "")
	payload, err := BuildInvite(ev, sampleRules()[0], nil)
	if err != nil {
		t.Fatal(err)
	}
	if payload.Bee != "drone" || payload.Intent != "grilling" {
		t.Fatalf("payload = %#v", payload)
	}
}

func TestAutoInviteFromEventMatch(t *testing.T) {
	repo := initTestRepo(t)
	res, err := colony.Init(colony.InitOptions{StartDir: repo})
	if err != nil {
		t.Fatal(err)
	}
	stub := &stubPublisher{}
	svc := &Service{
		Colony:    colony.Context{Slug: res.Slug, ColonyRoot: res.ColonyRoot},
		Publisher: stub,
	}
	reqRaw, _ := json.Marshal(map[string]any{"kind": "review.requested", "title": "My idea"})
	events := []protocol.Event{{Type: protocol.EventSignal, Payload: reqRaw}}
	ev, traceEvents := classifiedEvent("session", "needs grilling", events...)
	published, ok, err := svc.AutoInviteFromEvent(context.Background(), ev, sampleRules(), traceEvents)
	if err != nil {
		t.Fatal(err)
	}
	if !ok {
		t.Fatal("expected invite published")
	}
	if protocol.PayloadKind(published.Payload) != string(protocol.SignalSessionInvite) {
		t.Fatalf("kind = %q", protocol.PayloadKind(published.Payload))
	}
	if len(stub.published) != 1 {
		t.Fatalf("published = %d", len(stub.published))
	}
	invites, err := colony.ListInvites(res.Slug, colony.InviteStatusPending, "trace-1")
	if err != nil {
		t.Fatal(err)
	}
	if len(invites) != 1 {
		t.Fatalf("invites = %#v", invites)
	}
	if invites[0].Task != "Review: My idea" {
		t.Fatalf("task = %q", invites[0].Task)
	}
	if invites[0].DoneWhen == nil || invites[0].DoneWhen.When.Kind != "doc.ready" {
		t.Fatalf("doneWhen = %#v", invites[0].DoneWhen)
	}
}

func TestAutoInviteFromEventIdempotent(t *testing.T) {
	repo := initTestRepo(t)
	res, err := colony.Init(colony.InitOptions{StartDir: repo})
	if err != nil {
		t.Fatal(err)
	}
	if err := colony.UpsertInvite(res.Slug, colony.InviteEntry{
		InviteID: "inv-existing",
		TraceID:  "trace-1",
		Bee:      "drone",
		Intent:   "grilling",
		Task:     "Review item",
		Status:   colony.InviteStatusPending,
	}); err != nil {
		t.Fatal(err)
	}
	stub := &stubPublisher{}
	svc := &Service{
		Colony:    colony.Context{Slug: res.Slug, ColonyRoot: res.ColonyRoot},
		Publisher: stub,
	}
	ev, _ := classifiedEvent("session", "")
	_, ok, err := svc.AutoInviteFromEvent(context.Background(), ev, sampleRules(), nil)
	if err != nil {
		t.Fatal(err)
	}
	if ok {
		t.Fatal("expected idempotent skip")
	}
	if len(stub.published) != 0 {
		t.Fatal("expected no publish")
	}
}

func TestAutoInviteSkipsNonMatchingDecision(t *testing.T) {
	repo := initTestRepo(t)
	res, err := colony.Init(colony.InitOptions{StartDir: repo})
	if err != nil {
		t.Fatal(err)
	}
	stub := &stubPublisher{}
	svc := &Service{
		Colony:    colony.Context{Slug: res.Slug, ColonyRoot: res.ColonyRoot},
		Publisher: stub,
	}
	ev, _ := classifiedEvent("reject", "")
	_, ok, err := svc.AutoInviteFromEvent(context.Background(), ev, sampleRules(), nil)
	if err != nil {
		t.Fatal(err)
	}
	if ok {
		t.Fatal("expected skip")
	}
}

func TestAutoInviteEmptyRulesNoInvite(t *testing.T) {
	repo := initTestRepo(t)
	res, err := colony.Init(colony.InitOptions{StartDir: repo})
	if err != nil {
		t.Fatal(err)
	}
	stub := &stubPublisher{}
	svc := &Service{
		Colony:    colony.Context{Slug: res.Slug, ColonyRoot: res.ColonyRoot},
		Publisher: stub,
	}
	ev, _ := classifiedEvent("session", "")
	_, ok, err := svc.AutoInviteFromEvent(context.Background(), ev, nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	if ok {
		t.Fatal("expected no invite with empty rules")
	}
}

type stubPublisher struct {
	published []protocol.Event
}

func (s *stubPublisher) PublishEvent(_ context.Context, ev protocol.Event) error {
	s.published = append(s.published, ev)
	return nil
}
