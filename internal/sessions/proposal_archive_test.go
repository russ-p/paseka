package sessions_test

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/russ-p/paseka/internal/adapters"
	"github.com/russ-p/paseka/internal/patches"
	"github.com/russ-p/paseka/internal/protocol"
	"github.com/russ-p/paseka/internal/sessions"
)

// proposalSessionAdapter runs a shell command in the session workspace and exits with a
// chosen status, so the whole PTY lifecycle is exercised without a live LLM.
type proposalSessionAdapter struct {
	script string
	exit   int
}

func (p *proposalSessionAdapter) Name() string { return "cursor" }

func (p *proposalSessionAdapter) SessionCommand(req adapters.SessionRequest) (adapters.SessionCommand, error) {
	shell, err := exec.LookPath("sh")
	if err != nil {
		return adapters.SessionCommand{}, err
	}
	return adapters.SessionCommand{
		Binary: shell,
		Args:   []string{"-c", p.script + "\nexit " + strconv.Itoa(p.exit)},
		Env:    os.Environ(),
		Dir:    req.Workspace,
	}, nil
}

// initProposalSessionRepo builds a colony whose builder auto-publishes a root proposal and
// is session-capable, on top of the ordinary session repo.
func initProposalSessionRepo(t *testing.T) string {
	t.Helper()
	repo := initSessionRepo(t)
	paseka := filepath.Join(repo, ".paseka")
	builder := `role: builder
adapter: cursor
prompt_template: builder.md
params:
  model: composer-2.5
publishes:
  - type: MUTATION
    kind: code.proposal.root
`
	if err := os.WriteFile(filepath.Join(paseka, "bees", "builder.yaml"), []byte(builder), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(paseka, "prompts", "builder.md"), []byte("Task: {{.Task}}\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	// The collector only attributes tracked files, so the session edits one.
	if err := os.WriteFile(filepath.Join(repo, "tracked.txt"), []byte("base\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	runGit(t, repo, "add", "tracked.txt")
	runGit(t, repo, "commit", "-m", "add tracked")
	return repo
}

func runProposalSession(t *testing.T, adapter adapters.SessionAdapter, repo string) {
	t.Helper()
	mgr := sessions.NewManager()
	mgr.RegisterSessionAdapter("cursor", adapter)

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	done := make(chan error, 1)
	go func() {
		_, err := mgr.RunInteractive(ctx, sessions.RunRequest{
			StartDir: repo,
			Bee:      "builder",
			Task:     "interactive work",
		})
		done <- err
	}()
	select {
	case err := <-done:
		if err != nil {
			t.Fatal(err)
		}
	case <-time.After(8 * time.Second):
		t.Fatal("timeout waiting for session")
	}
}

// A successful interactive session archives an eligible auto-generated proposal with the
// same rules as an AFK run: one record, hitl mode, and a ref that resolves from the colony
// root.
func TestSuccessfulSessionArchivesAProposal(t *testing.T) {
	repo := initProposalSessionRepo(t)
	setupSessionHome(t, repo)

	adapter := &proposalSessionAdapter{script: "printf 'edit\\n' >> tracked.txt", exit: 0}
	runProposalSession(t, adapter, repo)

	// Find the run that the session created, then its trail's proposal store.
	trailID := onlyTrailID(t, repo)
	store := patches.NewStore(repo, trailID)
	listed, err := store.List()
	if err != nil {
		t.Fatal(err)
	}
	if len(listed.Records) != 1 {
		t.Fatalf("records = %+v, want one", listed.Records)
	}
	rec := listed.Records[0]
	if rec.Mode != patches.ModeHITL {
		t.Fatalf("mode = %q, want hitl", rec.Mode)
	}
	if rec.Kind != string(protocol.MutationCodeProposalRoot) {
		t.Fatalf("kind = %q", rec.Kind)
	}
	// No bus was reachable in this test, so the record must not claim publication.
	if rec.Publication != patches.PublicationUnpublished {
		t.Fatalf("publication = %q, want unpublished without a broker", rec.Publication)
	}
	if _, err := patches.ResolveRef(repo, store.Ref(rec.AgentID)); err != nil {
		t.Fatalf("ref does not resolve: %v", err)
	}
	content, err := store.ReadContent(rec.AgentID, patches.MaxPreviewBytes)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(content.Body), "tracked.txt") {
		t.Fatalf("archived patch does not describe the session's change: %q", content.Body)
	}
}

// A session that failed or was cancelled never reaches the preparation gate, so partial
// interactive work is not presented as a completed proposal attempt.
func TestFailedSessionLeavesNoProposalRecord(t *testing.T) {
	repo := initProposalSessionRepo(t)
	setupSessionHome(t, repo)

	adapter := &proposalSessionAdapter{script: "printf 'edit\\n' >> tracked.txt", exit: 1}
	runProposalSession(t, adapter, repo)

	trailID := onlyTrailID(t, repo)
	listed, err := patches.NewStore(repo, trailID).List()
	if err != nil {
		t.Fatal(err)
	}
	if len(listed.Records) != 0 {
		t.Fatalf("a failed session must leave no record: %+v", listed.Records)
	}
}

// onlyTrailID returns the single trail id under .paseka/runs.
func onlyTrailID(t *testing.T, repo string) string {
	t.Helper()
	entries, err := os.ReadDir(filepath.Join(repo, ".paseka", "runs"))
	if err != nil {
		t.Fatal(err)
	}
	var ids []string
	for _, entry := range entries {
		if entry.IsDir() {
			ids = append(ids, entry.Name())
		}
	}
	if len(ids) != 1 {
		t.Fatalf("expected exactly one trail, got %v", ids)
	}
	return ids[0]
}
