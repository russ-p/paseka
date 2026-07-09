package sessions_test

import (
	"context"
	"os"
	"os/exec"
	"strings"
	"testing"
	"time"

	"github.com/paseka/paseka/internal/adapters"
	"github.com/paseka/paseka/internal/runs"
	"github.com/paseka/paseka/internal/sessions"
)

func TestManagerAttachPTYFanOutAndScrollback(t *testing.T) {
	repo := initSessionRepo(t)
	setupSessionHome(t, repo)

	slow := &slowOutputSessionAdapter{}
	mgr := sessions.NewManager()
	mgr.RegisterSessionAdapter("cursor", slow)

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	res, err := mgr.StartDetached(ctx, sessions.RunRequest{
		StartDir: repo,
		Bee:      "scout",
		Task:     "pty attach test",
	})
	if err != nil {
		t.Fatal(err)
	}

	stream1, err := mgr.AttachPTY(res.SessionID)
	if err != nil {
		t.Fatal(err)
	}
	defer stream1.Close()

	stream2, err := mgr.AttachPTY(res.SessionID)
	if err != nil {
		t.Fatal(err)
	}
	defer stream2.Close()

	// Wait for first output chunk.
	var got1, got2 string
	deadline := time.Now().Add(3 * time.Second)
	for time.Now().Before(deadline) {
		select {
		case chunk := <-stream1.Output():
			got1 += string(chunk)
		default:
		}
		select {
		case chunk := <-stream2.Output():
			got2 += string(chunk)
		default:
		}
		if strings.Contains(got1, "hello-pty") && strings.Contains(got2, "hello-pty") {
			break
		}
		time.Sleep(10 * time.Millisecond)
	}
	if !strings.Contains(got1, "hello-pty") {
		t.Fatalf("stream1 output = %q", got1)
	}
	if !strings.Contains(got2, "hello-pty") {
		t.Fatalf("stream2 output = %q", got2)
	}

	// Late subscriber should receive scrollback.
	stream3, err := mgr.AttachPTY(res.SessionID)
	if err != nil {
		t.Fatal(err)
	}
	defer stream3.Close()
	scroll := stream3.Scrollback()
	if !strings.Contains(string(scroll), "hello-pty") {
		t.Fatalf("scrollback = %q", string(scroll))
	}

	// Wait for session exit.
	select {
	case <-stream1.Done():
	case <-time.After(5 * time.Second):
		t.Fatal("timeout waiting for session exit")
	}

	// Ensure process fully exited before temp dir cleanup.
	deadline = time.Now().Add(3 * time.Second)
	for time.Now().Before(deadline) && len(mgr.ListActive()) > 0 {
		time.Sleep(20 * time.Millisecond)
	}

	d := runs.Dir{ColonyRoot: repo, TraceID: res.TraceID, AgentID: res.AgentID}
	entries, err := d.ReadTranscript()
	if err != nil {
		t.Fatal(err)
	}
	found := false
	for _, e := range entries {
		if e.Role == "agent" && strings.Contains(e.Content, "hello-pty") {
			found = true
		}
	}
	if !found {
		t.Fatalf("expected transcript agent line, got %+v", entries)
	}
}

func TestManagerAttachPTYWrite(t *testing.T) {
	repo := initSessionRepo(t)
	setupSessionHome(t, repo)

	echo := &echoSessionAdapter{}
	mgr := sessions.NewManager()
	mgr.RegisterSessionAdapter("cursor", echo)

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	res, err := mgr.StartDetached(ctx, sessions.RunRequest{
		StartDir: repo,
		Bee:      "scout",
		Task:     "pty write test",
	})
	if err != nil {
		t.Fatal(err)
	}

	stream, err := mgr.AttachPTY(res.SessionID)
	if err != nil {
		t.Fatal(err)
	}
	defer stream.Close()

	if err := stream.Write([]byte("ping\n")); err != nil {
		t.Fatal(err)
	}

	deadline := time.Now().Add(3 * time.Second)
	var out string
	for time.Now().Before(deadline) {
		select {
		case chunk := <-stream.Output():
			out += string(chunk)
			if strings.Contains(out, "pong") {
				goto done
			}
		default:
			time.Sleep(10 * time.Millisecond)
		}
	}
done:
	if !strings.Contains(out, "pong") {
		t.Fatalf("expected pong in output, got %q", out)
	}

	select {
	case <-stream.Done():
	case <-time.After(5 * time.Second):
		t.Fatal("timeout waiting for echo session exit")
	}
}

func TestManagerAttachPTYNotDetached(t *testing.T) {
	repo := initSessionRepo(t)
	setupSessionHome(t, repo)

	fast := &instantSessionAdapter{}
	mgr := sessions.NewManager()
	mgr.RegisterSessionAdapter("cursor", fast)

	// RunInteractive sessions have no hub; AttachPTY should fail.
	// We can't easily test RunInteractive without blocking, so test error on unknown session.
	_, err := mgr.AttachPTY("nonexistent")
	if err == nil {
		t.Fatal("expected error for missing session")
	}
}

type slowOutputSessionAdapter struct{}

func (s *slowOutputSessionAdapter) Name() string { return "cursor" }

func (s *slowOutputSessionAdapter) SessionCommand(req adapters.SessionRequest) (adapters.SessionCommand, error) {
	shell, err := exec.LookPath("sh")
	if err != nil {
		return adapters.SessionCommand{}, err
	}
	script := `printf '\033[32mhello-pty\033[0m\n'; sleep 0.2; exit 0`
	return adapters.SessionCommand{
		Binary: shell,
		Args:   []string{"-c", script},
		Dir:    req.Workspace,
		Env:    os.Environ(),
	}, nil
}

type echoSessionAdapter struct{}

func (e *echoSessionAdapter) Name() string { return "cursor" }

func (e *echoSessionAdapter) SessionCommand(req adapters.SessionRequest) (adapters.SessionCommand, error) {
	shell, err := exec.LookPath("sh")
	if err != nil {
		return adapters.SessionCommand{}, err
	}
	script := `read -r line; printf 'pong:%s\n' "$line"; exit 0`
	return adapters.SessionCommand{
		Binary: shell,
		Args:   []string{"-c", script},
		Dir:    req.Workspace,
		Env:    os.Environ(),
	}, nil
}
