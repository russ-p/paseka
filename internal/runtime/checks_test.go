package runtime_test

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/russ-p/paseka/internal/adapters"
	"github.com/russ-p/paseka/internal/colony"
	"github.com/russ-p/paseka/internal/patches"
	"github.com/russ-p/paseka/internal/protocol"
	"github.com/russ-p/paseka/internal/runs"
	"github.com/russ-p/paseka/internal/runtime"
	"github.com/russ-p/paseka/internal/taskledger"
)

const gateParams = `params:
  model: composer-2.5
  yolo: true
`

func gatedBuilderYAML(checks string) string {
	return `role: builder
adapter: cursor
prompt_template: builder.md
` + gateParams + checks
}

func gatedProposalBuilderYAML(checks string) string {
	return `role: builder
adapter: cursor
worktree: true
prompt_template: builder.md
` + gateParams + `publishes:
  - type: MUTATION
    kind: code.proposal.isolated
` + checks
}

func checksCommand(command string) string {
	return "checks:\n  command: [" + command + "]\n"
}

func writeGateFile(t *testing.T, path, content string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
}

// onlyRunDir returns the single agent run directory under a trace.
func onlyRunDir(t *testing.T, root, traceID string) runs.Dir {
	t.Helper()
	entries, err := os.ReadDir(filepath.Join(root, ".paseka", "runs", traceID))
	if err != nil {
		t.Fatal(err)
	}
	var agentDirs []string
	for _, ent := range entries {
		if !ent.IsDir() {
			continue
		}
		d := runs.Dir{ColonyRoot: root, TraceID: traceID, AgentID: ent.Name()}
		if _, err := d.ReadRequest(); err != nil {
			continue // e.g. the trail's artifacts comb, not a run
		}
		agentDirs = append(agentDirs, ent.Name())
	}
	if len(agentDirs) != 1 {
		t.Fatalf("run dirs = %v, want exactly one", agentDirs)
	}
	return runs.Dir{ColonyRoot: root, TraceID: traceID, AgentID: agentDirs[0]}
}

func publishedVerification(pub *recordingPublisher, kind protocol.VerificationKind) []protocol.Event {
	var out []protocol.Event
	for _, ev := range pub.events {
		if ev.Type != protocol.EventVerification {
			continue
		}
		if protocol.PayloadKind(ev.Payload) != string(kind) {
			continue
		}
		out = append(out, ev)
	}
	return out
}

func publishedMutations(pub *recordingPublisher) int {
	count := 0
	for _, ev := range pub.events {
		if ev.Type == protocol.EventMutation {
			count++
		}
	}
	return count
}

func TestDispatchCheckGatePassesAndRecordsVerdict(t *testing.T) {
	root := t.TempDir()
	writeProposalColony(t, root, gatedProposalBuilderYAML(checksCommand(`"sh", "-c", "test -f README.md"`)))

	pub := &recordingPublisher{}
	d := runtime.NewDispatcher()
	d.RegisterAdapter("cursor", &mutatingAdapter{run: func(workspace string) error {
		readme := filepath.Join(workspace, "README.md")
		data, err := os.ReadFile(readme)
		if err != nil {
			return err
		}
		return os.WriteFile(readme, append(data, []byte("feature\n")...), 0o644)
	}})
	d.SetPublisher(pub, false)
	reg, err := runtime.BuildBeeRegistry(root)
	if err != nil {
		t.Fatal(err)
	}
	d.SetBeeRegistry(reg)

	result, err := d.Dispatch(context.Background(), runtime.DispatchRequest{
		ColonyRoot: root,
		Bee:        "builder",
		TraceID:    "trace-gate-pass",
		TaskID:     "task-1",
		Task:       "add feature",
	})
	if err != nil {
		t.Fatal(err)
	}
	if result.Status != string(protocol.StatusCompleted) {
		t.Fatalf("status = %q, want completed", result.Status)
	}
	if result.Gate == nil || !result.Gate.Passed {
		t.Fatalf("gate = %+v, want pass", result.Gate)
	}
	if result.Gate.LogPath == "" {
		t.Fatal("passing gate did not record a log path")
	}
	if _, err := os.Stat(filepath.Join(root, result.Gate.LogPath)); err != nil {
		t.Fatalf("gate log missing: %v", err)
	}

	runDir := onlyRunDir(t, root, "trace-gate-pass")
	snap, err := runDir.ReadStatus()
	if err != nil {
		t.Fatal(err)
	}
	if snap.State != protocol.StatusCompleted || snap.Gate == nil || !snap.Gate.Passed {
		t.Fatalf("run record = %s gate=%+v, want completed/pass", snap.State, snap.Gate)
	}

	listed, err := patches.NewStore(root, "trace-gate-pass").List()
	if err != nil {
		t.Fatal(err)
	}
	if len(listed.Records) != 1 {
		t.Fatalf("records = %d, want 1", len(listed.Records))
	}
	if listed.Records[0].Gate == nil || !listed.Records[0].Gate.Passed {
		t.Fatalf("proposal record gate = %+v, want pass", listed.Records[0].Gate)
	}

	// The proposal record keeps its gate verdict when the run directory is gone.
	if err := os.RemoveAll(runDir.Root()); err != nil {
		t.Fatal(err)
	}
	relisted, err := patches.NewStore(root, "trace-gate-pass").List()
	if err != nil {
		t.Fatal(err)
	}
	if len(relisted.Records) != 1 || relisted.Records[0].Gate == nil || !relisted.Records[0].Gate.Passed {
		t.Fatalf("gate verdict did not survive run pruning: %+v", relisted.Records)
	}
	if got := publishedMutations(pub); got != 1 {
		t.Fatalf("published MUTATION = %d, want 1", got)
	}
}

