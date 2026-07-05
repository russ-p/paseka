package protocol_test

import (
	"testing"

	"github.com/paseka/paseka/internal/protocol"
)

func TestNewEventDefaults(t *testing.T) {
	ev, err := protocol.NewEvent("t1", "a1", 1, protocol.EventInsight, map[string]string{"k": "v"})
	if err != nil {
		t.Fatal(err)
	}
	if ev.ProtocolVersion != protocol.Version {
		t.Fatalf("version = %q", ev.ProtocolVersion)
	}
	if ev.TraceID != "t1" || ev.AgentID != "a1" || ev.Seq != 1 {
		t.Fatalf("ids mismatch: %+v", ev)
	}
}
