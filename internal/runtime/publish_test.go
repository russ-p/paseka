package runtime_test

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/paseka/paseka/internal/adapters"
	"github.com/paseka/paseka/internal/protocol"
	"github.com/paseka/paseka/internal/runtime"
)

type recordingPublisher struct {
	events []protocol.Event
}

func (p *recordingPublisher) PublishEvent(_ context.Context, ev protocol.Event) error {
	p.events = append(p.events, ev)
	return nil
}

func TestDispatchPublishesDomainEvents(t *testing.T) {
	root := t.TempDir()
	writeColony(t, root)

	rec := &recordingAdapter{}
	pub := &recordingPublisher{}
	d := runtime.NewDispatcher()
	d.RegisterAdapter("cursor", rec)
	d.SetPublisher(pub, false)

	events := []protocol.Event{
		mustEvent(t, "trace-abc", "agent-1", protocol.EventInsight, `{"kind":"task.plan","tasks":[]}`),
		mustEvent(t, "trace-abc", "agent-1", protocol.EventLog, `{"message":"skip"}`),
	}
	rec.events = events

	_, err := d.Dispatch(context.Background(), runtime.DispatchRequest{
		ColonyRoot: root,
		Bee:        "builder",
		TraceID:    "trace-abc",
		Task:       "implement auth",
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(pub.events) != 1 {
		t.Fatalf("published %d events, want 1 domain event", len(pub.events))
	}
	if pub.events[0].Type != protocol.EventInsight {
		t.Fatalf("type = %q", pub.events[0].Type)
	}
}

func TestDispatchPublishesMutationForDiff(t *testing.T) {
	root := t.TempDir()
	writeColony(t, root)

	rec := &recordingAdapter{
		result: &adapters.RunResult{
			Status:  "completed",
			Summary: "done",
			Artifacts: []adapters.Artifact{
				{Kind: "diff", Content: "diff --git a/foo b/foo\n"},
			},
		},
	}
	pub := &recordingPublisher{}
	d := runtime.NewDispatcher()
	d.RegisterAdapter("cursor", rec)
	d.SetPublisher(pub, false)

	_, err := d.Dispatch(context.Background(), runtime.DispatchRequest{
		ColonyRoot: root,
		Bee:        "builder",
		TraceID:    "trace-abc",
		Task:       "implement auth",
	})
	if err != nil {
		t.Fatal(err)
	}
	found := false
	for _, ev := range pub.events {
		if ev.Type == protocol.EventMutation {
			found = true
		}
	}
	if !found {
		t.Fatalf("expected MUTATION event, got %+v", pub.events)
	}
}

func TestDispatchSkipsAutoMutationWhenBeeDoesNotDeclare(t *testing.T) {
	root := t.TempDir()
	writeColony(t, root)
	guardYAML := `role: guard
adapter: cursor
prompt_template: builder.md
params:
  model: composer-2.5
  trust: true
  force: true
publishes:
  - type: VERIFICATION
    kind: verification.success
`
	if err := os.WriteFile(filepath.Join(root, ".paseka/bees/guard.yaml"), []byte(guardYAML), 0o644); err != nil {
		t.Fatal(err)
	}

	rec := &recordingAdapter{
		result: &adapters.RunResult{
			Status:  "completed",
			Summary: "approved",
			Artifacts: []adapters.Artifact{
				{Kind: "diff", Content: "diff --git a/foo b/foo\n"},
			},
		},
	}
	pub := &recordingPublisher{}
	d := runtime.NewDispatcher()
	d.RegisterAdapter("cursor", rec)
	d.SetPublisher(pub, false)
	reg, err := runtime.BuildBeeRegistry(root)
	if err != nil {
		t.Fatal(err)
	}
	d.SetBeeRegistry(reg)

	_, err = d.Dispatch(context.Background(), runtime.DispatchRequest{
		ColonyRoot: root,
		Bee:        "guard",
		TraceID:    "trace-abc",
		Task:       "review changes",
	})
	if err != nil {
		t.Fatal(err)
	}
	for _, ev := range pub.events {
		if ev.Type == protocol.EventMutation {
			t.Fatalf("guard should not auto-publish MUTATION, got %+v", pub.events)
		}
	}
}

func mustEvent(t *testing.T, traceID, agentID string, typ protocol.EventType, payload string) protocol.Event {
	t.Helper()
	return protocol.Event{
		ProtocolVersion: protocol.Version,
		TraceID:         traceID,
		AgentID:         agentID,
		Seq:             1,
		Type:            typ,
		Payload:         json.RawMessage(payload),
	}
}
