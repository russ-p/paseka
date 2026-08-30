package gitroot

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"time"
)

const unpublishedCap = 20

// UnpublishedCommit is one commit in origin/<default>..HEAD.
type UnpublishedCommit struct {
	SHA     string `json:"sha"`
	Subject string `json:"subject"`
}

// RepoStatus is a read-only snapshot of the colony root vs origin. Never fetches.
type RepoStatus struct {
	Branch              string
	HeadSHA             string
	HeadSHAShort        string
	Dirty               bool
	DefaultBranch       string
	OriginURL           string
	Ahead               *int
	Behind              *int
	LastFetchAgeSeconds *int64
	Unpublished         []UnpublishedCommit
	Note                string
}

// BranchInfo is one local refs/heads/* row.
type BranchInfo struct {
	Name         string
	Current      bool
	Default      bool
	Merged       bool
	WorktreePath string
	Subject      string
	Leftover     bool
}

// PushOpts configures git push of the default branch.
type PushOpts struct {
	RepoRoot string
	RunHooks bool
}

// ResolvedDefaultBranch is HEAD's branch name, or "main" when detached/empty.
func ResolvedDefaultBranch(repoRoot string) (string, error) {
	b, err := DefaultBranch(repoRoot)
	if err != nil {
		return "", err
	}
	if b == "" || b == "HEAD" {
		return "main", nil
	}
	return b, nil
}

// TrackingRef is refs/remotes/origin/<default>.
func TrackingRef(defaultBranch string) string {
	return "refs/remotes/origin/" + strings.TrimSpace(defaultBranch)
}

// RemoteTrackingExists reports whether origin/<default> is present (no fetch).
func RemoteTrackingExists(repoRoot, defaultBranch string) bool {
	if strings.TrimSpace(defaultBranch) == "" {
		return false
	}
	_, err := Run(RunOpts{
		Dir:       repoRoot,
		Timeout:   StatusTimeout,
		AllowFail: true,
		Args:      []string{"show-ref", "--verify", "--quiet", "--", TrackingRef(defaultBranch)},
	})
	return err == nil
}

// Dirty reports uncommitted or untracked files (porcelain).
func Dirty(repoRoot string) (bool, error) {
	out, err := Run(RunOpts{
		Dir:     repoRoot,
		Timeout: StatusTimeout,
		Args:    []string{"status", "--porcelain"},
	})
	if err != nil {
		return false, err
	}
	return strings.TrimSpace(out) != "", nil
}

// LastFetchAgeSeconds is best-effort age of FETCH_HEAD. Omit (nil) when unknown.
func LastFetchAgeSeconds(repoRoot string) *int64 {
	pathOut, err := Run(RunOpts{
		Dir:     repoRoot,
		Timeout: StatusTimeout,
		Args:    []string{"rev-parse", "--git-path", "FETCH_HEAD"},
	})
	if err != nil {
		return nil
	}
	p := strings.TrimSpace(pathOut)
	if p == "" {
		return nil
	}
	if !filepath.IsAbs(p) {
		p = filepath.Join(repoRoot, p)
	}
	st, err := os.Stat(p)
	if err != nil || st.Size() == 0 {
		return nil
	}
	age := time.Since(st.ModTime())
	if age < 0 {
		age = 0
	}
	sec := int64(age.Seconds())
	return &sec
}

// AheadBehind vs origin/<default>. ok is false when the tracking ref is missing.
func AheadBehind(repoRoot, defaultBranch string) (ahead, behind int, ok bool, err error) {
	if !RemoteTrackingExists(repoRoot, defaultBranch) {
		return 0, 0, false, nil
	}
	spec := "origin/" + defaultBranch + "...HEAD"
	out, err := Run(RunOpts{
		Dir:     repoRoot,
		Timeout: StatusTimeout,
		Args:    []string{"rev-list", "--left-right", "--count", spec},
	})
	if err != nil {
		return 0, 0, false, err
	}
	fields := strings.Fields(strings.TrimSpace(out))
	if len(fields) != 2 {
		return 0, 0, false, fmt.Errorf("git: unexpected ahead/behind output %q", strings.TrimSpace(out))
	}
	behind, err = strconv.Atoi(fields[0])
	if err != nil {
		return 0, 0, false, err
	}
	ahead, err = strconv.Atoi(fields[1])
	if err != nil {
		return 0, 0, false, err
	}
	return ahead, behind, true, nil
}

