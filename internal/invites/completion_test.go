package invites

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/paseka/paseka/internal/colony"
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
	res, err := colony.Init(colony.InitOptions{StartDir: repo})
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
	if err := colony.UpsertInvite(res.Slug, colony.InviteEntry{
		InviteID: "inv-grill",
		TraceID:  "trace-1",
		Bee:      "drone",
		Intent:   "grilling",
		Task:     "Grill feature",
		Status:   colony.InviteStatusAccepted,
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
	invite, err := colony.FindInvite(res.Slug, "inv-grill")
	if err != nil {
		t.Fatal(err)
	}
	if invite.Status != colony.InviteStatusCompleted {
		t.Fatalf("status = %q", invite.Status)
	}
	if invite.ArtifactRef != "docs/specs/001-test.md" {
		t.Fatalf("artifactRef = %q", invite.ArtifactRef)
	}
}

func TestCompleteFromEventMissingFileIncomplete(t *testing.T) {
	repo := initTestRepo(t)
	res, err := colony.Init(colony.InitOptions{StartDir: repo})
	if err != nil {
		t.Fatal(err)
	}
	if err := colony.UpsertInvite(res.Slug, colony.InviteEntry{
		InviteID: "inv-grill",
		TraceID:  "trace-1",
		Bee:      "drone",
		Intent:   "grilling",
		Task:     "Grill feature",
		Status:   colony.InviteStatusAccepted,
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
	invite, err := colony.FindInvite(res.Slug, "inv-grill")
	if err != nil {
		t.Fatal(err)
	}
	if invite.Status != colony.InviteStatusIncomplete {
		t.Fatalf("status = %q", invite.Status)
	}
}

func TestCompleteFromEventUpgradesIncomplete(t *testing.T) {
	repo := initTestRepo(t)
	res, err := colony.Init(colony.InitOptions{StartDir: repo})
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
	if err := colony.UpsertInvite(res.Slug, colony.InviteEntry{
		InviteID: "inv-grill",
		TraceID:  "trace-1",
		Bee:      "drone",
		Intent:   "grilling",
		Task:     "Grill feature",
		Status:   colony.InviteStatusIncomplete,
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
	invite, err := colony.FindInvite(res.Slug, "inv-grill")
	if err != nil {
		t.Fatal(err)
	}
	if invite.Status != colony.InviteStatusCompleted {
		t.Fatalf("status = %q", invite.Status)
	}
}

func TestCompleteFromEventNoDoneWhenNoOp(t *testing.T) {
	repo := initTestRepo(t)
	res, err := colony.Init(colony.InitOptions{StartDir: repo})
	if err != nil {
		t.Fatal(err)
	}
	if err := colony.UpsertInvite(res.Slug, colony.InviteEntry{
		InviteID: "inv-grill",
		TraceID:  "trace-1",
		Bee:      "drone",
		Intent:   "grilling",
		Status:   colony.InviteStatusAccepted,
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
	res, err := colony.Init(colony.InitOptions{StartDir: repo})
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
	if err := colony.UpsertInvite(res.Slug, colony.InviteEntry{
		InviteID: "inv-bd",
		TraceID:  "trace-1",
		Bee:      "drone",
		Intent:   "breakdown",
		Status:   colony.InviteStatusAccepted,
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
	res, err := colony.Init(colony.InitOptions{StartDir: repo})
	if err != nil {
		t.Fatal(err)
	}
	if err := colony.UpsertInvite(res.Slug, colony.InviteEntry{
		InviteID:  "inv-grill",
		TraceID:   "trace-1",
		Bee:       "drone",
		Intent:    "grilling",
		Status:    colony.InviteStatusAccepted,
		SessionID: "sess-1",
	}); err != nil {
		t.Fatal(err)
	}
	if err := colony.MarkInviteIncompleteOnSessionEnd(res.Slug, "sess-1"); err != nil {
		t.Fatal(err)
	}
	invite, err := colony.FindInvite(res.Slug, "inv-grill")
	if err != nil {
		t.Fatal(err)
	}
	if invite.Status != colony.InviteStatusIncomplete {
		t.Fatalf("status = %q", invite.Status)
	}
}

func TestDefaultAutoInviteRulesIncludeSpecReady(t *testing.T) {
	rules := colony.DefaultAutoInviteRules()
	if len(rules) < 2 {
		t.Fatalf("rules = %d", len(rules))
	}
	if rules[1].When.Kind != "spec.ready" {
		t.Fatalf("second rule kind = %q", rules[1].When.Kind)
	}
}

func TestDefaultGrillRuleIncludesDoneWhen(t *testing.T) {
	rules := colony.DefaultAutoInviteRules()
	if rules[0].Invite.DoneWhen == nil {
		t.Fatal("expected grill rule done_when")
	}
	if rules[0].Invite.DoneWhen.When.Kind != "spec.ready" {
		t.Fatalf("done_when kind = %q", rules[0].Invite.DoneWhen.When.Kind)
	}
}

func TestAutoInviteFromSpecReadyBreakdown(t *testing.T) {
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
	raw, _ := json.Marshal(map[string]any{
		"kind": "spec.ready",
		"ref":  "docs/specs/001-feature.md",
	})
	ev := protocol.Event{
		TraceID: "trace-bd",
		Type:    protocol.EventSignal,
		Payload: raw,
	}
	_, ok, err := svc.AutoInviteFromEvent(context.Background(), ev, colony.DefaultAutoInviteRules(), nil)
	if err != nil {
		t.Fatal(err)
	}
	if !ok {
		t.Fatal("expected breakdown invite")
	}
	invites, err := colony.ListInvites(res.Slug, colony.InviteStatusPending, "trace-bd")
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

func TestBuildInviteIncludesDoneWhen(t *testing.T) {
	ev, _ := classifiedEvent("grill", "")
	payload, err := BuildInvite(ev, defaultGrillRules()[0], nil)
	if err != nil {
		t.Fatal(err)
	}
	if payload.DoneWhen == nil || payload.DoneWhen.When.Kind != "spec.ready" {
		t.Fatalf("doneWhen = %#v", payload.DoneWhen)
	}
}
