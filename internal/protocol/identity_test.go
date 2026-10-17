package protocol

import (
	"encoding/json"
	"strings"
	"testing"
)

func TestNewEventMintsIdentity(t *testing.T) {
	ev, err := NewEvent("trace-1", "builder", 0, EventInsight, map[string]any{"kind": "seam.note"})
	if err != nil {
		t.Fatal(err)
	}
	if ev.ID == "" {
		t.Fatal("event must carry an identity")
	}
	if len(ev.ID) <= len("evt-") || ev.ID[:4] != "evt-" {
		t.Fatalf("identity = %q, want an evt- prefixed id", ev.ID)
	}
}

func TestNewEventIdentityCarriesEnoughEntropy(t *testing.T) {
	ev, err := NewEvent("trace-1", "builder", 0, EventSignal, nil)
	if err != nil {
		t.Fatal(err)
	}
	body, ok := strings.CutPrefix(ev.ID, eventIDPrefix)
	if !ok {
		t.Fatalf("identity %q is not prefixed", ev.ID)
	}
	// The property, not a format: a body shorter than 128 bits cannot keep two
	// events minted in the same millisecond apart, and the colony deduplicates
	// on this id — so a narrow body loses an event silently. Asserting the width
	// catches that regression every run, where a sampling test caught it in
	// roughly one run out of four.
	if len(body) < 32 {
		t.Fatalf("identity body is %d hex chars, want at least 32 (128 bits): %q", len(body), ev.ID)
	}
}

func TestNewEventMintsDistinctIdentities(t *testing.T) {
	seen := map[string]struct{}{}
	for range 500 {
		ev, err := NewEvent("trace-1", "builder", 0, EventSignal, nil)
		if err != nil {
			t.Fatal(err)
		}
		if _, dup := seen[ev.ID]; dup {
			t.Fatalf("identity %q minted twice", ev.ID)
		}
		seen[ev.ID] = struct{}{}
	}
}

func TestEventIdentityIsNotPartOfTheAgentFacingShape(t *testing.T) {
	// Hand-authored input carries no identity: the colony mints one, and the
	// protocol version is unchanged.
	var in EventInput
	if err := json.Unmarshal([]byte(`{"traceId":"t","type":"SIGNAL","payload":{"kind":"feature.requested"}}`), &in); err != nil {
		t.Fatal(err)
	}
	ev, err := in.ToEvent("cli")
	if err != nil {
		t.Fatal(err)
	}
	if ev.ID == "" {
		t.Fatal("ToEvent must mint an identity")
	}
	if ev.ProtocolVersion != Version {
		t.Fatalf("protocolVersion = %q, want %q", ev.ProtocolVersion, Version)
	}
	if Version != "agent-runtime.v1" {
		t.Fatalf("protocol version changed: %q", Version)
	}
}

func TestHistoricalEventWithoutIdentityParses(t *testing.T) {
	var ev Event
	line := `{"protocolVersion":"agent-runtime.v1","traceId":"t","agentId":"a","seq":3,"type":"SIGNAL","createdAt":"2026-01-01T00:00:00Z"}`
	if err := json.Unmarshal([]byte(line), &ev); err != nil {
		t.Fatal(err)
	}
	if ev.ID != "" {
		t.Fatalf("id = %q, want absent for a record written before identities", ev.ID)
	}
	if ev.Seq != 3 {
		t.Fatalf("seq = %d, want 3", ev.Seq)
	}
}

func TestEventIdentitySurvivesRoundTrip(t *testing.T) {
	ev, err := NewEvent("trace-1", "cli", 0, EventSignal, map[string]any{"kind": "feature.requested"})
	if err != nil {
		t.Fatal(err)
	}
	data, err := json.Marshal(ev)
	if err != nil {
		t.Fatal(err)
	}
	var back Event
	if err := json.Unmarshal(data, &back); err != nil {
		t.Fatal(err)
	}
	if back.ID != ev.ID {
		t.Fatalf("identity changed across the wire: %q vs %q", back.ID, ev.ID)
	}
}