func TestDispatchCheckGateFailureSuppressesProposal(t *testing.T) {
	root := t.TempDir()
	writeProposalColony(t, root, gatedProposalBuilderYAML(checksCommand(`"sh", "-c", "echo boom; exit 3"`)))

	pub := &recordingPublisher{}
	d := runtime.NewDispatcher()
	d.RegisterAdapter("cursor", &mutatingAdapter{run: func(workspace string) error {
		readme := filepath.Join(workspace, "README.md")
		data, err := os.ReadFile(readme)
		if err != nil {
			return err
		}
		return os.WriteFile(readme, append(data, []byte("feature\n")...), 0o644)
	}})
	d.SetPublisher(pub, false)
	reg, err := runtime.BuildBeeRegistry(root)
	if err != nil {
		t.Fatal(err)
	}
	d.SetBeeRegistry(reg)

	result, err := d.Dispatch(context.Background(), runtime.DispatchRequest{
		ColonyRoot: root,
		Bee:        "builder",
		TraceID:    "trace-gate-fail",
		TaskID:     "task-1",
		Task:       "add feature",
	})
	if err != nil {
		t.Fatal(err)
	}
	if result.Status != string(protocol.StatusFailed) {
		t.Fatalf("status = %q, want failed", result.Status)
	}
	if result.Reason != protocol.ReasonChecksFailed {
		t.Fatalf("reason = %q, want %q", result.Reason, protocol.ReasonChecksFailed)
	}
	if result.Gate == nil || result.Gate.Passed {
		t.Fatalf("gate = %+v, want failure", result.Gate)
	}
	if result.Gate.ExitCode != 3 {
		t.Fatalf("exit code = %d, want 3", result.Gate.ExitCode)
	}

	logData, err := os.ReadFile(filepath.Join(root, result.Gate.LogPath))
	if err != nil {
		t.Fatalf("gate log missing: %v", err)
	}
	if !strings.Contains(string(logData), "boom") {
		t.Fatalf("gate log = %q, want captured output", logData)
	}

	snap, err := onlyRunDir(t, root, "trace-gate-fail").ReadStatus()
	if err != nil {
		t.Fatal(err)
	}
	if snap.State != protocol.StatusFailed || snap.Reason != protocol.ReasonChecksFailed {
		t.Fatalf("status record = %s/%s, want failed/%s", snap.State, snap.Reason, protocol.ReasonChecksFailed)
	}
	if snap.Gate == nil || snap.Gate.Passed {
		t.Fatalf("status gate = %+v, want failure", snap.Gate)
	}
	if !strings.Contains(snap.Error, "boom") {
		t.Fatalf("status error = %q, want output head", snap.Error)
	}

	if got := publishedMutations(pub); got != 0 {
		t.Fatalf("failing gate published %d MUTATIONs", got)
	}
	listed, err := patches.NewStore(root, "trace-gate-fail").List()
	if err != nil {
		t.Fatal(err)
	}
	if len(listed.Records) != 0 {
		t.Fatalf("failing gate archived a proposal: %+v", listed.Records)
	}
}

