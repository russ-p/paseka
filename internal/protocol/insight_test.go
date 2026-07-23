package protocol

import (
	"encoding/json"
	"testing"
	"time"
)

func TestValidateRunSummary(t *testing.T) {
	raw := []byte(`{"traceId":"trace-1","type":"INSIGHT","payload":{"kind":"run.summary","summary":"Implemented OAuth callback","taskId":"task-1"}}`)
	in, err := ParseEventInput(raw)
	if err != nil {
		t.Fatal(err)
	}
	if details := in.Validate(); len(details) != 0 {
		t.Fatalf("details = %#v", details)
	}
}

func TestValidateRunSummaryRequiresSummary(t *testing.T) {
	raw := []byte(`{"traceId":"trace-1","type":"INSIGHT","payload":{"kind":"run.summary","taskId":"task-1"}}`)
	in, err := ParseEventInput(raw)
	if err != nil {
		t.Fatal(err)
	}
	details := in.Validate()
	if len(details) != 1 || details[0].Path != "payload.summary" {
		t.Fatalf("details = %#v", details)
	}
}

func TestValidateReviewNote(t *testing.T) {
	raw := []byte(`{"traceId":"trace-1","type":"INSIGHT","payload":{"kind":"review.note","summary":"Missing retry handling","severity":"medium"}}`)
	in, err := ParseEventInput(raw)
	if err != nil {
		t.Fatal(err)
	}
	if details := in.Validate(); len(details) != 0 {
		t.Fatalf("details = %#v", details)
	}
}

func TestProjectInsightsExcludesTraceSummary(t *testing.T) {
	now := time.Now().UTC()
	events := []Event{
		mustInsightEvent(t, now, `{"kind":"trace.summary","summary":"Trail outcome description"}`),
		mustInsightEvent(t, now.Add(time.Second), `{"kind":"run.summary","summary":"Task run outcome","taskId":"task-1"}`),
	}
	got := ProjectInsights(events, DefaultInsightProjectionOptions("task-1"))
	if len(got) != 1 {
		t.Fatalf("got %d insights, want 1: %#v", len(got), got)
	}
	if got[0] != "Summary (agent-1): Task run outcome" {
		t.Fatalf("got %q", got[0])
	}
}

func TestProjectInsightsExcludesTaskPlan(t *testing.T) {
	now := time.Now().UTC()
	events := []Event{
		mustInsightEvent(t, now, `{"kind":"task.plan","tasks":[{"taskId":"task-1","title":"Add endpoint"}]}`),
		mustInsightEvent(t, now.Add(time.Second), `{"kind":"run.summary","summary":"Added endpoint","taskId":"task-1"}`),
	}
	got := ProjectInsights(events, DefaultInsightProjectionOptions("task-1"))
	if len(got) != 1 {
		t.Fatalf("got %d insights, want 1: %#v", len(got), got)
	}
	if got[0] != "Summary (agent-1): Added endpoint" {
		t.Fatalf("got %q", got[0])
	}
}

func TestProjectInsightsTaskScopedBeforeTraceScoped(t *testing.T) {
	now := time.Now().UTC()
	events := []Event{
		mustInsightEvent(t, now, `{"kind":"context.note","summary":"Trace-wide note"}`),
		mustInsightEvent(t, now.Add(time.Second), `{"kind":"run.summary","summary":"Task-specific summary","taskId":"task-1"}`),
	}
	got := ProjectInsights(events, DefaultInsightProjectionOptions("task-1"))
	if len(got) != 2 {
		t.Fatalf("got %#v", got)
	}
	if got[0] != "Summary (agent-1): Task-specific summary" {
		t.Fatalf("task-scoped first, got %q", got[0])
	}
	if got[1] != "Context (agent-1): Trace-wide note" {
		t.Fatalf("trace-scoped second, got %q", got[1])
	}
}

func mustInsightEvent(t *testing.T, at time.Time, payload string) Event {
	t.Helper()
	var payloadObj any
	if err := json.Unmarshal([]byte(payload), &payloadObj); err != nil {
		t.Fatal(err)
	}
	ev, err := NewEvent("trace-1", "agent-1", 1, EventInsight, payloadObj)
	if err != nil {
		t.Fatal(err)
	}
	ev.CreatedAt = at
	return ev
}
