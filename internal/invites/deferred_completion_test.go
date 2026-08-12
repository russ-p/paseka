package invites

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/paseka/paseka/internal/bus"
	"github.com/paseka/paseka/internal/colony"
	"github.com/paseka/paseka/internal/colonyinit"
	"github.com/paseka/paseka/internal/homestate"
	"github.com/paseka/paseka/internal/protocol"
	"github.com/paseka/paseka/internal/runs"
)

func TestDeferredSpecReadyCompletesInviteOnFlushNotQueue(t *testing.T) {
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

	runDir := runs.Dir{ColonyRoot: res.ColonyRoot, TraceID: "trace-1", AgentID: "agent-1"}
	if err := runDir.Prepare(); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(runDir.RequestPath(), []byte(`{}`), 0o644); err != nil {
		t.Fatal(err)
	}

	raw, _ := json.Marshal(map[string]any{
		"kind": "spec.ready",
		"ref":  "docs/specs/001-test.md",
	})
	queueResult, err := bus.ProcessEventInput(context.Background(), nil, "", []byte(`{"traceId":"trace-1","agentId":"agent-1","type":"SIGNAL","payload":`+string(raw)+`}`), "agent-1", true, true, res.ColonyRoot)
	if err != nil {
		t.Fatal(err)
	}
	if !queueResult.OK || !queueResult.Deferred {
		t.Fatalf("queue result = %#v", queueResult)
	}

	svc := &Service{Colony: colony.Context{Slug: res.Slug, ColonyRoot: res.ColonyRoot}}
	invite, err := homestate.FindInvite(res.Slug, "inv-grill")
	if err != nil {
		t.Fatal(err)
	}
	if invite.Status != protocol.InviteStatusAccepted {
		t.Fatalf("status after queue = %q, want accepted", invite.Status)
	}
	_, ok, err := svc.CompleteFromEvent(context.Background(), protocol.Event{})
	if err != nil {
		t.Fatal(err)
	}
	if ok {
		t.Fatal("expected no invite update without a matching published event")
	}

	pub := &flushInvitePublisher{}
	flushResult, err := bus.FlushPending(context.Background(), pub, res.ColonyRoot, "trace-1", "agent-1", false)
	if err != nil {
		t.Fatal(err)
	}
	if !flushResult.OK || flushResult.Published != 1 {
		t.Fatalf("flush result = %#v", flushResult)
	}
	if len(pub.events) != 1 {
		t.Fatalf("published = %d, want 1", len(pub.events))
	}

	_, ok, err = svc.CompleteFromEvent(context.Background(), pub.events[0])
	if err != nil {
		t.Fatal(err)
	}
	if !ok {
		t.Fatal("expected invite completion after flush publish")
	}
	invite, err = homestate.FindInvite(res.Slug, "inv-grill")
	if err != nil {
		t.Fatal(err)
	}
	if invite.Status != protocol.InviteStatusCompleted {
		t.Fatalf("status after flush = %q", invite.Status)
	}
}

type flushInvitePublisher struct {
	events []protocol.Event
}

func (p *flushInvitePublisher) PublishEvent(_ context.Context, ev protocol.Event) error {
	p.events = append(p.events, ev)
	return nil
}
