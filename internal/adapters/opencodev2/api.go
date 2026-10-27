package opencodev2

import (
	"bytes"
	"context"
	"crypto/rand"
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
	serveAttempt = 5 * time.Second
	serveRetry   = 200 * time.Millisecond
	apiUsername  = "opencode"

	// sessionIDPrefix and sessionIDLength mirror the provider's own id shape:
	// V2 accepts a caller-supplied id, and a longer or differently-prefixed one
	// is rejected rather than silently replaced.
	sessionIDPrefix = "ses_"
	sessionIDLength = 26
)

// serveClient talks to the loopback-only OpenCode server. Proxy is disabled so
// an inherited HTTP(S)_PROXY cannot intercept 127.0.0.1.
var serveClient = &http.Client{Transport: &http.Transport{Proxy: nil}}

// managedServer is an `opencode serve` process Paseka owns. For a HITL session it
// lives as long as the TUI; for AFK accounting it is a short-lived probe.
type managedServer struct {
	cmd      *exec.Cmd
	port     int
	password string
	done     chan struct{}
	output   *bytes.Buffer
}

func startServer(binary, workspace string, env []string) (*managedServer, error) {
	port, err := freeLoopbackPort()
	if err != nil {
		return nil, fmt.Errorf("opencodev2: find port: %w", err)
	}
	password, err := randomToken()
	if err != nil {
		return nil, fmt.Errorf("opencodev2: generate password: %w", err)
	}

	cmd := exec.Command(binary, "serve", "--hostname", "127.0.0.1", "--port", strconv.Itoa(port))
	cmd.Dir = workspace
	cmd.Env = append(envWithout(env, "OPENCODE_SERVER_PASSWORD", "OPENCODE_SERVER_USERNAME"),
		"OPENCODE_SERVER_PASSWORD="+password,
		"OPENCODE_SERVER_USERNAME="+apiUsername)
	var out bytes.Buffer
	cmd.Stdout = &out
	cmd.Stderr = &out
	if err := cmd.Start(); err != nil {
		return nil, fmt.Errorf("opencodev2: start serve: %w", err)
	}

	srv := &managedServer{cmd: cmd, port: port, password: password, done: make(chan struct{}), output: &out}
	go func() {
		_ = cmd.Wait()
		close(srv.done)
	}()
	return srv, nil
}

// URL is the loopback origin the TUI and API client talk to.
func (s *managedServer) URL() string {
	return fmt.Sprintf("http://127.0.0.1:%d", s.port)
}

func (s *managedServer) client() *apiClient {
	return newAPIClient(s.port, s.password)
}

// Close stops the server. It is idempotent: the process may already have exited.
func (s *managedServer) Close() {
	if s == nil || s.cmd == nil || s.cmd.Process == nil {
		return
	}
	_ = s.cmd.Process.Kill()
	<-s.done
}

func (s *managedServer) outputText() string {
	if s == nil || s.output == nil {
		return ""
	}
	return strings.TrimSpace(s.output.String())
}

// withServer starts a throwaway server and retries attempt until it answers, the
// server dies, or the attempt timeout expires. Session state is durable in
// OpenCode's store, so a throwaway server can answer for a session whose own TUI
// has already exited.
func withServer(binary, workspace string, env []string, attempt func(context.Context, *apiClient) error) error {
	srv, err := startServer(binary, workspace, env)
	if err != nil {
		return err
	}
	defer srv.Close()

	ctx, cancel := context.WithTimeout(context.Background(), serveTimeout)
	defer cancel()

	var lastErr error
	for {
		attemptCtx, attemptCancel := context.WithTimeout(ctx, serveAttempt)
		err := attempt(attemptCtx, srv.client())
		attemptCancel()
		if err == nil {
			return nil
		}
		lastErr = err
		if !isRetryable(err) {
			return err
		}
		select {
		case <-srv.done:
			return fmt.Errorf("opencodev2: server exited before accepting requests: %w%s", lastErr, serveOutput(srv.outputText()))
		case <-ctx.Done():
			return lastErr
		case <-time.After(serveRetry):
		}
	}
}

func serveOutput(s string) string {
	if s == "" {
		return ""
	}
	return " (serve output: " + s + ")"
}

// apiError is a non-2xx answer from the OpenCode API.
type apiError struct {
	Status  int
	Method  string
	Path    string
	Message string
}

func (e *apiError) Error() string {
	if e == nil {
		return ""
	}
	msg := strings.TrimSpace(e.Message)
	if msg == "" {
		return fmt.Sprintf("%s %s: HTTP %d", e.Method, e.Path, e.Status)
	}
	return fmt.Sprintf("%s %s: HTTP %d: %s", e.Method, e.Path, e.Status, msg)
}

// isRetryable reports whether an error is transient while the server boots. A
// 4xx answer is final: retrying a rejected caller-supplied session id would just
// stall before the fallback.
func isRetryable(err error) bool {
	var apiErr *apiError
	if errors.As(err, &apiErr) {
		return apiErr.Status >= 500
	}
	var netErr net.Error
	if errors.As(err, &netErr) {
		return true
	}
	return true
}

// apiClient is the typed HTTP client for the OpenCode 2 API. Credential handling
// is explicit here rather than delegated to `opencode api`, so calls stay
// testable against a local stub server.
type apiClient struct {
	baseURL  string
	username string
	password string
	http     *http.Client
}

func newAPIClient(port int, password string) *apiClient {
	return &apiClient{
		baseURL:  fmt.Sprintf("http://127.0.0.1:%d", port),
		username: apiUsername,
		password: password,
		http:     serveClient,
	}
}

