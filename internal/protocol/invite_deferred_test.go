package protocol

import "testing"

func TestValidateSessionInviteDeferredStatus(t *testing.T) {
	raw := []byte(`{"traceId":"trace-1","type":"SIGNAL","payload":{"kind":"session.invite","inviteId":"inv-001","bee":"drone","task":"t","status":"deferred"}}`)
	in, err := ParseEventInput(raw)
	if err != nil {
		t.Fatal(err)
	}
	if details := in.Validate(); len(details) != 0 {
		t.Fatalf("details = %#v", details)
	}
}

func TestIsInviteStatus(t *testing.T) {
	if !IsInviteStatus(InviteStatusPending) {
		t.Fatal("expected pending")
	}
	if IsInviteStatus(InviteStatus("bogus")) {
		t.Fatal("expected bogus to be invalid")
	}
}
