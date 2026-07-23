package review_test

import (
	"strings"
	"testing"

	"github.com/paseka/paseka/internal/review"
)

func TestComposeMergeMessageDefaultSubjectWithSummaryBody(t *testing.T) {
	traceID := "trace-abc"
	parts := review.ComposeMergeMessage(traceID, "", "Implemented OAuth callback and added focused tests")
	if parts.Subject != "paseka: merge trace trace-abc" {
		t.Fatalf("subject = %q", parts.Subject)
	}
	if parts.Body != "Implemented OAuth callback and added focused tests" {
		t.Fatalf("body = %q", parts.Body)
	}
	msg := parts.FormatMessage()
	if !strings.Contains(msg, "paseka: merge trace trace-abc") {
		t.Fatalf("message = %q", msg)
	}
	if !strings.Contains(msg, "Implemented OAuth callback") {
		t.Fatalf("message = %q", msg)
	}
}

func TestComposeMergeMessageExplicitSubjectWithSummaryBody(t *testing.T) {
	parts := review.ComposeMergeMessage("trace-1", "feat: add oauth", "Trail narrative summary")
	if parts.Subject != "feat: add oauth" {
		t.Fatalf("subject = %q", parts.Subject)
	}
	if parts.Body != "Trail narrative summary" {
		t.Fatalf("body = %q", parts.Body)
	}
}

func TestComposeMergeMessageMultilineHITLBodyNotDoubleAppended(t *testing.T) {
	hitl := "feat: add oauth\n\nManual merge body from HITL"
	parts := review.ComposeMergeMessage("trace-1", hitl, "Trail narrative summary")
	if parts.Subject != "feat: add oauth" {
		t.Fatalf("subject = %q", parts.Subject)
	}
	if parts.Body != "Manual merge body from HITL" {
		t.Fatalf("body = %q, want HITL body only", parts.Body)
	}
}

func TestComposeMergeMessageSingleNewlineHITLBody(t *testing.T) {
	hitl := "feat: add oauth\nManual body on second line"
	parts := review.ComposeMergeMessage("trace-1", hitl, "Trail narrative summary")
	if parts.Subject != "feat: add oauth" {
		t.Fatalf("subject = %q", parts.Subject)
	}
	if parts.Body != "Manual body on second line" {
		t.Fatalf("body = %q", parts.Body)
	}
}

func TestComposeMergeMessageNoBodyWhenSummaryAbsent(t *testing.T) {
	parts := review.ComposeMergeMessage("trace-1", "", "")
	if parts.Subject != "paseka: merge trace trace-1" {
		t.Fatalf("subject = %q", parts.Subject)
	}
	if parts.Body != "" {
		t.Fatalf("body = %q, want empty", parts.Body)
	}
	if parts.FormatMessage() != "paseka: merge trace trace-1" {
		t.Fatalf("message = %q", parts.FormatMessage())
	}
}

func TestComposeMergeMessageExplicitSubjectNoSummary(t *testing.T) {
	parts := review.ComposeMergeMessage("trace-1", "feat: ship it", "")
	if parts.Subject != "feat: ship it" {
		t.Fatalf("subject = %q", parts.Subject)
	}
	if parts.Body != "" {
		t.Fatalf("body = %q", parts.Body)
	}
}
