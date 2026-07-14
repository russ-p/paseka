package protocol

import "testing"

func TestValidateSessionInviteIncompleteStatus(t *testing.T) {
	raw := []byte(`{"traceId":"trace-1","type":"SIGNAL","payload":{"kind":"session.invite","inviteId":"inv-001","bee":"drone","task":"t","status":"incomplete"}}`)
	in, err := ParseEventInput(raw)
	if err != nil {
		t.Fatal(err)
	}
	if details := in.Validate(); len(details) != 0 {
		t.Fatalf("details = %#v", details)
	}
}