func TestDispatchCheckGateFailsClosed(t *testing.T) {
	tests := []struct {
		name   string
		checks string
		check  func(t *testing.T, result *adapters.RunResult)
	}{
		{
			name: "cannot start",
			checks: `checks:
  command: ["/nonexistent/paseka-check-gate"]
`,
			check: func(t *testing.T, result *adapters.RunResult) {
				if result.Gate.Reason != "exec" {
					t.Fatalf("reason = %q, want exec", result.Gate.Reason)
				}
			},
		},
		{
			name: "timeout",
			checks: `checks:
  command: ["sleep", "5"]
  timeout: 100ms
`,
			check: func(t *testing.T, result *adapters.RunResult) {
				if !result.Gate.TimedOut {
					t.Fatal("timedOut = false, want true")
				}
				if result.Gate.Reason != "timeout" {
					t.Fatalf("reason = %q, want timeout", result.Gate.Reason)
				}
			},
		},
		{
			name: "killed by signal",
			checks: `checks:
  command: ["sh", "-c", "kill -TERM $$"]
`,
			check: func(t *testing.T, result *adapters.RunResult) {
				if result.Gate.Reason != "signal" {
					t.Fatalf("reason = %q, want signal", result.Gate.Reason)
				}
				if result.Gate.Signal == "" {
					t.Fatal("signal name is empty")
				}
			},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			root := t.TempDir()
			writeColony(t, root)
			writeGateFile(t, filepath.Join(root, ".paseka/bees/builder.yaml"), gatedBuilderYAML(tc.checks))

			d := runtime.NewDispatcher()
			d.RegisterAdapter("cursor", &recordingAdapter{})
			result, err := d.Dispatch(context.Background(), runtime.DispatchRequest{
				ColonyRoot: root,
				Bee:        "builder",
				TraceID:    "trace-closed",
				Task:       "work",
			})
			if err != nil {
				t.Fatal(err)
			}
			if result.Status != string(protocol.StatusFailed) {
				t.Fatalf("status = %q, want failed", result.Status)
			}
			if result.Gate == nil || result.Gate.Passed {
				t.Fatalf("gate = %+v, want failure", result.Gate)
			}
			tc.check(t, result)
		})
	}
}

func TestDispatchCheckGateTruncatesOutput(t *testing.T) {
	root := t.TempDir()
	writeColony(t, root)
	writeGateFile(t, filepath.Join(root, ".paseka/bees/builder.yaml"),
		gatedBuilderYAML(checksCommand(`"sh", "-c", "head -c 1200000 /dev/zero | tr '\\0' x; exit 1"`)))

	d := runtime.NewDispatcher()
	d.RegisterAdapter("cursor", &recordingAdapter{})
	result, err := d.Dispatch(context.Background(), runtime.DispatchRequest{
		ColonyRoot: root,
		Bee:        "builder",
		TraceID:    "trace-truncated",
		Task:       "work",
	})
	if err != nil {
		t.Fatal(err)
	}
	if result.Status != string(protocol.StatusFailed) {
		t.Fatalf("status = %q, want failed", result.Status)
	}
	if result.Gate == nil || !result.Gate.Truncated {
		t.Fatalf("gate = %+v, want truncated", result.Gate)
	}
	logData, err := os.ReadFile(filepath.Join(root, result.Gate.LogPath))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(logData), "output truncated") {
		t.Fatal("gate log has no explicit truncation marker")
	}
	if len(logData) > 1<<20+256 {
		t.Fatalf("gate log = %d bytes, exceeds the hard cap", len(logData))
	}
}

func TestDispatchCheckGateSkipsNonCompletedRun(t *testing.T) {
	root := t.TempDir()
	writeColony(t, root)
	marker := filepath.Join(root, "gate-ran")
	writeGateFile(t, filepath.Join(root, ".paseka/bees/builder.yaml"),
		gatedBuilderYAML(checksCommand(`"sh", "-c", "touch `+marker+`"`)))

	d := runtime.NewDispatcher()
	d.RegisterAdapter("cursor", &recordingAdapter{result: &adapters.RunResult{Status: "failed", Err: os.ErrNotExist}})
	result, err := d.Dispatch(context.Background(), runtime.DispatchRequest{
		ColonyRoot: root,
		Bee:        "builder",
		TraceID:    "trace-skip",
		Task:       "work",
	})
	if err != nil {
		t.Fatal(err)
	}
	if result.Status != string(protocol.StatusFailed) {
		t.Fatalf("status = %q, want failed", result.Status)
	}
	if result.Gate != nil {
		t.Fatalf("gate ran on a non-completed run: %+v", result.Gate)
	}
	if _, err := os.Stat(marker); !os.IsNotExist(err) {
		t.Fatal("gate command ran on a non-completed run")
	}
}

