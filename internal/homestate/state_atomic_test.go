package homestate_test

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"

	"github.com/russ-p/paseka/internal/homestate"
)

func TestLoadStateToleratesEmptyFile(t *testing.T) {
	slug, homeDir := setupStateHome(t)
	if err := os.WriteFile(filepath.Join(homeDir, "state.json"), nil, 0o644); err != nil {
		t.Fatal(err)
	}
	st, err := homestate.LoadState(slug)
	if err != nil {
		t.Fatalf("empty state must read as a fresh registry: %v", err)
	}
	if len(st.Worktrees) != 0 || len(st.Sessions) != 0 || len(st.Invites) != 0 || st.Runtime != nil {
		t.Fatalf("state = %+v, want zero value", st)
	}
}

func TestSaveStateLeavesNoTempFile(t *testing.T) {
	slug, homeDir := setupStateHome(t)
	if err := homestate.RegisterWorktree(slug, homestate.WorktreeEntry{TraceID: "trace-1", Path: "/tmp/wt"}); err != nil {
		t.Fatal(err)
	}
	entries, err := os.ReadDir(homeDir)
	if err != nil {
		t.Fatal(err)
	}
	for _, entry := range entries {
		if strings.HasPrefix(entry.Name(), ".state-") {
			t.Fatalf("temporary state file left behind: %s", entry.Name())
		}
	}
}

func TestConcurrentWorktreeRegistrationsKeepEveryEntry(t *testing.T) {
	slug, _ := setupStateHome(t)
	const count = 24
	var wg sync.WaitGroup
	errs := make(chan error, count)
	for i := 0; i < count; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			err := homestate.RegisterWorktree(slug, homestate.WorktreeEntry{
				TraceID: fmt.Sprintf("trace-%02d", i),
				Path:    fmt.Sprintf("/tmp/wt-%02d", i),
			})
			if err != nil {
				errs <- err
			}
		}(i)
	}
	wg.Wait()
	close(errs)
	for err := range errs {
		t.Fatal(err)
	}

	st, err := homestate.LoadState(slug)
	if err != nil {
		t.Fatal(err)
	}
	if len(st.Worktrees) != count {
		t.Fatalf("worktrees = %d, want %d (a concurrent write lost entries)", len(st.Worktrees), count)
	}
	seen := make(map[string]struct{}, count)
	for _, entry := range st.Worktrees {
		seen[entry.TraceID] = struct{}{}
	}
	for i := 0; i < count; i++ {
		id := fmt.Sprintf("trace-%02d", i)
		if _, ok := seen[id]; !ok {
			t.Fatalf("missing %s after concurrent registration", id)
		}
	}
}

// A reader must never observe a truncated file. Before the write was atomic,
// truncate-then-write exposed an empty or half-written state.json and every
// concurrent reader failed with "unexpected end of JSON input".
func TestConcurrentReadsNeverSeePartialState(t *testing.T) {
	slug, _ := setupStateHome(t)
	if err := homestate.RegisterWorktree(slug, homestate.WorktreeEntry{TraceID: "trace-seed", Path: "/tmp/wt"}); err != nil {
		t.Fatal(err)
	}
	state, err := homestate.LoadState(slug)
	if err != nil {
		t.Fatal(err)
	}

	var wg sync.WaitGroup
	errs := make(chan error, 8)
	wg.Add(1)
	go func() {
		defer wg.Done()
		for i := 0; i < 200; i++ {
			if err := homestate.SaveState(slug, state); err != nil {
				errs <- err
				return
			}
		}
	}()
	for reader := 0; reader < 4; reader++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for i := 0; i < 400; i++ {
				if _, err := homestate.LoadState(slug); err != nil {
					errs <- err
					return
				}
			}
		}()
	}
	wg.Wait()
	close(errs)
	for err := range errs {
		t.Fatalf("reader observed a damaged state: %v", err)
	}
}
