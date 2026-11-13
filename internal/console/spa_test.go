package console

import (
	"io/fs"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"testing/fstest"
)

// builtConsoleTree is the embedded tree release archives, the container image,
// and a developer's `pnpm --dir web build` produce: the generated bundle beside
// the committed fallback. The production build is not committed, so the root
// contract is exercised against a tree shaped like it rather than against
// whatever happens to be on the build machine.
func builtConsoleTree(t *testing.T) fs.FS {
	t.Helper()
	return fstest.MapFS{
		"ui/fallback.html": &fstest.MapFile{Data: []byte(fallbackPage(t))},
		"ui/dist/200.html": &fstest.MapFile{Data: []byte(entryPage)},
		"ui/dist/_app/start.js": &fstest.MapFile{
			Data: []byte(`console.start()`),
		},
		"ui/dist/favicon.svg": &fstest.MapFile{
			Data: []byte(`<svg xmlns="http://www.w3.org/2000/svg"></svg>`),
		},
		"ui/dist/favicon.ico": &fstest.MapFile{
			Data: []byte("\x00\x00\x01\x00"),
		},
		"ui/dist/apple-touch-icon.png": &fstest.MapFile{
			Data: []byte("\x89PNG"),
		},
	}
}

// entryPage is the SvelteKit static adapter's single-page entry, reduced to the
// facts a route test can name: the product title, the mount point, the root
// asset references, and the icon set the console declares.
const entryPage = `<!doctype html>
<html lang="en">
<head>
<title>Queen Console</title>
<link rel="icon" href="/favicon.svg" type="image/svg+xml">
<link rel="icon" href="/favicon.ico" sizes="any">
<link rel="apple-touch-icon" href="/apple-touch-icon.png">
<script src="/_app/start.js"></script>
</head>
<body><div id="app"></div></body>
</html>`

// The root is the console: the single-page entry, every deep route at its root
// path, the bundle's own assets, and a real 404 when an asset is missing.
func TestConsoleServesTheBundleAtRoot(t *testing.T) {
	handler := consoleSPAHandler(builtConsoleTree(t))

	tests := []struct {
		name        string
		path        string
		status      int
		contains    []string
		notContains []string
	}{
		{
			name:        "serves the entry at the root",
			path:        "/",
			status:      http.StatusOK,
			contains:    []string{`<title>Queen Console</title>`, `id="app"`, `/_app/start.js`, `/favicon.svg`},
			notContains: []string{"console bundle not built"},
		},
		{
			name:     "serves the entry for a trail",
			path:     "/traces/trace-01a1d62ac0c152e7",
			status:   http.StatusOK,
			contains: []string{`id="app"`},
		},
		{
			name:     "serves the entry for a run",
			path:     "/runs/trace-01a1d62ac0c152e7/01a1d63321dc5c65",
			status:   http.StatusOK,
			contains: []string{`id="app"`},
		},
		{
			name:     "serves the entry for a task",
			path:     "/tasks/trace-01a1d62ac0c152e7/001-mount-console-at-root",
			status:   http.StatusOK,
			contains: []string{`id="app"`},
		},
		{
			name:     "serves the entry for a review proposal",
			path:     "/reviews/trace-01a1d62ac0c152e7/001-mount-console-at-root",
			status:   http.StatusOK,
			contains: []string{`id="app"`},
		},
		{
			name:     "serves the entry for a merge preview",
			path:     "/reviews/trace-01a1d62ac0c152e7/001-mount-console-at-root/preview",
			status:   http.StatusOK,
			contains: []string{`id="app"`},
		},
		{
			name:     "serves the entry for a session",
			path:     "/sessions/session-01a1d62ac0c152e7",
			status:   http.StatusOK,
			contains: []string{`id="app"`},
		},
		{
			name:     "serves the entry for an unknown non-asset path",
			path:     "/future/route",
			status:   http.StatusOK,
			contains: []string{`id="app"`},
		},
		{
			name:     "serves a built script from the bundle root",
			path:     "/_app/start.js",
			status:   http.StatusOK,
			contains: []string{`console.start()`},
		},
		{
			name:        "missing script is not the shell",
			path:        "/_app/missing.js",
			status:      http.StatusNotFound,
			notContains: []string{`id="app"`, `<title>Queen Console</title>`},
		},
		{
			name:     "declared favicon is served from the bundle",
			path:     "/favicon.svg",
			status:   http.StatusOK,
			contains: []string{`<svg`},
		},
		{
			name:     "declared touch icon is served from the bundle",
			path:     "/apple-touch-icon.png",
			status:   http.StatusOK,
			contains: []string{`PNG`},
		},
		{
			name:        "unknown API path is never the shell",
			path:        "/api/not-found",
			status:      http.StatusNotFound,
			notContains: []string{`id="app"`},
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			recorder := httptest.NewRecorder()
			handler.ServeHTTP(recorder, httptest.NewRequest(http.MethodGet, test.path, nil))

			if recorder.Code != test.status {
				t.Fatalf("status = %d, want %d", recorder.Code, test.status)
			}
			for _, want := range test.contains {
				if !strings.Contains(recorder.Body.String(), want) {
					t.Fatalf("body does not contain %q", want)
				}
			}
			for _, want := range test.notContains {
				if strings.Contains(recorder.Body.String(), want) {
					t.Fatalf("body unexpectedly contains %q", want)
				}
			}
		})
	}
}