func TestDispatchCheckGateTasklessRunFailsQuietly(t *testing.T) {
	root := t.TempDir()
	writeColony(t, root)
	writeGateFile(t, filepath.Join(root, ".paseka/bees/builder.yaml"),
		gatedBuilderYAML(checksCommand(`"sh", "-c", "exit 1"`)))

	pub := &recordingPublisher{}
	d := runtime.NewDispatcher()
	d.RegisterAdapter("cursor", &recordingAdapter{})
	d.SetPublisher(pub, false)

	result, err := d.Dispatch(context.Background(), runtime.DispatchRequest{
		ColonyRoot: root,
		Bee:        "builder",
		TraceID:    "trace-taskless",
		Task:       "work",
	})
	if err != nil {
		t.Fatal(err)
	}
	if result.Status != string(protocol.StatusFailed) {
		t.Fatalf("status = %q, want failed", result.Status)
	}
	if result.GateReworkSuppressed {
		t.Fatal("taskless run reported a suppressed rework")
	}
	if got := publishedVerification(pub, protocol.VerificationFailed); len(got) != 0 {
		t.Fatalf("taskless run published %d rework signals", len(got))
	}
}

func TestDispatchCheckGateRejectionIsPlatformAuthored(t *testing.T) {
	root := t.TempDir()
	writeColony(t, root)
	writeGateFile(t, filepath.Join(root, ".paseka/bees/builder.yaml"),
		gatedBuilderYAML(checksCommand(`"sh", "-c", "echo failing; exit 2"`)))

	pub := &recordingPublisher{}
	d := runtime.NewDispatcher()
	d.RegisterAdapter("cursor", &recordingAdapter{})
	d.SetPublisher(pub, false)

	result, err := d.Dispatch(context.Background(), runtime.DispatchRequest{
		ColonyRoot: root,
		Bee:        "builder",
		TraceID:    "trace-reject",
		TaskID:     "task-1",
		Task:       "work",
	})
	if err != nil {
		t.Fatal(err)
	}
	rejections := publishedVerification(pub, protocol.VerificationFailed)
	if len(rejections) != 1 {
		t.Fatalf("rejections = %d, want 1", len(rejections))
	}
	ev := rejections[0]
	if ev.AgentID != protocol.RuntimeAgentID {
		t.Fatalf("rejection agent = %q, want %q (role suppression would swallow rework)", ev.AgentID, protocol.RuntimeAgentID)
	}
	var payload protocol.VerificationPayload
	if err := json.Unmarshal(ev.Payload, &payload); err != nil {
		t.Fatal(err)
	}
	if payload.TaskID != "task-1" || payload.Reason != protocol.ReasonChecksFailed {
		t.Fatalf("payload = %+v", payload)
	}
	if strings.Contains(payload.Summary, "\n") {
		t.Fatalf("summary spans lines: %q", payload.Summary)
	}
	if !strings.Contains(payload.Summary, "checks failed (exit 2)") {
		t.Fatalf("summary = %q", payload.Summary)
	}
	if strings.Contains(payload.Summary, "boom") {
		t.Fatalf("summary carries captured output: %q", payload.Summary)
	}
	if payload.Details != result.Gate.LogPath {
		t.Fatalf("details = %q, want %q", payload.Details, result.Gate.LogPath)
	}
	if _, err := os.Stat(filepath.Join(root, payload.Details)); err != nil {
		t.Fatalf("referenced gate log missing: %v", err)
	}

	// The rejection is part of the run's event log, not just the bus.
	stored, err := onlyRunDir(t, root, "trace-reject").ReadEvents()
	if err != nil {
		t.Fatal(err)
	}
	found := false
	for _, recorded := range stored {
		if recorded.ID == ev.ID {
			found = true
		}
	}
	if !found {
		t.Fatal("rejection not recorded in the run event log")
	}
}

