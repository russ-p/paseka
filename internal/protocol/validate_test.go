package protocol

import (
	"encoding/json"
	"testing"
)

func TestValidateVerificationSuccess(t *testing.T) {
	raw := []byte(`{"traceId":"trace-1","type":"VERIFICATION","payload":{"kind":"verification.success","summary":"ok"}}`)
	in, err := ParseEventInput(raw)
	if err != nil {
		t.Fatal(err)
	}
	if details := in.Validate(); len(details) != 0 {
		t.Fatalf("details = %#v", details)
	}
	ev, err := in.ToEvent("agent-1")
	if err != nil {
		t.Fatal(err)
	}
	if ev.TraceID != "trace-1" || ev.AgentID != "agent-1" {
		t.Fatalf("event = %#v", ev)
	}
}

func TestValidateMissingKind(t *testing.T) {
	raw := []byte(`{"traceId":"trace-1","type":"INSIGHT","payload":{"summary":"x"}}`)
	in, err := ParseEventInput(raw)
	if err != nil {
		t.Fatal(err)
	}
	details := in.Validate()
	if len(details) != 1 || details[0].Path != "payload.kind" {
		t.Fatalf("details = %#v", details)
	}
}

func TestValidateTaskPlanRequiresTasks(t *testing.T) {
	raw := []byte(`{"traceId":"trace-1","type":"INSIGHT","payload":{"kind":"task.plan","tasks":[]}}`)
	in, err := ParseEventInput(raw)
	if err != nil {
		t.Fatal(err)
	}
	details := in.Validate()
	if len(details) != 1 || details[0].Path != "payload.tasks" {
		t.Fatalf("details = %#v", details)
	}
}

func TestValidateInvalidJSON(t *testing.T) {
	_, err := ParseEventInput([]byte(`not-json`))
	if err == nil {
		t.Fatal("expected error")
	}
	var verr *ValidationError
	if !asTestValidationError(err, &verr) || verr.Code != "invalid_json" {
		t.Fatalf("err = %v", err)
	}
}

func asTestValidationError(err error, target **ValidationError) bool {
	verr, ok := err.(*ValidationError)
	if !ok {
		return false
	}
	*target = verr
	return true
}

func TestEventCLIResultJSON(t *testing.T) {
	result := EventCLIResult{
		OK:      true,
		TraceID: "trace-1",
		Type:    EventVerification,
		Kind:    "verification.success",
		Subject: "demo.events.VERIFICATION.verification.success",
	}
	data, err := json.Marshal(result)
	if err != nil {
		t.Fatal(err)
	}
	if !json.Valid(data) {
		t.Fatalf("invalid json: %s", data)
	}
}