// The preview prefix is retired rather than remounted: every path beneath it
// answers one permanent redirect to the same path at the root, query string
// included. The whole-prefix match is what keeps an open tab working, because a
// browser still holding the preview shell asks for its cached `_app` assets
// under the same prefix. This runs against the server's real mux, so it also
// pins the ordering: the redirect has to answer before the root SPA fallback.
func TestRetiredPreviewPrefixRedirectsToRoot(t *testing.T) {
	handler := NewServer(Options{}).Handler()

	tests := []struct {
		name     string
		path     string
		location string
	}{
		{
			name:     "the bare prefix",
			path:     "/next",
			location: "/",
		},
		{
			name:     "the prefix with a trailing slash",
			path:     "/next/",
			location: "/",
		},
		{
			name:     "a list route",
			path:     "/next/traces",
			location: "/traces",
		},
		{
			name:     "a deep route keeps its query",
			path:     "/next/traces/trace-01a1d62ac0c152e7?q=x",
			location: "/traces/trace-01a1d62ac0c152e7?q=x",
		},
		{
			name:     "a cached asset resolves at the root",
			path:     "/next/_app/start.js",
			location: "/_app/start.js",
		},
		{
			name:     "a missing asset redirects instead of 404ing",
			path:     "/next/_app/missing.js",
			location: "/_app/missing.js",
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			recorder := httptest.NewRecorder()
			handler.ServeHTTP(recorder, httptest.NewRequest(http.MethodGet, test.path, nil))

			if recorder.Code != http.StatusPermanentRedirect {
				t.Fatalf("status = %d, want %d", recorder.Code, http.StatusPermanentRedirect)
			}
			if got := recorder.Header().Get("Location"); got != test.location {
				t.Fatalf("Location = %q, want %q", got, test.location)
			}
		})
	}
}

// A source-only build — a fresh clone, or any `go install` binary — has no
// generated bundle, and every console route must say how to get one rather than
// serving a page that looks like a broken console. The copy names both build
// steps and the delivery paths that already run them, and it no longer talks
// about a preview or a legacy console still answering somewhere else.
func TestBuildlessConsoleExplainsTheMissingBundle(t *testing.T) {
	required := []string{
		"console bundle not built",
		"pnpm --dir web build",
		"go build -o paseka ./cmd/paseka",
		"Release archives",
		"container image",
		"go install",
		"does not carry a usable console",
	}
	retired := []string{"preview", "Queen Console Next", "legacy console"}

	handler := consoleSPAHandler(buildlessConsoleTree(t))

	rootRecorder := serveConsole(t, handler, "/")
	if rootRecorder.Code != http.StatusOK {
		t.Fatalf("root status = %d, want %d", rootRecorder.Code, http.StatusOK)
	}
	if got := rootRecorder.Header().Get("Content-Type"); !strings.HasPrefix(got, "text/html") {
		t.Fatalf("root Content-Type = %q, want text/html", got)
	}
	for _, needle := range required {
		if !strings.Contains(rootRecorder.Body.String(), needle) {
			t.Fatalf("root body does not contain %q", needle)
		}
	}
	for _, needle := range retired {
		if strings.Contains(rootRecorder.Body.String(), needle) {
			t.Fatalf("root body unexpectedly contains %q", needle)
		}
	}

	deepRecorder := serveConsole(t, handler, "/traces/trace-01a1d62ac0c152e7")
	if deepRecorder.Code != http.StatusOK {
		t.Fatalf("deep route status = %d, want %d", deepRecorder.Code, http.StatusOK)
	}
	if deepRecorder.Body.String() != rootRecorder.Body.String() {
		t.Fatal("a deep route did not serve the same fallback as the root")
	}

	assetRecorder := serveConsole(t, handler, "/_app/missing.js")
	if assetRecorder.Code != http.StatusNotFound {
		t.Fatalf("missing asset status = %d, want %d", assetRecorder.Code, http.StatusNotFound)
	}
	if strings.Contains(assetRecorder.Body.String(), "console bundle not built") {
		t.Fatal("a missing asset was answered with HTML")
	}
}

