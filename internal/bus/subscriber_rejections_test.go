package bus

import (
	"errors"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/nats-io/nats.go"
	"github.com/russ-p/paseka/internal/protocol"
	"github.com/russ-p/paseka/internal/runs"
)

func TestRedeliveryBackoff(t *testing.T) {
	cases := []struct {
		attempt uint64
		want    time.Duration
	}{
		{0, time.Second},
		{1, time.Second},
		{2, 2 * time.Second},
		{3, 4 * time.Second},
		{4, 8 * time.Second},
		{5, 16 * time.Second},
		{6, subscribeMaxBackoff},
		{99, subscribeMaxBackoff},
	}
	for _, tc := range cases {
		if got := redeliveryBackoff(tc.attempt); got != tc.want {
			t.Errorf("redeliveryBackoff(%d) = %s, want %s", tc.attempt, got, tc.want)
		}
	}
}

func TestDeliveryKeyAndServerCountReadMetadata(t *testing.T) {
	msg := &nats.Msg{
		Sub:   &nats.Subscription{},
		Reply: "$JS.ACK.STREAM.CONSUMER.7.42.7.1600000000000000000.0",
	}
	if got := serverDeliveryCount(msg); got != 7 {
		t.Fatalf("serverDeliveryCount = %d, want 7", got)
	}
	if got := deliveryKey(msg, protocol.Event{}); got != "seq:42" {
		t.Fatalf("deliveryKey = %q, want seq:42", got)
	}
	if got := serverDeliveryCount(&nats.Msg{}); got != 0 {
		t.Fatalf("non-JetStream server deliveries = %d, want 0", got)
	}
	fallback := protocol.Event{ID: "evt-1"}
	if got := deliveryKey(&nats.Msg{Subject: "s"}, fallback); got != "event:evt-1" {
		t.Fatalf("event fallback key = %q", got)
	}
	if got := deliveryKey(&nats.Msg{Subject: "s"}, protocol.Event{}); got != "subject:s" {
		t.Fatalf("subject fallback key = %q", got)
	}
}

func TestDeliveryTrackerCountsPerMessage(t *testing.T) {
	tracker := newDeliveryTracker()
	if got := tracker.next("seq:1"); got != 1 {
		t.Fatalf("first attempt = %d", got)
	}
	if got := tracker.next("seq:1"); got != 2 {
		t.Fatalf("second attempt = %d", got)
	}
	if got := tracker.next("seq:2"); got != 1 {
		t.Fatalf("other message attempt = %d", got)
	}
	tracker.clear("seq:1")
	if got := tracker.next("seq:1"); got != 1 {
		t.Fatalf("attempt after clear = %d", got)
	}
}

func TestRecordRejectionWritesTrailEvidence(t *testing.T) {
	root := t.TempDir()
	c := &Client{cfg: Config{ColonyRoot: root}}
	ev, err := protocol.NewEvent("trace-1", "console", 0, protocol.EventSignal, protocol.TaskStatusPayload{
		Kind:   protocol.TaskEventStatus,
		TaskID: "_review",
		Status: protocol.TaskStatusCancelled,
	})
	if err != nil {
		t.Fatal(err)
	}
	msg := &nats.Msg{Subject: "paseka.test.events.SIGNAL.task.status"}
	c.recordRejection(msg, ev, 5, 114249, errors.New("boom"))

	path := filepath.Join(root, ".paseka", "runs", "trace-1", runs.RejectedEventsFileName)
	summaries, err := runs.ListTrailRejections(root)
	if err != nil {
		t.Fatal(err)
	}
	if len(summaries) != 1 || summaries[0].Count != 1 {
		t.Fatalf("summaries = %+v, want one record at %s", summaries, path)
	}
	if !strings.Contains(summaries[0].LastError, "boom") {
		t.Fatalf("last error = %q", summaries[0].LastError)
	}
}

func TestRecordRejectionSkipsBusOnlyClient(t *testing.T) {
	c := &Client{cfg: Config{}}
	ev, err := protocol.NewEvent("trace-1", "console", 0, protocol.EventSignal, protocol.TaskStatusPayload{
		Kind: protocol.TaskEventStatus, TaskID: "task-1", Status: protocol.TaskStatusCancelled,
	})
	if err != nil {
		t.Fatal(err)
	}
	c.recordRejection(&nats.Msg{Subject: "s"}, ev, 5, 0, errors.New("boom"))
}

func TestRejectedEventWarnings(t *testing.T) {
	root := t.TempDir()
	if warnings := rejectedEventWarnings(""); len(warnings) != 0 {
		t.Fatalf("warnings = %+v, want none", warnings)
	}
	if warnings := rejectedEventWarnings(root); len(warnings) != 0 {
		t.Fatalf("warnings = %+v, want none", warnings)
	}
	trail := runs.Trail{ColonyRoot: root, TraceID: "trace-1"}
	if err := trail.AppendRejection(runs.RejectedEvent{
		TraceID: "trace-1", Subject: "s", Attempts: 5, Error: "work revision missing",
	}); err != nil {
		t.Fatal(err)
	}
	warnings := rejectedEventWarnings(root)
	if len(warnings) != 1 {
		t.Fatalf("warnings = %+v, want one", warnings)
	}
	if !strings.Contains(warnings[0], "trace-1") || !strings.Contains(warnings[0], "work revision missing") {
		t.Fatalf("warning = %q", warnings[0])
	}
}
