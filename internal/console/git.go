package console

import (
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"path/filepath"
	"strings"

	"github.com/paseka/paseka/internal/gitroot"
	"github.com/paseka/paseka/internal/worktree"
)

// GitCommitView is one unpublished commit.
type GitCommitView struct {
	SHA     string `json:"sha"`
	Subject string `json:"subject"`
}

// GitWorktreeView is one isolated worktree row.
type GitWorktreeView struct {
	TraceID string `json:"traceId,omitempty"`
	Path    string `json:"path"`
	Branch  string `json:"branch,omitempty"`
	BaseSHA string `json:"baseSha,omitempty"`
	Dirty   bool   `json:"dirty"`
}

// GitBranchView is one local branch row.
type GitBranchView struct {
	Name         string `json:"name"`
	Current      bool   `json:"current"`
	Default      bool   `json:"default"`
	Merged       bool   `json:"merged"`
	WorktreePath string `json:"worktreePath,omitempty"`
	TraceID      string `json:"traceId,omitempty"`
	Subject      string `json:"subject,omitempty"`
	Leftover     bool   `json:"leftover"`
}

// GitView is GET /api/git.
type GitView struct {
	Branch              string            `json:"branch"`
	HeadSHA             string            `json:"headSha"`
	HeadSHAShort        string            `json:"headShaShort"`
	Dirty               bool              `json:"dirty"`
	DefaultBranch       string            `json:"defaultBranch"`
	OriginURL           string            `json:"originUrl,omitempty"`
	Ahead               *int              `json:"ahead,omitempty"`
	Behind              *int              `json:"behind,omitempty"`
	LastFetchAgeSeconds *int64            `json:"lastFetchAgeSeconds,omitempty"`
	Unpublished         []GitCommitView   `json:"unpublished,omitempty"`
	Note                string            `json:"note,omitempty"`
	Worktrees           []GitWorktreeView `json:"worktrees"`
	Branches            []GitBranchView   `json:"branches"`
}

// GitActionResult is a POST mutation response.
type GitActionResult struct {
	OK      bool                  `json:"ok"`
	Message string                `json:"message,omitempty"`
	Results []GitBranchDeleteItem `json:"results,omitempty"`
}

// GitBranchDeleteItem is one name in a batch delete.
type GitBranchDeleteItem struct {
	Name  string `json:"name"`
	OK    bool   `json:"ok"`
	Error string `json:"error,omitempty"`
}

type gitPushBody struct {
	RunHooks bool `json:"runHooks"`
}

type gitDeleteBody struct {
	Names []string `json:"names"`
}

func (a *api) handleGit(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	view, err := GetGit(a.ctx.ColonyRoot, a.ctx.Slug)
	if err != nil {
		writeGitError(w, err)
		return
	}
	writeJSON(w, view)
}

func (a *api) handleGitFetch(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	out, err := gitroot.Fetch(a.ctx.ColonyRoot)
	if err != nil {
		writeGitError(w, err)
		return
	}
	writeJSON(w, GitActionResult{OK: true, Message: strings.TrimSpace(out)})
}

func (a *api) handleGitPush(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	var body gitPushBody
	if r.Body != nil {
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil && !errors.Is(err, io.EOF) {
			http.Error(w, "invalid json", http.StatusBadRequest)
			return
		}
	}
	out, err := gitroot.Push(gitroot.PushOpts{RepoRoot: a.ctx.ColonyRoot, RunHooks: body.RunHooks})
	if err != nil {
		writeGitError(w, err)
		return
	}
	writeJSON(w, GitActionResult{OK: true, Message: strings.TrimSpace(out)})
}

func (a *api) handleGitPull(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	if err := refusePullIfRootBees(a.ctx, a.sessions); err != nil {
		writeGitError(w, err)
		return
	}
	out, err := gitroot.PullFF(a.ctx.ColonyRoot)
	if err != nil {
		writeGitError(w, err)
		return
	}
	writeJSON(w, GitActionResult{OK: true, Message: strings.TrimSpace(out)})
}

