package runtime_test

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/russ-p/paseka/internal/adapters"
	"github.com/russ-p/paseka/internal/bus"
	"github.com/russ-p/paseka/internal/colony"
	"github.com/russ-p/paseka/internal/patches"
	"github.com/russ-p/paseka/internal/protocol"
	"github.com/russ-p/paseka/internal/runs"
	"github.com/russ-p/paseka/internal/runtime"
)

func TestPlanAutoProposalRouting(t *testing.T) {
	tests := []struct {
		name      string
		bee       colony.Bee
		wantOK    bool
		wantKind  protocol.MutationKind
		wantSpace protocol.ProposalWorkspace
	}{
		{
			name: "worktree isolated",
			bee: colony.Bee{
				Role:     "builder",
				Worktree: true,
				Publishes: []colony.PublicationRule{
					{EventRule: colony.EventRule{Type: "MUTATION", Kind: "code.proposal.isolated"}},
				},
			},
			wantOK:    true,
			wantKind:  protocol.MutationCodeProposalIsolated,
			wantSpace: protocol.ProposalWorkspaceIsolated,
		},
		{
			name: "worktree alias",
			bee: colony.Bee{
				Role:     "builder",
				Worktree: true,
				Publishes: []colony.PublicationRule{
					{EventRule: colony.EventRule{Type: "MUTATION", Kind: "code.proposal"}},
				},
			},
			wantOK:    true,
			wantKind:  protocol.MutationCodeProposalIsolated,
			wantSpace: protocol.ProposalWorkspaceIsolated,
		},
		{
			name: "root",
			bee: colony.Bee{
				Role: "hivewright",
				Publishes: []colony.PublicationRule{
					{EventRule: colony.EventRule{Type: "MUTATION", Kind: "code.proposal.root"}},
				},
			},
			wantOK:    true,
			wantKind:  protocol.MutationCodeProposalRoot,
			wantSpace: protocol.ProposalWorkspaceRoot,
		},
		{
			name: "worktree root mismatch",
			bee: colony.Bee{
				Role:     "builder",
				Worktree: true,
				Publishes: []colony.PublicationRule{
					{EventRule: colony.EventRule{Type: "MUTATION", Kind: "code.proposal.root"}},
				},
			},
			wantOK: true,
		},
		{
			name: "root isolated mismatch",
			bee: colony.Bee{
				Role: "hivewright",
				Publishes: []colony.PublicationRule{
					{EventRule: colony.EventRule{Type: "MUTATION", Kind: "code.proposal"}},
				},
			},
			wantOK: true,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			reg := runtime.NewBeeRegistryFromBees(map[string]colony.Bee{tc.bee.Role: tc.bee})
			got := reg.ShouldAutoPublishMutation(tc.bee.Role)
			if got != tc.wantOK {
				t.Fatalf("ShouldAutoPublishMutation = %v, want %v", got, tc.wantOK)
			}
		})
	}
}

type mutatingAdapter struct {
	run func(workspace string) error
}

func (m *mutatingAdapter) Name() string { return "cursor" }

func (m *mutatingAdapter) Run(_ context.Context, req adapters.RunRequest) (*adapters.RunResult, error) {
	if m.run != nil {
		if err := m.run(req.Workspace); err != nil {
			return nil, err
		}
	}
	return &adapters.RunResult{Status: "completed", Summary: "done"}, nil
}

