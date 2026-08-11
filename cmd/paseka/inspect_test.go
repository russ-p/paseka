package main

import (
	"bytes"
	"os/exec"
	"strings"
	"testing"
	"time"

	"github.com/paseka/paseka/internal/colonyinit"
	"github.com/paseka/paseka/internal/protocol"
	"github.com/paseka/paseka/internal/runs"
)

func TestPrintTraceUsageAggregate(t *testing.T) {
	root := t.TempDir()
	started := time.Now().UTC()
	writeInspectRun(t, root, "trace-1", "agent-a", started, &protocol.Usage{
		InputTokens: 100, OutputTokens: 10, CacheReadTokens: 50,
	})
	writeInspectRun(t, root, "trace-1", "agent-b", started.Add(time.Minute), &protocol.Usage{
		InputTokens: 200, OutputTokens: 30, CacheWriteTokens: 5,
	})

	var buf bytes.Buffer
	if err := printTraceUsage(&buf, root, "trace-1"); err != nil {
		t.Fatal(err)
	}
	got := buf.String()
	for _, want := range []string{
		"Trace: trace-1\n",
		"  runs: 2 (2 with usage)\n",
		"  input:       300\n",
		"  output:      40\n",
		"  cache read:  50\n",
		"  cache write: 5\n",
	} {
		if !strings.Contains(got, want) {
			t.Fatalf("output missing %q:\n%s", want, got)
		}
	}
}

func TestPrintTraceUsageNone(t *testing.T) {
	root := t.TempDir()
	started := time.Now().UTC()
	writeInspectRun(t, root, "trace-empty", "agent-a", started, nil)

	var buf bytes.Buffer
	if err := printTraceUsage(&buf, root, "trace-empty"); err != nil {
		t.Fatal(err)
	}
	want := "Trace: trace-empty\n  runs: 1\n  usage: (none)\n"
	if buf.String() != want {
		t.Fatalf("output = %q, want %q", buf.String(), want)
	}
}

func TestPrintRunUsageWithTokens(t *testing.T) {
	root := t.TempDir()
	started := time.Now().UTC()
	writeInspectRun(t, root, "trace-1", "agent-1", started, &protocol.Usage{
		InputTokens:      8848,
		OutputTokens:     56,
		CacheReadTokens:  5472,
		CacheWriteTokens: 0,
		Source:           protocol.UsageSourceCursorStreamJSON,
	})

	var buf bytes.Buffer
	if err := printRunUsage(&buf, root, "trace-1", "agent-1"); err != nil {
		t.Fatal(err)
	}
	got := buf.String()
	for _, want := range []string{
		"Run: trace-1/agent-1\n",
		"  bee:     builder\n",
		"  input:   8848\n",
		"  output:  56\n",
		"  cache read:  5472\n",
		"  cache write: 0\n",
		"  source:  cursor.stream-json\n",
	} {
		if !strings.Contains(got, want) {
			t.Fatalf("output missing %q:\n%s", want, got)
		}
	}
}

func TestPrintRunUsageNone(t *testing.T) {
	root := t.TempDir()
	started := time.Now().UTC()
	writeInspectRun(t, root, "trace-1", "agent-1", started, nil)

	var buf bytes.Buffer
	if err := printRunUsage(&buf, root, "trace-1", "agent-1"); err != nil {
		t.Fatal(err)
	}
	want := "Run: trace-1/agent-1\n  bee:     builder\n  usage: (none)\n"
	if buf.String() != want {
		t.Fatalf("output = %q, want %q", buf.String(), want)
	}
}

func TestPrintRunUsageNotFound(t *testing.T) {
	root := t.TempDir()
	err := printRunUsage(&bytes.Buffer{}, root, "trace-1", "missing")
	if err == nil || !strings.Contains(err.Error(), "run not found") {
		t.Fatalf("err = %v", err)
	}
}

func TestInspectUsageCLI(t *testing.T) {
	repo := initInspectFixtureRepo(t)

	root := newRoot()
	var out bytes.Buffer
	root.SetOut(&out)
	root.SetErr(&out)
	root.SetArgs([]string{"inspect", "usage", "-C", repo, "--trace", "trace-cli"})
	if err := root.Execute(); err != nil {
		t.Fatalf("execute: %v\n%s", err, out.String())
	}
	got := out.String()
	if !strings.Contains(got, "Trace: trace-cli") || !strings.Contains(got, "input:       100") {
		t.Fatalf("unexpected output:\n%s", got)
	}
}

func initInspectFixtureRepo(t *testing.T) string {
	t.Helper()
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	dir := t.TempDir()
	runGitInspect(t, dir, "init")
	runGitInspect(t, dir, "config", "user.email", "test@test.com")
	runGitInspect(t, dir, "config", "user.name", "test")

	res, err := colonyinit.Init(colonyinit.InitOptions{StartDir: dir})
	if err != nil {
		t.Fatal(err)
	}
	writeInspectRun(t, res.ColonyRoot, "trace-cli", "agent-1", time.Now().UTC(), &protocol.Usage{
		InputTokens: 100, OutputTokens: 20,
	})
	runGitInspect(t, dir, "add", ".paseka")
	runGitInspect(t, dir, "commit", "-m", "inspect fixture")

	return res.ColonyRoot
}

func writeInspectRun(t *testing.T, root, traceID, agentID string, started time.Time, usage *protocol.Usage) {
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
		CreatedAt:       started,
	}); err != nil {
		t.Fatal(err)
	}
	if err := d.WriteResult(protocol.Result{
		ProtocolVersion: protocol.Version,
		TraceID:         traceID,
		AgentID:         agentID,
		Status:          protocol.StatusCompleted,
		Summary:         "ok",
		Usage:           usage,
		FinishedAt:      started.Add(time.Minute),
	}); err != nil {
		t.Fatal(err)
	}
}

func runGitInspect(t *testing.T, dir string, args ...string) {
	t.Helper()
	cmd := exec.Command("git", args...)
	cmd.Dir = dir
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("git %s in %s: %v\n%s", strings.Join(args, " "), dir, err, out)
	}
}