func TestDispatchCheckGateDeferredProposalDoesNotLeaveRun(t *testing.T) {
	root := t.TempDir()
	writeProposalColony(t, root, gatedProposalBuilderYAML(checksCommand(`"sh", "-c", "exit 1"`)))

	pub := &recordingPublisher{}
	d := runtime.NewDispatcher()
	d.RegisterAdapter("cursor", queuedProposalAdapter{})
	d.SetPublisher(pub, false)
	reg, err := runtime.BuildBeeRegistry(root)
	if err != nil {
		t.Fatal(err)
	}
	d.SetBeeRegistry(reg)

	result, err := d.Dispatch(context.Background(), runtime.DispatchRequest{
		ColonyRoot: root,
		Bee:        "builder",
		TraceID:    "trace-deferred",
		TaskID:     "task-1",
		Task:       "work",
	})
	if err != nil {
		t.Fatal(err)
	}
	if result.Status != string(protocol.StatusFailed) {
		t.Fatalf("status = %q, want failed", result.Status)
	}
	for _, ev := range pub.events {
		if ev.Type == protocol.EventMutation {
			t.Fatalf("deferred proposal reached the bus: %+v", ev)
		}
		if ev.Type == protocol.EventInsight && protocol.PayloadKind(ev.Payload) == string(protocol.InsightRunSummary) {
			t.Fatalf("failing gated run synthesized a completed summary: %+v", ev)
		}
	}
	pending, err := onlyRunDir(t, root, "trace-deferred").ReadPending()
	if err != nil {
		t.Fatal(err)
	}
	if len(pending) == 0 {
		t.Fatal("deferred proposal was flushed out of the run")
	}
}

func TestDispatchCheckGateRootProposalCheckedInColonyCheckout(t *testing.T) {
	root := t.TempDir()
	writeProposalColony(t, root, `role: builder
adapter: cursor
worktree: false
prompt_template: builder.md
`+gateParams+`publishes:
  - type: MUTATION
    kind: code.proposal.root
checks:
  command: ["sh", "-c", "test -f README.md && exit 1"]
`)

	pub := &recordingPublisher{}
	d := runtime.NewDispatcher()
	d.RegisterAdapter("cursor", &mutatingAdapter{run: func(workspace string) error {
		readme := filepath.Join(workspace, "README.md")
		data, err := os.ReadFile(readme)
		if err != nil {
			return err
		}
		return os.WriteFile(readme, append(data, []byte("root edit\n")...), 0o644)
	}})
	d.SetPublisher(pub, false)
	reg, err := runtime.BuildBeeRegistry(root)
	if err != nil {
		t.Fatal(err)
	}
	d.SetBeeRegistry(reg)

	result, err := d.Dispatch(context.Background(), runtime.DispatchRequest{
		ColonyRoot: root,
		Bee:        "builder",
		TraceID:    "trace-root-gate",
		TaskID:     "task-1",
		Task:       "work",
	})
	if err != nil {
		t.Fatal(err)
	}
	if result.Status != string(protocol.StatusFailed) || result.Gate == nil || result.Gate.Passed {
		t.Fatalf("result = %s gate=%+v, want failed gate", result.Status, result.Gate)
	}
	if got := publishedMutations(pub); got != 0 {
		t.Fatalf("root proposal published despite gate failure: %d", got)
	}
}

func TestDispatchCheckGateRunsInWorkspaceWithPlanEnv(t *testing.T) {
	root := t.TempDir()
	writeColony(t, root)
	writeGateFile(t, filepath.Join(root, ".paseka/bees/builder.yaml"), `role: builder
adapter: cursor
prompt_template: builder.md
params:
  model: composer-2.5
  yolo: true
  env:
    PLAN_TOKEN: declared-token
checks:
  command: ["sh", "-c", "printenv PLAN_TOKEN > gate-env.txt && printenv PASEKA_TRACE_ID >> gate-env.txt && printenv PASEKA_AGENT_ID >> gate-env.txt && pwd >> gate-env.txt"]
`)

	d := runtime.NewDispatcher()
	d.RegisterAdapter("cursor", &recordingAdapter{})
	result, err := d.Dispatch(context.Background(), runtime.DispatchRequest{
		ColonyRoot: root,
		Bee:        "builder",
		TraceID:    "trace-gate-env",
		Task:       "work",
	})
	if err != nil {
		t.Fatal(err)
	}
	if result.Gate == nil || !result.Gate.Passed {
		t.Fatalf("gate = %+v, want pass", result.Gate)
	}
	data, err := os.ReadFile(filepath.Join(root, "gate-env.txt"))
	if err != nil {
		t.Fatalf("gate env output missing: %v", err)
	}
	lines := strings.Split(strings.TrimSpace(string(data)), "\n")
	if len(lines) != 4 {
		t.Fatalf("gate env lines = %v", lines)
	}
	if lines[0] != "declared-token" {
		t.Fatalf("PLAN_TOKEN = %q", lines[0])
	}
	if lines[1] != "trace-gate-env" {
		t.Fatalf("PASEKA_TRACE_ID = %q", lines[1])
	}
	if lines[2] == "" {
		t.Fatal("PASEKA_AGENT_ID is empty")
	}
	wantDir, err := filepath.EvalSymlinks(root)
	if err != nil {
		wantDir = root
	}
	gotDir, err := filepath.EvalSymlinks(strings.TrimSpace(lines[3]))
	if err != nil {
		gotDir = strings.TrimSpace(lines[3])
	}
	if gotDir != wantDir {
		t.Fatalf("gate cwd = %q, want the dispatch workspace %q", gotDir, wantDir)
	}
}

