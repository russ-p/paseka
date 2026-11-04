package runs_test

import (
	"os"
	"path/filepath"
	"sync"
	"testing"

	"github.com/russ-p/paseka/internal/runs"
)

func TestTrailOriginMissingIsUnknownNotManual(t *testing.T) {
	root := t.TempDir()
	origin, ok, err := runs.ReadTrailOrigin(root, "trace-missing")
	if err != nil {
		t.Fatal(err)
	}
	if ok {
		t.Fatalf("missing origin read as %+v", origin)
	}
}

func TestEnsureTrailOriginKeepsTheFirstOrigin(t *testing.T) {
	root := t.TempDir()
	first, created, err := runs.EnsureTrailOrigin(root, "trace-1", runs.TrailOrigin{
		Kind:         runs.OriginKindCue,
		Source:       "cli",
		InvocationID: "cueinv-1",
		CueID:        "feature",
	})
	if err != nil {
		t.Fatal(err)
	}
	if !created || first.Kind != runs.OriginKindCue || first.InvocationID != "cueinv-1" || first.CueID != "feature" {
		t.Fatalf("first origin = %+v created=%v", first, created)
	}

	kept, created, err := runs.EnsureTrailOrigin(root, "trace-1", runs.TrailOrigin{
		Kind:   runs.OriginKindManual,
		Source: "task.create",
	})
	if err != nil {
		t.Fatal(err)
	}
	if created {
		t.Fatal("second call replaced the origin")
	}
	if kept.Kind != runs.OriginKindCue || kept.InvocationID != "cueinv-1" || kept.CueID != "feature" {
		t.Fatalf("origin changed: %+v", kept)
	}
	if _, err := os.Stat(filepath.Join(root, ".paseka", "runs", "trace-1", runs.TrailOriginFileName)); err != nil {
		t.Fatalf("origin record missing: %v", err)
	}
}

func TestTrailOriginValidationDoesNotInventCueIdentity(t *testing.T) {
	root := t.TempDir()
	for _, origin := range []runs.TrailOrigin{
		{Kind: runs.OriginKindCue, InvocationID: "cueinv-1"},
		{Kind: runs.OriginKindCue, CueID: "feature"},
		{Kind: runs.OriginKindManual, CueID: "feature"},
		{Kind: runs.OriginKindFork, InvocationID: "cueinv-1"},
		{Kind: runs.OriginKindUnknown},
	} {
		if _, _, err := runs.EnsureTrailOrigin(root, "trace-invalid", origin); err == nil {
			t.Fatalf("accepted invalid origin %+v", origin)
		}
	}
}

func TestEnsureTrailOriginConcurrentCreationConverges(t *testing.T) {
	root := t.TempDir()
	const callers = 16
	var wg sync.WaitGroup
	results := make(chan runs.TrailOrigin, callers)
	created := make(chan bool, callers)
	for i := 0; i < callers; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			origin := runs.TrailOrigin{Kind: runs.OriginKindManual, Source: "task.create"}
			if i%2 == 0 {
				origin = runs.TrailOrigin{
					Kind:         runs.OriginKindCue,
					Source:       "cli",
					InvocationID: "cueinv-race",
					CueID:        "feature",
				}
			}
			got, wasCreated, err := runs.EnsureTrailOrigin(root, "trace-race", origin)
			if err != nil {
				t.Errorf("ensure origin: %v", err)
				return
			}
			results <- got
			created <- wasCreated
		}(i)
	}
	wg.Wait()
	close(results)
	close(created)

	winners := 0
	var winner runs.TrailOrigin
	for wasCreated := range created {
		if wasCreated {
			winners++
		}
	}
	if winners != 1 {
		t.Fatalf("created %d origins, want exactly one", winners)
	}
	for got := range results {
		if winner.Kind == "" {
			winner = got
		}
		if got.Kind != winner.Kind || got.InvocationID != winner.InvocationID || got.CueID != winner.CueID {
			t.Fatalf("concurrent callers saw different origins: %+v and %+v", got, winner)
		}
	}
}

func TestTrailOriginSurvivesWorktreeOnlyCleanup(t *testing.T) {
	root := t.TempDir()
	if _, _, err := runs.EnsureTrailOrigin(root, "trace-worktree", runs.TrailOrigin{
		Kind:   runs.OriginKindManual,
		Source: "task.create",
	}); err != nil {
		t.Fatal(err)
	}
	worktree := filepath.Join(root, ".paseka", "worktrees", "trace-worktree")
	if err := os.MkdirAll(worktree, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.RemoveAll(worktree); err != nil {
		t.Fatal(err)
	}
	origin, ok, err := runs.ReadTrailOrigin(root, "trace-worktree")
	if err != nil || !ok {
		t.Fatalf("origin after worktree cleanup: ok=%v err=%v", ok, err)
	}
	if origin.Kind != runs.OriginKindManual {
		t.Fatalf("origin = %+v", origin)
	}
}
