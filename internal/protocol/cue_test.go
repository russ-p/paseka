package protocol_test

import (
	"encoding/json"
	"testing"
	"time"

	"github.com/russ-p/paseka/internal/protocol"
)

func TestIsPlatformObservation(t *testing.T) {
	for _, kind := range []string{
		protocol.SignalCueIngress,
		string(protocol.SignalEnergySeeded),
		string(protocol.SignalEnergyPaused),
		string(protocol.SignalEnergyResumed),
	} {
		if !protocol.IsPlatformObservation(kind) {
			t.Fatalf("%q should be a platform observation", kind)
		}
	}
	for _, kind := range []string{
		string(protocol.TaskEventReady),
		"triage.tick",
		"feature.requested",
	} {
		if protocol.IsPlatformObservation(kind) {
			t.Fatalf("%q should be work, not a platform observation", kind)
		}
	}
}

func TestCueIngressValidation(t *testing.T) {
	payload := protocol.CueIngressPayload{
		Kind:         protocol.SignalCueIngress,
		InvocationID: "cueinv-1",
		CueID:        "feature",
		Source:       "cli",
		Mode:         protocol.CueIngressModeSignal,
		InvokedAt:    time.Now().UTC(),
		EventIDs:     []string{"evt-1"},
	}
	raw, err := json.Marshal(payload)
	if err != nil {
		t.Fatal(err)
	}
	input := protocol.EventInput{TraceID: "trace-1", Type: protocol.EventSignal, Payload: raw}
	if details := input.Validate(); len(details) != 0 {
		t.Fatalf("details = %+v", details)
	}

	input.Type = protocol.EventInsight
	details := input.Validate()
	if len(details) == 0 {
		t.Fatal("cue.ingress accepted as INSIGHT")
	}

	bad, err := json.Marshal(protocol.CueIngressPayload{
		Kind: protocol.SignalCueIngress,
		Mode: "bogus",
	})
	if err != nil {
		t.Fatal(err)
	}
	details = protocol.EventInput{TraceID: "trace-1", Type: protocol.EventSignal, Payload: bad}.Validate()
	if len(details) == 0 {
		t.Fatal("incomplete cue.ingress payload accepted")
	}
}
