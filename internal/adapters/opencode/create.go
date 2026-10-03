package opencode

import (
	"bytes"
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"os/exec"
	"strconv"
	"strings"
	"time"
)

const (
	serveTimeout = 20 * time.Second
	serveAttempt = 3 * time.Second
	serveRetry   = 250 * time.Millisecond
)

// createSessionFunc allocates a new OpenCode provider session without running a
// model turn. It is a package seam so tests can stub the serve + HTTP pre-create.
var createSessionFunc = createSession

// serveClient talks to the loopback-only OpenCode server. Proxy is disabled so
// an inherited HTTP(S)_PROXY cannot intercept 127.0.0.1.
var serveClient = &http.Client{
	Transport: &http.Transport{Proxy: nil},
}

// createSession is the OpenCode equivalent of Cursor's create-chat: it starts a
// short-lived `opencode serve`, asks it for a fresh session id, then stops the
// server. The session is durable in OpenCode's store, so the TUI can continue it
// with --session.
func createSession(binary, workspace string, env []string) (string, error) {
	var id string
	err := withServer(binary, workspace, env, func(ctx context.Context, port int, password string) error {
		created, err := postSession(ctx, port, password, workspace)
		if err != nil {
			return err
		}
		id = created
		return nil
	})
	if err != nil {
		return "", err
	}
	return id, nil
}

// withServer starts a short-lived `opencode serve` on a free loopback port, runs
// attempt against it until the server answers, then always stops the server.
// Because session state is durable in OpenCode's store, a throwaway server also
// answers questions about sessions whose own TUI has already exited.
func withServer(binary, workspace string, env []string, attempt func(context.Context, int, string) error) error {
	port, err := freeLoopbackPort()
	if err != nil {
		return fmt.Errorf("opencode: find port: %w", err)
	}
	password, err := randomToken()
	if err != nil {
		return fmt.Errorf("opencode: generate password: %w", err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), serveTimeout)
	defer cancel()

	cmd := exec.CommandContext(ctx, binary, "serve", "--hostname", "127.0.0.1", "--port", strconv.Itoa(port))
	cmd.Dir = workspace
	cmd.Env = append(envWithout(env, "OPENCODE_SERVER_PASSWORD", "OPENCODE_SERVER_USERNAME"),
		"OPENCODE_SERVER_PASSWORD="+password,
		"OPENCODE_SERVER_USERNAME=opencode")
	var out bytes.Buffer
	cmd.Stdout = &out
	cmd.Stderr = &out
	if err := cmd.Start(); err != nil {
		return fmt.Errorf("opencode: start serve: %w", err)
	}

	done := make(chan struct{})
	go func() {
		_ = cmd.Wait()
		close(done)
	}()

	runErr := retryUntilServed(ctx, done, func(ctx context.Context) error {
		return attempt(ctx, port, password)
	})
	_ = cmd.Process.Kill()
	<-done
	if runErr != nil {
		return fmt.Errorf("opencode: serve: %w%s", runErr, serveOutput(out.String()))
	}
	return nil
}

func serveOutput(s string) string {
	s = strings.TrimSpace(s)
	if s == "" {
		return ""
	}
	return " (serve output: " + s + ")"
}

// retryUntilServed repeats attempt until it succeeds, the server dies, or ctx
// ends — `opencode serve` needs a moment before it accepts requests.
func retryUntilServed(ctx context.Context, serverDone <-chan struct{}, attempt func(context.Context) error) error {
	var lastErr error
	for {
		err := attempt(ctx)
		if err == nil {
			return nil
		}
		lastErr = err
		select {
		case <-serverDone:
			if lastErr != nil {
				return fmt.Errorf("server exited before accepting requests: %w", lastErr)
			}
			return errors.New("server exited before accepting requests")
		case <-ctx.Done():
			if lastErr != nil {
				return lastErr
			}
			return ctx.Err()
		case <-time.After(serveRetry):
		}
	}
}

func postSession(parent context.Context, port int, password, workspace string) (string, error) {
	ctx, cancel := context.WithTimeout(parent, serveAttempt)
	defer cancel()

	body, err := json.Marshal(map[string]string{"directory": workspace})
	if err != nil {
		return "", err
	}
	url := fmt.Sprintf("http://127.0.0.1:%d/session", port)
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(body))
	if err != nil {
		return "", err
	}
	req.Header.Set("Content-Type", "application/json")
	req.SetBasicAuth("opencode", password)

	resp, err := serveClient.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		_, _ = io.Copy(io.Discard, resp.Body)
		return "", fmt.Errorf("opencode: create session: HTTP %d", resp.StatusCode)
	}
	var out struct {
		ID string `json:"id"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&out); err != nil {
		return "", fmt.Errorf("opencode: decode session: %w", err)
	}
	if out.ID == "" {
		return "", errors.New("opencode: create session: empty id")
	}
	return out.ID, nil
}

func freeLoopbackPort() (int, error) {
	l, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		return 0, err
	}
	defer l.Close()
	addr, ok := l.Addr().(*net.TCPAddr)
	if !ok {
		return 0, errors.New("opencode: unexpected listener address")
	}
	return addr.Port, nil
}

func randomToken() (string, error) {
	var b [16]byte
	if _, err := rand.Read(b[:]); err != nil {
		return "", err
	}
	return hex.EncodeToString(b[:]), nil
}
