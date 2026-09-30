package console

import (
	"io/fs"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"testing/fstest"
)

func TestNextConsoleRoutesDoNotReplaceLegacyConsole(t *testing.T) {
	server := NewServer(Options{})

	tests := []struct {
		name       string
		path       string
		status     int
		location   string
		contains   string
		notContain string
	}{
		{
			name:     "redirects preview base",
			path:     "/next",
			status:   http.StatusPermanentRedirect,
			location: "/next/",
		},
		{
			name:     "serves preview root",
			path:     "/next/",
			status:   http.StatusOK,
			contains: `<title>Queen Console Next</title>`,
		},
		{
			name:     "uses preview SPA fallback",
			path:     "/next/future/route",
			status:   http.StatusOK,
			contains: `<title>Queen Console Next</title>`,
		},
		{
			name:       "missing preview asset returns not found",
			path:       "/next/_app/missing.js",
			status:     http.StatusNotFound,
			notContain: `id="app"`,
		},
		{
			name:       "legacy root remains default",
			path:       "/",
			status:     http.StatusOK,
			contains:   `id="tab-dashboard"`,
			notContain: `id="app"`,
		},
		{
			name:       "unknown API route remains outside preview",
			path:       "/api/not-found",
			status:     http.StatusNotFound,
			notContain: `id="app"`,
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			recorder := httptest.NewRecorder()
			request := httptest.NewRequest(http.MethodGet, test.path, nil)
			server.Handler().ServeHTTP(recorder, request)

			if recorder.Code != test.status {
				t.Fatalf("status = %d, want %d", recorder.Code, test.status)
			}
			if location := recorder.Header().Get("Location"); location != test.location {
				t.Fatalf("location = %q, want %q", location, test.location)
			}
			if test.contains != "" && !strings.Contains(recorder.Body.String(), test.contains) {
				t.Fatalf("body does not contain %q", test.contains)
			}
			if test.notContain != "" && strings.Contains(recorder.Body.String(), test.notContain) {
				t.Fatalf("body unexpectedly contains %q", test.notContain)
			}
		})
	}
}

func TestNextSPAHandlerFallsBackWithoutGeneratedBundle(t *testing.T) {
	files := fstest.MapFS{
		"next/fallback.html": &fstest.MapFile{Data: []byte("<h1>Preview fallback</h1>")},
	}
	recorder := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodGet, "/next/", nil)

	nextSPAHandler(files).ServeHTTP(recorder, request)

	if recorder.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d", recorder.Code, http.StatusOK)
	}
	if recorder.Body.String() != "<h1>Preview fallback</h1>" {
		t.Fatalf("body = %q", recorder.Body.String())
	}
}

// A source-only build — a fresh clone, or any `go install` binary — has no
// generated bundle, and the preview must say how to get one rather than
// serving a page that looks like a broken console. This runs against the real
// embedded tree, whose `next/dist` is absent from a fresh clone, so it is the
// handler's production path and not a fixture of it.
func TestBuildlessPreviewExplainsTheMissingBundle(t *testing.T) {
	tests := []struct {
		name     string
		path     string
		status   int
		contains []string
	}{
		{
			name:   "base serves the fallback",
			path:   "/next/",
			status: http.StatusOK,
			contains: []string{
				"preview bundle not built",
				"pnpm --dir web build",
				"go build -o paseka ./cmd/paseka",
			},
		},
		{
			name:   "deep route serves the same fallback",
			path:   "/next/bees",
			status: http.StatusOK,
			contains: []string{
				"preview bundle not built",
			},
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			handler := nextSPAHandler(buildlessNextTree(t))
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
		handler := nextSPAHandler(buildlessNextTree(t))
		handler.ServeHTTP(recorder, httptest.NewRequest(http.MethodGet, "/next/_app/missing.js", nil))

		if recorder.Code != http.StatusNotFound {
			t.Fatalf("status = %d, want %d", recorder.Code, http.StatusNotFound)
		}
		if strings.Contains(recorder.Body.String(), "preview bundle not built") {
			t.Fatal("a missing asset was answered with HTML")
		}
	})
}

// buildlessNextTree is the embedded tree as a fresh clone has it: the checked-in
// fallback and no generated bundle.
func buildlessNextTree(t *testing.T) fs.FS {
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
