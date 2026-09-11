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
	createSessionTimeout = 20 * time.Second
	createSessionAttempt = 3 * time.Second
	createSessionRetry   = 250 * time.Millisecond
)

// createSessionFunc allocates a new OpenCode provider session without running a
// model turn. It is a package seam so tests can stub the serve + HTTP pre-create.
var createSessionFunc = createSession

// preCreateClient talks to the loopback-only pre-create server. Proxy is
// disabled so an inherited HTTP(S)_PROXY cannot intercept 127.0.0.1.
var preCreateClient = &http.Client{
	Transport: &http.Transport{Proxy: nil},
}

// createSession is the OpenCode equivalent of Cursor's create-chat: it starts a
// short-lived `opencode serve`, asks it for a fresh session id, then stops the
// server. The session is durable in OpenCode's store, so the TUI can continue it
// with --session.
func createSession(binary, workspace string, env []string) (string, error) {
	port, err := freeLoopbackPort()
	if err != nil {
		return "", fmt.Errorf("opencode: find port: %w", err)
	}
	password, err := randomToken()
	if err != nil {
		return "", fmt.Errorf("opencode: generate password: %w", err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), createSessionTimeout)
	defer cancel()

	cmd := exec.CommandContext(ctx, binary, "serve", "--hostname", "127.0.0.1", "--port", strconv.Itoa(port))
	cmd.Dir = workspace
	cmd.Env = append(append([]string{}, env...), "OPENCODE_SERVER_PASSWORD="+password)
	var out bytes.Buffer
	cmd.Stdout = &out
	cmd.Stderr = &out
	if err := cmd.Start(); err != nil {
		return "", fmt.Errorf("opencode: start serve: %w", err)
	}

	done := make(chan struct{})
	go func() {
		_ = cmd.Wait()
		close(done)
	}()

	id, runErr := waitForSession(ctx, port, password, workspace, done)
	_ = cmd.Process.Kill()
	<-done
	if runErr != nil {
		return "", fmt.Errorf("opencode: serve: %w%s", runErr, serveOutput(out.String()))
	}
	return id, nil
}

func serveOutput(s string) string {
	s = strings.TrimSpace(s)
	if s == "" {
		return ""
	}
	return " (serve output: " + s + ")"
}

func waitForSession(ctx context.Context, port int, password, workspace string, serverDone <-chan struct{}) (string, error) {
	var lastErr error
	for {
		id, err := postSession(ctx, port, password, workspace)
		if err == nil {
			return id, nil
		}
		lastErr = err
		select {
		case <-serverDone:
			if lastErr != nil {
				return "", fmt.Errorf("server exited before accepting requests: %w", lastErr)
			}
			return "", errors.New("server exited before accepting requests")
		case <-ctx.Done():
			if lastErr != nil {
				return "", lastErr
			}
			return "", ctx.Err()
		case <-time.After(createSessionRetry):
		}
	}
}

func postSession(parent context.Context, port int, password, workspace string) (string, error) {
	ctx, cancel := context.WithTimeout(parent, createSessionAttempt)
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

	resp, err := preCreateClient.Do(req)
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