// The handler decides that a build exists by finding the SPA entry, not by the
// directory being there. A checkout can hold a partial or stale `ui/dist` — an
// interrupted copy, or a tree a build wrote into but did not finish — and that
// must read as buildless rather than as a broken console.
func TestConsoleBuildDetectionLooksForTheSPAEntry(t *testing.T) {
	files := fstest.MapFS{
		"ui/fallback.html": &fstest.MapFile{Data: []byte(fallbackPage(t))},
		"ui/dist/_app/start.js": &fstest.MapFile{
			Data: []byte(`console.start()`),
		},
		"ui/dist/favicon.svg": &fstest.MapFile{
			Data: []byte(`<svg xmlns="http://www.w3.org/2000/svg"></svg>`),
		},
	}

	handler := consoleSPAHandler(files)
	rootRecorder := serveConsole(t, handler, "/")
	if rootRecorder.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d", rootRecorder.Code, http.StatusOK)
	}
	if !strings.Contains(rootRecorder.Body.String(), "console bundle not built") {
		t.Fatal("a dist without the SPA entry was treated as a build")
	}
	if strings.Contains(rootRecorder.Body.String(), `console.start()`) {
		t.Fatal("a dist without the SPA entry served its stray assets")
	}

	assetRecorder := serveConsole(t, handler, "/_app/start.js")
	if assetRecorder.Code != http.StatusNotFound {
		t.Fatalf("emitted asset status = %d, want %d", assetRecorder.Code, http.StatusNotFound)
	}
}

// The fallback is committed inside the embedded tree, so a fresh clone where
// `ui/dist` does not exist still matches `//go:embed all:ui`. Reading it through
// the real embed also pins the copy a buildless binary actually shows.
func TestConsoleFallbackIsCommittedInsideTheEmbedTree(t *testing.T) {
	page := fallbackPage(t)
	for _, want := range []string{
		"console bundle not built",
		"pnpm --dir web build",
		"go build -o paseka ./cmd/paseka",
		"Release archives",
		"container image",
		"go install",
	} {
		if !strings.Contains(page, want) {
			t.Fatalf("committed fallback does not contain %q", want)
		}
	}
	for _, unwanted := range []string{"preview", "Queen Console Next", "legacy console"} {
		if strings.Contains(page, unwanted) {
			t.Fatalf("committed fallback still contains %q", unwanted)
		}
	}
}

// buildlessConsoleTree is the embedded tree as a fresh clone has it: the checked-in
// fallback and no generated bundle.
func buildlessConsoleTree(t *testing.T) fs.FS {
	t.Helper()
	return fstest.MapFS{
		"ui/fallback.html": &fstest.MapFile{Data: []byte(fallbackPage(t))},
	}
}

func fallbackPage(t *testing.T) string {
	t.Helper()
	data, err := fs.ReadFile(uiFiles, "ui/fallback.html")
	if err != nil {
		t.Fatalf("read ui/fallback.html: %v", err)
	}
	return string(data)
}

func serveConsole(t *testing.T, handler http.Handler, path string) *httptest.ResponseRecorder {
	t.Helper()
	recorder := httptest.NewRecorder()
	handler.ServeHTTP(recorder, httptest.NewRequest(http.MethodGet, path, nil))
	return recorder
}
