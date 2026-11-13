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
		"next/fallback.html": &fstest.MapFile{Data: []byte(fallbackPage(t))},
		"next/dist/200.html": &fstest.MapFile{Data: []byte(entryPage)},
		"next/dist/_app/start.js": &fstest.MapFile{
			Data: []byte(`console.start()`),
		},
		"next/dist/favicon.svg": &fstest.MapFile{
			Data: []byte(`<svg xmlns="http://www.w3.org/2000/svg"></svg>`),
		},
		"next/dist/favicon.ico": &fstest.MapFile{
			Data: []byte("\x00\x00\x01\x00"),
		},
		"next/dist/apple-touch-icon.png": &fstest.MapFile{
			Data: []byte("\x89PNG"),
		},
	}
}

// entryPage is the SvelteKit static adapter's single-page entry, reduced to the
// facts a route test can name: the product title, the mount point, the root
// asset references, and the icon set the legacy shell used to declare.
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
	handler := consoleSPAHandler(builtConsoleTree(t), "/")

	tests := []struct {
		name        string
		path        string
		status      int
		contains    []string
		notContains []string
	}{
		{
			name:     "serves the entry at the root",
			path:     "/",
			status:   http.StatusOK,
			contains: []string{`<title>Queen Console</title>`, `id="app"`, `/_app/start.js`, `/favicon.svg`},
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

// The legacy root handler stops answering `/`: its files stay on disk for the
// deletion slice, but the root now belongs to the console. A source-only build
// serves the buildless fallback there instead of the old console.
func TestLegacyConsoleNoLongerAnswersTheRoot(t *testing.T) {
	server := NewServer(Options{})

	recorder := httptest.NewRecorder()
	server.Handler().ServeHTTP(recorder, httptest.NewRequest(http.MethodGet, "/", nil))

	if recorder.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d", recorder.Code, http.StatusOK)
	}
	if strings.Contains(recorder.Body.String(), `id="tab-dashboard"`) {
		t.Fatal("the legacy console still answers /")
	}
}

// The preview mount shares the console handler until the permanent redirect
// replaces it, so the intermediate commit is not console-less. The mount is not
// asserted here because a built tree and a buildless one answer with different
// bytes; the redirect slice owns that route.

func TestConsoleRootFallsBackWithoutGeneratedBundle(t *testing.T) {
	files := fstest.MapFS{
		"next/fallback.html": &fstest.MapFile{Data: []byte("<h1>Console fallback</h1>")},
	}
	recorder := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodGet, "/", nil)

	consoleSPAHandler(files, "/").ServeHTTP(recorder, request)

	if recorder.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d", recorder.Code, http.StatusOK)
	}
	if recorder.Body.String() != "<h1>Console fallback</h1>" {
		t.Fatalf("body = %q", recorder.Body.String())
	}
}

// A source-only build — a fresh clone, or any `go install` binary — has no
// generated bundle, and the root must say how to get one rather than serving a
// page that looks like a broken console or resurrecting the legacy UI. This runs
// against the real embedded tree, whose `next/dist` is absent from a fresh clone,
// so it is the handler's production path and not a fixture of it.
func TestBuildlessConsoleExplainsTheMissingBundle(t *testing.T) {
	tests := []struct {
		name     string
		path     string
		status   int
		contains []string
	}{
		{
			name:   "root serves the fallback",
			path:   "/",
			status: http.StatusOK,
			contains: []string{
				"bundle not built",
				"pnpm --dir web build",
				"go build -o paseka ./cmd/paseka",
			},
		},
		{
			name:   "deep route serves the same fallback",
			path:   "/bees",
			status: http.StatusOK,
			contains: []string{
				"bundle not built",
			},
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			handler := consoleSPAHandler(buildlessConsoleTree(t), "/")
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
		})
	}

	t.Run("missing asset still 404s", func(t *testing.T) {
		recorder := httptest.NewRecorder()
		handler := consoleSPAHandler(buildlessConsoleTree(t), "/")
		handler.ServeHTTP(recorder, httptest.NewRequest(http.MethodGet, "/_app/missing.js", nil))

		if recorder.Code != http.StatusNotFound {
			t.Fatalf("status = %d, want %d", recorder.Code, http.StatusNotFound)
		}
		if strings.Contains(recorder.Body.String(), "bundle not built") {
			t.Fatal("a missing asset was answered with HTML")
		}
	})
}

// buildlessConsoleTree is the embedded tree as a fresh clone has it: the checked-in
// fallback and no generated bundle.
func buildlessConsoleTree(t *testing.T) fs.FS {
	t.Helper()
	return fstest.MapFS{
		"next/fallback.html": &fstest.MapFile{Data: []byte(fallbackPage(t))},
	}
}

func fallbackPage(t *testing.T) string {
	t.Helper()
	data, err := fs.ReadFile(nextFiles, "next/fallback.html")
	if err != nil {
		t.Fatalf("read next/fallback.html: %v", err)
	}
	return string(data)
}
