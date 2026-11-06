package runtime

import (
	"strconv"
	"testing"
	"time"

	"github.com/russ-p/paseka/internal/protocol"
)

// A dispatch run inside the subscription callback holds the consumer for
// minutes. JetStream redelivers unacked messages after AckWait, so the reactor
// sees its own seed events again long after it published them. The suppression
// window must outlive that, and every redelivered copy must be recognized — not
// only the first one.
func TestLocalEchoSuppressionSurvivesSlowHandler(t *testing.T) {
	r := &Reactor{recentLocal: make(map[string]time.Time)}
	ev := protocol.Event{ID: "evt-slow-handler", TraceID: "trace-1", Type: protocol.EventSignal}

	r.rememberLocalEvent(ev)
	// The handler blocked for far longer than the old two-minute window.
	r.mu.Lock()
	r.recentLocal[localEchoKey(ev)] = time.Now().Add(-10 * time.Minute)
	r.mu.Unlock()

	for copyIndex := 0; copyIndex < 3; copyIndex++ {
		if !r.takeLocalEcho(ev) {
			t.Fatalf("echo copy %d was not suppressed", copyIndex+1)
		}
	}
}

func TestLocalEchoSuppressionExpires(t *testing.T) {
	r := &Reactor{recentLocal: make(map[string]time.Time)}
	ev := protocol.Event{ID: "evt-expired", TraceID: "trace-1", Type: protocol.EventSignal}

	r.rememberLocalEvent(ev)
	r.mu.Lock()
	r.recentLocal[localEchoKey(ev)] = time.Now().Add(-2 * time.Hour)
	r.mu.Unlock()

	if r.takeLocalEcho(ev) {
		t.Fatal("an entry older than the suppression window must not be treated as an echo")
	}
}

// Two events that share a payload are not the same event; only the stable
// identity marks an echo.
func TestLocalEchoUsesEventIdentity(t *testing.T) {
	r := &Reactor{recentLocal: make(map[string]time.Time)}
	first := protocol.Event{ID: "evt-a", TraceID: "trace-1", Type: protocol.EventSignal, CreatedAt: time.Now().UTC()}
	second := first
	second.ID = "evt-b"

	r.rememberLocalEvent(first)
	if r.takeLocalEcho(second) {
		t.Fatal("an event with a different identity must not be suppressed")
	}
	if !r.takeLocalEcho(first) {
		t.Fatal("the published event must still be suppressed")
	}
}

// Keeping matched entries must not let the memory grow without bound.
func TestLocalEchoMemoryIsBounded(t *testing.T) {
	r := &Reactor{recentLocal: make(map[string]time.Time)}
	for i := 0; i < localEchoMaxEntries+100; i++ {
		r.rememberLocalEvent(protocol.Event{ID: "evt-" + strconv.Itoa(i), Type: protocol.EventSignal})
	}
	r.mu.Lock()
	size := len(r.recentLocal)
	r.mu.Unlock()
	if size > localEchoMaxEntries {
		t.Fatalf("suppression memory grew to %d entries, bound is %d", size, localEchoMaxEntries)
	}
}
