package protocol

import (
	"encoding/json"
	"strings"
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

func TestValidateTraceSummary(t *testing.T) {
	raw := []byte(`{"traceId":"trace-1","type":"INSIGHT","payload":{"kind":"trace.summary","summary":"Implemented OAuth callback and added focused tests"}}`)
	in, err := ParseEventInput(raw)
	if err != nil {
		t.Fatal(err)
	}
	if details := in.Validate(); len(details) != 0 {
		t.Fatalf("details = %#v", details)
	}

	empty := []byte(`{"traceId":"trace-1","type":"INSIGHT","payload":{"kind":"trace.summary","summary":"   "}}`)
	in2, err := ParseEventInput(empty)
	if err != nil {
		t.Fatal(err)
	}
	details := in2.Validate()
	if len(details) != 1 || details[0].Path != "payload.summary" {
		t.Fatalf("details = %#v", details)
	}

	over := []byte(`{"traceId":"trace-1","type":"INSIGHT","payload":{"kind":"trace.summary","summary":"` + strings.Repeat("x", MaxTraceSummaryLen+1) + `"}}`)
	in3, err := ParseEventInput(over)
	if err != nil {
		t.Fatal(err)
	}
	details = in3.Validate()
	if len(details) != 1 || details[0].Path != "payload.summary" {
		t.Fatalf("details = %#v", details)
	}
}

func TestValidateTraceTitle(t *testing.T) {
	raw := []byte(`{"traceId":"trace-1","type":"INSIGHT","payload":{"kind":"trace.title","title":"Live bees header"}}`)
	in, err := ParseEventInput(raw)
	if err != nil {
		t.Fatal(err)
	}
	if details := in.Validate(); len(details) != 0 {
		t.Fatalf("details = %#v", details)
	}

	empty := []byte(`{"traceId":"trace-1","type":"INSIGHT","payload":{"kind":"trace.title","title":"   "}}`)
	in2, err := ParseEventInput(empty)
	if err != nil {
		t.Fatal(err)
	}
	details := in2.Validate()
	if len(details) != 1 || details[0].Path != "payload.title" {
		t.Fatalf("details = %#v", details)
	}
}

func TestValidateWorktreeBranch(t *testing.T) {
	raw := []byte(`{"traceId":"trace-1","type":"INSIGHT","payload":{"kind":"worktree.branch","branch":"feature/live-bees-header"}}`)
	in, err := ParseEventInput(raw)
	if err != nil {
		t.Fatal(err)
	}
	if details := in.Validate(); len(details) != 0 {
		t.Fatalf("details = %#v", details)
	}

	empty := []byte(`{"traceId":"trace-1","type":"INSIGHT","payload":{"kind":"worktree.branch","branch":"   "}}`)
	in2, err := ParseEventInput(empty)
	if err != nil {
		t.Fatal(err)
	}
	details := in2.Validate()
	if len(details) != 1 || details[0].Path != "payload.branch" {
		t.Fatalf("details = %#v", details)
	}

	main := []byte(`{"traceId":"trace-1","type":"INSIGHT","payload":{"kind":"worktree.branch","branch":"main"}}`)
	in3, err := ParseEventInput(main)
	if err != nil {
		t.Fatal(err)
	}
	details = in3.Validate()
	if len(details) != 1 {
		t.Fatalf("details = %#v", details)
	}

	dash := []byte(`{"traceId":"trace-1","type":"INSIGHT","payload":{"kind":"worktree.branch","branch":"--help"}}`)
	in4, err := ParseEventInput(dash)
	if err != nil {
		t.Fatal(err)
	}
	details = in4.Validate()
	if len(details) != 1 {
		t.Fatalf("leading-dash details = %#v", details)
	}

	origin := []byte(`{"traceId":"trace-1","type":"INSIGHT","payload":{"kind":"worktree.branch","branch":"origin/main"}}`)
	in5, err := ParseEventInput(origin)
	if err != nil {
		t.Fatal(err)
	}
	details = in5.Validate()
	if len(details) != 1 {
		t.Fatalf("origin details = %#v", details)
	}

	head := []byte(`{"traceId":"trace-1","type":"INSIGHT","payload":{"kind":"worktree.branch","branch":"HEAD"}}`)
	in6, err := ParseEventInput(head)
	if err != nil {
		t.Fatal(err)
	}
	details = in6.Validate()
	if len(details) != 1 {
		t.Fatalf("HEAD details = %#v", details)
	}

	invalid := []byte(`{"traceId":"trace-1","type":"INSIGHT","payload":{"kind":"worktree.branch","branch":"has spaces"}}`)
	in7, err := ParseEventInput(invalid)
	if err != nil {
		t.Fatal(err)
	}
	details = in7.Validate()
	if len(details) != 1 {
		t.Fatalf("spaces details = %#v", details)
	}

	over := []byte(`{"traceId":"trace-1","type":"INSIGHT","payload":{"kind":"worktree.branch","branch":"` + strings.Repeat("a", MaxWorktreeBranchLen+1) + `"}}`)
	in8, err := ParseEventInput(over)
	if err != nil {
		t.Fatal(err)
	}
	details = in8.Validate()
	if len(details) != 1 || details[0].Path != "payload.branch" {
		t.Fatalf("overlong details = %#v", details)
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