func writeProposalColony(t *testing.T, root string, builderYAML string) {
	t.Helper()
	writeColony(t, root)
	if builderYAML != "" {
		if err := os.WriteFile(filepath.Join(root, ".paseka/bees/builder.yaml"), []byte(builderYAML), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	initGitInColony(t, root)
}

func initGitInColony(t *testing.T, root string) {
	t.Helper()
	runGit(t, root, "init")
	runGit(t, root, "config", "user.email", "bee@test.local")
	runGit(t, root, "config", "user.name", "Bee")
	if err := os.WriteFile(filepath.Join(root, "README.md"), []byte("base\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	runGit(t, root, "add", "README.md")
	runGit(t, root, "commit", "-m", "init")
}

func TestDispatchAutoPublishesIsolatedProposal(t *testing.T) {
	root := t.TempDir()
	writeProposalColony(t, root, `role: builder
adapter: cursor
worktree: true
prompt_template: builder.md
params:
  model: composer-2.5
  yolo: true
publishes:
  - type: MUTATION
    kind: code.proposal.isolated
`)

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

	_, err = d.Dispatch(context.Background(), runtime.DispatchRequest{
		ColonyRoot: root,
		Bee:        "builder",
		TraceID:    "trace-abc",
		TaskID:     "task-1",
		Task:       "add feature",
	})
	if err != nil {
		t.Fatal(err)
	}

	ev := findMutation(t, pub.events)
	if protocol.PayloadKind(ev.Payload) != string(protocol.MutationCodeProposalIsolated) {
		t.Fatalf("kind = %q, want code.proposal.isolated", protocol.PayloadKind(ev.Payload))
	}
	var payload protocol.MutationPayload
	if err := json.Unmarshal(ev.Payload, &payload); err != nil {
		t.Fatal(err)
	}
	if payload.Workspace != protocol.ProposalWorkspaceIsolated {
		t.Fatalf("workspace = %q", payload.Workspace)
	}
	if payload.BaseSha == "" {
		t.Fatal("expected baseSha provenance")
	}
	if payload.WorktreePath != ".paseka/worktrees/trace-abc" {
		t.Fatalf("worktreePath = %q", payload.WorktreePath)
	}
	if payload.Diff != "" {
		t.Fatal("synthesized proposal must not carry an inline diff")
	}
	// The ref resolves against the colony root, not the caller's cwd or the worktree.
	body := readArchivedPatch(t, root, payload.Ref)
	if !strings.Contains(body, "README.md") {
		t.Fatalf("archived patch missing the change: %q", body)
	}
	rec := readPatchRecord(t, root, payload.Ref)
	if rec.Publication != patches.PublicationPublished {
		t.Fatalf("publication = %q, want published", rec.Publication)
	}
	if rec.Bytes != int64(len(body)) {
		t.Fatalf("recorded bytes = %d, want %d", rec.Bytes, len(body))
	}
	if rec.Mode != patches.ModeAFK {
		t.Fatalf("mode = %q, want afk", rec.Mode)
	}
	if rec.Kind != string(protocol.MutationCodeProposalIsolated) {
		t.Fatalf("record kind = %q", rec.Kind)
	}
}

func TestDispatchAutoPublishesRootProposal(t *testing.T) {
	root := t.TempDir()
	writeProposalColony(t, root, `role: builder
adapter: cursor
worktree: false
prompt_template: builder.md
params:
  model: composer-2.5
  yolo: true
publishes:
  - type: MUTATION
    kind: code.proposal.root
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

	_, err = d.Dispatch(context.Background(), runtime.DispatchRequest{
		ColonyRoot: root,
		Bee:        "builder",
		TraceID:    "trace-root",
		Task:       "tune config",
	})
	if err != nil {
		t.Fatal(err)
	}

	ev := findMutation(t, pub.events)
	if protocol.PayloadKind(ev.Payload) != string(protocol.MutationCodeProposalRoot) {
		t.Fatalf("kind = %q, want code.proposal.root", protocol.PayloadKind(ev.Payload))
	}
	var payload protocol.MutationPayload
	if err := json.Unmarshal(ev.Payload, &payload); err != nil {
		t.Fatal(err)
	}
	if payload.Workspace != protocol.ProposalWorkspaceRoot {
		t.Fatalf("workspace = %q", payload.Workspace)
	}
	if payload.WorktreePath != "" {
		t.Fatalf("worktreePath = %q, want empty", payload.WorktreePath)
	}
}

func TestDispatchSkipsAutoMutationOnWorktreeKindMismatch(t *testing.T) {
	root := t.TempDir()
	writeProposalColony(t, root, `role: builder
adapter: cursor
worktree: false
prompt_template: builder.md
params:
  model: composer-2.5
  yolo: true
publishes:
  - type: MUTATION
    kind: code.proposal.isolated
`)

	_, err := runtime.BuildBeeRegistry(root)
	if err == nil || !strings.Contains(err.Error(), "isolated code.proposal with worktree: false") {
		t.Fatalf("BuildBeeRegistry() err = %v, want load-time mismatch error", err)
	}
}

func TestDispatchBaselineExcludesPreExistingDirty(t *testing.T) {
	root := t.TempDir()
	writeProposalColony(t, root, `role: builder
adapter: cursor
worktree: false
prompt_template: builder.md
params:
  model: composer-2.5
  yolo: true
publishes:
  - type: MUTATION
    kind: code.proposal.root
`)

	if err := os.WriteFile(filepath.Join(root, "README.md"), []byte("base\npre-existing\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	if err := os.WriteFile(filepath.Join(root, "tracked.txt"), []byte("tracked\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	runGit(t, root, "add", "tracked.txt")
	runGit(t, root, "commit", "-m", "add tracked")

	pub := &recordingPublisher{}
	d := runtime.NewDispatcher()
	d.RegisterAdapter("cursor", &mutatingAdapter{run: func(workspace string) error {
		return os.WriteFile(filepath.Join(workspace, "tracked.txt"), []byte("tracked\nrun-only\n"), 0o644)
	}})
	d.SetPublisher(pub, false)
	reg, err := runtime.BuildBeeRegistry(root)
	if err != nil {
		t.Fatal(err)
	}
	d.SetBeeRegistry(reg)

	_, err = d.Dispatch(context.Background(), runtime.DispatchRequest{
		ColonyRoot: root,
		Bee:        "builder",
		TraceID:    "trace-base",
		Task:       "work",
	})
	if err != nil {
		t.Fatal(err)
	}

	ev := findMutation(t, pub.events)
	var payload protocol.MutationPayload
	if err := json.Unmarshal(ev.Payload, &payload); err != nil {
		t.Fatal(err)
	}
	body := readArchivedPatch(t, root, payload.Ref)
	if strings.Contains(body, "pre-existing") {
		t.Fatalf("proposal must not attribute pre-existing README dirt: %q", body)
	}
	if !strings.Contains(body, "tracked.txt") {
		t.Fatalf("proposal missing tracked.txt: %q", body)
	}
}

func TestDispatchSkipsAutoMutationWithEmptyPublishes(t *testing.T) {
	root := t.TempDir()
	writeProposalColony(t, root, `role: builder
adapter: cursor
prompt_template: builder.md
params:
  model: composer-2.5
  yolo: true
`)

	pub := &recordingPublisher{}
	d := runtime.NewDispatcher()
	d.RegisterAdapter("cursor", &mutatingAdapter{run: func(workspace string) error {
		readme := filepath.Join(workspace, "README.md")
		data, err := os.ReadFile(readme)
		if err != nil {
			return err
		}
		return os.WriteFile(readme, append(data, []byte("x\n")...), 0o644)
	}})
	d.SetPublisher(pub, false)
	reg, err := runtime.BuildBeeRegistry(root)
	if err != nil {
		t.Fatal(err)
	}
	d.SetBeeRegistry(reg)

	_, err = d.Dispatch(context.Background(), runtime.DispatchRequest{
		ColonyRoot: root,
		Bee:        "builder",
		TraceID:    "trace-empty",
		Task:       "work",
	})
	if err != nil {
		t.Fatal(err)
	}
	for _, ev := range pub.events {
		if ev.Type == protocol.EventMutation {
			t.Fatalf("empty publishes must not auto-publish mutation, got %+v", pub.events)
		}
	}
}

func findMutation(t *testing.T, events []protocol.Event) protocol.Event {
	t.Helper()
	for _, ev := range events {
		if ev.Type == protocol.EventMutation {
			return ev
		}
	}
	t.Fatalf("no MUTATION in %+v", events)
	return protocol.Event{}
}

// readArchivedPatch reads an archive ref the way a history surface does: resolved against
// the colony root, independent of the caller's working directory.
func readArchivedPatch(t *testing.T, colonyRoot, ref string) string {
	t.Helper()
	abs, err := patches.ResolveRef(colonyRoot, ref)
	if err != nil {
		t.Fatalf("resolve ref %q: %v", ref, err)
	}
	data, err := os.ReadFile(abs)
	if err != nil {
		t.Fatalf("read %s: %v", abs, err)
	}
	return string(data)
}

// readPatchRecord reads the sidecar behind an archive ref.
func readPatchRecord(t *testing.T, colonyRoot, ref string) patches.Record {
	t.Helper()
	parsed, ok := patches.ParseArchiveRef(ref)
	if !ok {
		t.Fatalf("ref %q is not an archive ref", ref)
	}
	store := patches.NewStore(colonyRoot, parsed.TraceID)
	rec, ok, err := store.Read(parsed.AgentID)
	if err != nil || !ok {
		t.Fatalf("read record for %q: ok=%v err=%v", parsed.AgentID, ok, err)
	}
	return rec
}

// seamPublisher is the shape a colony's publisher has in production: record the
// event locally, then forward it to the transport.
type seamPublisher struct {
	local   bus.Publisher
	forward *recordingPublisher
}

func (p *seamPublisher) PublishEvent(ctx context.Context, ev protocol.Event) error {
	if err := p.local.PublishEvent(ctx, ev); err != nil {
		return err
	}
	return p.forward.PublishEvent(ctx, ev)
}

func TestSynthesizedProposalLandsInTrailRecordWithoutBus(t *testing.T) {
	root := t.TempDir()
	writeProposalColony(t, root, `role: builder
adapter: cursor
worktree: true
prompt_template: builder.md
params:
  model: composer-2.5
  yolo: true
publishes:
  - type: MUTATION
    kind: code.proposal.isolated
`)

	traceID := "trace-trail-record"
	pub := &recordingPublisher{}
	local := bus.LocalPublisher{ColonyRoot: root}
	// The seam records and forwards, exactly as a bus-backed publisher does.
	d := runtime.NewDispatcher()
	d.RegisterAdapter("cursor", &mutatingAdapter{run: func(workspace string) error {
		readme := filepath.Join(workspace, "README.md")
		data, err := os.ReadFile(readme)
		if err != nil {
			return err
		}
		return os.WriteFile(readme, append(data, []byte("feature\n")...), 0o644)
	}})
	d.SetPublisher(&seamPublisher{local: local, forward: pub}, false)
	reg, err := runtime.BuildBeeRegistry(root)
	if err != nil {
		t.Fatal(err)
	}
	d.SetBeeRegistry(reg)

	if _, err := d.Dispatch(context.Background(), runtime.DispatchRequest{
		ColonyRoot: root,
		Bee:        "builder",
		TraceID:    traceID,
		TaskID:     "task-1",
		Task:       "add feature",
	}); err != nil {
		t.Fatal(err)
	}

	published := findMutation(t, pub.events)
	recorded, err := runs.ReadTrailEvents(root, traceID)
	if err != nil {
		t.Fatal(err)
	}
	if len(recorded) != 1 {
		t.Fatalf("trail record holds %d events, want the synthesized proposal", len(recorded))
	}
	if recorded[0].ID != published.ID {
		t.Fatalf("recorded identity = %q, want the published event's %q", recorded[0].ID, published.ID)
	}

	// The same event is also the trail's history, once, even though the run's
	// own audit log exists beside it.
	merged, err := runs.ReadTraceEvents(root, traceID)
	if err != nil {
		t.Fatal(err)
	}
	var mutations int
	for _, ev := range merged {
		if ev.Type == protocol.EventMutation {
			mutations++
		}
	}
	if mutations != 1 {
		t.Fatalf("merged trail history holds %d proposals, want 1", mutations)
	}
}

// statusAdapter reports whatever status the test wants without a Go error, so the gate is
// exercised on the status the run actually finished with.
type statusAdapter struct {
	status  string
	summary string
	events  []protocol.Event
}

func (s *statusAdapter) Name() string { return "cursor" }

func (s *statusAdapter) Run(_ context.Context, _ adapters.RunRequest) (*adapters.RunResult, error) {
	return &adapters.RunResult{Status: s.status, Summary: s.summary, Events: s.events}, nil
}

func proposalBuilderYAML() string {
	return `role: builder
adapter: cursor
worktree: false
prompt_template: builder.md
params:
  model: composer-2.5
  yolo: true
publishes:
  - type: MUTATION
    kind: code.proposal.root
`
}

func dispatchProposalRun(t *testing.T, adapter adapters.Adapter, traceID string) (*recordingPublisher, string) {
	t.Helper()
	root := t.TempDir()
	writeProposalColony(t, root, proposalBuilderYAML())
	pub := &recordingPublisher{}
	d := runtime.NewDispatcher()
	d.RegisterAdapter("cursor", adapter)
	d.SetPublisher(pub, false)
	reg, err := runtime.BuildBeeRegistry(root)
	if err != nil {
		t.Fatal(err)
	}
	d.SetBeeRegistry(reg)
	if _, err := d.Dispatch(context.Background(), runtime.DispatchRequest{
		ColonyRoot: root,
		Bee:        "builder",
		TraceID:    traceID,
		Task:       "work",
	}); err != nil {
		t.Fatal(err)
	}
	return pub, root
}

// A run that failed, was cancelled, or was downgraded by its completion contract reached
// the end of a process without proposing anything, so no record is installed even though
// the adapter returned no Go error.
func TestDispatchSuppressesSynthesisForUnsuccessfulRuns(t *testing.T) {
	for _, status := range []string{"failed", "cancelled"} {
		t.Run(status, func(t *testing.T) {
			adapter := &statusAdapter{status: status}
			pub, root := dispatchProposalRun(t, adapter, "trace-"+status)

			if len(pub.events) != 0 {
				t.Fatalf("a %s run must publish nothing, got %+v", status, pub.events)
			}
			listed, err := patches.NewStore(root, "trace-"+status).List()
			if err != nil {
				t.Fatal(err)
			}
			if len(listed.Records) != 0 {
				t.Fatalf("a %s run must leave no proposal record, got %+v", status, listed.Records)
			}
		})
	}
}

// A run that emitted its own proposal is passed through unchanged: synthesizing a second
// one would dispatch a Guard twice for one proposal.
func TestDispatchExplicitProposalSuppressesSynthesis(t *testing.T) {
	explicit, err := protocol.NewEvent("trace-explicit", "builder", 0, protocol.EventMutation, protocol.MutationPayload{
		Kind:    protocol.MutationCodeProposalRoot,
		Summary: "hand-authored",
		Diff:    "diff --git a/x b/x\n+explicit\n",
	})
	if err != nil {
		t.Fatal(err)
	}
	adapter := &statusAdapter{status: "completed", events: []protocol.Event{explicit}}
	pub, root := dispatchProposalRun(t, adapter, "trace-explicit")

	var mutations []protocol.Event
	for _, ev := range pub.events {
		if ev.Type == protocol.EventMutation {
			mutations = append(mutations, ev)
		}
	}
	if len(mutations) != 1 {
		t.Fatalf("expected exactly the explicit proposal, got %d mutations", len(mutations))
	}
	var payload protocol.MutationPayload
	if err := json.Unmarshal(mutations[0].Payload, &payload); err != nil {
		t.Fatal(err)
	}
	if payload.Diff != "diff --git a/x b/x\n+explicit\n" {
		t.Fatalf("the explicit payload was rewritten: %+v", payload)
	}
	listed, err := patches.NewStore(root, "trace-explicit").List()
	if err != nil {
		t.Fatal(err)
	}
	if len(listed.Records) != 0 {
		t.Fatalf("an explicit proposal must not be archived as a runtime patch: %+v", listed.Records)
	}
}

// A collector error is surfaced and no ref is published against a partial or broken record.
func TestDispatchCollectorErrorPublishesNoBrokenRef(t *testing.T) {
	root := t.TempDir()
	writeColony(t, root)
	// A builder that auto-publishes, but a workspace that is not a git repository, so the
	// collector cannot produce a diff at all.
	if err := os.WriteFile(filepath.Join(root, ".paseka/bees/builder.yaml"), []byte(proposalBuilderYAML()), 0o644); err != nil {
		t.Fatal(err)
	}

	pub := &recordingPublisher{}
	d := runtime.NewDispatcher()
	d.RegisterAdapter("cursor", &mutatingAdapter{})
	d.SetPublisher(pub, false)
	reg, err := runtime.BuildBeeRegistry(root)
	if err != nil {
		t.Fatal(err)
	}
	d.SetBeeRegistry(reg)

	result, err := d.Dispatch(context.Background(), runtime.DispatchRequest{
		ColonyRoot: root,
		Bee:        "builder",
		TraceID:    "trace-collector-error",
		Task:       "work",
	})
	if err != nil {
		t.Fatal(err)
	}
	for _, ev := range pub.events {
		if ev.Type == protocol.EventMutation {
			t.Fatalf("a collector error must not publish a proposal, got %+v", ev)
		}
	}
	joined := strings.Join(result.Warnings, " | ")
	if !strings.Contains(joined, "proposal finalization") {
		t.Fatalf("the collector error must be visible, warnings = %q", joined)
	}
	listed, err := patches.NewStore(root, "trace-collector-error").List()
	if err != nil {
		t.Fatal(err)
	}
	if len(listed.Records) != 0 {
		t.Fatalf("a collector error must not install a record: %+v", listed.Records)
	}
}

// queuedProposalAdapter edits the workspace and queues an explicit proposal instead of
// publishing it live, then exits successfully.
type queuedProposalAdapter struct{}

func (queuedProposalAdapter) Name() string { return "cursor" }

func (queuedProposalAdapter) Run(_ context.Context, req adapters.RunRequest) (*adapters.RunResult, error) {
	readme := filepath.Join(req.Workspace, "README.md")
	data, err := os.ReadFile(readme)
	if err != nil {
		return nil, err
	}
	if err := os.WriteFile(readme, append(data, []byte("queued edit\n")...), 0o644); err != nil {
		return nil, err
	}
	runDir := runs.Dir{ColonyRoot: req.ColonyRoot, TraceID: req.TraceID, AgentID: req.AgentID}
	ev, err := protocol.NewEvent(req.TraceID, req.AgentID, 0, protocol.EventMutation, protocol.MutationPayload{
		Kind:    protocol.MutationCodeProposalRoot,
		Summary: "queued explicit proposal",
		Diff:    "diff --git a/README.md b/README.md\n+queued\n",
	})
	if err != nil {
		return nil, err
	}
	if err := runDir.AppendPending(ev); err != nil {
		return nil, err
	}
	return &adapters.RunResult{Status: string(protocol.StatusCompleted)}, nil
}

// offlineProposalPublisher fails only the queued proposal's publish, so the pending queue
// survives the flush and the fallback detection is what has to suppress synthesis.
type offlineProposalPublisher struct{}

func (offlineProposalPublisher) PublishEvent(_ context.Context, ev protocol.Event) error {
	var payload protocol.MutationPayload
	if err := json.Unmarshal(ev.Payload, &payload); err == nil && payload.Diff != "" {
		return errors.New("offline")
	}
	return nil
}

// A queued proposal is still the run's own proposal. When the flush fails and leaves it in
// the pending queue, the runtime must not synthesize a second one.
func TestDispatchQueuedExplicitProposalSuppressesSynthesisWhenFlushFails(t *testing.T) {
	root := t.TempDir()
	writeProposalColony(t, root, proposalBuilderYAML())
	d := runtime.NewDispatcher()
	d.RegisterAdapter("cursor", queuedProposalAdapter{})
	d.SetPublisher(offlineProposalPublisher{}, false)
	reg, err := runtime.BuildBeeRegistry(root)
	if err != nil {
		t.Fatal(err)
	}
	d.SetBeeRegistry(reg)

	if _, err := d.Dispatch(context.Background(), runtime.DispatchRequest{
		ColonyRoot: root,
		Bee:        "builder",
		TraceID:    "trace-queued-explicit",
		Task:       "work",
	}); err != nil {
		t.Fatal(err)
	}

	listed, err := patches.NewStore(root, "trace-queued-explicit").List()
	if err != nil {
		t.Fatal(err)
	}
	if len(listed.Records) != 0 {
		t.Fatalf("a queued explicit proposal was synthesized again: %+v", listed.Records)
	}
}

// Two runs of the same bee are two proposal attempts, and each gets its own record: a
// rework cycle leaves a readable history rather than one overwritten entry.
func TestDispatchTwoRunsProduceDistinctRecords(t *testing.T) {
	root := t.TempDir()
	writeProposalColony(t, root, proposalBuilderYAML())
	pub := &recordingPublisher{}
	d := runtime.NewDispatcher()
	d.RegisterAdapter("cursor", &mutatingAdapter{run: func(workspace string) error {
		readme := filepath.Join(workspace, "README.md")
		data, err := os.ReadFile(readme)
		if err != nil {
			return err
		}
		return os.WriteFile(readme, append(data, []byte("more\n")...), 0o644)
	}})
	d.SetPublisher(pub, false)
	reg, err := runtime.BuildBeeRegistry(root)
	if err != nil {
		t.Fatal(err)
	}
	d.SetBeeRegistry(reg)

	for _, traceID := range []string{"trace-first", "trace-second"} {
		if _, err := d.Dispatch(context.Background(), runtime.DispatchRequest{
			ColonyRoot: root,
			Bee:        "builder",
			TraceID:    traceID,
			Task:       "work",
		}); err != nil {
			t.Fatal(err)
		}
	}

	first, err := patches.NewStore(root, "trace-first").List()
	if err != nil {
		t.Fatal(err)
	}
	second, err := patches.NewStore(root, "trace-second").List()
	if err != nil {
		t.Fatal(err)
	}
	if len(first.Records) != 1 || len(second.Records) != 1 {
		t.Fatalf("expected one record per trail, got %d and %d", len(first.Records), len(second.Records))
	}
	if first.Records[0].AgentID == second.Records[0].AgentID {
		t.Fatal("two runs must not share one agent id")
	}
}
