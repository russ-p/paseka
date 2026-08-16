package artifacts_test

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/paseka/paseka/internal/artifacts"
	"github.com/paseka/paseka/internal/protocol"
)

type recordingPublisher struct {
	events []protocol.Event
}

func (p *recordingPublisher) PublishEvent(_ context.Context, ev protocol.Event) error {
	p.events = append(p.events, ev)
	return nil
}

func TestWriteAndAnnounce(t *testing.T) {
	root := t.TempDir()
	traceID := "trace-human"
	pub := &recordingPublisher{}
	err := artifacts.WriteAndAnnounce(context.Background(), pub, root, "", traceID, "review-comments.md", artifacts.ProducerConsole, []byte("# Review\n"))
	if err != nil {
		t.Fatal(err)
	}
	if len(pub.events) != 1 {
		t.Fatalf("events = %d", len(pub.events))
	}
	ev := pub.events[0]
	if ev.Type != protocol.EventSignal || protocol.PayloadKind(ev.Payload) != string(protocol.SignalArtifactWritten) {
		t.Fatalf("event = %+v", ev)
	}
	if ev.AgentID != artifacts.ProducerConsole {
		t.Fatalf("agentId = %q", ev.AgentID)
	}
}

func TestWriteAndAnnounceNoEventOnWriteFailure(t *testing.T) {
	root := t.TempDir()
	traceID := "trace-bad"
	pub := &recordingPublisher{}
	err := artifacts.WriteAndAnnounce(context.Background(), pub, root, "", traceID, "../../../escape.md", artifacts.ProducerConsole, []byte("x"))
	if err == nil {
		t.Fatal("expected path escape error")
	}
	if len(pub.events) != 0 {
		t.Fatal("expected no publish on write failure")
	}
}

func TestWriteAndAnnounceCreatesCombFile(t *testing.T) {
	root := t.TempDir()
	traceID := "trace-file"
	pub := &recordingPublisher{}
	if err := artifacts.WriteAndAnnounce(context.Background(), pub, root, "", traceID, "notes.md", artifacts.ProducerConsole, []byte("body")); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(root, ".paseka", "runs", traceID, "artifacts", "notes.md")
	if _, err := os.Stat(path); err != nil {
		t.Fatal(err)
	}
}
