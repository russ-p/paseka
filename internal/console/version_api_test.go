package console_test

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/russ-p/paseka/internal/console"
	"github.com/russ-p/paseka/internal/sessions"
)

func TestVersionAPIHandler(t *testing.T) {
	repo := initConsoleRepo(t)
	ctxColony := setupConsoleHome(t, repo)
	srv := console.NewServer(console.Options{
		Addr:     "127.0.0.1:0",
		Colony:   ctxColony,
		Sessions: sessions.NewManager(),
	})

	req := httptest.NewRequest(http.MethodGet, "/api/version", nil)
	rec := httptest.NewRecorder()
	srv.Handler().ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d body=%s", rec.Code, rec.Body.String())
	}
	var view console.BuildView
	if err := json.NewDecoder(rec.Body).Decode(&view); err != nil {
		t.Fatal(err)
	}
	// The test binary is built from this checkout, so it must name itself. A
	// console that cannot say which build it is cannot be told apart from another
	// one in a bug report.
	if view.Version == "" {
		t.Fatalf("version missing: %+v", view)
	}
	if view.Display == "" {
		t.Fatalf("display missing: %+v", view)
	}
	if view.GoVersion == "" {
		t.Fatalf("goVersion missing: %+v", view)
	}
	// `display` is the one line the UI shows, so it has to agree with the fields
	// it was built from rather than being assembled a second time.
	if !strings.Contains(view.Display, view.Version) {
		t.Fatalf("display %q does not carry version %q", view.Display, view.Version)
	}
	if view.ShortCommit != "" && !strings.Contains(view.Display, view.ShortCommit) {
		t.Fatalf("display %q does not carry shortCommit %q", view.Display, view.ShortCommit)
	}
	if view.Released && view.Version == "dev" {
		t.Fatalf("released dev build: %+v", view)
	}
	// The repository travels with the stamp so the console can link a build to its
	// source instead of printing a sha with nowhere to go.
	if view.Repository != "https://github.com/russ-p/paseka" {
		t.Fatalf("repository = %q", view.Repository)
	}
	if view.CommitURL != "" && view.CommitURL != view.Repository+"/commit/"+view.Commit {
		t.Fatalf("commitUrl = %q for commit %q", view.CommitURL, view.Commit)
	}
	if view.Commit != "" && view.CommitURL == "" {
		t.Fatalf("commit %q has no commitUrl", view.Commit)
	}

	post := httptest.NewRequest(http.MethodPost, "/api/version", nil)
	postRec := httptest.NewRecorder()
	srv.Handler().ServeHTTP(postRec, post)
	if postRec.Code != http.StatusMethodNotAllowed {
		t.Fatalf("POST status = %d", postRec.Code)
	}
}
