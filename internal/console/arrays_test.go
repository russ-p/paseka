package console_test

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/russ-p/paseka/internal/console"
	"github.com/russ-p/paseka/internal/sessions"
)

// Every list in the console API is a JSON array, never `null`.
//
// The projections behind these endpoints build their lists by appending onto a
// nil slice, which Go marshals as `null`. A browser that asked a page to render
// such a list — `items.length`, `items.map`, a spread — throws, and the route
// is dead. So this is stated once, over every reachable endpoint, on a colony
// that has nothing in it: the case where a nil slice hides.
func TestNoArrayFieldIsNullOnAnEmptyColony(t *testing.T) {
	repo := initConsoleRepo(t)
	ctxColony := setupConsoleHome(t, repo)

	mgr := sessions.NewManager()
	mgr.RegisterSessionAdapter("opencode", &outputSessionAdapter{})
	srv := console.NewServer(console.Options{
		Addr:     "127.0.0.1:0",
		Colony:   ctxColony,
		Sessions: mgr,
	})

	// A colony that has run nothing. Only the paths reachable from an empty
	// colony are here; views behind a run id are covered by the hiveview tests
	// that build a run directly.
	for _, tc := range []struct {
		path string
		// The array fields that must be `[]` on this response, by JSON name.
		arrays []string
	}{
		{path: "/api/runs"},
		{path: "/api/bees", arrays: []string{`"intents":[]`}},
		{path: "/api/sessions"},
		{path: "/api/invites"},
		{path: "/api/cues"},
		{path: "/api/traces"},
		{path: "/api/events", arrays: []string{`"items":[]`}},
		{path: "/api/agents", arrays: []string{`"items":[]`}},
		{path: "/api/review-queue", arrays: []string{`"items":[]`}},
		{path: "/api/tasks", arrays: []string{`"groups":[]`}},
		{path: "/api/dashboard", arrays: []string{
			`"recentTraces":[]`, `"failedRuns":[]`, `"recentInsights":[]`,
		}},
		{path: "/api/config", arrays: []string{
			`"adapters":[`, `"allowFrom":[]`, `"chatIds":[]`, `"notify":[`, `"colony":[]`, `"home":[]`,
		}},
		{path: "/api/colony/topology", arrays: []string{
			`"bees":[`, `"events":[`, `"edges":[`,
		}},
	} {
		req := httptest.NewRequest(http.MethodGet, tc.path, nil)
		rec := httptest.NewRecorder()
		srv.Handler().ServeHTTP(rec, req)
		if rec.Code != http.StatusOK {
			t.Fatalf("%s status = %d body=%s", tc.path, rec.Code, rec.Body.String())
		}
		body := rec.Body.String()

		// A top-level list answers with an array and nothing else.
		if len(tc.arrays) == 0 {
			if strings.HasPrefix(body, "null") {
				t.Fatalf("%s = null, want an array", tc.path)
			}
			continue
		}
		for _, want := range tc.arrays {
			if !strings.Contains(body, want) {
				t.Fatalf("%s = %s\nwant %s", tc.path, body, want)
			}
		}
	}
}

// `/api/git` reads the working tree rather than the run history, so an empty
// colony does not reach it above — a repository with no branches and no
// worktrees still has to report both as arrays.
func TestGitViewListsAreEmptyNotNil(t *testing.T) {
	repo := initConsoleRepo(t)
	runGit(t, repo, "commit", "--allow-empty", "-m", "second")
	ctxColony := setupConsoleHome(t, repo)

	srv := console.NewServer(console.Options{Addr: "127.0.0.1:0", Colony: ctxColony})

	req := httptest.NewRequest(http.MethodGet, "/api/git", nil)
	rec := httptest.NewRecorder()
	srv.Handler().ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("git status = %d body=%s", rec.Code, rec.Body.String())
	}
	for _, want := range []string{`"worktrees":[]`, `"branches":[`} {
		if !strings.Contains(rec.Body.String(), want) {
			t.Fatalf("/api/git = %s\nwant %s", rec.Body.String(), want)
		}
	}
}