func TestDispatchUngatedBeeRecordsNoGate(t *testing.T) {
	root := t.TempDir()
	writeProposalColony(t, root, gatedProposalBuilderYAML(""))

	d := runtime.NewDispatcher()
	d.RegisterAdapter("cursor", &mutatingAdapter{run: func(workspace string) error {
		readme := filepath.Join(workspace, "README.md")
		data, err := os.ReadFile(readme)
		if err != nil {
			return err
		}
		return os.WriteFile(readme, append(data, []byte("feature\n")...), 0o644)
	}})
	reg, err := runtime.BuildBeeRegistry(root)
	if err != nil {
		t.Fatal(err)
	}
	d.SetBeeRegistry(reg)

	result, err := d.Dispatch(context.Background(), runtime.DispatchRequest{
		ColonyRoot: root,
		Bee:        "builder",
		TraceID:    "trace-ungated",
		TaskID:     "task-1",
		Task:       "add feature",
	})
	if err != nil {
		t.Fatal(err)
	}
	if result.Status != string(protocol.StatusCompleted) || result.Gate != nil {
		t.Fatalf("result = %s gate=%+v, want completed with no gate", result.Status, result.Gate)
	}

	snap, err := onlyRunDir(t, root, "trace-ungated").ReadStatus()
	if err != nil {
		t.Fatal(err)
	}
	if snap.Gate != nil {
		t.Fatalf("un-gated run recorded a gate: %+v", snap.Gate)
	}
	listed, err := patches.NewStore(root, "trace-ungated").List()
	if err != nil {
		t.Fatal(err)
	}
	if len(listed.Records) != 1 || listed.Records[0].Gate != nil {
		t.Fatalf("un-gated proposal record = %+v, want no gate", listed.Records)
	}
}

func TestDispatchCheckGateRejectionCapAndReset(t *testing.T) {
	root := t.TempDir()
	failing := gatedBuilderYAML("checks:\n  command: [\"sh\", \"-c\", \"exit 1\"]\n  rejection_cap: 2\n")
	passing := gatedBuilderYAML("checks:\n  command: [\"sh\", \"-c\", \"exit 0\"]\n  rejection_cap: 2\n")
	writeColony(t, root)
	writeGateFile(t, filepath.Join(root, ".paseka/bees/builder.yaml"), failing)

	pub := &recordingPublisher{}
	dispatch := func(d *runtime.Dispatcher) *adapters.RunResult {
		t.Helper()
		result, err := d.Dispatch(context.Background(), runtime.DispatchRequest{
			ColonyRoot: root,
			Bee:        "builder",
			TraceID:    "trace-cap",
			TaskID:     "task-1",
			AttemptID:  "attempt-1",
			Task:       "work",
		})
		if err != nil {
			t.Fatal(err)
		}
		return result
	}
	newDispatcher := func() *runtime.Dispatcher {
		d := runtime.NewDispatcher()
		d.RegisterAdapter("cursor", &recordingAdapter{})
		d.SetPublisher(pub, false)
		return d
	}

	d := newDispatcher()
	for i := 1; i <= 2; i++ {
		result := dispatch(d)
		if result.GateReworkSuppressed {
			t.Fatalf("rejection %d was suppressed before the cap", i)
		}
	}
	if got := len(publishedVerification(pub, protocol.VerificationFailed)); got != 2 {
		t.Fatalf("rejections = %d, want 2 before the cap", got)
	}

	// A fresh dispatcher reads the same recorded verdicts: the count survives a
	// runtime restart.
	result := dispatch(newDispatcher())
	if !result.GateReworkSuppressed {
		t.Fatal("third consecutive rejection was not suppressed")
	}
	if result.Gate == nil || !strings.Contains(result.Gate.Summary, "checks failed") {
		t.Fatalf("gate summary = %+v", result.Gate)
	}
	if got := len(publishedVerification(pub, protocol.VerificationFailed)); got != 2 {
		t.Fatalf("capped run signalled rework: %d rejections", got)
	}

	// A pass clears the count.
	writeGateFile(t, filepath.Join(root, ".paseka/bees/builder.yaml"), passing)
	passResult := dispatch(newDispatcher())
	if passResult.Gate == nil || !passResult.Gate.Passed {
		t.Fatalf("pass result gate = %+v", passResult.Gate)
	}

	writeGateFile(t, filepath.Join(root, ".paseka/bees/builder.yaml"), failing)
	afterReset := dispatch(newDispatcher())
	if afterReset.GateReworkSuppressed {
		t.Fatal("rejection after a passing gate was suppressed")
	}
	if got := len(publishedVerification(pub, protocol.VerificationFailed)); got != 3 {
		t.Fatalf("rejections after reset = %d, want 3", got)
	}
}

