package console_test

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"strings"
	"testing"
	"time"

	"github.com/coder/websocket"
	"github.com/paseka/paseka/internal/adapters"
	"github.com/paseka/paseka/internal/console"
	"github.com/paseka/paseka/internal/sessions"
)

func TestSessionPTYWebSocketRelay(t *testing.T) {
	repo := initConsoleRepo(t)
	ctxColony := setupConsoleHome(t, repo)

	mgr := sessions.NewManager()
	mgr.RegisterSessionAdapter("cursor", &slowPTYSessionAdapter{})

	srv := console.NewServer(console.Options{
		Addr:     "127.0.0.1:0",
		Colony:   ctxColony,
		Sessions: mgr,
	})

	ts := httptest.NewServer(srv.Handler())
	defer ts.Close()

	createBody := `{"bee":"scout","task":"pty ws test"}`
	createReq, err := http.NewRequest(http.MethodPost, ts.URL+"/api/sessions", bytes.NewBufferString(createBody))
	if err != nil {
		t.Fatal(err)
	}
	createResp, err := http.DefaultClient.Do(createReq)
	if err != nil {
		t.Fatal(err)
	}
	defer createResp.Body.Close()
	if createResp.StatusCode != http.StatusCreated {
		t.Fatalf("create status = %d", createResp.StatusCode)
	}
	var created console.SessionView
	if err := json.NewDecoder(createResp.Body).Decode(&created); err != nil {
		t.Fatal(err)
	}

	wsURL := strings.Replace(ts.URL, "http://", "ws://", 1) + "/api/sessions/" + created.SessionID + "/pty"
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	conn, _, err := websocket.Dial(ctx, wsURL, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer conn.Close(websocket.StatusNormalClosure, "")

	var out strings.Builder
	resizeSent := false
	exited := false
	readCtx, readCancel := context.WithTimeout(ctx, 3*time.Second)
	defer readCancel()
	for !exited {
		msgType, data, err := conn.Read(readCtx)
		if err != nil {
			t.Fatalf("read before exited status: %v", err)
		}
		if msgType == websocket.MessageBinary {
			out.Write(data)
			if strings.Contains(out.String(), "hello-ws-pty") && !resizeSent {
				resize, _ := json.Marshal(map[string]any{"type": "resize", "cols": 80, "rows": 24})
				if err := conn.Write(ctx, websocket.MessageText, resize); err != nil {
					t.Fatal(err)
				}
				resizeSent = true
			}
		}
		if msgType == websocket.MessageText {
			var status struct {
				Type  string `json:"type"`
				State string `json:"state"`
			}
			_ = json.Unmarshal(data, &status)
			if status.Type == "status" && status.State == "exited" {
				exited = true
			}
		}
	}
	if !strings.Contains(out.String(), "hello-ws-pty") {
		t.Fatalf("ws output = %q", out.String())
	}
	if !resizeSent {
		t.Fatal("resize was not sent")
	}
}

func TestSessionPTYWebSocketTextInput(t *testing.T) {
	repo := initConsoleRepo(t)
	ctxColony := setupConsoleHome(t, repo)

	mgr := sessions.NewManager()
	mgr.RegisterSessionAdapter("cursor", &echoPTYSessionAdapter{})

	srv := console.NewServer(console.Options{
		Addr:     "127.0.0.1:0",
		Colony:   ctxColony,
		Sessions: mgr,
	})

	ts := httptest.NewServer(srv.Handler())
	defer ts.Close()

	createBody := `{"bee":"scout","task":"pty text input"}`
	createReq, err := http.NewRequest(http.MethodPost, ts.URL+"/api/sessions", bytes.NewBufferString(createBody))
	if err != nil {
		t.Fatal(err)
	}
	createResp, err := http.DefaultClient.Do(createReq)
	if err != nil {
		t.Fatal(err)
	}
	defer createResp.Body.Close()
	if createResp.StatusCode != http.StatusCreated {
		t.Fatalf("create status = %d", createResp.StatusCode)
	}
	var created console.SessionView
	if err := json.NewDecoder(createResp.Body).Decode(&created); err != nil {
		t.Fatal(err)
	}

	wsURL := strings.Replace(ts.URL, "http://", "ws://", 1) + "/api/sessions/" + created.SessionID + "/pty"
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	conn, _, err := websocket.Dial(ctx, wsURL, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer conn.Close(websocket.StatusNormalClosure, "")

	if err := conn.Write(ctx, websocket.MessageText, []byte("ping-from-text\n")); err != nil {
		t.Fatal(err)
	}

	var out strings.Builder
	deadline := time.Now().Add(3 * time.Second)
	for time.Now().Before(deadline) {
		readCtx, readCancel := context.WithTimeout(ctx, 200*time.Millisecond)
		msgType, data, err := conn.Read(readCtx)
		readCancel()
		if err != nil {
			continue
		}
		if msgType == websocket.MessageBinary {
			out.Write(data)
			if strings.Contains(out.String(), "ping-from-text") {
				return
			}
		}
	}
	t.Fatalf("expected echo of text input, got %q", out.String())
}

func TestSessionPTYWebSocketInactiveSession(t *testing.T) {
	repo := initConsoleRepo(t)
	ctxColony := setupConsoleHome(t, repo)

	mgr := sessions.NewManager()
	mgr.RegisterSessionAdapter("cursor", &slowPTYSessionAdapter{})

	srv := console.NewServer(console.Options{
		Addr:     "127.0.0.1:0",
		Colony:   ctxColony,
		Sessions: mgr,
	})

	ts := httptest.NewServer(srv.Handler())
	defer ts.Close()

	wsURL := strings.Replace(ts.URL, "http://", "ws://", 1) + "/api/sessions/nonexistent/pty"
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	conn, _, err := websocket.Dial(ctx, wsURL, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer conn.Close(websocket.StatusNormalClosure, "")

	msgType, data, err := conn.Read(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if msgType != websocket.MessageText {
		t.Fatalf("expected text status, got type %v", msgType)
	}
	var status struct {
		Type   string `json:"type"`
		State  string `json:"state"`
		Reason string `json:"reason"`
	}
	if err := json.Unmarshal(data, &status); err != nil {
		t.Fatal(err)
	}
	if status.Type != "status" || status.State != "exited" || status.Reason == "" {
		t.Fatalf("status = %+v", status)
	}
}

func TestSessionPTYWebSocketRejectsCrossOrigin(t *testing.T) {
	repo := initConsoleRepo(t)
	ctxColony := setupConsoleHome(t, repo)

	mgr := sessions.NewManager()
	mgr.RegisterSessionAdapter("cursor", &slowPTYSessionAdapter{})

	srv := console.NewServer(console.Options{
		Addr:     "127.0.0.1:0",
		Colony:   ctxColony,
		Sessions: mgr,
	})

	ts := httptest.NewServer(srv.Handler())
	defer ts.Close()

	createBody := `{"bee":"scout","task":"pty origin test"}`
	createReq, err := http.NewRequest(http.MethodPost, ts.URL+"/api/sessions", bytes.NewBufferString(createBody))
	if err != nil {
		t.Fatal(err)
	}
	createResp, err := http.DefaultClient.Do(createReq)
	if err != nil {
		t.Fatal(err)
	}
	defer createResp.Body.Close()
	if createResp.StatusCode != http.StatusCreated {
		t.Fatalf("create status = %d", createResp.StatusCode)
	}
	var created console.SessionView
	if err := json.NewDecoder(createResp.Body).Decode(&created); err != nil {
		t.Fatal(err)
	}

	wsURL := strings.Replace(ts.URL, "http://", "ws://", 1) + "/api/sessions/" + created.SessionID + "/pty"
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	_, _, err = websocket.Dial(ctx, wsURL, &websocket.DialOptions{
		HTTPHeader: http.Header{
			"Origin": []string{"https://evil.example"},
		},
	})
	if err == nil {
		t.Fatal("expected cross-origin websocket dial to fail")
	}
}

type slowPTYSessionAdapter struct{}

func (s *slowPTYSessionAdapter) Name() string { return "cursor" }

func (s *slowPTYSessionAdapter) SessionCommand(req adapters.SessionRequest) (adapters.SessionCommand, error) {
	shell, err := exec.LookPath("sh")
	if err != nil {
		return adapters.SessionCommand{}, err
	}
	script := `printf '\033[32mhello-ws-pty\033[0m\n'; sleep 0.5; exit 0`
	return adapters.SessionCommand{
		Binary: shell,
		Args:   []string{"-c", script},
		Env:    os.Environ(),
		Dir:    req.Workspace,
	}, nil
}

type echoPTYSessionAdapter struct{}

func (e *echoPTYSessionAdapter) Name() string { return "cursor" }

func (e *echoPTYSessionAdapter) SessionCommand(req adapters.SessionRequest) (adapters.SessionCommand, error) {
	shell, err := exec.LookPath("sh")
	if err != nil {
		return adapters.SessionCommand{}, err
	}
	script := `IFS= read -r line; printf '%s\n' "$line"; sleep 0.2; exit 0`
	return adapters.SessionCommand{
		Binary: shell,
		Args:   []string{"-c", script},
		Env:    os.Environ(),
		Dir:    req.Workspace,
	}, nil
}
