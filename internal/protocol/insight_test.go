package protocol

import (
	"encoding/json"
	"strings"
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

func TestReviewNoteSeverityNormalizedAtConstruction(t *testing.T) {
	raw := []byte(`{"traceId":"trace-1","type":"INSIGHT","payload":{"kind":"review.note","summary":"Missing retry handling","severity":" Medium "}}`)
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
	var payload NarrativeInsightPayload
	if err := json.Unmarshal(ev.Payload, &payload); err != nil {
		t.Fatal(err)
	}
	if payload.Severity != ReviewSeverityMedium {
		t.Fatalf("severity = %q, want %q", payload.Severity, ReviewSeverityMedium)
	}
}

func TestReviewNoteSeverityRejectsUnknownValue(t *testing.T) {
	raw := []byte(`{"traceId":"trace-1","type":"INSIGHT","payload":{"kind":"review.note","summary":"Missing retry handling","severity":"severe"}}`)
	in, err := ParseEventInput(raw)
	if err != nil {
		t.Fatal(err)
	}
	details := in.Validate()
	if len(details) != 1 {
		t.Fatalf("details = %#v", details)
	}
	if details[0].Path != "payload.severity" {
		t.Fatalf("path = %q, want payload.severity", details[0].Path)
	}
	for _, value := range ReviewSeverityValues() {
		if !strings.Contains(details[0].Message, value) {
			t.Fatalf("message %q must name %q", details[0].Message, value)
		}
	}

	_, err = in.ToEvent("agent-1")
	verr, ok := err.(*ValidationError)
	if !ok {
		t.Fatalf("error = %v, want *ValidationError", err)
	}
	if verr.Code != "schema_validation_failed" {
		t.Fatalf("code = %q", verr.Code)
	}
	if len(verr.Details) != 1 || verr.Details[0].Path != "payload.severity" {
		t.Fatalf("details = %#v", verr.Details)
	}
}

func TestReviewNoteSeverityAbsentAndWhitespaceValid(t *testing.T) {
	for name, raw := range map[string][]byte{
		"absent":     []byte(`{"traceId":"trace-1","type":"INSIGHT","payload":{"kind":"review.note","summary":"Missing retry handling"}}`),
		"whitespace": []byte(`{"traceId":"trace-1","type":"INSIGHT","payload":{"kind":"review.note","summary":"Missing retry handling","severity":"   "}}`),
	} {
		t.Run(name, func(t *testing.T) {
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
			var payload NarrativeInsightPayload
			if err := json.Unmarshal(ev.Payload, &payload); err != nil {
				t.Fatal(err)
			}
			if ClassifyReviewSeverity(payload.Severity) != ReviewSeverityUnspecified {
				t.Fatalf("severity = %q, want unspecified", payload.Severity)
			}
		})
	}
}

func TestClassifyReviewSeverity(t *testing.T) {
	tests := map[string]string{
		"":           ReviewSeverityUnspecified,
		"   ":        ReviewSeverityUnspecified,
		"info":       ReviewSeverityInfo,
		" LOW ":      ReviewSeverityLow,
		"Medium":     ReviewSeverityMedium,
		"high":       ReviewSeverityHigh,
		"critical":   ReviewSeverityCritical,
		"warning":    ReviewSeverityUnknown,
		"medium-ish": ReviewSeverityUnknown,
	}
	for raw, want := range tests {
		if got := ClassifyReviewSeverity(raw); got != want {
			t.Errorf("ClassifyReviewSeverity(%q) = %q, want %q", raw, got, want)
		}
	}
}

func TestReviewNoteSeverityVocabularyIsNotSharedWithOtherNarrativeKinds(t *testing.T) {
	raw := []byte(`{"traceId":"trace-1","type":"INSIGHT","payload":{"kind":"run.summary","summary":"Implemented OAuth callback","severity":"warning"}}`)
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
	var payload NarrativeInsightPayload
	if err := json.Unmarshal(ev.Payload, &payload); err != nil {
		t.Fatal(err)
	}
	if payload.Severity != "warning" {
		t.Fatalf("severity = %q, want warning untouched", payload.Severity)
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

func TestProjectInsightsExcludesPRBody(t *testing.T) {
	now := time.Now().UTC()
	events := []Event{
		mustInsightEvent(t, now, `{"kind":"pr.body","body":"## Why\nPR copy"}`),
		mustInsightEvent(t, now.Add(time.Second), `{"kind":"run.summary","summary":"Task run outcome","taskId":"task-1"}`),
	}
	got := ProjectInsights(events, DefaultInsightProjectionOptions("task-1"))
	if len(got) != 1 {
		t.Fatalf("got %d insights, want 1: %#v", len(got), got)
	}
}

func TestProjectInsightsExcludesWorktreeBranch(t *testing.T) {
	now := time.Now().UTC()
	events := []Event{
		mustInsightEvent(t, now, `{"kind":"worktree.branch","branch":"feature/foo"}`),
		mustInsightEvent(t, now.Add(time.Second), `{"kind":"run.summary","summary":"Task run outcome","taskId":"task-1"}`),
	}
	got := ProjectInsights(events, DefaultInsightProjectionOptions("task-1"))
	if len(got) != 1 {
		t.Fatalf("got %d insights, want 1: %#v", len(got), got)
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

func TestValidateHumanFeedbackVerdict(t *testing.T) {
	for _, verdict := range FeedbackVerdictValues() {
		t.Run(verdict, func(t *testing.T) {
			raw := []byte(`{"traceId":"trace-1","type":"INSIGHT","payload":{"kind":"human.feedback","taskId":"task-1","message":"Please revise","verdict":"` + verdict + `"}}`)
			in, err := ParseEventInput(raw)
			if err != nil {
				t.Fatal(err)
			}
			if details := in.Validate(); len(details) != 0 {
				t.Fatalf("details = %#v", details)
			}
		})
	}
}

func TestHumanFeedbackVerdictNormalizedAtConstruction(t *testing.T) {
	raw := []byte(`{"traceId":"trace-1","type":"INSIGHT","payload":{"kind":"human.feedback","taskId":"task-1","message":"Please revise","verdict":" Negative "}}`)
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
	var payload HumanFeedbackPayload
	if err := json.Unmarshal(ev.Payload, &payload); err != nil {
		t.Fatal(err)
	}
	if payload.Verdict != FeedbackVerdictNegative {
		t.Fatalf("verdict = %q, want %q", payload.Verdict, FeedbackVerdictNegative)
	}
}

func TestHumanFeedbackVerdictRejectsUnknownValue(t *testing.T) {
	raw := []byte(`{"traceId":"trace-1","type":"INSIGHT","payload":{"kind":"human.feedback","taskId":"task-1","message":"Please revise","verdict":"blocking"}}`)
	in, err := ParseEventInput(raw)
	if err != nil {
		t.Fatal(err)
	}
	details := in.Validate()
	if len(details) != 1 {
		t.Fatalf("details = %#v", details)
	}
	if details[0].Path != "payload.verdict" {
		t.Fatalf("path = %q, want payload.verdict", details[0].Path)
	}
	for _, value := range FeedbackVerdictValues() {
		if !strings.Contains(details[0].Message, value) {
			t.Fatalf("message %q must name %q", details[0].Message, value)
		}
	}

	_, err = in.ToEvent("agent-1")
	verr, ok := err.(*ValidationError)
	if !ok {
		t.Fatalf("error = %v, want *ValidationError", err)
	}
	if verr.Code != "schema_validation_failed" {
		t.Fatalf("code = %q", verr.Code)
	}
	if len(verr.Details) != 1 || verr.Details[0].Path != "payload.verdict" {
		t.Fatalf("details = %#v", verr.Details)
	}
}

func TestHumanFeedbackVerdictAbsentAndWhitespaceValid(t *testing.T) {
	for name, raw := range map[string][]byte{
		"absent":     []byte(`{"traceId":"trace-1","type":"INSIGHT","payload":{"kind":"human.feedback","taskId":"task-1","message":"Please revise"}}`),
		"whitespace": []byte(`{"traceId":"trace-1","type":"INSIGHT","payload":{"kind":"human.feedback","taskId":"task-1","message":"Please revise","verdict":"   "}}`),
	} {
		t.Run(name, func(t *testing.T) {
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
			var payload HumanFeedbackPayload
			if err := json.Unmarshal(ev.Payload, &payload); err != nil {
				t.Fatal(err)
			}
			if ClassifyFeedbackVerdict(payload.Verdict) != FeedbackVerdictUnspecified {
				t.Fatalf("verdict = %q, want unspecified", payload.Verdict)
			}
		})
	}
}

func TestClassifyFeedbackVerdict(t *testing.T) {
	tests := map[string]string{
		"":          FeedbackVerdictUnspecified,
		"   ":       FeedbackVerdictUnspecified,
		"positive":  FeedbackVerdictPositive,
		" Negative": FeedbackVerdictNegative,
		"NEUTRAL":   FeedbackVerdictNeutral,
		"blocking":  FeedbackVerdictUnknown,
		"maybe":     FeedbackVerdictUnknown,
	}
	for raw, want := range tests {
		if got := ClassifyFeedbackVerdict(raw); got != want {
			t.Errorf("ClassifyFeedbackVerdict(%q) = %q, want %q", raw, got, want)
		}
	}
}

func TestHumanFeedbackVerdictRenderedInPromptMemory(t *testing.T) {
	now := time.Now().UTC()
	events := []Event{
		mustInsightEvent(t, now, `{"kind":"human.feedback","taskId":"task-1","message":"Please revise","verdict":"negative"}`),
		mustInsightEvent(t, now.Add(time.Second), `{"kind":"human.feedback","taskId":"task-1","message":"Looks good","verdict":"positive"}`),
		mustInsightEvent(t, now.Add(2*time.Second), `{"kind":"human.feedback","taskId":"task-1","message":"Noted"}`),
	}
	got := ProjectInsights(events, DefaultInsightProjectionOptions("task-1"))
	if len(got) != 3 {
		t.Fatalf("got %#v", got)
	}
	if !strings.Contains(got[0], "Beekeeper feedback (negative)") {
		t.Fatalf("got %q", got[0])
	}
	if !strings.Contains(got[1], "Beekeeper feedback (positive)") {
		t.Fatalf("got %q", got[1])
	}
	if got[2] != "Beekeeper feedback: Noted" {
		t.Fatalf("verdict-less feedback = %q", got[2])
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
