package console_test

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/russ-p/paseka/internal/console"
	"github.com/russ-p/paseka/internal/sessions"
)

var errSchema = errors.New("chrome schemaVersion != 1")

func TestChromeStreamFirstFrame(t *testing.T) {
	repo := initConsoleRepo(t)
	ctxColony := setupConsoleHome(t, repo)
	srv := console.NewServer(console.Options{
		Addr:     "127.0.0.1:0",
		Colony:   ctxColony,
		Sessions: sessions.NewManager(),
	})
	ts := httptest.NewServer(srv.Handler())
	t.Cleanup(ts.Close)

	ctx, cancel := context.WithTimeout(context.Background(), 8*time.Second)
	defer cancel()
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, ts.URL+"/api/chrome/stream", nil)
	if err != nil {
		t.Fatal(err)
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("status = %d", resp.StatusCode)
	}
	if ct := resp.Header.Get("Content-Type"); !strings.Contains(ct, "text/event-stream") {
		t.Fatalf("Content-Type = %q", ct)
	}
	if resp.Header.Get("X-Accel-Buffering") != "no" {
		t.Fatalf("X-Accel-Buffering = %q", resp.Header.Get("X-Accel-Buffering"))
	}

	raw := readSSEData(t, resp.Body, "chrome")
	cancel()

	var env map[string]json.RawMessage
	if err := json.Unmarshal(raw, &env); err != nil {
		t.Fatalf("chrome json: %v body=%s", err, raw)
	}
	var ver int
	if err := json.Unmarshal(env["schemaVersion"], &ver); err != nil || ver != 1 {
		t.Fatalf("schemaVersion = %s", env["schemaVersion"])
	}
	var host map[string]json.RawMessage
	if err := json.Unmarshal(env["host"], &host); err != nil {
		t.Fatalf("host: %v", err)
	}
	if _, ok := host["processes"]; ok {
		t.Fatal("host plaque must omit processes")
	}
	var git map[string]json.RawMessage
	if err := json.Unmarshal(env["git"], &git); err != nil {
		t.Fatalf("git: %v", err)
	}
	for _, key := range []string{"worktrees", "branches", "unpublished"} {
		if _, ok := git[key]; ok {
			t.Fatalf("git plaque must omit %s", key)
		}
	}
}

func TestChromeStreamRejectsPost(t *testing.T) {
	repo := initConsoleRepo(t)
	ctxColony := setupConsoleHome(t, repo)
	srv := console.NewServer(console.Options{Colony: ctxColony, Sessions: sessions.NewManager()})
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/api/chrome/stream", nil)
	srv.Handler().ServeHTTP(rec, req)
	if rec.Code != http.StatusMethodNotAllowed {
		t.Fatalf("POST status = %d", rec.Code)
	}
}

// A failed read must omit its section rather than zero it: the client keeps its
// last known runtime and bee list while a section is absent, but a zero-valued
// runtime reads as real data and blanks the panels until the read recovers.
func TestChromeStreamOmitsFailedSections(t *testing.T) {
	repo := initConsoleRepo(t)
	ctxColony := setupConsoleHome(t, repo)

	// Both GetRuntime and GetAgents resolve through homestate's state.json, so
	// an unparsable file fails the two sections at once — the same pairing the
	// header shows (Hive runtime beside Live bees).
	home := os.Getenv("XDG_CONFIG_HOME")
	statePath := filepath.Join(home, "paseka", ctxColony.Slug, "state.json")
	if err := os.WriteFile(statePath, []byte("{not json"), 0o644); err != nil {
		t.Fatal(err)
	}

	srv := console.NewServer(console.Options{
		Addr:     "127.0.0.1:0",
		Colony:   ctxColony,
		Sessions: sessions.NewManager(),
	})
	ts := httptest.NewServer(srv.Handler())
	t.Cleanup(ts.Close)

	ctx, cancel := context.WithTimeout(context.Background(), 8*time.Second)
	defer cancel()
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, ts.URL+"/api/chrome/stream", nil)
	if err != nil {
		t.Fatal(err)
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()

	raw := readSSEData(t, resp.Body, "chrome")
	cancel()

	var env map[string]json.RawMessage
	if err := json.Unmarshal(raw, &env); err != nil {
		t.Fatalf("chrome json: %v body=%s", err, raw)
	}
	for _, key := range []string{"runtime", "agents"} {
		if _, ok := env[key]; ok {
			t.Fatalf("%s must be omitted when the read behind it fails", key)
		}
	}
	for _, key := range []string{"runtimeError", "agentsError"} {
		var msg string
		if err := json.Unmarshal(env[key], &msg); err != nil || msg == "" {
			t.Fatalf("%s must name the failed read, got %s", key, env[key])
		}
	}
	// The host plaque cannot hard-fail and stays present beside the omissions.
	var host map[string]json.RawMessage
	if err := json.Unmarshal(env["host"], &host); err != nil {
		t.Fatalf("host: %v", err)
	}
}

func TestChromeStreamTwoClients(t *testing.T) {
	repo := initConsoleRepo(t)
	ctxColony := setupConsoleHome(t, repo)
	srv := console.NewServer(console.Options{Colony: ctxColony, Sessions: sessions.NewManager()})
	ts := httptest.NewServer(srv.Handler())
	t.Cleanup(ts.Close)

	var wg sync.WaitGroup
	errCh := make(chan error, 2)
	for i := 0; i < 2; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			ctx, cancel := context.WithTimeout(context.Background(), 8*time.Second)
			defer cancel()
			req, err := http.NewRequestWithContext(ctx, http.MethodGet, ts.URL+"/api/chrome/stream", nil)
			if err != nil {
				errCh <- err
				return
			}
			resp, err := http.DefaultClient.Do(req)
			if err != nil {
				errCh <- err
				return
			}
			defer resp.Body.Close()
			raw, err := readSSEDataErr(resp.Body, "chrome", 8*time.Second)
			if err != nil {
				errCh <- err
				return
			}
			var env struct {
				SchemaVersion int `json:"schemaVersion"`
			}
			if err := json.Unmarshal(raw, &env); err != nil {
				errCh <- err
				return
			}
			if env.SchemaVersion != 1 {
				errCh <- errSchema
			}
		}()
	}
	wg.Wait()
	close(errCh)
	for err := range errCh {
		if err != nil {
			t.Fatal(err)
		}
	}
}

func readSSEData(t *testing.T, r io.Reader, eventName string) []byte {
	t.Helper()
	raw, err := readSSEDataErr(r, eventName, 8*time.Second)
	if err != nil {
		t.Fatal(err)
	}
	return raw
}

func readSSEDataErr(r io.Reader, eventName string, timeout time.Duration) ([]byte, error) {
	br := bufio.NewReader(r)
	deadline := time.Now().Add(timeout)
	gotEvent := false
	for time.Now().Before(deadline) {
		line, err := br.ReadString('\n')
		if err != nil {
			return nil, err
		}
		line = strings.TrimRight(line, "\r\n")
		if strings.HasPrefix(line, "event: ") && strings.TrimPrefix(line, "event: ") == eventName {
			gotEvent = true
			continue
		}
		if gotEvent && strings.HasPrefix(line, "data: ") {
			return []byte(strings.TrimPrefix(line, "data: ")), nil
		}
	}
	return nil, fmt.Errorf("timeout waiting for sse event %q", eventName)
}
