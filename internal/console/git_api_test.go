package console_test

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/paseka/paseka/internal/console"
	"github.com/paseka/paseka/internal/gitroot"
	"github.com/paseka/paseka/internal/protocol"
	"github.com/paseka/paseka/internal/runs"
	"github.com/paseka/paseka/internal/sessions"
	"github.com/paseka/paseka/internal/worktree"
)

func TestGitAPIStatusNoOrigin(t *testing.T) {
	repo := initConsoleRepo(t)
	ctxColony := setupConsoleHome(t, repo)
	srv := console.NewServer(console.Options{
		Addr:     "127.0.0.1:0",
		Colony:   ctxColony,
		Sessions: sessions.NewManager(),
	})
	req := httptest.NewRequest(http.MethodGet, "/api/git", nil)
	rec := httptest.NewRecorder()
	srv.Handler().ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d body=%s", rec.Code, rec.Body.String())
	}
	var view console.GitView
	if err := json.NewDecoder(rec.Body).Decode(&view); err != nil {
		t.Fatal(err)
	}
	if view.OriginURL != "" {
		t.Fatalf("origin = %q", view.OriginURL)
	}
	if !strings.Contains(view.Note, "no origin") {
		t.Fatalf("note = %q", view.Note)
	}
}

func TestGitAPIFetchPushPullAndDelete(t *testing.T) {
	repo := initConsoleRepo(t)
	if err := os.WriteFile(filepath.Join(repo, ".gitignore"), []byte(".paseka/\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	runGit(t, repo, "add", ".gitignore")
	runGit(t, repo, "commit", "-m", "gitignore paseka")
	bare := t.TempDir()
	runGit(t, bare, "init", "--bare")
	def, err := gitroot.ResolvedDefaultBranch(repo)
	if err != nil {
		t.Fatal(err)
	}
	runGit(t, repo, "remote", "add", "origin", bare)
	runGit(t, repo, "push", "-u", "origin", def)

	ctxColony := setupConsoleHome(t, repo)
	srv := console.NewServer(console.Options{
		Addr:     "127.0.0.1:0",
		Colony:   ctxColony,
		Sessions: sessions.NewManager(),
	})

	if rec := gitPOST(t, srv, "/api/git/push", `{}`); rec.Code != http.StatusConflict {
		t.Fatalf("push up-to-date = %d %s", rec.Code, rec.Body.String())
	}

	if err := os.WriteFile(filepath.Join(repo, "pub.txt"), []byte("pub\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	runGit(t, repo, "add", "pub.txt")
	runGit(t, repo, "commit", "-m", "unpublished")

	get := httptest.NewRequest(http.MethodGet, "/api/git", nil)
	grec := httptest.NewRecorder()
	srv.Handler().ServeHTTP(grec, get)
	var view console.GitView
	if err := json.NewDecoder(grec.Body).Decode(&view); err != nil {
		t.Fatal(err)
	}
	if view.Ahead == nil || *view.Ahead != 1 {
		t.Fatalf("ahead = %v", view.Ahead)
	}

	prec := gitPOST(t, srv, "/api/git/push", `{"runHooks":false}`)
	if prec.Code != http.StatusOK {
		t.Fatalf("push = %d %s", prec.Code, prec.Body.String())
	}

	other := t.TempDir()
	cmd := exec.Command("git", "clone", bare, other)
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("clone: %v\n%s", err, out)
	}
	runGit(t, other, "config", "user.email", "test@test.com")
	runGit(t, other, "config", "user.name", "test")
	if err := os.WriteFile(filepath.Join(other, "ff.txt"), []byte("ff\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	runGit(t, other, "add", "ff.txt")
	runGit(t, other, "commit", "-m", "from origin")
	runGit(t, other, "push", "origin", def)

	beforeTrack := gitOut(t, repo, "rev-parse", "refs/remotes/origin/"+def)
	get2 := httptest.NewRequest(http.MethodGet, "/api/git", nil)
	grec2 := httptest.NewRecorder()
	srv.Handler().ServeHTTP(grec2, get2)
	afterTrack := gitOut(t, repo, "rev-parse", "refs/remotes/origin/"+def)
	if beforeTrack != afterTrack {
		t.Fatal("GET /api/git must not fetch")
	}

	frec := gitPOST(t, srv, "/api/git/fetch", "")
	if frec.Code != http.StatusOK {
		t.Fatalf("fetch = %d %s", frec.Code, frec.Body.String())
	}
	if gitOut(t, repo, "rev-parse", "HEAD") == gitOut(t, other, "rev-parse", "HEAD") {
		t.Fatal("fetch must not move HEAD")
	}

	pull := gitPOST(t, srv, "/api/git/pull", "")
	if pull.Code != http.StatusOK {
		t.Fatalf("pull = %d %s", pull.Code, pull.Body.String())
	}
	if !fileExists(filepath.Join(repo, "ff.txt")) {
		t.Fatal("expected ff.txt after pull")
	}

	runGit(t, repo, "branch", "feature/leftover")
	del := gitPOST(t, srv, "/api/git/branches/delete", `{"names":["feature/leftover","`+def+`"]}`)
	if del.Code != http.StatusOK {
		t.Fatalf("delete = %d %s", del.Code, del.Body.String())
	}
	var delRes console.GitActionResult
	if err := json.NewDecoder(del.Body).Decode(&delRes); err != nil {
		t.Fatal(err)
	}
	if delRes.OK {
		t.Fatal("expected partial batch failure")
	}
	if len(delRes.Results) != 2 {
		t.Fatalf("results = %+v", delRes.Results)
	}

	pr := gitPOST(t, srv, "/api/git/worktrees/prune", "")
	if pr.Code != http.StatusOK {
		t.Fatalf("prune = %d %s", pr.Code, pr.Body.String())
	}
}

func TestGitAPIPullRefusesRootBee(t *testing.T) {
	repo := initConsoleRepo(t)
	bare := t.TempDir()
	runGit(t, bare, "init", "--bare")
	def, err := gitroot.ResolvedDefaultBranch(repo)
	if err != nil {
		t.Fatal(err)
	}
	runGit(t, repo, "remote", "add", "origin", bare)
	runGit(t, repo, "push", "-u", "origin", def)
	ctxColony := setupConsoleHome(t, repo)
	writeLiveAFKRun(t, repo, "trace-root", "agent-1", "scout", time.Now(), os.Getpid())

	srv := console.NewServer(console.Options{
		Addr:     "127.0.0.1:0",
		Colony:   ctxColony,
		Sessions: sessions.NewManager(),
	})
	rec := gitPOST(t, srv, "/api/git/pull", "")
	if rec.Code != http.StatusConflict {
		t.Fatalf("status = %d body=%s", rec.Code, rec.Body.String())
	}
	if !strings.Contains(rec.Body.String(), "colony root") {
		t.Fatalf("body = %s", rec.Body.String())
	}
}

func TestGitAPIPullRefusesUnclassifiedBee(t *testing.T) {
	repo := initConsoleRepo(t)
	bare := t.TempDir()
	runGit(t, bare, "init", "--bare")
	def, err := gitroot.ResolvedDefaultBranch(repo)
	if err != nil {
		t.Fatal(err)
	}
	runGit(t, repo, "remote", "add", "origin", bare)
	runGit(t, repo, "push", "-u", "origin", def)
	ctxColony := setupConsoleHome(t, repo)
	outside := t.TempDir()
	writeLiveAFKRunWorkspace(t, repo, outside, "trace-unknown", "agent-u", "scout", time.Now(), os.Getpid())

	srv := console.NewServer(console.Options{
		Addr:     "127.0.0.1:0",
		Colony:   ctxColony,
		Sessions: sessions.NewManager(),
	})
	rec := gitPOST(t, srv, "/api/git/pull", "")
	if rec.Code != http.StatusConflict {
		t.Fatalf("status = %d body=%s", rec.Code, rec.Body.String())
	}
	if !strings.Contains(rec.Body.String(), "classified") {
		t.Fatalf("body = %s", rec.Body.String())
	}
}

func TestGitAPIMergeDiffOriginBehind(t *testing.T) {
	repo := initConsoleRepo(t)
	bare := t.TempDir()
	runGit(t, bare, "init", "--bare")
	def, err := gitroot.ResolvedDefaultBranch(repo)
	if err != nil {
		t.Fatal(err)
	}
	runGit(t, repo, "remote", "add", "origin", bare)
	runGit(t, repo, "push", "-u", "origin", def)

	ctxColony := setupConsoleHome(t, repo)
	traceID := "trace-git-behind"
	entry, err := worktree.Ensure(worktree.EnsureOptions{
		ColonyRoot: repo,
		TraceID:    traceID,
		Slug:       ctxColony.Slug,
	})
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(entry.Path, "feature.txt"), []byte("x\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	runGit(t, entry.Path, "add", "feature.txt")
	runGit(t, entry.Path, "commit", "-m", "feature")

	other := t.TempDir()
	cmd := exec.Command("git", "clone", bare, other)
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("clone: %v\n%s", err, out)
	}
	runGit(t, other, "config", "user.email", "test@test.com")
	runGit(t, other, "config", "user.name", "test")
	if err := os.WriteFile(filepath.Join(other, "origin.txt"), []byte("o\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	runGit(t, other, "add", "origin.txt")
	runGit(t, other, "commit", "-m", "origin")
	runGit(t, other, "push", "origin", def)
	if _, err := gitroot.Fetch(repo); err != nil {
		t.Fatal(err)
	}

	srv := console.NewServer(console.Options{
		Addr:     "127.0.0.1:0",
		Colony:   ctxColony,
		Sessions: sessions.NewManager(),
	})
	req := httptest.NewRequest(http.MethodGet, "/api/traces/"+traceID+"/merge-diff", nil)
	rec := httptest.NewRecorder()
	srv.Handler().ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d body=%s", rec.Code, rec.Body.String())
	}
	var view console.MergeDiffView
	if err := json.NewDecoder(rec.Body).Decode(&view); err != nil {
		t.Fatal(err)
	}
	if view.OriginBehindCount == nil || *view.OriginBehindCount < 1 {
		t.Fatalf("originBehindCount = %v", view.OriginBehindCount)
	}
}

func gitPOST(t *testing.T, srv *console.Server, path, body string) *httptest.ResponseRecorder {
	t.Helper()
	var r *http.Request
	if body == "" {
		r = httptest.NewRequest(http.MethodPost, path, nil)
	} else {
		r = httptest.NewRequest(http.MethodPost, path, bytes.NewBufferString(body))
		r.Header.Set("Content-Type", "application/json")
	}
	rec := httptest.NewRecorder()
	srv.Handler().ServeHTTP(rec, r)
	return rec
}

func gitOut(t *testing.T, dir string, args ...string) string {
	t.Helper()
	cmd := exec.Command("git", append([]string{"-C", dir}, args...)...)
	out, err := cmd.Output()
	if err != nil {
		t.Fatalf("git %v: %v", args, err)
	}
	return strings.TrimSpace(string(out))
}

func fileExists(path string) bool {
	_, err := os.Stat(path)
	return err == nil
}

func writeLiveAFKRunWorkspace(t *testing.T, root, workspace, traceID, agentID, bee string, started time.Time, pid int) {
	t.Helper()
	d := runs.Dir{ColonyRoot: root, TraceID: traceID, AgentID: agentID}
	if err := d.Prepare(); err != nil {
		t.Fatal(err)
	}
	if err := d.WriteRequest(protocol.Request{
		ProtocolVersion: protocol.Version,
		TraceID:         traceID,
		AgentID:         agentID,
		Bee:             bee,
		Adapter:         "cursor",
		Workspace:       workspace,
		ColonyRoot:      root,
		CreatedAt:       started,
	}); err != nil {
		t.Fatal(err)
	}
	snap := protocol.StatusSnapshot{
		ProtocolVersion: protocol.Version,
		State:           protocol.StatusRunning,
		StartedAt:       started,
	}
	if pid > 0 {
		snap.PID = pid
	}
	if err := d.WriteStatusSnapshot(snap); err != nil {
		t.Fatal(err)
	}
}
