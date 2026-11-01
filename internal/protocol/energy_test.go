package protocol

import (
	"testing"
)

func TestValidateEnergyAdd(t *testing.T) {
	raw := []byte(`{"traceId":"trace-1","type":"SIGNAL","payload":{"kind":"energy.add","amount":5}}`)
	in, err := ParseEventInput(raw)
	if err != nil {
		t.Fatal(err)
	}
	if details := in.Validate(); len(details) != 0 {
		t.Fatalf("details = %#v", details)
	}
}

func TestValidateEnergyAddRequiresPositiveAmount(t *testing.T) {
	raw := []byte(`{"traceId":"trace-1","type":"SIGNAL","payload":{"kind":"energy.add","amount":0}}`)
	in, err := ParseEventInput(raw)
	if err != nil {
		t.Fatal(err)
	}
	details := in.Validate()
	if len(details) != 1 || details[0].Path != "payload.amount" {
		t.Fatalf("details = %#v", details)
	}
}

func TestValidateEnergyConsume(t *testing.T) {
	raw := []byte(`{"traceId":"trace-1","type":"SIGNAL","payload":{"kind":"energy.consume","amount":1,"reason":"task.dispatch","taskId":"task-1"}}`)
	in, err := ParseEventInput(raw)
	if err != nil {
		t.Fatal(err)
	}
	if details := in.Validate(); len(details) != 0 {
		t.Fatalf("details = %#v", details)
	}
}

func TestValidateEnergyStipend(t *testing.T) {
	raw := []byte(`{"traceId":"trace-1","type":"SIGNAL","payload":{"kind":"energy.stipend","amount":4}}`)
	in, err := ParseEventInput(raw)
	if err != nil {
		t.Fatal(err)
	}
	if details := in.Validate(); len(details) != 0 {
		t.Fatalf("details = %#v", details)
	}
}

func TestValidateEnergyStipendRequiresPositiveAmount(t *testing.T) {
	raw := []byte(`{"traceId":"trace-1","type":"SIGNAL","payload":{"kind":"energy.stipend","amount":0}}`)
	in, err := ParseEventInput(raw)
	if err != nil {
		t.Fatal(err)
	}
	details := in.Validate()
	if len(details) != 1 || details[0].Path != "payload.amount" {
		t.Fatalf("details = %#v", details)
	}
}

func TestValidateEnergyPaused(t *testing.T) {
	raw := []byte(`{"traceId":"trace-1","type":"SIGNAL","payload":{"kind":"energy.paused","episodeId":"pause-1","reason":"Honey reserve exhausted","remaining":0,"budget":12,"pendingCount":2}}`)
	in, err := ParseEventInput(raw)
	if err != nil {
		t.Fatal(err)
	}
	if details := in.Validate(); len(details) != 0 {
		t.Fatalf("details = %#v", details)
	}
	if !IsEnergyObservation("energy.paused") {
		t.Fatal("energy.paused must read as an observation")
	}
}

func TestValidateEnergyPausedRequiresEpisode(t *testing.T) {
	raw := []byte(`{"traceId":"trace-1","type":"SIGNAL","payload":{"kind":"energy.paused","remaining":0,"pendingCount":1}}`)
	in, err := ParseEventInput(raw)
	if err != nil {
		t.Fatal(err)
	}
	details := in.Validate()
	if len(details) != 1 || details[0].Path != "payload.episodeId" {
		t.Fatalf("details = %#v", details)
	}
}

func TestValidateEnergyResumed(t *testing.T) {
	raw := []byte(`{"traceId":"trace-1","type":"SIGNAL","payload":{"kind":"energy.resumed","episodeId":"pause-1","source":"energy.stipend","remaining":4,"budget":12,"pendingBefore":2,"pendingAfter":0,"dispatched":0,"abandoned":2}}`)
	in, err := ParseEventInput(raw)
	if err != nil {
		t.Fatal(err)
	}
	if details := in.Validate(); len(details) != 0 {
		t.Fatalf("details = %#v", details)
	}
	if !IsEnergyObservation("energy.resumed") {
		t.Fatal("energy.resumed must read as an observation")
	}
}

func TestValidateEnergyResumedRequiresKnownSource(t *testing.T) {
	raw := []byte(`{"traceId":"trace-1","type":"SIGNAL","payload":{"kind":"energy.resumed","episodeId":"pause-1","source":"timer","remaining":4}}`)
	in, err := ParseEventInput(raw)
	if err != nil {
		t.Fatal(err)
	}
	details := in.Validate()
	if len(details) != 1 || details[0].Path != "payload.source" {
		t.Fatalf("details = %#v", details)
	}
}
