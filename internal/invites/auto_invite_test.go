package invites

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/russ-p/paseka/internal/colony"
	"github.com/russ-p/paseka/internal/colonyinit"
	"github.com/russ-p/paseka/internal/homestate"
	"github.com/russ-p/paseka/internal/protocol"
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

// Energy pause/resume observations describe admission; a rule that names them
// must still never turn one into an invite.
func TestMatchAutoInviteRejectsEnergyObservations(t *testing.T) {
	rule := colony.AutoInviteRule{
		When: colony.EventRule{Type: "SIGNAL", Kind: "energy.paused"},
		Invite: colony.AutoInviteInviteSpec{
			Bee:  colony.InviteStringField{Default: "drone"},
			Task: colony.InviteTaskField{Default: "Review energy pause"},
		},
	}
	raw, _ := json.Marshal(map[string]any{
		"kind":         "energy.paused",
		"episodeId":    "pause-1",
		"remaining":    0,
		"pendingCount": 1,
	})
	ev := protocol.Event{TraceID: "trace-1", Type: protocol.EventSignal, Payload: raw}
	if MatchAutoInvite(ev, rule) {
		t.Fatal("energy.paused must not match an auto-invite rule")
	}

	resumedRule := rule
	resumedRule.When = colony.EventRule{Type: "SIGNAL", Kind: "energy.resumed"}
	raw, _ = json.Marshal(map[string]any{
		"kind":      "energy.resumed",
		"episodeId": "pause-1",
		"source":    "energy.add",
	})
	resumed := protocol.Event{TraceID: "trace-1", Type: protocol.EventSignal, Payload: raw}
	if MatchAutoInvite(resumed, resumedRule) {
		t.Fatal("energy.resumed must not match an auto-invite rule")
	}
}

// Cue ingress reports which invocation ran; it is not work, however broadly a
// rule names SIGNAL events.
func TestMatchAutoInviteRejectsCueIngress(t *testing.T) {
	rule := colony.AutoInviteRule{
		When: colony.EventRule{Type: "SIGNAL"},
		Invite: colony.AutoInviteInviteSpec{
			Bee:  colony.InviteStringField{Default: "drone"},
			Task: colony.InviteTaskField{Default: "Review ingress"},
		},
	}
	raw, _ := json.Marshal(map[string]any{
		"kind":         "cue.ingress",
		"invocationId": "cueinv-1",
		"cueId":        "feature",
		"mode":         "signal",
	})
	ev := protocol.Event{TraceID: "trace-1", Type: protocol.EventSignal, Payload: raw}
	if MatchAutoInvite(ev, rule) {
		t.Fatal("cue.ingress must not match an auto-invite rule")
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
	res, err := colonyinit.Init(colonyinit.InitOptions{StartDir: repo})
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
	invites, err := homestate.ListInvites(res.Slug, protocol.InviteStatusPending, "trace-1")
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
	res, err := colonyinit.Init(colonyinit.InitOptions{StartDir: repo})
	if err != nil {
		t.Fatal(err)
	}
	if err := homestate.UpsertInvite(res.Slug, homestate.InviteEntry{
		InviteID: "inv-existing",
		TraceID:  "trace-1",
		Bee:      "drone",
		Intent:   "grilling",
		Task:     "Review item",
		Status:   protocol.InviteStatusPending,
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
	res, err := colonyinit.Init(colonyinit.InitOptions{StartDir: repo})
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
	res, err := colonyinit.Init(colonyinit.InitOptions{StartDir: repo})
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
	err       error
}

func (s *stubPublisher) PublishEvent(_ context.Context, ev protocol.Event) error {
	if s.err != nil {
		return s.err
	}
	s.published = append(s.published, ev)
	return nil
}
