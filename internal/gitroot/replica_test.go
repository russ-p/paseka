package gitroot

import (
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

func TestStatusNoOrigin(t *testing.T) {
	repo := initRepo(t)
	st, err := Status(repo)
	if err != nil {
		t.Fatal(err)
	}
	if st.OriginURL != "" {
		t.Fatalf("origin = %q", st.OriginURL)
	}
	if st.Note != "no origin remote" {
		t.Fatalf("note = %q", st.Note)
	}
	if st.Ahead != nil || st.Behind != nil {
		t.Fatalf("ahead/behind should be omitted: %+v", st)
	}
}

func TestStatusMissingTrackingAfterNoFetch(t *testing.T) {
	repo := initRepo(t)
	bare := t.TempDir()
	runGit(t, bare, "init", "--bare")
	runGit(t, repo, "remote", "add", "origin", bare)
	st, err := Status(repo)
	if err != nil {
		t.Fatal(err)
	}
	if st.OriginURL == "" {
		t.Fatal("expected origin")
	}
	if st.Ahead != nil || st.Behind != nil {
		t.Fatalf("ahead/behind should be omitted before fetch: %+v", st)
	}
	if !strings.Contains(st.Note, "fetch") {
		t.Fatalf("note = %q", st.Note)
	}
}

func TestStatusSyncedAheadBehindDirty(t *testing.T) {
	repo, bare := cloneWithBare(t)
	runGit(t, repo, "push", "-u", "origin", "main")

	st, err := Status(repo)
	if err != nil {
		t.Fatal(err)
	}
	if st.Ahead == nil || *st.Ahead != 0 || st.Behind == nil || *st.Behind != 0 {
		t.Fatalf("synced status = ahead=%v behind=%v", st.Ahead, st.Behind)
	}
	if st.Dirty {
		t.Fatal("expected clean")
	}

	if err := os.WriteFile(filepath.Join(repo, "wip.txt"), []byte("wip\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	st, err = Status(repo)
	if err != nil {
		t.Fatal(err)
	}
	if !st.Dirty {
		t.Fatal("expected dirty")
	}

	runGit(t, repo, "add", "wip.txt")
	runGit(t, repo, "commit", "-m", "ahead")
	st, err = Status(repo)
	if err != nil {
		t.Fatal(err)
	}
	if st.Ahead == nil || *st.Ahead != 1 {
		t.Fatalf("ahead = %v", st.Ahead)
	}
	if len(st.Unpublished) != 1 {
		t.Fatalf("unpublished = %+v", st.Unpublished)
	}

	other := cloneBare(t, bare)
	if err := os.WriteFile(filepath.Join(other, "remote.txt"), []byte("from origin\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	runGit(t, other, "add", "remote.txt")
	runGit(t, other, "commit", "-m", "on origin")
	runGit(t, other, "push", "origin", "main")

	if _, err := Fetch(repo); err != nil {
		t.Fatal(err)
	}
	st, err = Status(repo)
	if err != nil {
		t.Fatal(err)
	}
	if st.Behind == nil || *st.Behind < 1 {
		t.Fatalf("behind = %v", st.Behind)
	}
}

func TestFetchDoesNotMoveHEAD(t *testing.T) {
	repo, bare := cloneWithBare(t)
	runGit(t, repo, "push", "-u", "origin", "main")
	head := gitOut(t, repo, "rev-parse", "HEAD")

	other := cloneBare(t, bare)
	if err := os.WriteFile(filepath.Join(other, "n.txt"), []byte("n\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	runGit(t, other, "add", "n.txt")
	runGit(t, other, "commit", "-m", "remote")
	runGit(t, other, "push", "origin", "main")
	remoteSHA := gitOut(t, other, "rev-parse", "HEAD")

	if _, err := Fetch(repo); err != nil {
		t.Fatal(err)
	}
	if got := gitOut(t, repo, "rev-parse", "HEAD"); got != head {
		t.Fatalf("HEAD moved: %s -> %s", head, got)
	}
	tracking := gitOut(t, repo, "rev-parse", "refs/remotes/origin/main")
	if tracking != remoteSHA {
		t.Fatalf("origin/main = %s want %s", tracking, remoteSHA)
	}
	if LastFetchAgeSeconds(repo) == nil {
		t.Fatal("expected last fetch age after fetch")
	}
}

func TestStatusDoesNotFetch(t *testing.T) {
	repo, bare := cloneWithBare(t)
	runGit(t, repo, "push", "-u", "origin", "main")
	other := cloneBare(t, bare)
	if err := os.WriteFile(filepath.Join(other, "x.txt"), []byte("x\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	runGit(t, other, "add", "x.txt")
	runGit(t, other, "commit", "-m", "remote")
	runGit(t, other, "push", "origin", "main")
	before := gitOut(t, repo, "rev-parse", "refs/remotes/origin/main")
	if _, err := Status(repo); err != nil {
		t.Fatal(err)
	}
	after := gitOut(t, repo, "rev-parse", "refs/remotes/origin/main")
	if before != after {
		t.Fatal("Status must not fetch")
	}
}

func TestPushSuccessAndRefuseBehind(t *testing.T) {
	repo, bare := cloneWithBare(t)
	runGit(t, repo, "push", "-u", "origin", "main")

	if _, err := Push(PushOpts{RepoRoot: repo}); err == nil {
		t.Fatal("expected refuse when nothing to push")
	} else if !errors.Is(err, ErrRefused) {
		t.Fatalf("err = %v", err)
	}

	if err := os.WriteFile(filepath.Join(repo, "p.txt"), []byte("p\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	runGit(t, repo, "add", "p.txt")
	runGit(t, repo, "commit", "-m", "local")
	if _, err := Push(PushOpts{RepoRoot: repo}); err != nil {
		t.Fatal(err)
	}
	bareSHA := gitOut(t, bare, "rev-parse", "refs/heads/main")
	localSHA := gitOut(t, repo, "rev-parse", "HEAD")
	if bareSHA != localSHA {
		t.Fatalf("bare main %s != local %s", bareSHA, localSHA)
	}

	other := cloneBare(t, bare)
	if err := os.WriteFile(filepath.Join(other, "o.txt"), []byte("o\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	runGit(t, other, "add", "o.txt")
	runGit(t, other, "commit", "-m", "origin newer")
	runGit(t, other, "push", "origin", "main")
	if _, err := Fetch(repo); err != nil {
		t.Fatal(err)
	}
	beforeBare := gitOut(t, bare, "rev-parse", "refs/heads/main")
	if _, err := Push(PushOpts{RepoRoot: repo}); err == nil {
		t.Fatal("expected refuse behind")
	} else if !errors.Is(err, ErrRefused) {
		t.Fatalf("err = %v", err)
	}
	if gitOut(t, bare, "rev-parse", "refs/heads/main") != beforeBare {
		t.Fatal("push behind must not rewrite origin")
	}
}

func TestPushFirstPublishWithoutTracking(t *testing.T) {
	repo := initRepo(t)
	bare := t.TempDir()
	runGit(t, bare, "init", "--bare")
	runGit(t, repo, "remote", "add", "origin", bare)
	if RemoteTrackingExists(repo, "main") {
		t.Fatal("expected no origin/main tracking")
	}
	if _, err := Push(PushOpts{RepoRoot: repo}); err != nil {
		t.Fatal(err)
	}
	if gitOut(t, bare, "rev-parse", "refs/heads/main") != gitOut(t, repo, "rev-parse", "HEAD") {
		t.Fatal("bare main should match first push")
	}
}

func TestPushSkipHooksDefault(t *testing.T) {
	repo, _ := cloneWithBare(t)
	runGit(t, repo, "push", "-u", "origin", "main")
	hook := filepath.Join(repo, ".git", "hooks", "pre-push")
	if err := os.WriteFile(hook, []byte("#!/bin/sh\nexit 1\n"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(repo, "h.txt"), []byte("h\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	runGit(t, repo, "add", "h.txt")
	runGit(t, repo, "commit", "-m", "hooks")

	if _, err := Push(PushOpts{RepoRoot: repo, RunHooks: false}); err != nil {
		t.Fatalf("default skip hooks: %v", err)
	}

	if err := os.WriteFile(filepath.Join(repo, "h2.txt"), []byte("h2\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	runGit(t, repo, "add", "h2.txt")
	runGit(t, repo, "commit", "-m", "hooks2")
	if _, err := Push(PushOpts{RepoRoot: repo, RunHooks: true}); err == nil {
		t.Fatal("expected pre-push to fail when runHooks true")
	}
}

func TestPullFFAndRefusals(t *testing.T) {
	repo, bare := cloneWithBare(t)
	runGit(t, repo, "push", "-u", "origin", "main")

	other := cloneBare(t, bare)
	if err := os.WriteFile(filepath.Join(other, "ff.txt"), []byte("ff\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	runGit(t, other, "add", "ff.txt")
	runGit(t, other, "commit", "-m", "ff")
	runGit(t, other, "push", "origin", "main")

	if _, err := PullFF(repo); err != nil {
		t.Fatal(err)
	}
	if !fileExists(filepath.Join(repo, "ff.txt")) {
		t.Fatal("expected fast-forward file")
	}

	if err := os.WriteFile(filepath.Join(repo, "dirty.txt"), []byte("d\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := PullFF(repo); err == nil {
		t.Fatal("expected dirty refuse")
	} else if !errors.Is(err, ErrRefused) {
		t.Fatalf("err = %v", err)
	}
	_ = os.Remove(filepath.Join(repo, "dirty.txt"))

	if err := os.WriteFile(filepath.Join(repo, "local.txt"), []byte("l\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	runGit(t, repo, "add", "local.txt")
	runGit(t, repo, "commit", "-m", "diverge local")
	if err := os.WriteFile(filepath.Join(other, "r2.txt"), []byte("r2\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	runGit(t, other, "add", "r2.txt")
	runGit(t, other, "commit", "-m", "diverge remote")
	runGit(t, other, "push", "origin", "main")
	if _, err := Fetch(repo); err != nil {
		t.Fatal(err)
	}
	if _, err := PullFF(repo); err == nil {
		t.Fatal("expected non-ff refuse")
	} else if !errors.Is(err, ErrRefused) {
		t.Fatalf("err = %v", err)
	}
}

func TestBranchesLeftoverAndDelete(t *testing.T) {
	repo := initRepo(t)
	runGit(t, repo, "checkout", "-b", "feature/done")
	if err := os.WriteFile(filepath.Join(repo, "f.txt"), []byte("f\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	runGit(t, repo, "add", "f.txt")
	runGit(t, repo, "commit", "-m", "feature")
	runGit(t, repo, "checkout", "main")
	runGit(t, repo, "merge", "--no-ff", "-m", "merge feature", "feature/done")

	runGit(t, repo, "checkout", "-b", "feature/unmerged")
	if err := os.WriteFile(filepath.Join(repo, "u.txt"), []byte("u\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	runGit(t, repo, "add", "u.txt")
	runGit(t, repo, "commit", "-m", "unmerged")
	runGit(t, repo, "checkout", "main")

	rows, err := ListLocalBranches(repo)
	if err != nil {
		t.Fatal(err)
	}
	var done, unmerged, mainRow *BranchInfo
	for i := range rows {
		switch rows[i].Name {
		case "feature/done":
			done = &rows[i]
		case "feature/unmerged":
			unmerged = &rows[i]
		case "main":
			mainRow = &rows[i]
		}
	}
	if done == nil || !done.Leftover || !done.Merged {
		t.Fatalf("feature/done = %+v", done)
	}
	if unmerged == nil || unmerged.Leftover || unmerged.Merged {
		t.Fatalf("feature/unmerged = %+v", unmerged)
	}
	if mainRow == nil || mainRow.Leftover {
		t.Fatalf("main = %+v", mainRow)
	}

	if err := DeleteBranch(repo, "main"); err == nil || !errors.Is(err, ErrRefused) {
		t.Fatalf("delete default: %v", err)
	}
	if err := DeleteBranch(repo, "feature/unmerged"); err == nil || !errors.Is(err, ErrRefused) {
		t.Fatalf("delete unmerged: %v", err)
	}
	if err := DeleteBranch(repo, "feature/done"); err != nil {
		t.Fatal(err)
	}
	if LocalBranchExists(repo, "feature/done") {
		t.Fatal("expected feature/done deleted")
	}
}

func initRepo(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	runGit(t, dir, "init", "-b", "main")
	runGit(t, dir, "config", "user.email", "test@test.com")
	runGit(t, dir, "config", "user.name", "test")
	runGit(t, dir, "commit", "--allow-empty", "-m", "init")
	return dir
}

func cloneWithBare(t *testing.T) (clone, bare string) {
	t.Helper()
	src := initRepo(t)
	bare = t.TempDir()
	runGit(t, bare, "init", "--bare")
	runGit(t, src, "remote", "add", "origin", bare)
	runGit(t, src, "push", "-u", "origin", "main")
	clone = cloneBare(t, bare)
	return clone, bare
}

func cloneBare(t *testing.T, bare string) string {
	t.Helper()
	dir := t.TempDir()
	cmd := exec.Command("git", "clone", "-b", "main", bare, dir)
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("git clone: %v\n%s", err, out)
	}
	runGit(t, dir, "config", "user.email", "test@test.com")
	runGit(t, dir, "config", "user.name", "test")
	return dir
}

func runGit(t *testing.T, dir string, args ...string) {
	t.Helper()
	cmd := exec.Command("git", append([]string{"-C", dir}, args...)...)
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("git %v: %v\n%s", args, err, out)
	}
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
