package main

import (
	"bytes"
	"os"
	"os/exec"
	"strings"
	"testing"
	"time"

	"github.com/russ-p/paseka/internal/colonyinit"
	"github.com/russ-p/paseka/internal/protocol"
	"github.com/russ-p/paseka/internal/runs"
)

func initRunsFixtureRepo(t *testing.T) string {
	t.Helper()
	t.Setenv("PASEKA_PROFILE", "")
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	dir := t.TempDir()

	cmd := exec.Command("git", "init")
	cmd.Dir = dir
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("git init: %v\n%s", err, out)
	}
	for _, args := range [][]string{
		{"config", "user.email", "test@test.com"},
		{"config", "user.name", "test"},
	} {
		cmd := exec.Command("git", args...)
		cmd.Dir = dir
		if out, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("git %s: %v\n%s", strings.Join(args, " "), err, out)
		}
	}

	res, err := colonyinit.Init(colonyinit.InitOptions{StartDir: dir})
	if err != nil {
		t.Fatal(err)
	}
	return res.ColonyRoot
}

func unusedCLIPID(t *testing.T) int {
	t.Helper()
	for pid := 4_000_000; pid > 3_900_000; pid-- {
		if !runs.PIDAlive(pid) {
			return pid
		}
	}
	t.Skip("no unused high pid available on this host")
	return 0
}

func writeStuckCLIRun(t *testing.T, root, traceID, agentID string, pid int) runs.Dir {
	t.Helper()
	d := runs.Dir{ColonyRoot: root, TraceID: traceID, AgentID: agentID}
	if err := d.Prepare(); err != nil {
		t.Fatal(err)
	}
	if err := d.WriteRequest(protocol.Request{
		ProtocolVersion: protocol.Version,
		TraceID:         traceID,
		AgentID:         agentID,
		Bee:             "builder",
		Adapter:         "cursor",
		Workspace:       root,
		ColonyRoot:      root,
		CreatedAt:       time.Now().UTC(),
	}); err != nil {
		t.Fatal(err)
	}
	if err := d.WriteStatusSnapshot(protocol.StatusSnapshot{
		ProtocolVersion: protocol.Version,
		State:           protocol.StatusRunning,
		PID:             pid,
		StartedAt:       time.Now().UTC().Add(-time.Hour),
	}); err != nil {
		t.Fatal(err)
	}
	return d
}

func TestRunsReconcileCLIWritesBothOutcomeRecords(t *testing.T) {
	root := initRunsFixtureRepo(t)
	d := writeStuckCLIRun(t, root, "trace-stuck", "agent-1", unusedCLIPID(t))

	var out bytes.Buffer
	run := func() {
		t.Helper()
		out.Reset()
		rootCmd := newRoot()
		rootCmd.SetOut(&out)
		rootCmd.SetErr(&out)
		rootCmd.SetArgs([]string{"runs", "reconcile", "-C", root, "--trace", "trace-stuck", "--agent", "agent-1"})
		if err := rootCmd.Execute(); err != nil {
			t.Fatalf("execute: %v\n%s", err, out.String())
		}
	}
	run()
	if !strings.Contains(out.String(), "reconciled trace-stuck/agent-1 as failed") {
		t.Fatalf("output = %s", out.String())
	}

	snap, err := d.ReadStatus()
	if err != nil {
		t.Fatal(err)
	}
	if snap.State != protocol.StatusFailed || snap.Reason != protocol.ReasonProcessGone {
		t.Fatalf("status = %+v, want failed/%s", snap, protocol.ReasonProcessGone)
	}
	res, err := d.ReadResultJSON()
	if err != nil {
		t.Fatalf("result.json: %v", err)
	}
	if res.Status != protocol.StatusFailed || res.Reason != protocol.ReasonProcessGone {
		t.Fatalf("result = %+v, want failed/%s", res, protocol.ReasonProcessGone)
	}

	statusBefore, err := os.ReadFile(d.StatusPath())
	if err != nil {
		t.Fatal(err)
	}
	run()
	if !strings.Contains(out.String(), "nothing to reconcile") {
		t.Fatalf("second run output = %s", out.String())
	}
	statusAfter, err := os.ReadFile(d.StatusPath())
	if err != nil {
		t.Fatal(err)
	}
	if string(statusBefore) != string(statusAfter) {
		t.Fatal("a repeated command changed an already-terminal run")
	}
}

func TestRunsReconcileCLIRefusesLivePID(t *testing.T) {
	root := initRunsFixtureRepo(t)
	writeStuckCLIRun(t, root, "trace-live", "agent-1", os.Getpid())

	rootCmd := newRoot()
	var out bytes.Buffer
	rootCmd.SetOut(&out)
	rootCmd.SetErr(&out)
	rootCmd.SetArgs([]string{"runs", "reconcile", "-C", root, "--trace", "trace-live", "--agent", "agent-1"})
	err := rootCmd.Execute()
	if err == nil || !strings.Contains(err.Error(), "still running") {
		t.Fatalf("execute error = %v, want a live-pid refusal", err)
	}
}
