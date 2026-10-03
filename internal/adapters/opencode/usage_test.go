package opencode

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/russ-p/paseka/internal/adapters"
	"github.com/russ-p/paseka/internal/protocol"
)

func TestGetSessionUsage(t *testing.T) {
	var gotPath, gotUser string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotPath = r.URL.Path
		gotUser, _, _ = r.BasicAuth()
		_, _ = w.Write([]byte(`{"id":"ses_1","tokens":{"input":120,"output":34,"reasoning":7,"cache":{"read":900,"write":12}}}`))
	}))
	defer srv.Close()

	got, err := getSessionUsage(context.Background(), serverPort(t, srv), "secret", "ses_1")
	if err != nil {
		t.Fatal(err)
	}
	if got == nil || got.InputTokens != 120 || got.OutputTokens != 34 {
		t.Fatalf("usage = %+v", got)
	}
	if got.CacheReadTokens != 900 || got.CacheWriteTokens != 12 {
		t.Fatalf("cache = %d/%d", got.CacheReadTokens, got.CacheWriteTokens)
	}
	if got.Source != protocol.UsageSourceOpenCodeServer {
		t.Fatalf("source = %q", got.Source)
	}
	if gotPath != "/session/ses_1" || gotUser != "opencode" {
		t.Fatalf("request = %q as %q", gotPath, gotUser)
	}
}

func TestGetSessionUsageEmptySpendIsNoUsage(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte(`{"id":"ses_1","tokens":{"input":0,"output":0,"cache":{"read":0,"write":0}}}`))
	}))
	defer srv.Close()

	got, err := getSessionUsage(context.Background(), serverPort(t, srv), "secret", "ses_1")
	if err != nil {
		t.Fatal(err)
	}
	if got != nil {
		t.Fatalf("expected no usage for a session that spent nothing, got %+v", got)
	}
}

func TestGetSessionUsageAnswersWithoutTokens(t *testing.T) {
	// A server that answers at all settles the question: unknown sessions, empty
	// spends, and unreadable bodies all mean "no usage" rather than a retry.
	tests := map[string]http.HandlerFunc{
		"unauthorized": func(w http.ResponseWriter, _ *http.Request) {
			w.WriteHeader(http.StatusUnauthorized)
		},
		"not found": func(w http.ResponseWriter, _ *http.Request) {
			w.WriteHeader(http.StatusNotFound)
		},
		"no token block": func(w http.ResponseWriter, _ *http.Request) {
			_, _ = w.Write([]byte(`{"id":"ses_1","title":"session"}`))
		},
		"bad json": func(w http.ResponseWriter, _ *http.Request) {
			_, _ = w.Write([]byte(`not-json`))
		},
	}
	for name, handler := range tests {
		t.Run(name, func(t *testing.T) {
			srv := httptest.NewServer(handler)
			defer srv.Close()
			got, err := getSessionUsage(context.Background(), serverPort(t, srv), "secret", "ses_1")
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if got != nil {
				t.Fatalf("expected no usage, got %+v", got)
			}
		})
	}
}

func TestSessionUsageNeedsProviderSessionAndBinary(t *testing.T) {
	handle := adapters.SessionHandle{Workspace: t.TempDir(), ProviderSessionID: "ses_1"}
	if got := NewSession().SessionUsage(context.Background(), handle); got != nil {
		t.Fatalf("missing binary must not report usage, got %+v", got)
	}
	handle.Binary = "opencode"
	handle.ProviderSessionID = ""
	if got := NewSession().SessionUsage(context.Background(), handle); got != nil {
		t.Fatalf("missing provider session id must not report usage, got %+v", got)
	}
}

func TestSessionUsageSurvivesUnreachableServer(t *testing.T) {
	workspace := t.TempDir()
	fake := filepath.Join(workspace, "fake-opencode")
	if err := os.WriteFile(fake, []byte("#!/bin/sh\nexit 1\n"), 0o755); err != nil {
		t.Fatal(err)
	}

	got := NewSession().SessionUsage(context.Background(), adapters.SessionHandle{
		Binary:            fake,
		Workspace:         workspace,
		ProviderSessionID: "ses_1",
	})
	if got != nil {
		t.Fatalf("expected nil usage when the provider server cannot start, got %+v", got)
	}
}

func TestWithServerRunsAttemptAgainstLoopbackPort(t *testing.T) {
	workspace := t.TempDir()
	fake := filepath.Join(workspace, "fake-opencode")
	if err := os.WriteFile(fake, []byte("#!/bin/sh\nsleep 30\n"), 0o755); err != nil {
		t.Fatal(err)
	}

	var gotPort int
	var gotPassword string
	env := append(os.Environ(), "OPENCODE_SERVER_PASSWORD=inherited", "OPENCODE_SERVER_USERNAME=inherited")
	err := withServer(fake, workspace, env, func(_ context.Context, port int, password string) error {
		gotPort = port
		gotPassword = password
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	if gotPort <= 0 {
		t.Fatalf("port = %d", gotPort)
	}
	if gotPassword == "" || gotPassword == "inherited" {
		t.Fatalf("password = %q", gotPassword)
	}
}

func TestWithServerFailsWhenServerExitsFast(t *testing.T) {
	workspace := t.TempDir()
	fake := filepath.Join(workspace, "fake-opencode")
	if err := os.WriteFile(fake, []byte("#!/bin/sh\nexit 1\n"), 0o755); err != nil {
		t.Fatal(err)
	}

	start := time.Now()
	err := withServer(fake, workspace, os.Environ(), func(context.Context, int, string) error {
		return errors.New("connection refused")
	})
	if err == nil {
		t.Fatal("expected error")
	}
	if elapsed := time.Since(start); elapsed > 5*time.Second {
		t.Fatalf("server exit not detected fast: %v", elapsed)
	}
}