// Status snapshots the repo without fetching.
func Status(repoRoot string) (RepoStatus, error) {
	var st RepoStatus
	def, err := ResolvedDefaultBranch(repoRoot)
	if err != nil {
		return st, err
	}
	st.DefaultBranch = def
	branch, err := DefaultBranch(repoRoot)
	if err != nil {
		return st, err
	}
	st.Branch = branch

	full, err := Run(RunOpts{
		Dir:     repoRoot,
		Timeout: StatusTimeout,
		Args:    []string{"rev-parse", "HEAD"},
	})
	if err != nil {
		return st, err
	}
	st.HeadSHA = strings.TrimSpace(full)
	short, err := Run(RunOpts{
		Dir:     repoRoot,
		Timeout: StatusTimeout,
		Args:    []string{"rev-parse", "--short", "HEAD"},
	})
	if err != nil {
		return st, err
	}
	st.HeadSHAShort = strings.TrimSpace(short)

	dirty, err := Dirty(repoRoot)
	if err != nil {
		return st, err
	}
	st.Dirty = dirty

	origin, err := OriginURL(repoRoot)
	if err != nil {
		return st, err
	}
	st.OriginURL = origin
	if origin == "" {
		st.Note = "no origin remote"
		return st, nil
	}

	st.LastFetchAgeSeconds = LastFetchAgeSeconds(repoRoot)

	ahead, behind, ok, err := AheadBehind(repoRoot, def)
	if err != nil {
		return st, err
	}
	if !ok {
		st.Note = "fetch to update remote-tracking refs"
		return st, nil
	}
	st.Ahead = &ahead
	st.Behind = &behind

	unpublished, err := unpublishedCommits(repoRoot, def)
	if err != nil {
		return st, err
	}
	st.Unpublished = unpublished
	return st, nil
}

func unpublishedCommits(repoRoot, defaultBranch string) ([]UnpublishedCommit, error) {
	if !RemoteTrackingExists(repoRoot, defaultBranch) {
		return nil, nil
	}
	rangeSpec := "origin/" + defaultBranch + "..HEAD"
	out, err := Run(RunOpts{
		Dir:     repoRoot,
		Timeout: StatusTimeout,
		Args:    []string{"log", "--format=%H%x09%s", "-n", strconv.Itoa(unpublishedCap + 1), rangeSpec},
	})
	if err != nil {
		return nil, err
	}
	var commits []UnpublishedCommit
	for line := range strings.SplitSeq(strings.TrimSpace(out), "\n") {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		sha, subject, _ := strings.Cut(line, "\t")
		commits = append(commits, UnpublishedCommit{SHA: sha, Subject: subject})
		if len(commits) >= unpublishedCap {
			break
		}
	}
	return commits, nil
}

// Fetch updates remote-tracking refs only (`git fetch origin`).
func Fetch(repoRoot string) (string, error) {
	origin, err := OriginURL(repoRoot)
	if err != nil {
		return "", err
	}
	if origin == "" {
		return "", Refused("no origin remote")
	}
	return Run(RunOpts{
		Dir:     repoRoot,
		Timeout: NetworkTimeout,
		Args:    []string{"fetch", "origin"},
	})
}

