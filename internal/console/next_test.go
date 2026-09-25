package console

import (
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
