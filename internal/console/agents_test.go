package console_test

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os/exec"
	"testing"
	"time"

	"github.com/paseka/paseka/internal/colony"
	"github.com/paseka/paseka/internal/console"
	"github.com/paseka/paseka/internal/protocol"
	"github.com/paseka/paseka/internal/runs"
	"github.com/paseka/paseka/internal/sessions"
)

func TestAgentsAPIHandlers(t *testing.T) {
	repo := initConsoleRepo(t)
	ctxColony := setupConsoleHome(t, repo)

	child := exec.Command("sleep", "300")
	if err := child.Start(); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if child.Process != nil {
			_ = child.Process.Kill()
		}
		_ = child.Wait()
	})

	started := time.Now().UTC().Add(-2 * time.Minute)
	writeLiveAFKRun(t, repo, "trace-live", "agent-live", "drone", started, child.Process.Pid)

	legacyStarted := started.Add(-time.Minute)
	writeLiveAFKRun(t, repo, "trace-legacy", "agent-legacy", "scout", legacyStarted, 0)

	deadPID := child.Process.Pid + 50000
	writeLiveAFKRun(t, repo, "trace-dead", "agent-dead", "scout", legacyStarted, deadPID)

	srv := console.NewServer(console.Options{
		Addr:     "127.0.0.1:0",
		Colony:   ctxColony,
		Sessions: sessions.NewManager(),
	})

	req := httptest.NewRequest(http.MethodGet, "/api/agents", nil)
	rec := httptest.NewRecorder()
	srv.Handler().ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d body=%s", rec.Code, rec.Body.String())
	}
	var view console.AgentsView
	if err := json.NewDecoder(rec.Body).Decode(&view); err != nil {
		t.Fatal(err)
	}
	if view.Count != 1 || view.AFK != 1 || view.Sessions != 0 {
		t.Fatalf("live view = %+v", view)
	}
	if len(view.Items) != 1 || view.Items[0].Kind != "afk" || view.Items[0].Bee != "drone" {
		t.Fatalf("items = %+v", view.Items)
	}
	if view.Items[0].PID != child.Process.Pid {
		t.Fatalf("pid = %d want %d", view.Items[0].PID, child.Process.Pid)
	}

	_ = child.Process.Kill()
	_ = child.Wait()

	rec2 := httptest.NewRecorder()
	srv.Handler().ServeHTTP(rec2, req)
	if rec2.Code != http.StatusOK {
		t.Fatalf("status after kill = %d body=%s", rec2.Code, rec2.Body.String())
	}
	var afterKill console.AgentsView
	if err := json.NewDecoder(rec2.Body).Decode(&afterKill); err != nil {
		t.Fatal(err)
	}
	if afterKill.Count != 0 {
		t.Fatalf("after kill = %+v", afterKill)
	}
}

func TestAgentsAPIRegistersLiveSession(t *testing.T) {
	repo := initConsoleRepo(t)
	ctxColony := setupConsoleHome(t, repo)

	child := exec.Command("sleep", "300")
	if err := child.Start(); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if child.Process != nil {
			_ = child.Process.Kill()
		}
		_ = child.Wait()
	})

	started := time.Now().UTC()
	if err := colony.RegisterSession(ctxColony.Slug, colony.SessionEntry{
		SessionID: "sess-1",
		TraceID:   "trace-sess",
		AgentID:   "agent-sess",
		Bee:       "hivewright",
		PID:       child.Process.Pid,
		StartedAt: started,
	}); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = colony.UnregisterSession(ctxColony.Slug, "sess-1") })

	srv := console.NewServer(console.Options{
		Addr:     "127.0.0.1:0",
		Colony:   ctxColony,
		Sessions: sessions.NewManager(),
	})

	req := httptest.NewRequest(http.MethodGet, "/api/agents", nil)
	rec := httptest.NewRecorder()
	srv.Handler().ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d body=%s", rec.Code, rec.Body.String())
	}
	var view console.AgentsView
	if err := json.NewDecoder(rec.Body).Decode(&view); err != nil {
		t.Fatal(err)
	}
	if view.Count != 1 || view.Sessions != 1 || view.AFK != 0 {
		t.Fatalf("view = %+v", view)
	}
	if view.Items[0].Kind != "session" || view.Items[0].SessionID != "sess-1" {
		t.Fatalf("items = %+v", view.Items)
	}
}

func writeLiveAFKRun(t *testing.T, root, traceID, agentID, bee string, started time.Time, pid int) {
	t.Helper()
	d := runs.Dir{ColonyRoot: root, TraceID: traceID, AgentID: agentID}
	if err := d.Prepare(); err != nil {
		t.Fatal(err)
	}
	if err := d.WriteRequest(protocol.Request{
		ProtocolVersion: protocol.Version,
		TraceID:         traceID,
		AgentID:         agentID,
		Bee:             bee,
		Adapter:         "cursor",
		Workspace:       root,
		ColonyRoot:      root,
		CreatedAt:       started,
	}); err != nil {
		t.Fatal(err)
	}
	snap := protocol.StatusSnapshot{
		ProtocolVersion: protocol.Version,
		State:           protocol.StatusRunning,
		StartedAt:       started,
	}
	if pid > 0 {
		snap.PID = pid
	}
	if err := d.WriteStatusSnapshot(snap); err != nil {
		t.Fatal(err)
	}
}
