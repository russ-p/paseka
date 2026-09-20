package purge

import (
	"reflect"
	"testing"
	"time"
)

func TestBusPruneTargets(t *testing.T) {
	cutoff := time.Date(2025, 1, 1, 0, 0, 0, 0, time.UTC)
	plan := PrunePlan{
		Cutoff: cutoff,
		Runs: []PruneCandidate{
			{TraceID: "fs-old"},
			{TraceID: "fs-no-ledger"},
		},
		Worktrees: []PruneCandidate{{TraceID: "wt-old"}},
	}
	fsTraces := map[string]bool{"fs-old": true, "fs-no-ledger": true, "wt-old": true}
	ledgerLast := map[string]time.Time{
		"fs-old":   cutoff.Add(-time.Hour),
		"wt-old":   cutoff.Add(-time.Hour),
		"bus-old":  cutoff.Add(-time.Hour),
		"bus-new":  cutoff.Add(time.Hour),
		"bus-zero": {},
	}
	ledgerExists := map[string]bool{
		"fs-old":   true,
		"wt-old":   true,
		"bus-old":  true,
		"bus-new":  true,
		"bus-zero": true,
	}

	got := busPruneTargets(plan, fsTraces, ledgerLast, ledgerExists)
	want := []string{"bus-old", "fs-old", "wt-old"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("busPruneTargets = %v, want %v", got, want)
	}
}

func TestOlderThanFiltersByCutoff(t *testing.T) {
	cutoff := time.Date(2025, 1, 1, 0, 0, 0, 0, time.UTC)
	candidates := []PruneCandidate{
		{TraceID: "old", LastUsed: cutoff.Add(-time.Minute)},
		{TraceID: "exact", LastUsed: cutoff},
		{TraceID: "new", LastUsed: cutoff.Add(time.Minute)},
		{TraceID: "dirty", LastUsed: cutoff.Add(-time.Hour), Protected: true},
	}
	got := olderThan(candidates, cutoff)
	if len(got) != 1 || got[0].TraceID != "old" {
		t.Fatalf("olderThan = %+v", got)
	}
}
