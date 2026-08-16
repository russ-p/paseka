package invites

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/paseka/paseka/internal/colony"
	"github.com/paseka/paseka/internal/colonyinit"
	"github.com/paseka/paseka/internal/homestate"
	"github.com/paseka/paseka/internal/protocol"
)

func defaultGrillDoneWhen() *colony.InviteDoneWhen {
	return &colony.InviteDoneWhen{
		When:           colony.EventRule{Type: "SIGNAL", Kind: "spec.ready"},
		RequireFile:    colony.InviteStringField{From: "ref"},
		SetArtifactRef: colony.InviteStringField{From: "ref"},
	}
}

func TestCompleteFromEventMarksCompleted(t *testing.T) {
	repo := initTestRepo(t)
	res, err := colonyinit.Init(colonyinit.InitOptions{StartDir: repo})
	if err != nil {
		t.Fatal(err)
	}
	specPath := filepath.Join(repo, "docs", "specs", "001-test.md")
	if err := os.MkdirAll(filepath.Dir(specPath), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(specPath, []byte("# test\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := homestate.UpsertInvite(res.Slug, homestate.InviteEntry{
		InviteID: "inv-grill",
		TraceID:  "trace-1",
		Bee:      "drone",
		Intent:   "grilling",
		Task:     "Grill feature",
		Status:   protocol.InviteStatusAccepted,
		DoneWhen: defaultGrillDoneWhen(),
	}); err != nil {
		t.Fatal(err)
	}
	raw, _ := json.Marshal(map[string]any{
		"kind": "spec.ready",
		"ref":  "docs/specs/001-test.md",
	})
	ev := protocol.Event{
		TraceID: "trace-1",
		Type:    protocol.EventSignal,
		Payload: raw,
	}
	svc := &Service{Colony: colony.Context{Slug: res.Slug, ColonyRoot: res.ColonyRoot}}
	_, ok, err := svc.CompleteFromEvent(context.Background(), ev)
	if err != nil {
		t.Fatal(err)
	}
	if !ok {
		t.Fatal("expected completion")
	}
	invite, err := homestate.FindInvite(res.Slug, "inv-grill")
	if err != nil {
		t.Fatal(err)
	}
	if invite.Status != protocol.InviteStatusCompleted {
		t.Fatalf("status = %q", invite.Status)
	}
	if invite.ArtifactRef != "docs/specs/001-test.md" {
		t.Fatalf("artifactRef = %q", invite.ArtifactRef)
	}
}

func TestCompleteFromEventMissingFileIncomplete(t *testing.T) {
	repo := initTestRepo(t)
	res, err := colonyinit.Init(colonyinit.InitOptions{StartDir: repo})
	if err != nil {
		t.Fatal(err)
	}
	if err := homestate.UpsertInvite(res.Slug, homestate.InviteEntry{
		InviteID: "inv-grill",
		TraceID:  "trace-1",
		Bee:      "drone",
		Intent:   "grilling",
		Task:     "Grill feature",
		Status:   protocol.InviteStatusAccepted,
		DoneWhen: defaultGrillDoneWhen(),
	}); err != nil {
		t.Fatal(err)
	}
	raw, _ := json.Marshal(map[string]any{
		"kind": "spec.ready",
		"ref":  "docs/specs/missing.md",
	})
	ev := protocol.Event{
		TraceID: "trace-1",
		Type:    protocol.EventSignal,
		Payload: raw,
	}
	svc := &Service{Colony: colony.Context{Slug: res.Slug, ColonyRoot: res.ColonyRoot}}
	_, ok, err := svc.CompleteFromEvent(context.Background(), ev)
	if err != nil {
		t.Fatal(err)
	}
	if !ok {
		t.Fatal("expected update")
	}
	invite, err := homestate.FindInvite(res.Slug, "inv-grill")
	if err != nil {
		t.Fatal(err)
	}
	if invite.Status != protocol.InviteStatusIncomplete {
		t.Fatalf("status = %q", invite.Status)
	}
}

func TestCompleteFromEventUpgradesIncomplete(t *testing.T) {
	repo := initTestRepo(t)
	res, err := colonyinit.Init(colonyinit.InitOptions{StartDir: repo})
	if err != nil {
		t.Fatal(err)
	}
	specPath := filepath.Join(repo, "docs", "specs", "002-test.md")
	if err := os.MkdirAll(filepath.Dir(specPath), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(specPath, []byte("# test\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := homestate.UpsertInvite(res.Slug, homestate.InviteEntry{
		InviteID: "inv-grill",
		TraceID:  "trace-1",
		Bee:      "drone",
		Intent:   "grilling",
		Task:     "Grill feature",
		Status:   protocol.InviteStatusIncomplete,
		DoneWhen: defaultGrillDoneWhen(),
	}); err != nil {
		t.Fatal(err)
	}
	raw, _ := json.Marshal(map[string]any{
		"kind": "spec.ready",
		"ref":  "docs/specs/002-test.md",
	})
	ev := protocol.Event{
		TraceID: "trace-1",
		Type:    protocol.EventSignal,
		Payload: raw,
	}
	svc := &Service{Colony: colony.Context{Slug: res.Slug, ColonyRoot: res.ColonyRoot}}
	_, ok, err := svc.CompleteFromEvent(context.Background(), ev)
	if err != nil {
		t.Fatal(err)
	}
	if !ok {
		t.Fatal("expected completion")
	}
	invite, err := homestate.FindInvite(res.Slug, "inv-grill")
	if err != nil {
		t.Fatal(err)
	}
	if invite.Status != protocol.InviteStatusCompleted {
		t.Fatalf("status = %q", invite.Status)
	}
}

func TestCompleteFromEventNoDoneWhenNoOp(t *testing.T) {
	repo := initTestRepo(t)
	res, err := colonyinit.Init(colonyinit.InitOptions{StartDir: repo})
	if err != nil {
		t.Fatal(err)
	}
	if err := homestate.UpsertInvite(res.Slug, homestate.InviteEntry{
		InviteID: "inv-grill",
		TraceID:  "trace-1",
		Bee:      "drone",
		Intent:   "grilling",
		Status:   protocol.InviteStatusAccepted,
	}); err != nil {
		t.Fatal(err)
	}
	raw, _ := json.Marshal(map[string]any{"kind": "spec.ready", "ref": "docs/specs/x.md"})
	ev := protocol.Event{TraceID: "trace-1", Type: protocol.EventSignal, Payload: raw}
	svc := &Service{Colony: colony.Context{Slug: res.Slug, ColonyRoot: res.ColonyRoot}}
	_, ok, err := svc.CompleteFromEvent(context.Background(), ev)
	if err != nil {
		t.Fatal(err)
	}
	if ok {
		t.Fatal("expected no-op without done_when")
	}
}

func TestCompleteFromEventWrongDoneWhenNoOp(t *testing.T) {
	repo := initTestRepo(t)
	res, err := colonyinit.Init(colonyinit.InitOptions{StartDir: repo})
	if err != nil {
		t.Fatal(err)
	}
	specPath := filepath.Join(repo, "docs", "specs", "003-test.md")
	if err := os.MkdirAll(filepath.Dir(specPath), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(specPath, []byte("# test\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := homestate.UpsertInvite(res.Slug, homestate.InviteEntry{
		InviteID: "inv-bd",
		TraceID:  "trace-1",
		Bee:      "drone",
		Intent:   "breakdown",
		Status:   protocol.InviteStatusAccepted,
		DoneWhen: &colony.InviteDoneWhen{
			When:        colony.EventRule{Type: "SIGNAL", Kind: "task.ready"},
			RequireFile: colony.InviteStringField{From: "ref"},
		},
	}); err != nil {
		t.Fatal(err)
	}
	raw, _ := json.Marshal(map[string]any{"kind": "spec.ready", "ref": "docs/specs/003-test.md"})
	ev := protocol.Event{TraceID: "trace-1", Type: protocol.EventSignal, Payload: raw}
	svc := &Service{Colony: colony.Context{Slug: res.Slug, ColonyRoot: res.ColonyRoot}}
	_, ok, err := svc.CompleteFromEvent(context.Background(), ev)
	if err != nil {
		t.Fatal(err)
	}
	if ok {
		t.Fatal("expected no update for non-matching done_when")
	}
}

func TestMarkInviteIncompleteOnSessionEnd(t *testing.T) {
	repo := initTestRepo(t)
	res, err := colonyinit.Init(colonyinit.InitOptions{StartDir: repo})
	if err != nil {
		t.Fatal(err)
	}
	if err := homestate.UpsertInvite(res.Slug, homestate.InviteEntry{
		InviteID:  "inv-grill",
		TraceID:   "trace-1",
		Bee:       "drone",
		Intent:    "grilling",
		Status:    protocol.InviteStatusAccepted,
		SessionID: "sess-1",
	}); err != nil {
		t.Fatal(err)
	}
	if err := homestate.MarkInviteIncompleteOnSessionEnd(res.Slug, "sess-1"); err != nil {
		t.Fatal(err)
	}
	invite, err := homestate.FindInvite(res.Slug, "inv-grill")
	if err != nil {
		t.Fatal(err)
	}
	if invite.Status != protocol.InviteStatusIncomplete {
		t.Fatalf("status = %q", invite.Status)
	}
}

func TestSampleAutoInviteRulesIncludeDocReady(t *testing.T) {
	rules := colony.SampleAutoInviteRules()
	if len(rules) < 2 {
		t.Fatalf("rules = %d", len(rules))
	}
	if rules[1].When.Kind != "doc.ready" {
		t.Fatalf("second rule kind = %q", rules[1].When.Kind)
	}
}

func TestSampleRuleIncludesDoneWhen(t *testing.T) {
	rules := colony.SampleAutoInviteRules()
	if rules[0].Invite.DoneWhen == nil {
		t.Fatal("expected sample rule done_when")
	}
	if rules[0].Invite.DoneWhen.When.Kind != "doc.ready" {
		t.Fatalf("done_when kind = %q", rules[0].Invite.DoneWhen.When.Kind)
	}
}

func TestAutoInviteFromDocReadyBreakdown(t *testing.T) {
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
	raw, _ := json.Marshal(map[string]any{
		"kind": "doc.ready",
		"ref":  "docs/specs/001-feature.md",
	})
	ev := protocol.Event{
		TraceID: "trace-bd",
		Type:    protocol.EventSignal,
		Payload: raw,
	}
	_, ok, err := svc.AutoInviteFromEvent(context.Background(), ev, colony.SampleAutoInviteRules(), nil)
	if err != nil {
		t.Fatal(err)
	}
	if !ok {
		t.Fatal("expected breakdown invite")
	}
	invites, err := homestate.ListInvites(res.Slug, protocol.InviteStatusPending, "trace-bd")
	if err != nil {
		t.Fatal(err)
	}
	if len(invites) != 1 {
		t.Fatalf("invites = %#v", invites)
	}
	if invites[0].Intent != "breakdown" || invites[0].ArtifactRef != "docs/specs/001-feature.md" {
		t.Fatalf("invite = %#v", invites[0])
	}
}

func TestCompleteFromEventArtifactWrittenCombRef(t *testing.T) {
	repo := initTestRepo(t)
	res, err := colonyinit.Init(colonyinit.InitOptions{StartDir: repo})
	if err != nil {
		t.Fatal(err)
	}
	traceID := "trace-artifact"
	combPath := filepath.Join(repo, ".paseka", "runs", traceID, "artifacts", "handoff.md")
	if err := os.MkdirAll(filepath.Dir(combPath), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(combPath, []byte("# Handoff\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	doneWhen := &colony.InviteDoneWhen{
		When:        colony.EventRule{Type: "SIGNAL", Kind: "artifact.written"},
		RequireFile: colony.InviteStringField{From: "ref"},
	}
	if err := homestate.UpsertInvite(res.Slug, homestate.InviteEntry{
		InviteID: "inv-comb",
		TraceID:  traceID,
		Bee:      "drone",
		Intent:   "grilling",
		Task:     "Comb handoff",
		Status:   protocol.InviteStatusAccepted,
		DoneWhen: doneWhen,
	}); err != nil {
		t.Fatal(err)
	}
	raw, _ := json.Marshal(map[string]any{
		"kind": "artifact.written",
		"artifacts": []map[string]any{{
			"ref":          ".paseka/runs/" + traceID + "/artifacts/handoff.md",
			"artifactKind": "handoff",
		}},
	})
	ev := protocol.Event{
		TraceID: traceID,
		Type:    protocol.EventSignal,
		Payload: raw,
	}
	svc := &Service{Colony: colony.Context{Slug: res.Slug, ColonyRoot: res.ColonyRoot}}
	_, completed, err := svc.CompleteFromEvent(context.Background(), ev)
	if err != nil {
		t.Fatal(err)
	}
	if !completed {
		t.Fatal("expected invite completed on comb artifact.written")
	}
}

func TestBuildInviteIncludesDoneWhen(t *testing.T) {
	ev, _ := classifiedEvent("session", "")
	payload, err := BuildInvite(ev, sampleRules()[0], nil)
	if err != nil {
		t.Fatal(err)
	}
	if payload.DoneWhen == nil || payload.DoneWhen.When.Kind != "doc.ready" {
		t.Fatalf("doneWhen = %#v", payload.DoneWhen)
	}
}