func TestReactorGateRejectionRedispatchesBuilderOnce(t *testing.T) {
	root := t.TempDir()
	bees := map[string]colony.Bee{
		"builder": {
			Role: "builder",
			Subscribes: []colony.SubscriptionRule{
				{EventRule: colony.EventRule{Type: "SIGNAL", Kind: "task.ready"}, Dispatch: colony.DispatchTask},
				{EventRule: colony.EventRule{Type: "VERIFICATION", Kind: "verification.failed"}, Dispatch: colony.DispatchDirect},
			},
		},
	}
	mustWriteTestColony(t, root, bees)
	writeGateFile(t, filepath.Join(root, ".paseka/prompts/builder.md"), "Work on {{.Task}}")
	writeGateFile(t, filepath.Join(root, ".paseka/bees/builder.yaml"),
		gatedBuilderYAML(checksCommand(`"sh", "-c", "echo reject; exit 1"`))+`subscribes:
  - type: SIGNAL
    kind: task.ready
    dispatch: task
  - type: VERIFICATION
    kind: verification.failed
    dispatch: direct
`)

	pub := &recordingPublisher{}
	rec := &recordingAdapter{}
	d := runtime.NewDispatcher()
	d.RegisterAdapter("cursor", rec)
	d.SetPublisher(pub, false)
	reg := runtime.NewBeeRegistryFromBees(bees)
	d.SetBeeRegistry(reg)
	r := runtime.NewTestReactor(runtime.TestReactorOptions{
		ColonyRoot: root,
		Dispatcher: d,
		Registry:   reg,
		Ledger:     taskledger.NewMemoryLedger(),
		Publisher:  pub,
	})

	plan, err := protocol.NewEvent("trace-rework-gate", "scout", 1, protocol.EventInsight, protocol.TaskPlanPayload{
		Kind:  protocol.TaskEventPlan,
		Tasks: []protocol.TaskSpec{{TaskID: "task-1", Title: "work", Bee: "builder"}},
	})
	if err != nil {
		t.Fatal(err)
	}
	ready, err := protocol.NewEvent("trace-rework-gate", "scout", 2, protocol.EventSignal, protocol.TaskReadyPayload{
		Kind:   protocol.TaskEventReady,
		TaskID: "task-1",
		Bee:    "builder",
	})
	if err != nil {
		t.Fatal(err)
	}
	if err := r.ProcessEvent(context.Background(), plan); err != nil {
		t.Fatal(err)
	}
	if err := r.ProcessEvent(context.Background(), ready); err != nil {
		t.Fatal(err)
	}
	if rec.calls != 1 {
		t.Fatalf("adapter calls = %d, want 1", rec.calls)
	}
	first := rec.requests[0]
	snap, err := r.Ledger().Snapshot("trace-rework-gate")
	if err != nil {
		t.Fatal(err)
	}
	if got := snap.Tasks["task-1"].Status; got != protocol.TaskStatusRunning {
		t.Fatalf("task status = %q, want running (rejection is not a task failure)", got)
	}

	rejections := publishedVerification(pub, protocol.VerificationFailed)
	if len(rejections) != 1 {
		t.Fatalf("rejections = %d, want 1", len(rejections))
	}
	if err := r.ProcessEvent(context.Background(), rejections[0]); err != nil {
		t.Fatal(err)
	}
	if rec.calls != 2 {
		t.Fatalf("adapter calls = %d, want 2 (one rework dispatch)", rec.calls)
	}
	rework := rec.requests[1]
	if rework.AttemptID != first.AttemptID {
		t.Fatalf("rework attempt = %q, want %q", rework.AttemptID, first.AttemptID)
	}
	if rework.Workspace != first.Workspace {
		t.Fatalf("rework workspace = %q, want %q", rework.Workspace, first.Workspace)
	}
	if !strings.Contains(rework.Task, "Full details:") {
		t.Fatalf("rework task does not name the gate log: %q", rework.Task)
	}
	snap, err = r.Ledger().Snapshot("trace-rework-gate")
	if err != nil {
		t.Fatal(err)
	}
	if got := snap.Tasks["task-1"].Status; got != protocol.TaskStatusRunning {
		t.Fatalf("task status after rework dispatch = %q, want running", got)
	}

	// A redelivered rejection dispatches nothing further.
	if err := r.ProcessEvent(context.Background(), rejections[0]); err != nil {
		t.Fatal(err)
	}
	if rec.calls != 2 {
		t.Fatalf("adapter calls after redelivery = %d, want 2", rec.calls)
	}
}