func (c *apiClient) do(ctx context.Context, method, path string, body, out any) error {
	var reader io.Reader
	if body != nil {
		data, err := json.Marshal(body)
		if err != nil {
			return err
		}
		reader = bytes.NewReader(data)
	}
	req, err := http.NewRequestWithContext(ctx, method, c.baseURL+path, reader)
	if err != nil {
		return err
	}
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	req.SetBasicAuth(c.username, c.password)
	resp, err := c.http.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		msg, _ := io.ReadAll(io.LimitReader(resp.Body, 4096))
		return &apiError{Status: resp.StatusCode, Method: method, Path: path, Message: string(msg)}
	}
	if out == nil {
		_, _ = io.Copy(io.Discard, resp.Body)
		return nil
	}
	data, err := io.ReadAll(io.LimitReader(resp.Body, 8<<20))
	if err != nil {
		return err
	}
	return decodeEnvelope(data, out)
}

// decodeEnvelope unwraps the `{"data": ...}` shape V2 uses. A body without the
// envelope is decoded as-is, so a future unwrapped answer still reads.
func decodeEnvelope(data []byte, out any) error {
	var env struct {
		Data json.RawMessage `json:"data"`
	}
	if err := json.Unmarshal(data, &env); err == nil && len(env.Data) > 0 {
		data = env.Data
	}
	return json.Unmarshal(data, out)
}

type sessionModel struct {
	ProviderID string `json:"providerID,omitempty"`
	ModelID    string `json:"modelID,omitempty"`
	Variant    string `json:"variant,omitempty"`
}

type createSessionBody struct {
	ID       string        `json:"id,omitempty"`
	Location string        `json:"location,omitempty"`
	Title    string        `json:"title,omitempty"`
	Agent    string        `json:"agent,omitempty"`
	Model    *sessionModel `json:"model,omitempty"`
}

type sessionRecord struct {
	ID       string                     `json:"id"`
	Location string                     `json:"location"`
	Title    string                     `json:"title"`
	Tokens   map[string]json.RawMessage `json:"tokens"`
	Cost     float64                    `json:"cost"`
	Outcome  string                     `json:"outcome"`
}

// CreateSession creates a session with the caller-supplied id. If the provider
// rejects that id, it retries without one and reads the assigned id back.
func (c *apiClient) CreateSession(ctx context.Context, body createSessionBody) (sessionRecord, error) {
	var rec sessionRecord
	chosen := body.ID
	err := c.do(ctx, http.MethodPost, "/api/session", body, &rec)
	if err == nil && rec.ID != "" {
		return rec, nil
	}
	if err == nil && chosen != "" {
		rec.ID = chosen
		return rec, nil
	}
	if chosen == "" || isRetryable(err) {
		if err == nil {
			err = errors.New("opencodev2: create session: empty id")
		}
		return sessionRecord{}, err
	}
	fallback := body
	fallback.ID = ""
	var assigned sessionRecord
	if fallbackErr := c.do(ctx, http.MethodPost, "/api/session", fallback, &assigned); fallbackErr != nil {
		return sessionRecord{}, fmt.Errorf("opencodev2: create session rejected id %q (%v); fallback failed: %w", chosen, err, fallbackErr)
	}
	if assigned.ID == "" {
		return sessionRecord{}, errors.New("opencodev2: create session fallback: empty id")
	}
	return assigned, nil
}

// Session reads one session record: usage, cost, location, and outcome.
func (c *apiClient) Session(ctx context.Context, id string) (sessionRecord, error) {
	var rec sessionRecord
	err := c.do(ctx, http.MethodGet, "/api/session/"+id, nil, &rec)
	return rec, err
}

// Prompt admits a turn into a session. The V2 TUI delivers the kickoff through
// its own --prompt flag; this call exists for clients that drive the API
// directly and keeps the credential handling in one place.
func (c *apiClient) Prompt(ctx context.Context, id, text string) error {
	body := map[string]string{"text": text}
	return c.do(ctx, http.MethodPost, "/api/session/"+id+"/prompt", body, nil)
}

func freeLoopbackPort() (int, error) {
	l, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		return 0, err
	}
	defer l.Close()
	addr, ok := l.Addr().(*net.TCPAddr)
	if !ok {
		return 0, errors.New("opencodev2: unexpected listener address")
	}
	return addr.Port, nil
}

func randomToken() (string, error) {
	var b [16]byte
	if _, err := rand.Read(b[:]); err != nil {
		return "", err
	}
	return fmt.Sprintf("%x", b), nil
}

// NewSessionID mints an id in the provider's charset and length. Supplying our
// own id removes the pre-create race and keeps the stored provider id meaningful.
func NewSessionID() (string, error) {
	const alphabet = "0123456789abcdefghijklmnopqrstuvwxyz"
	buf := make([]byte, sessionIDLength)
	if _, err := rand.Read(buf); err != nil {
		return "", err
	}
	for i := range buf {
		buf[i] = alphabet[int(buf[i])%len(alphabet)]
	}
	return sessionIDPrefix + string(buf), nil
}

// envWithout returns env with any entries for the given keys removed, so a
// freshly generated server secret always wins over an inherited one.
func envWithout(env []string, keys ...string) []string {
	out := make([]string, 0, len(env))
	for _, e := range env {
		skip := false
		for _, k := range keys {
			if strings.HasPrefix(e, k+"=") {
				skip = true
				break
			}
		}
		if !skip {
			out = append(out, e)
		}
	}
	return out
}
