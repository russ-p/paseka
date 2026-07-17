package runtime_test

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/paseka/paseka/internal/adapters"
	"github.com/paseka/paseka/internal/colony"
	"github.com/paseka/paseka/internal/protocol"
	"github.com/paseka/paseka/internal/runtime"
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
  trust: true
  force: true
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
	if strings.TrimSpace(payload.Diff) == "" {
		t.Fatal("expected diff in payload")
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
  trust: true
  force: true
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
  trust: true
  force: true
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
  trust: true
  force: true
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
	if strings.Contains(payload.Diff, "pre-existing") {
		t.Fatalf("proposal must not attribute pre-existing README dirt: %q", payload.Diff)
	}
	if !strings.Contains(payload.Diff, "tracked.txt") {
		t.Fatalf("proposal missing tracked.txt: %q", payload.Diff)
	}
}

func TestDispatchSkipsAutoMutationWithEmptyPublishes(t *testing.T) {
	root := t.TempDir()
	writeProposalColony(t, root, `role: builder
adapter: cursor
prompt_template: builder.md
params:
  model: composer-2.5
  trust: true
  force: true
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
