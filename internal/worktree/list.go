package worktree

import (
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/paseka/paseka/internal/gitroot"
	"github.com/paseka/paseka/internal/homestate"
)

// Snapshot is one colony-managed or leftover isolated worktree.
type Snapshot struct {
	TraceID string
	Path    string
	Branch  string
	BaseSHA string
	Dirty   bool
}

// List combines the home registry, git worktree list, and .paseka/worktrees/ on disk.
func List(colonyRoot, slug string) ([]Snapshot, error) {
	colonyRoot, err := absPath(colonyRoot)
	if err != nil {
		return nil, err
	}
	byPath := map[string]*Snapshot{}

	if slug != "" {
		st, err := homestate.LoadState(slug)
		if err != nil {
			return nil, err
		}
		for _, w := range st.Worktrees {
			p := filepath.Clean(w.Path)
			byPath[p] = &Snapshot{
				TraceID: w.TraceID,
				Path:    p,
				Branch:  w.Branch,
				BaseSHA: w.BaseSHA,
			}
		}
	}

	porcelain, err := gitroot.Run(gitroot.RunOpts{
		Dir:     colonyRoot,
		Timeout: gitroot.StatusTimeout,
		Args:    []string{"worktree", "list", "--porcelain"},
	})
	if err != nil {
		return nil, err
	}
	mergePorcelain(byPath, colonyRoot, porcelain)

	wtRoot := filepath.Join(colonyRoot, ".paseka", "worktrees")
	entries, err := os.ReadDir(wtRoot)
	if err != nil && !os.IsNotExist(err) {
		return nil, err
	}
	for _, e := range entries {
		if !e.IsDir() {
			continue
		}
		p := filepath.Join(wtRoot, e.Name())
		p = filepath.Clean(p)
		if _, ok := byPath[p]; ok {
			continue
		}
		byPath[p] = &Snapshot{
			TraceID: e.Name(),
			Path:    p,
		}
	}

	out := make([]Snapshot, 0, len(byPath))
	for _, snap := range byPath {
		fillSnapshot(snap)
		if !gitroot.IsCheckoutRoot(snap.Path) {
			continue
		}
		out = append(out, *snap)
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].TraceID != out[j].TraceID {
			return out[i].TraceID < out[j].TraceID
		}
		return out[i].Path < out[j].Path
	})
	return out, nil
}

func mergePorcelain(byPath map[string]*Snapshot, colonyRoot, porcelain string) {
	var currentPath, head, branch string
	flush := func() {
		if currentPath == "" {
			return
		}
		p := filepath.Clean(currentPath)
		if p == filepath.Clean(colonyRoot) {
			currentPath, head, branch = "", "", ""
			return
		}
		snap := byPath[p]
		if snap == nil {
			snap = &Snapshot{Path: p}
			if rel, err := filepath.Rel(filepath.Join(colonyRoot, ".paseka", "worktrees"), p); err == nil && rel != "." && !strings.HasPrefix(rel, "..") {
				snap.TraceID = filepath.Base(p)
			}
			byPath[p] = snap
		}
		if branch != "" && snap.Branch == "" {
			snap.Branch = branch
		}
		if head != "" && snap.BaseSHA == "" {
			snap.BaseSHA = head
		}
		currentPath, head, branch = "", "", ""
	}
	for line := range strings.SplitSeq(porcelain, "\n") {
		switch {
		case line == "":
			flush()
		case strings.HasPrefix(line, "worktree "):
			flush()
			currentPath = strings.TrimPrefix(line, "worktree ")
		case strings.HasPrefix(line, "HEAD "):
			head = strings.TrimPrefix(line, "HEAD ")
		case strings.HasPrefix(line, "branch "):
			branch = strings.TrimPrefix(strings.TrimPrefix(line, "branch "), "refs/heads/")
		}
	}
	flush()
}

func fillSnapshot(snap *Snapshot) {
	if snap.Path == "" {
		return
	}
	if gitroot.IsCheckoutRoot(snap.Path) {
		if snap.Branch == "" || snap.Branch == "HEAD" {
			if b, err := gitroot.DefaultBranch(snap.Path); err == nil {
				snap.Branch = b
			}
		}
		if snap.BaseSHA == "" {
			if sha, err := gitroot.Run(gitroot.RunOpts{
				Dir:     snap.Path,
				Timeout: gitroot.StatusTimeout,
				Args:    []string{"rev-parse", "HEAD"},
			}); err == nil {
				snap.BaseSHA = strings.TrimSpace(sha)
			}
		}
		if dirty, err := gitroot.Dirty(snap.Path); err == nil {
			snap.Dirty = dirty
		}
		if snap.TraceID == "" {
			snap.TraceID = filepath.Base(snap.Path)
		}
	}
}

// PruneResult reports orphan cleanup.
type PruneResult struct {
	Message      string
	Unregistered []string
	RemovedDirs  []string
}

// PruneOrphans runs git worktree prune, drops stale registry rows, and removes empty leftover dirs.
// It never force-removes a registered live checkout.
func PruneOrphans(colonyRoot, slug string) (PruneResult, error) {
	var res PruneResult
	colonyRoot, err := absPath(colonyRoot)
	if err != nil {
		return res, err
	}

	out, err := gitroot.Run(gitroot.RunOpts{
		Dir:     colonyRoot,
		Timeout: gitroot.StatusTimeout,
		Args:    []string{"worktree", "prune"},
	})
	if err != nil {
		return res, err
	}
	res.Message = strings.TrimSpace(out)

	if slug != "" {
		st, err := homestate.LoadState(slug)
		if err != nil {
			return res, err
		}
		for _, w := range st.Worktrees {
			live := gitroot.IsCheckoutRoot(w.Path)
			if live {
				continue
			}
			if err := homestate.UnregisterWorktree(slug, w.TraceID); err != nil {
				return res, err
			}
			res.Unregistered = append(res.Unregistered, w.TraceID)
		}
	}

	wtRoot := filepath.Join(colonyRoot, ".paseka", "worktrees")
	entries, err := os.ReadDir(wtRoot)
	if err != nil && !os.IsNotExist(err) {
		return res, err
	}
	for _, e := range entries {
		if !e.IsDir() {
			continue
		}
		p := filepath.Join(wtRoot, e.Name())
		if gitroot.IsCheckoutRoot(p) {
			continue
		}
		empty, err := dirEmpty(p)
		if err != nil {
			return res, err
		}
		if !empty {
			continue
		}
		if err := os.Remove(p); err != nil {
			return res, err
		}
		res.RemovedDirs = append(res.RemovedDirs, p)
	}
	if res.Message == "" && len(res.Unregistered) == 0 && len(res.RemovedDirs) == 0 {
		res.Message = "no orphan worktrees"
	}
	return res, nil
}

func dirEmpty(path string) (bool, error) {
	ents, err := os.ReadDir(path)
	if err != nil {
		return false, err
	}
	return len(ents) == 0, nil
}
