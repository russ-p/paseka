package colony_test

import (
	"strings"
	"testing"
	"time"
	"unicode"

	"github.com/paseka/paseka/internal/colony"
)

func TestNewTraceIDFormat(t *testing.T) {
	id, err := colony.NewTraceID()
	if err != nil {
		t.Fatal(err)
	}
	if !strings.HasPrefix(id, "trace-") {
		t.Fatalf("prefix: %q", id)
	}
	body := strings.TrimPrefix(id, "trace-")
	if len(body) != 16 {
		t.Fatalf("body length = %d, want 16: %q", len(body), body)
	}
	for _, r := range body {
		if !unicode.IsDigit(r) && (r < 'a' || r > 'f') {
			t.Fatalf("non-lowercase hex: %q in %q", r, id)
		}
	}
}

func TestNewTraceIDSortable(t *testing.T) {
	before := time.Now().UTC().Add(-time.Millisecond)
	time.Sleep(2 * time.Millisecond)

	id1, err := colony.NewTraceID()
	if err != nil {
		t.Fatal(err)
	}
	time.Sleep(2 * time.Millisecond)
	id2, err := colony.NewTraceID()
	if err != nil {
		t.Fatal(err)
	}

	if id1 >= id2 {
		t.Fatalf("expected %q < %q", id1, id2)
	}

	ts1, ok := colony.ParseTraceIDTime(id1)
	if !ok {
		t.Fatalf("parse %q", id1)
	}
	ts2, ok := colony.ParseTraceIDTime(id2)
	if !ok {
		t.Fatalf("parse %q", id2)
	}
	if !ts1.Before(ts2) {
		t.Fatalf("timestamps not ordered: %v vs %v", ts1, ts2)
	}
	if ts1.Before(before) {
		t.Fatalf("timestamp too old: %v", ts1)
	}
}

func TestParseTraceIDTimeRejectsManualIDs(t *testing.T) {
	if _, ok := colony.ParseTraceIDTime("trace-auth-01"); ok {
		t.Fatal("expected manual trace id to be rejected")
	}
	if _, ok := colony.ParseTraceIDTime("trace-deadbeef"); ok {
		t.Fatal("expected short body to be rejected")
	}
}
