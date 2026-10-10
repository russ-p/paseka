package export

import (
	"strings"
	"testing"

	"github.com/russ-p/paseka/internal/protocol"
	"github.com/russ-p/paseka/internal/runs"
)

func TestCompactTokenCount(t *testing.T) {
	tests := []struct {
		in   int64
		want string
	}{
		{in: 0, want: "0"},
		{in: 30, want: "30"},
		{in: 999, want: "999"},
		{in: 1000, want: "1k"},
		{in: 1499, want: "1k"},
		{in: 1500, want: "2k"},
		{in: 85988, want: "86k"},
		{in: 874823, want: "880k"},
		{in: 994999, want: "1M"},
		{in: 995000, want: "1M"},
		{in: 1_000_000, want: "1M"},
		{in: 1_049_000, want: "1.05M"},
		{in: 2_500_000, want: "2.5M"},
	}
	for _, tt := range tests {
		if got := compactTokenCount(tt.in); got != tt.want {
			t.Errorf("compactTokenCount(%d) = %q, want %q", tt.in, got, tt.want)
		}
	}
}

func TestFormatUsageHead(t *testing.T) {
	if got := formatUsageHead(120, 30, 0, 0); got != "in 120 / out 30" {
		t.Errorf("formatUsageHead(120, 30, 0, 0) = %q", got)
	}
	if got := formatUsageHead(120, 30, 0, 7); got != "in 120 / out 30, cache write 7" {
		t.Errorf("formatUsageHead(120, 30, 0, 7) = %q", got)
	}
	if got := formatUsageHead(874823, 85988, 0, 0); got != "in 880k / out 86k (874823 / 85988)" {
		t.Errorf("formatUsageHead(874823, 85988, 0, 0) = %q", got)
	}
	if got := formatUsageHead(874823, 85988, 12300544, 0); got != "in 880k / out 86k (874823 / 85988, cache read 12300544)" {
		t.Errorf("formatUsageHead(874823, 85988, 12300544, 0) = %q", got)
	}
	if got := formatUsageHead(120, 85988, 0, 0); got != "in 120 / out 86k (120 / 85988)" {
		t.Errorf("formatUsageHead(120, 85988, 0, 0) = %q", got)
	}
}

func TestFormatUsageAggregate(t *testing.T) {
	if got := formatUsageAggregate(nil); got != "" {
		t.Errorf("formatUsageAggregate(nil) = %q", got)
	}
	empty := &runs.UsageAggregate{RunCountWithUsage: 0}
	if got := formatUsageAggregate(empty); got != "" {
		t.Errorf("formatUsageAggregate(empty) = %q", got)
	}
	aggregate := &runs.UsageAggregate{
		InputTokens:       874823,
		OutputTokens:      85988,
		CacheReadTokens:   12300544,
		RunCountWithUsage: 4,
	}
	want := "in 880k / out 86k (874823 / 85988, cache read 12300544), 4 runs with usage"
	if got := formatUsageAggregate(aggregate); got != want {
		t.Errorf("formatUsageAggregate = %q, want %q", got, want)
	}
	plain := &runs.UsageAggregate{
		InputTokens:       120,
		OutputTokens:      30,
		CacheWriteTokens:  7,
		RunCountWithUsage: 1,
	}
	plainWant := "in 120 / out 30, cache write 7, 1 runs with usage"
	if got := formatUsageAggregate(plain); got != plainWant {
		t.Errorf("formatUsageAggregate = %q, want %q", got, plainWant)
	}
}

func TestFormatUsageTokens(t *testing.T) {
	if got := formatUsageTokens(nil); got != "" {
		t.Errorf("formatUsageTokens(nil) = %q", got)
	}
	empty := &protocol.Usage{}
	if got := formatUsageTokens(empty); got != "" {
		t.Errorf("formatUsageTokens(empty) = %q", got)
	}
	plain := &protocol.Usage{InputTokens: 120, OutputTokens: 30}
	if got := formatUsageTokens(plain); got != "in 120 / out 30" {
		t.Errorf("formatUsageTokens = %q, want %q", got, "in 120 / out 30")
	}
	compact := &protocol.Usage{
		InputTokens:     874823,
		OutputTokens:    85988,
		CacheReadTokens: 12300544,
		DurationMs:      125000,
	}
	want := "in 880k / out 86k (874823 / 85988, cache read 12300544), adapter 2m 5s"
	got := formatUsageTokens(compact)
	if got != want {
		t.Errorf("formatUsageTokens = %q, want %q", got, want)
	}
	if !strings.Contains(got, "adapter") {
		t.Errorf("formatUsageTokens missing adapter segment: %q", got)
	}
}
