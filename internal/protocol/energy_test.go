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
