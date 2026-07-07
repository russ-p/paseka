package ids_test

import (
	"strings"
	"testing"
	"time"
	"unicode"

	"github.com/paseka/paseka/internal/ids"
)

func TestMiniULIDFormat(t *testing.T) {
	body, err := ids.MiniULID()
	if err != nil {
		t.Fatal(err)
	}
	if len(body) != 16 {
		t.Fatalf("body length = %d, want 16: %q", len(body), body)
	}
	for _, r := range body {
		if !unicode.IsDigit(r) && (r < 'a' || r > 'f') {
			t.Fatalf("non-lowercase hex: %q in %q", r, body)
		}
	}
}

func TestMiniULIDSortable(t *testing.T) {
	before := time.Now().UTC().Add(-time.Millisecond)
	time.Sleep(2 * time.Millisecond)

	body1, err := ids.MiniULID()
	if err != nil {
		t.Fatal(err)
	}
	time.Sleep(2 * time.Millisecond)
	body2, err := ids.MiniULID()
	if err != nil {
		t.Fatal(err)
	}

	if body1 >= body2 {
		t.Fatalf("expected %q < %q", body1, body2)
	}

	ts1, ok := ids.ParseMiniULIDTime(body1)
	if !ok {
		t.Fatalf("parse %q", body1)
	}
	ts2, ok := ids.ParseMiniULIDTime(body2)
	if !ok {
		t.Fatalf("parse %q", body2)
	}
	if !ts1.Before(ts2) {
		t.Fatalf("timestamps not ordered: %v vs %v", ts1, ts2)
	}
	if ts1.Before(before) {
		t.Fatalf("timestamp too old: %v", ts1)
	}
}

func TestPrefixed(t *testing.T) {
	id, err := ids.Prefixed("trace-")
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
}

func TestParsePrefixedTimeRejectsManualIDs(t *testing.T) {
	if _, ok := ids.ParsePrefixedTime("trace-", "trace-auth-01"); ok {
		t.Fatal("expected manual trace id to be rejected")
	}
	if _, ok := ids.ParsePrefixedTime("trace-", "trace-deadbeef"); ok {
		t.Fatal("expected short body to be rejected")
	}
}
