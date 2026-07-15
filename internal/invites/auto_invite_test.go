package invites

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/paseka/paseka/internal/colony"
	"github.com/paseka/paseka/internal/protocol"
)

func defaultGrillRules() []colony.AutoInviteRule {
	return colony.DefaultAutoInviteRules()
}

func classifiedEvent(decision, rationale string, events ...protocol.Event) (protocol.Event, []protocol.Event) {
	raw, _ := json.Marshal(map[string]any{
		"kind":      "feature.classified",
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

func TestMatchAutoInviteGrill(t *testing.T) {
	ev, _ := classifiedEvent("grill", "")
	if !MatchAutoInvite(ev, defaultGrillRules()[0]) {
		t.Fatal("expected grill decision to match")
	}
	ev, _ = classifiedEvent("plan", "")
	if MatchAutoInvite(ev, defaultGrillRules()[0]) {
		t.Fatal("expected plan decision to skip")
	}
}

func TestBuildInviteTaskFromTrace(t *testing.T) {
	reqRaw, _ := json.Marshal(map[string]any{
		"kind":  "feature.requested",
		"title": "Live bees header",
	})
	events := []protocol.Event{{Type: protocol.EventSignal, Payload: reqRaw}}
	ev, traceEvents := classifiedEvent("grill", "fallback", events...)
	payload, err := BuildInvite(ev, defaultGrillRules()[0], traceEvents)
	if err != nil {
		t.Fatal(err)
	}
	if payload.Task != "Grill feature: Live bees header" {
		t.Fatalf("task = %q", payload.Task)
	}
}

func TestBuildInviteTaskFallbackRationale(t *testing.T) {
	ev, _ := classifiedEvent("grill", "Needs grilling before breakdown")
	payload, err := BuildInvite(ev, defaultGrillRules()[0], nil)
	if err != nil {
		t.Fatal(err)
	}
	if payload.Task != "Needs grilling before breakdown" {
		t.Fatalf("task = %q", payload.Task)
	}
}

func TestBuildInviteTaskDefault(t *testing.T) {
	ev, _ := classifiedEvent("grill", "")
	payload, err := BuildInvite(ev, defaultGrillRules()[0], nil)
	if err != nil {
		t.Fatal(err)
	}
	if payload.Task != "Grill feature" {
		t.Fatalf("task = %q", payload.Task)
	}
}

func TestBuildInviteDefaultsBeeIntent(t *testing.T) {
	ev, _ := classifiedEvent("grill", "")
	payload, err := BuildInvite(ev, defaultGrillRules()[0], nil)
	if err != nil {
		t.Fatal(err)
	}
	if payload.Bee != "drone" || payload.Intent != "grilling" {
		t.Fatalf("payload = %#v", payload)
	}
}

func TestAutoInviteFromEventGrill(t *testing.T) {
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
	reqRaw, _ := json.Marshal(map[string]any{"kind": "feature.requested", "title": "My idea"})
	events := []protocol.Event{{Type: protocol.EventSignal, Payload: reqRaw}}
	ev, traceEvents := classifiedEvent("grill", "needs grilling", events...)
	published, ok, err := svc.AutoInviteFromEvent(context.Background(), ev, defaultGrillRules(), traceEvents)
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
	if invites[0].Task != "Grill feature: My idea" {
		t.Fatalf("task = %q", invites[0].Task)
	}
	if invites[0].DoneWhen == nil || invites[0].DoneWhen.When.Kind != "spec.ready" {
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
		Task:     "Grill feature",
		Status:   colony.InviteStatusPending,
	}); err != nil {
		t.Fatal(err)
	}
	stub := &stubPublisher{}
	svc := &Service{
		Colony:    colony.Context{Slug: res.Slug, ColonyRoot: res.ColonyRoot},
		Publisher: stub,
	}
	ev, _ := classifiedEvent("grill", "")
	_, ok, err := svc.AutoInviteFromEvent(context.Background(), ev, defaultGrillRules(), nil)
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

func TestAutoInviteSkipsNonGrillDecision(t *testing.T) {
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
	_, ok, err := svc.AutoInviteFromEvent(context.Background(), ev, defaultGrillRules(), nil)
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
	ev, _ := classifiedEvent("grill", "")
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