func TestReactorGateCapFailsTaskAndStopsRework(t *testing.T) {
	root := t.TempDir()
	bees := map[string]colony.Bee{
		"builder": {
			Role: "builder",
			Subscribes: []colony.SubscriptionRule{
				{EventRule: colony.EventRule{Type: "SIGNAL", Kind: "task.ready"}, Dispatch: colony.DispatchTask},
				{EventRule: colony.EventRule{Type: "VERIFICATION", Kind: "verification.failed"}, Dispatch: colony.DispatchDirect},
			},
		},
	}
	mustWriteTestColony(t, root, bees)
	writeGateFile(t, filepath.Join(root, ".paseka/prompts/builder.md"), "Work on {{.Task}}")
	writeGateFile(t, filepath.Join(root, ".paseka/bees/builder.yaml"),
		gatedBuilderYAML("checks:\n  command: [\"sh\", \"-c\", \"exit 1\"]\n  rejection_cap: 1\n")+`subscribes:
  - type: SIGNAL
    kind: task.ready
    dispatch: task
  - type: VERIFICATION
    kind: verification.failed
    dispatch: direct
`)

	pub := &recordingPublisher{}
	rec := &recordingAdapter{}
	d := runtime.NewDispatcher()
	d.RegisterAdapter("cursor", rec)
	d.SetPublisher(pub, false)
	reg := runtime.NewBeeRegistryFromBees(bees)
	d.SetBeeRegistry(reg)
	r := runtime.NewTestReactor(runtime.TestReactorOptions{
		ColonyRoot: root,
		Dispatcher: d,
		Registry:   reg,
		Ledger:     taskledger.NewMemoryLedger(),
		Publisher:  pub,
	})

	plan, err := protocol.NewEvent("trace-cap-rework", "scout", 1, protocol.EventInsight, protocol.TaskPlanPayload{
		Kind:  protocol.TaskEventPlan,
		Tasks: []protocol.TaskSpec{{TaskID: "task-1", Title: "work", Bee: "builder"}},
	})
	if err != nil {
		t.Fatal(err)
	}
	ready, err := protocol.NewEvent("trace-cap-rework", "scout", 2, protocol.EventSignal, protocol.TaskReadyPayload{
		Kind:   protocol.TaskEventReady,
		TaskID: "task-1",
		Bee:    "builder",
	})
	if err != nil {
		t.Fatal(err)
	}
	if err := r.ProcessEvent(context.Background(), plan); err != nil {
		t.Fatal(err)
	}
	if err := r.ProcessEvent(context.Background(), ready); err != nil {
		t.Fatal(err)
	}
	rejections := publishedVerification(pub, protocol.VerificationFailed)
	if len(rejections) != 1 {
		t.Fatalf("rejections = %d, want 1", len(rejections))
	}
	if err := r.ProcessEvent(context.Background(), rejections[0]); err != nil {
		t.Fatal(err)
	}
	if rec.calls != 2 {
		t.Fatalf("adapter calls = %d, want 2 (initial + one rework)", rec.calls)
	}
	if got := len(publishedVerification(pub, protocol.VerificationFailed)); got != 1 {
		t.Fatalf("capped rework signalled again: %d rejections", got)
	}
	snap, err := r.Ledger().Snapshot("trace-cap-rework")
	if err != nil {
		t.Fatal(err)
	}
	if got := snap.Tasks["task-1"].Status; got != protocol.TaskStatusFailed {
		t.Fatalf("task status = %q, want failed after the cap", got)
	}
	if !strings.Contains(snap.Tasks["task-1"].Summary, "checks failed") {
		t.Fatalf("task summary = %q, want the gate summary", snap.Tasks["task-1"].Summary)
	}
}
