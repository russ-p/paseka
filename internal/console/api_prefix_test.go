package console_test

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/russ-p/paseka/internal/console"
	"github.com/russ-p/paseka/internal/sessions"
)

// apiNotFoundBody is the shape the API already returns for an unknown id:
// net/http's plain-text 404. The prefix fallback has to match it, because a
// caller branches on the status and may read the reason, and because anything
// else — the console shell above all — would be HTML where the client expects
// an API answer.
const apiNotFoundBody = "404 page not found\n"

// The JSON API owns its whole prefix. A path nobody claimed is an API error,
// never a page: an unclaimed prefix could otherwise fall through to the console
// handler at `/` and answer a JSON client with an HTML shell. The registered
// trace route is in the list because an unknown id has to keep the same shape.
func TestUnknownAPIPathsAnswerAnAPIError(t *testing.T) {
	handler := newAPIPrefixServer(t)

	for _, path := range []string{
		"/api",                             // the bare prefix is not an endpoint
		"/api/",                            // nor is the root of its subtree
		"/api/no-such-endpoint",            // an unknown endpoint
		"/api/no-such-endpoint/deeper",     // and an unknown deep path
		"/api/traces/trace-does-not-exist", // a registered route, an unknown id
	} {
		recorder := httptest.NewRecorder()
		handler.ServeHTTP(recorder, httptest.NewRequest(http.MethodGet, path, nil))

		if recorder.Code != http.StatusNotFound {
			t.Fatalf("%s status = %d, want %d (body=%q)", path, recorder.Code, http.StatusNotFound, recorder.Body.String())
		}
		if got := recorder.Header().Get("Content-Type"); got != "text/plain; charset=utf-8" {
			t.Fatalf("%s Content-Type = %q, want text/plain; charset=utf-8", path, got)
		}
		if got := recorder.Header().Get("X-Content-Type-Options"); got != "nosniff" {
			t.Fatalf("%s X-Content-Type-Options = %q, want nosniff", path, got)
		}
		if body := recorder.Body.String(); body != apiNotFoundBody {
			t.Fatalf("%s body = %q, want %q", path, body, apiNotFoundBody)
		}
	}
}

// A test that only checked the status could not tell the two answers apart —
// the console handler answers 404 as well. The body is what separates them, so
// this states the shell's own markers are absent: a future change that lets the
// console answer `/api` fails here rather than in a browser that cannot parse
// what it received.
func TestUnknownAPIPathsNeverContainTheConsoleShell(t *testing.T) {
	handler := newAPIPrefixServer(t)

	shellMarkers := []string{"<!doctype html", "<html", `id="app"`, "Queen Console"}

	for _, path := range []string{"/api", "/api/", "/api/no-such-endpoint"} {
		recorder := httptest.NewRecorder()
		handler.ServeHTTP(recorder, httptest.NewRequest(http.MethodGet, path, nil))

		contentType := strings.ToLower(recorder.Header().Get("Content-Type"))
		if strings.Contains(contentType, "text/html") {
			t.Fatalf("%s Content-Type = %q, want an API error rather than HTML", path, contentType)
		}
		body := strings.ToLower(recorder.Body.String())
		for _, marker := range shellMarkers {
			if strings.Contains(body, strings.ToLower(marker)) {
				t.Fatalf("%s body contains the console shell marker %q: %q", path, marker, recorder.Body.String())
			}
		}
	}
}

// The prefix fallback must not shadow a real endpoint: naming one still gets
// its JSON, and only the paths nobody claimed become 404s. These three are the
// list endpoints a console page reads on mount.
func TestRegisteredAPIEndpointsKeepTheirResponse(t *testing.T) {
	handler := newAPIPrefixServer(t)

	for _, path := range []string{"/api/traces", "/api/tasks", "/api/sessions"} {
		recorder := httptest.NewRecorder()
		handler.ServeHTTP(recorder, httptest.NewRequest(http.MethodGet, path, nil))

		if recorder.Code != http.StatusOK {
			t.Fatalf("%s status = %d, want %d (body=%q)", path, recorder.Code, http.StatusOK, recorder.Body.String())
		}
		if got := recorder.Header().Get("Content-Type"); got != "application/json" {
			t.Fatalf("%s Content-Type = %q, want application/json", path, got)
		}
	}
}

// newAPIPrefixServer is the real mux, because the ownership being tested is
// routing: the fallback has to answer before the root console handler, and only
// the server's own registration can prove that ordering.
func newAPIPrefixServer(t *testing.T) http.Handler {
	t.Helper()
	repo := initConsoleRepo(t)
	ctxColony := setupConsoleHome(t, repo)
	srv := console.NewServer(console.Options{
		Addr:     "127.0.0.1:0",
		Colony:   ctxColony,
		Sessions: sessions.NewManager(),
	})
	return srv.Handler()
}
