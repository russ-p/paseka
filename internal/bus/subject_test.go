package bus_test

import (
	"encoding/json"
	"testing"

	"github.com/paseka/paseka/internal/bus"
	"github.com/paseka/paseka/internal/protocol"
)

func TestEventSubjectWithPayloadKind(t *testing.T) {
	payload, _ := json.Marshal(map[string]string{"kind": "task.ready"})
	subject := bus.EventSubject("paseka.demo", protocol.Event{
		Type:    protocol.EventSignal,
		Payload: payload,
	})
	want := "paseka.demo.events.SIGNAL.task.ready"
	if subject != want {
		t.Fatalf("subject = %q, want %q", subject, want)
	}
}

func TestEventSubjectWithoutKind(t *testing.T) {
	subject := bus.EventSubject("paseka.demo", protocol.Event{
		Type: protocol.EventInsight,
	})
	want := "paseka.demo.events.INSIGHT"
	if subject != want {
		t.Fatalf("subject = %q, want %q", subject, want)
	}
}

func TestEventsWildcard(t *testing.T) {
	got := bus.EventsWildcard("paseka.demo")
	if got != "paseka.demo.events.>" {
		t.Fatalf("wildcard = %q", got)
	}
}

func TestConfigFromContextDefaultPrefix(t *testing.T) {
	cfg := bus.ConfigFromContext(colonyCtx("my-slug", "nats://127.0.0.1:4222"), colonyManifest(""))
	if cfg.SubjectPrefix != "paseka.my-slug" {
		t.Fatalf("prefix = %q", cfg.SubjectPrefix)
	}
	if !cfg.Enabled() {
		t.Fatal("expected enabled")
	}
}

func TestNewEventFromCLI(t *testing.T) {
	ev, err := bus.NewEventFromCLI("trace-1", "cli", "SIGNAL", `{"kind":"task.ready","taskId":"task-1"}`)
	if err != nil {
		t.Fatal(err)
	}
	if ev.Type != protocol.EventSignal {
		t.Fatalf("type = %q", ev.Type)
	}
	if ev.TraceID != "trace-1" {
		t.Fatalf("trace = %q", ev.TraceID)
	}
}

func TestNewEventFromCLIRejectsRunLocalType(t *testing.T) {
	_, err := bus.NewEventFromCLI("t", "a", "LOG", `{}`)
	if err == nil {
		t.Fatal("expected error for LOG type")
	}
}