// Push publishes the default branch to origin. Never --force.
func Push(opts PushOpts) (string, error) {
	repoRoot := opts.RepoRoot
	origin, err := OriginURL(repoRoot)
	if err != nil {
		return "", err
	}
	if origin == "" {
		return "", Refused("no origin remote")
	}
	def, err := ResolvedDefaultBranch(repoRoot)
	if err != nil {
		return "", err
	}
	current, err := DefaultBranch(repoRoot)
	if err != nil {
		return "", err
	}
	if current != def {
		return "", Refused("HEAD is not the default branch " + def)
	}
	ahead, behind, ok, err := AheadBehind(repoRoot, def)
	if err != nil {
		return "", err
	}
	if ok && behind > 0 {
		return "", Refused("branch is behind origin/" + def + " (non-fast-forward; never --force)")
	}
	if ok && ahead == 0 {
		return "", Refused("nothing to push; already up to date with origin/" + def)
	}

	args := []string{"push", "origin", def}
	var extra []string
	if !opts.RunHooks {
		args = []string{"push", "--no-verify", "origin", def}
		extra = []string{"HUSKY=0"}
	}
	return Run(RunOpts{
		Dir:      repoRoot,
		Timeout:  NetworkTimeout,
		Args:     args,
		ExtraEnv: extra,
	})
}

// PullFF fast-forwards the default branch from origin. Refuses dirty root and in-progress ops.
func PullFF(repoRoot string) (string, error) {
	origin, err := OriginURL(repoRoot)
	if err != nil {
		return "", err
	}
	if origin == "" {
		return "", Refused("no origin remote")
	}
	def, err := ResolvedDefaultBranch(repoRoot)
	if err != nil {
		return "", err
	}
	current, err := DefaultBranch(repoRoot)
	if err != nil {
		return "", err
	}
	if current != def {
		return "", Refused("HEAD is not the default branch " + def)
	}
	dirty, err := Dirty(repoRoot)
	if err != nil {
		return "", err
	}
	if dirty {
		return "", Refused("colony root is dirty; refuse ff-only pull")
	}
	if kind, err := OperationInProgress(repoRoot); err != nil {
		return "", err
	} else if kind != "" {
		return "", Refused("git " + kind + " in progress")
	}
	ahead, behind, ok, err := AheadBehind(repoRoot, def)
	if err != nil {
		return "", err
	}
	if ok && ahead > 0 && behind > 0 {
		return "", Refused("histories have diverged; refuse non-fast-forward pull")
	}
	out, err := Run(RunOpts{
		Dir:     repoRoot,
		Timeout: NetworkTimeout,
		Args:    []string{"pull", "--ff-only", "origin", def},
	})
	if err != nil {
		if strings.Contains(out, "Not possible to fast-forward") || strings.Contains(out, "diverging") {
			return out, Refused("histories have diverged; refuse non-fast-forward pull")
		}
		return out, err
	}
	return out, nil
}

// OperationInProgress reports merge, rebase, or cherry-pick when a lock file exists.
func OperationInProgress(repoRoot string) (string, error) {
	checks := []struct {
		gitPath string
		kind    string
	}{
		{"MERGE_HEAD", "merge"},
		{"CHERRY_PICK_HEAD", "cherry-pick"},
		{"rebase-merge", "rebase"},
		{"rebase-apply", "rebase"},
	}
	for _, c := range checks {
		p, err := gitPath(repoRoot, c.gitPath)
		if err != nil {
			continue
		}
		if _, err := os.Stat(p); err == nil {
			return c.kind, nil
		}
	}
	return "", nil
}

func gitPath(repoRoot, name string) (string, error) {
	out, err := Run(RunOpts{
		Dir:     repoRoot,
		Timeout: StatusTimeout,
		Args:    []string{"rev-parse", "--git-path", name},
	})
	if err != nil {
		return "", err
	}
	p := strings.TrimSpace(out)
	if p == "" {
		return "", fmt.Errorf("git: empty git path for %s", name)
	}
	if !filepath.IsAbs(p) {
		p = filepath.Join(repoRoot, p)
	}
	return p, nil
}

// IsLeftoverName is paseka/* or conventional feature/hotfix/fix prefixes.
func IsLeftoverName(name string) bool {
	name = strings.TrimSpace(name)
	return strings.HasPrefix(name, "paseka/") ||
		strings.HasPrefix(name, "feature/") ||
		strings.HasPrefix(name, "hotfix/") ||
		strings.HasPrefix(name, "fix/")
}