func (a *api) handleGitBranchesDelete(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	var body gitDeleteBody
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		http.Error(w, "invalid json", http.StatusBadRequest)
		return
	}
	if len(body.Names) == 0 {
		writeGitError(w, gitroot.Refused("branch name is required"))
		return
	}
	result := GitActionResult{OK: true}
	for _, name := range body.Names {
		item := GitBranchDeleteItem{Name: name}
		if err := gitroot.DeleteBranch(a.ctx.ColonyRoot, name); err != nil {
			item.Error = err.Error()
			result.OK = false
		} else {
			item.OK = true
		}
		result.Results = append(result.Results, item)
	}
	writeJSON(w, result)
}

func (a *api) handleGitWorktreesPrune(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	res, err := worktree.PruneOrphans(a.ctx.ColonyRoot, a.ctx.Slug)
	if err != nil {
		writeGitError(w, err)
		return
	}
	msg := res.Message
	if len(res.Unregistered) > 0 {
		msg = strings.TrimSpace(msg + " unregistered: " + strings.Join(res.Unregistered, ", "))
	}
	writeJSON(w, GitActionResult{OK: true, Message: msg})
}

func writeGitError(w http.ResponseWriter, err error) {
	if errors.Is(err, gitroot.ErrRefused) {
		http.Error(w, err.Error(), http.StatusConflict)
		return
	}
	writeError(w, err)
}

// GetGit builds the plaque/tab snapshot without fetching.
func GetGit(colonyRoot, slug string) (GitView, error) {
	st, err := gitroot.Status(colonyRoot)
	if err != nil {
		return GitView{}, err
	}
	snaps, err := worktree.List(colonyRoot, slug)
	if err != nil {
		return GitView{}, err
	}
	byBranch := map[string]string{}
	wts := make([]GitWorktreeView, 0, len(snaps))
	for _, s := range snaps {
		wts = append(wts, GitWorktreeView{
			TraceID: s.TraceID,
			Path:    s.Path,
			Branch:  s.Branch,
			BaseSHA: s.BaseSHA,
			Dirty:   s.Dirty,
		})
		if s.Branch != "" {
			byBranch[s.Branch] = s.TraceID
		}
	}
	rows, err := gitroot.ListLocalBranches(colonyRoot)
	if err != nil {
		return GitView{}, err
	}
	branches := make([]GitBranchView, 0, len(rows))
	for _, b := range rows {
		tid := byBranch[b.Name]
		if tid == "" && b.WorktreePath != "" {
			tid = filepath.Base(b.WorktreePath)
			if tid == filepath.Base(colonyRoot) {
				tid = ""
			}
		}
		branches = append(branches, GitBranchView{
			Name:         b.Name,
			Current:      b.Current,
			Default:      b.Default,
			Merged:       b.Merged,
			WorktreePath: b.WorktreePath,
			TraceID:      tid,
			Subject:      b.Subject,
			Leftover:     b.Leftover,
		})
	}
	unpublished := make([]GitCommitView, 0, len(st.Unpublished))
	for _, c := range st.Unpublished {
		unpublished = append(unpublished, GitCommitView{SHA: c.SHA, Subject: c.Subject})
	}
	return GitView{
		Branch:              st.Branch,
		HeadSHA:             st.HeadSHA,
		HeadSHAShort:        st.HeadSHAShort,
		Dirty:               st.Dirty,
		DefaultBranch:       st.DefaultBranch,
		OriginURL:           st.OriginURL,
		Ahead:               st.Ahead,
		Behind:              st.Behind,
		LastFetchAgeSeconds: st.LastFetchAgeSeconds,
		Unpublished:         unpublished,
		Note:                st.Note,
		Worktrees:           wts,
		Branches:            branches,
	}, nil
}
