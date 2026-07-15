package protocol

import (
	"testing"
)

func TestValidateSessionInvite(t *testing.T) {
	raw := []byte(`{"traceId":"trace-1","type":"SIGNAL","payload":{"kind":"session.invite","inviteId":"inv-001","bee":"drone","intent":"grilling","task":"Grill feature","status":"pending"}}`)
	in, err := ParseEventInput(raw)
	if err != nil {
		t.Fatal(err)
	}
	if details := in.Validate(); len(details) != 0 {
		t.Fatalf("details = %#v", details)
	}
}

func TestValidateSessionInviteRequiresFields(t *testing.T) {
	raw := []byte(`{"traceId":"trace-1","type":"SIGNAL","payload":{"kind":"session.invite","status":"pending"}}`)
	in, err := ParseEventInput(raw)
	if err != nil {
		t.Fatal(err)
	}
	details := in.Validate()
	if len(details) < 3 {
		t.Fatalf("expected multiple validation errors, got %#v", details)
	}
}

func TestValidateSessionInviteRejectsWrongType(t *testing.T) {
	raw := []byte(`{"traceId":"trace-1","type":"INSIGHT","payload":{"kind":"session.invite","inviteId":"inv-001","bee":"drone","task":"t","status":"pending"}}`)
	in, err := ParseEventInput(raw)
	if err != nil {
		t.Fatal(err)
	}
	details := in.Validate()
	if len(details) != 1 || details[0].Path != "type" {
		t.Fatalf("details = %#v", details)
	}
}

func TestValidateBeekeeperReady(t *testing.T) {
	raw := []byte(`{"traceId":"trace-1","type":"SIGNAL","payload":{"kind":"beekeeper.ready","inviteId":"inv-001","action":"accept"}}`)
	in, err := ParseEventInput(raw)
	if err != nil {
		t.Fatal(err)
	}
	if details := in.Validate(); len(details) != 0 {
		t.Fatalf("details = %#v", details)
	}
}

func TestValidateBeekeeperReadyRequiresAction(t *testing.T) {
	raw := []byte(`{"traceId":"trace-1","type":"SIGNAL","payload":{"kind":"beekeeper.ready","inviteId":"inv-001"}}`)
	in, err := ParseEventInput(raw)
	if err != nil {
		t.Fatal(err)
	}
	details := in.Validate()
	if len(details) != 1 || details[0].Path != "payload.action" {
		t.Fatalf("details = %#v", details)
	}
}
