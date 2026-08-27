package hiveview_test

import (
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"testing"
	"time"

	"github.com/paseka/paseka/internal/colony"
	"github.com/paseka/paseka/internal/hiveview"
	"github.com/paseka/paseka/internal/homestate"
	"github.com/paseka/paseka/internal/protocol"
	"github.com/paseka/paseka/internal/runs"
	"github.com/paseka/paseka/internal/runtime"
)

func TestBuildColonySnapshotStoppedRuntime(t *testing.T) {
	ctx := setupSnapshotColony(t, "")

	snap, err := hiveview.BuildColonySnapshot(ctx, nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	if snap.SchemaVersion != 1 {
		t.Fatalf("schemaVersion = %d", snap.SchemaVersion)
	}
	if snap.Slug != "topology-fixture" {
		t.Fatalf("slug = %q", snap.Slug)
	}
	if snap.Runtime.Status != runtime.RuntimeStatusStopped || snap.Runtime.Alive {
		t.Fatalf("runtime = %+v", snap.Runtime)
	}
	if snap.Attention.WaitingReview == nil || snap.Attention.FailedTasks == nil {
		t.Fatalf("attention arrays should be present: %+v", snap.Attention)
	}
	if snap.Agents.Items == nil {
		t.Fatal("agents.items should be empty slice, not nil")
	}
	raw, err := json.Marshal(snap)
	if err != nil {
		t.Fatal(err)
	}
	var decoded map[string]any
	if err := json.Unmarshal(raw, &decoded); err != nil {
		t.Fatal(err)
	}
	agents, _ := decoded["agents"].(map[string]any)
	if agents["items"] == nil {
		t.Fatalf("json agents.items is null: %s", raw)
	}
}

func TestBuildColonySnapshotLiveAFKAgent(t *testing.T) {
	ctx := setupSnapshotColony(t, "")
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

	started := time.Now().UTC().Add(-time.Minute)
	writeSnapshotAFKRun(t, ctx.ColonyRoot, "trace-live", "agent-live", "drone", started, child.Process.Pid)

	snap, err := hiveview.BuildColonySnapshot(ctx, nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	if snap.Agents.Count != 1 || snap.Agents.AFK != 1 {
		t.Fatalf("agents = %+v", snap.Agents)
	}
	if len(snap.Agents.Items) != 1 || snap.Agents.Items[0].TraceID != "trace-live" {
		t.Fatalf("items = %+v", snap.Agents.Items)
	}
}

func TestBuildColonySnapshotAttentionTasks(t *testing.T) {
	ctx := setupSnapshotColony(t, "")
	traceID := "trace-attn"
	taskDir, err := runs.NewTaskDir(ctx.ColonyRoot, traceID, "task-review")
	if err != nil {
		t.Fatal(err)
	}
	if err := taskDir.WriteTask(runs.TaskFrontmatter{
		TraceID: traceID,
		TaskID:  "task-review",
		Title:   "Review me",
		Bee:     "scout",
		Status:  protocol.TaskStatusWaitingReview,
		Review:  protocol.TaskReviewRequired,
	}, "body"); err != nil {
		t.Fatal(err)
	}
	failDir, err := runs.NewTaskDir(ctx.ColonyRoot, traceID, "task-fail")
	if err != nil {
		t.Fatal(err)
	}
	if err := failDir.WriteTask(runs.TaskFrontmatter{
		TraceID: traceID,
		TaskID:  "task-fail",
		Title:   "Failed task",
		Bee:     "scout",
		Status:  protocol.TaskStatusFailed,
	}, "body"); err != nil {
		t.Fatal(err)
	}

	snap, err := hiveview.BuildColonySnapshot(ctx, nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	if len(snap.Attention.WaitingReview) != 1 || snap.Attention.WaitingReview[0].TaskID != "task-review" {
		t.Fatalf("waitingReview = %+v", snap.Attention.WaitingReview)
	}
	if len(snap.Attention.FailedTasks) != 1 || snap.Attention.FailedTasks[0].TaskID != "task-fail" {
		t.Fatalf("failedTasks = %+v", snap.Attention.FailedTasks)
	}
	if snap.TaskCounts[string(protocol.TaskStatusWaitingReview)] != 1 {
		t.Fatalf("taskCounts = %+v", snap.TaskCounts)
	}
}

func TestBuildColonySnapshotEnergyUnavailableWithoutNATS(t *testing.T) {
	ctx := setupSnapshotColony(t, "")
	snap, err := hiveview.BuildColonySnapshot(ctx, nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	if snap.Energy.Available {
		t.Fatalf("energy = %+v", snap.Energy)
	}
	if len(snap.Energy.Traces) != 0 {
		t.Fatalf("traces = %+v", snap.Energy.Traces)
	}
}

func TestBuildColonySnapshotNATSUnconfiguredNotDown(t *testing.T) {
	ctx := setupSnapshotColony(t, "")
	snap, err := hiveview.BuildColonySnapshot(ctx, nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	if snap.Attention.NatsDown {
		t.Fatalf("natsDown should be false when unconfigured")
	}
}

func TestBuildColonySnapshotStaleRuntimeAttention(t *testing.T) {
	ctx := setupSnapshotColony(t, "")
	if err := homestate.RegisterRuntime(ctx.Slug, homestate.RuntimeEntry{
		PID:       999999999,
		StartedAt: time.Now().UTC(),
		Status:    runtime.RuntimeStatusRunning,
	}); err != nil {
		t.Fatal(err)
	}

	snap, err := hiveview.BuildColonySnapshot(ctx, nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	if snap.Runtime.Status != runtime.RuntimeStatusStale || snap.Runtime.Alive {
		t.Fatalf("runtime = %+v", snap.Runtime)
	}
	if !snap.Attention.RuntimeStale {
		t.Fatalf("runtimeStale = false")
	}
}

func TestBuildColonySnapshotSubstrateHealthyAliveRuntime(t *testing.T) {
	ctx := setupSnapshotColony(t, "")
	if err := homestate.RegisterRuntime(ctx.Slug, homestate.RuntimeEntry{
		PID:       os.Getpid(),
		StartedAt: time.Now().UTC(),
		Status:    runtime.RuntimeStatusRunning,
	}); err != nil {
		t.Fatal(err)
	}

	snap, err := hiveview.BuildColonySnapshot(ctx, nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	if !snap.Runtime.Alive || !snap.SubstrateHealthy() {
		t.Fatalf("runtime = %+v healthy=%v", snap.Runtime, snap.SubstrateHealthy())
	}
}

func TestFormatColonySnapshotIncludesBlocks(t *testing.T) {
	text := hiveview.FormatColonySnapshot(hiveview.ColonySnapshot{
		Slug: "demo",
		Runtime: hiveview.SnapshotRuntime{
			Status: runtime.RuntimeStatusStopped,
			Alive:  false,
		},
		Agents: hiveview.AgentsView{},
		TaskCounts: map[string]int{
			string(protocol.TaskStatusRunning):       1,
			string(protocol.TaskStatusWaitingReview): 2,
			string(protocol.TaskStatusFailed):        0,
		},
		Energy: hiveview.SnapshotEnergy{Traces: []hiveview.SnapshotEnergyTrace{}},
		Attention: hiveview.SnapshotAttention{
			WaitingReview:   []hiveview.SnapshotAttentionTask{},
			PendingInvites:  []hiveview.SnapshotAttentionInvite{},
			FailedTasks:     []hiveview.SnapshotAttentionTask{},
			LowEnergyTraces: []hiveview.SnapshotLowEnergy{},
		},
	})
	if text == "" {
		t.Fatal("empty text")
	}
	for _, want := range []string{"Paseka · demo", "Runtime:", "Tasks:", "running=1"} {
		if !contains(text, want) {
			t.Fatalf("text missing %q:\n%s", want, text)
		}
	}
}

func TestFormatColonySnapshotHoneyUsesAllocated(t *testing.T) {
	text := hiveview.FormatColonySnapshot(hiveview.ColonySnapshot{
		Slug:    "demo",
		Runtime: hiveview.SnapshotRuntime{Status: runtime.RuntimeStatusStopped},
		Agents:  hiveview.AgentsView{},
		Energy: hiveview.SnapshotEnergy{
			Available: true,
			Traces: []hiveview.SnapshotEnergyTrace{
				{TraceID: "t1", Remaining: 5, Budget: 12, Added: 8, Allocated: 20},
			},
		},
		Attention: hiveview.SnapshotAttention{
			LowEnergyTraces: []hiveview.SnapshotLowEnergy{
				{TraceID: "t2", Remaining: 0, Budget: 12, Added: 3, Allocated: 15},
			},
		},
	})
	if !contains(text, "t1: 5/20 remaining") {
		t.Fatalf("honey line missing allocated denom:\n%s", text)
	}
	if !contains(text, "t2 (0/15)") {
		t.Fatalf("low energy line missing allocated denom:\n%s", text)
	}
}

func contains(s, sub string) bool {
	return len(s) >= len(sub) && (s == sub || len(sub) == 0 || indexOf(s, sub) >= 0)
}

func indexOf(s, sub string) int {
	for i := 0; i+len(sub) <= len(s); i++ {
		if s[i:i+len(sub)] == sub {
			return i
		}
	}
	return -1
}

func setupSnapshotColony(t *testing.T, natsURL string) colony.Context {
	t.Helper()
	repo := t.TempDir()
	home := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", home)
	slug := "topology-fixture"
	homeDir := filepath.Join(home, "paseka", slug)
	if err := os.MkdirAll(homeDir, 0o755); err != nil {
		t.Fatal(err)
	}
	cfg := "colony_root: " + repo + "\nslug: " + slug + "\n"
	if natsURL != "" {
		cfg += "nats:\n  url: " + natsURL + "\n"
	}
	if err := os.WriteFile(filepath.Join(homeDir, "config.yaml"), []byte(cfg), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(homeDir, "state.json"), []byte("{}\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Join(repo, ".paseka"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(repo, ".paseka", "colony.yaml"), []byte("name: status-fixture\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	ctx := colony.Context{
		ColonyRoot: repo,
		Slug:       slug,
		Home: colony.HomeConfig{
			ColonyRoot: repo,
			Slug:       slug,
		},
	}
	if natsURL != "" {
		ctx.Home.NATS = colony.NATSConfig{URL: natsURL}
	}
	return ctx
}

func writeSnapshotAFKRun(t *testing.T, root, traceID, agentID, bee string, started time.Time, pid int) {
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
		PID:             pid,
	}
	if err := d.WriteStatusSnapshot(snap); err != nil {
		t.Fatal(err)
	}
}
