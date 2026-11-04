package protocol_test

import (
	"encoding/json"
	"testing"
	"time"

	"github.com/russ-p/paseka/internal/protocol"
)

func workCompletedPayload() protocol.TrailWorkCompletedPayload {
	return protocol.TrailWorkCompletedPayload{
		Kind:         protocol.SignalTrailWorkCompleted,
		CompletionID: "completion-r1-abc",
		WorkRevision: 1,
		CompletedAt:  time.Date(2026, 11, 4, 10, 0, 0, 0, time.UTC),
		ResultRef:    ".paseka/runs/trace-1/completions/completion-r1-abc/result.json",
		MetricsRef:   ".paseka/runs/trace-1/completions/completion-r1-abc/metrics.json",
	}
}

func eventInput(t *testing.T, typ protocol.EventType, payload any) protocol.EventInput {
	t.Helper()
	raw, err := json.Marshal(map[string]any{
		"traceId": "trace-1",
		"type":    typ,
		"payload": payload,
	})
	if err != nil {
		t.Fatal(err)
	}
	in, err := protocol.ParseEventInput(raw)
	if err != nil {
		t.Fatal(err)
	}
	return in
}

func TestTrailWorkCompletedValidation(t *testing.T) {
	in := eventInput(t, protocol.EventSignal, workCompletedPayload())
	if details := in.Validate(); len(details) != 0 {
		t.Fatalf("details = %#v", details)
	}
	if !protocol.IsTrailWorkCompletion(protocol.SignalTrailWorkCompleted) {
		t.Fatal("trail.work.completed must be recognized")
	}
	if protocol.IsPlatformObservation(protocol.SignalTrailWorkCompleted) {
		t.Fatal("the milestone is addressable by analysis subscribers, not a platform observation")
	}
}

func TestTrailWorkCompletedRequiresEvidence(t *testing.T) {
	in := eventInput(t, protocol.EventSignal, protocol.TrailWorkCompletedPayload{
		Kind: protocol.SignalTrailWorkCompleted,
	})
	details := in.Validate()
	paths := map[string]bool{}
	for _, detail := range details {
		paths[detail.Path] = true
	}
	for _, want := range []string{
		"payload.completionId",
		"payload.workRevision",
		"payload.completedAt",
		"payload.resultRef",
		"payload.metricsRef",
	} {
		if !paths[want] {
			t.Fatalf("missing %s in %#v", want, details)
		}
	}
}

func TestTrailWorkCompletedRequiresSignalType(t *testing.T) {
	in := eventInput(t, protocol.EventInsight, workCompletedPayload())
	details := in.Validate()
	if len(details) != 1 || details[0].Path != "type" {
		t.Fatalf("details = %#v", details)
	}
}
