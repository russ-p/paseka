package opencode

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"strconv"
	"testing"
	"time"
)

func TestPostSession(t *testing.T) {
	var gotDir string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost || r.URL.Path != "/session" {
			w.WriteHeader(http.StatusNotFound)
			return
		}
		user, pass, ok := r.BasicAuth()
		if !ok || user != "opencode" || pass != "secret" {
			w.WriteHeader(http.StatusUnauthorized)
			return
		}
		var body map[string]string
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			w.WriteHeader(http.StatusBadRequest)
			return
		}
		gotDir = body["directory"]
		_, _ = w.Write([]byte(`{"id":"ses_test"}`))
	}))
	defer srv.Close()

	id, err := postSession(context.Background(), serverPort(t, srv), "secret", "/ws")
	if err != nil {
		t.Fatal(err)
	}
	if id != "ses_test" {
		t.Fatalf("id = %q", id)
	}
	if gotDir != "/ws" {
		t.Fatalf("directory = %q", gotDir)
	}
}

func TestPostSessionErrors(t *testing.T) {
	tests := []struct {
		name    string
		handler http.HandlerFunc
	}{
		{name: "unauthorized", handler: func(w http.ResponseWriter, _ *http.Request) {
			w.WriteHeader(http.StatusUnauthorized)
		}},
		{name: "empty id", handler: func(w http.ResponseWriter, _ *http.Request) {
			_, _ = w.Write([]byte(`{}`))
		}},
		{name: "bad json", handler: func(w http.ResponseWriter, _ *http.Request) {
			_, _ = w.Write([]byte(`not-json`))
		}},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			srv := httptest.NewServer(tc.handler)
			defer srv.Close()
			if _, err := postSession(context.Background(), serverPort(t, srv), "secret", "/ws"); err == nil {
				t.Fatal("expected error")
			}
		})
	}
}

func TestCreateSessionServerExitsFast(t *testing.T) {
	fake := filepath.Join(t.TempDir(), "fake-opencode")
	if err := os.WriteFile(fake, []byte("#!/bin/sh\nexit 1\n"), 0o755); err != nil {
		t.Fatal(err)
	}
	start := time.Now()
	if _, err := createSession(fake, t.TempDir(), os.Environ()); err == nil {
		t.Fatal("expected error")
	}
	if elapsed := time.Since(start); elapsed > 5*time.Second {
		t.Fatalf("server exit not detected fast: %v", elapsed)
	}
}

func TestFreeLoopbackPortAndToken(t *testing.T) {
	port, err := freeLoopbackPort()
	if err != nil {
		t.Fatal(err)
	}
	if port <= 0 || port > 65535 {
		t.Fatalf("port = %d", port)
	}
	t1, err := randomToken()
	if err != nil {
		t.Fatal(err)
	}
	t2, err := randomToken()
	if err != nil {
		t.Fatal(err)
	}
	if len(t1) != 32 || t1 == t2 {
		t.Fatalf("tokens = %q %q", t1, t2)
	}
}

func serverPort(t *testing.T, srv *httptest.Server) int {
	t.Helper()
	u, err := url.Parse(srv.URL)
	if err != nil {
		t.Fatal(err)
	}
	port, err := strconv.Atoi(u.Port())
	if err != nil {
		t.Fatal(err)
	}
	return port
}
