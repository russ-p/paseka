package adapters_test

import (
	"context"
	"errors"
	"testing"

	"github.com/russ-p/paseka/internal/adapters"
	"github.com/russ-p/paseka/internal/protocol"
)

func TestResolveStatusProcessOutcome(t *testing.T) {
	t.Run("completed on clean exit", func(t *testing.T) {
		status, reason, msg := adapters.ResolveStatus(nil, nil)
		if status != protocol.StatusCompleted || reason != "" || msg != "" {
			t.Fatalf("status=%q reason=%q msg=%q", status, reason, msg)
		}
	})
	t.Run("failed on run error", func(t *testing.T) {
		runErr := errors.New("exit 1")
		status, reason, _ := adapters.ResolveStatus(nil, runErr)
		if status != protocol.StatusFailed || reason != "" {
			t.Fatalf("status=%q reason=%q", status, reason)
		}
	})
	t.Run("cancelled on context cancel", func(t *testing.T) {
		status, reason, _ := adapters.ResolveStatus(context.Canceled, nil)
		if status != protocol.StatusCancelled || reason != "" {
			t.Fatalf("status=%q reason=%q, want cancelled without reason", status, reason)
		}
	})
	t.Run("timeout on context deadline", func(t *testing.T) {
		status, reason, msg := adapters.ResolveStatus(context.DeadlineExceeded, nil)
		if status != protocol.StatusCancelled {
			t.Fatalf("status=%q, want cancelled", status)
		}
		if reason != protocol.ReasonTimeout {
			t.Fatalf("reason=%q, want %q", reason, protocol.ReasonTimeout)
		}
		if msg == "" {
			t.Fatal("timeout should carry a diagnostic message")
		}
	})
}

func TestPickOutputPrefersSummary(t *testing.T) {
	if got := adapters.PickOutput("done", "raw"); got != "done" {
		t.Fatalf("got %q", got)
	}
	if got := adapters.PickOutput("", "raw"); got != "raw" {
		t.Fatalf("got %q", got)
	}
}

func TestPickSummaryPrefersFile(t *testing.T) {
	if got := adapters.PickSummary("file", "stream"); got != "file" {
		t.Fatalf("got %q", got)
	}
	if got := adapters.PickSummary("", "stream"); got != "stream" {
		t.Fatalf("got %q", got)
	}
}

func TestIsStreamFormat(t *testing.T) {
	if !adapters.IsStreamFormat("") {
		t.Fatal("empty format should default to stream")
	}
	if !adapters.IsStreamFormat("stream-json") {
		t.Fatal("stream-json should be stream format")
	}
	if adapters.IsStreamFormat("text") {
		t.Fatal("text should not be stream format")
	}
}