// ListLocalBranches lists refs/heads/* with merge/worktree flags. Does not fetch.
func ListLocalBranches(repoRoot string) ([]BranchInfo, error) {
	def, err := ResolvedDefaultBranch(repoRoot)
	if err != nil {
		return nil, err
	}
	current, err := DefaultBranch(repoRoot)
	if err != nil {
		return nil, err
	}
	occupied, err := worktreeBranches(repoRoot)
	if err != nil {
		return nil, err
	}
	out, err := Run(RunOpts{
		Dir:     repoRoot,
		Timeout: StatusTimeout,
		Args:    []string{"for-each-ref", "--format=%(refname:short)%09%(contents:subject)", "refs/heads/"},
	})
	if err != nil {
		return nil, err
	}
	var rows []BranchInfo
	for line := range strings.SplitSeq(strings.TrimRight(out, "\n"), "\n") {
		if strings.TrimSpace(line) == "" {
			continue
		}
		name, subject, _ := strings.Cut(line, "\t")
		wt := occupied[name]
		merged, err := branchMerged(repoRoot, name, def)
		if err != nil {
			return nil, err
		}
		row := BranchInfo{
			Name:         name,
			Current:      name == current,
			Default:      name == def,
			Merged:       merged,
			WorktreePath: wt,
			Subject:      subject,
		}
		row.Leftover = IsLeftoverName(name) && merged && !row.Default && wt == ""
		rows = append(rows, row)
	}
	sort.Slice(rows, func(i, j int) bool {
		if rows[i].Leftover != rows[j].Leftover {
			return rows[i].Leftover
		}
		return rows[i].Name < rows[j].Name
	})
	return rows, nil
}

func worktreeBranches(repoRoot string) (map[string]string, error) {
	out, err := Run(RunOpts{
		Dir:     repoRoot,
		Timeout: StatusTimeout,
		Args:    []string{"worktree", "list", "--porcelain"},
	})
	if err != nil {
		return nil, err
	}
	occupied := map[string]string{}
	var currentPath string
	for line := range strings.SplitSeq(out, "\n") {
		if strings.HasPrefix(line, "worktree ") {
			currentPath = strings.TrimPrefix(line, "worktree ")
			continue
		}
		if strings.HasPrefix(line, "branch ") {
			ref := strings.TrimPrefix(line, "branch ")
			ref = strings.TrimPrefix(ref, "refs/heads/")
			if ref != "" && currentPath != "" {
				occupied[ref] = currentPath
			}
		}
	}
	return occupied, nil
}

func branchMerged(repoRoot, name, defaultBranch string) (bool, error) {
	if name == defaultBranch {
		return true, nil
	}
	_, err := Run(RunOpts{
		Dir:     repoRoot,
		Timeout: StatusTimeout,
		Args:    []string{"merge-base", "--is-ancestor", "refs/heads/" + name, "refs/heads/" + defaultBranch},
	})
	if err != nil {
		return false, nil
	}
	return true, nil
}

// DeleteBranch runs git branch -d after policy guards. Never -D.
func DeleteBranch(repoRoot, name string) error {
	name = strings.TrimSpace(name)
	if name == "" {
		return Refused("branch name is required")
	}
	rows, err := ListLocalBranches(repoRoot)
	if err != nil {
		return err
	}
	var row *BranchInfo
	for i := range rows {
		if rows[i].Name == name {
			row = &rows[i]
			break
		}
	}
	if row == nil {
		return Refused("branch not found: " + name)
	}
	if row.Default {
		return Refused("cannot delete the default branch")
	}
	if row.Current {
		return Refused("cannot delete HEAD")
	}
	if row.WorktreePath != "" {
		return Refused("branch is checked out in a worktree")
	}
	if !row.Merged {
		return Refused("branch is not merged into the default branch")
	}
	_, err = Run(RunOpts{
		Dir:     repoRoot,
		Timeout: StatusTimeout,
		Args:    []string{"branch", "-d", "--", name},
	})
	return err
}
