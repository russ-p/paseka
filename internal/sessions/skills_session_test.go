package sessions_test

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/russ-p/paseka/internal/adapters"
	"github.com/russ-p/paseka/internal/runs"
	"github.com/russ-p/paseka/internal/sessions"
	"github.com/russ-p/paseka/internal/skills"
)

// addSessionSkill writes one store skill and lists it for the scout role, so an
// interactive session has a resolved set to advertise.
func addSessionSkill(t *testing.T, repo, description string) {
	t.Helper()
	dir := filepath.Join(repo, ".paseka", "skills", "paseka-release")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	body := "---\nname: paseka-release\ndescription: " + description + "\n---\nSESSION-SKILL-BODY\n"
	if err := os.WriteFile(filepath.Join(dir, skills.ManifestName), []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
	bee := `role: scout
adapter: cursor
prompt_template: scout.md
skills:
  - paseka-release
`
	if err := os.WriteFile(filepath.Join(repo, ".paseka", "bees", "scout.yaml"), []byte(bee), 0o644); err != nil {
		t.Fatal(err)
	}
}

func TestSessionAdvertisesAndRecordsSkills(t *testing.T) {
	repo := initSessionRepo(t)
	setupSessionHome(t, repo)
	addSessionSkill(t, repo, "Run the colony release procedure.")

	fast := &instantSessionAdapter{}
	mgr := sessions.NewManager()
	mgr.RegisterSessionAdapter("cursor", fast)

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	done := make(chan error, 1)
	go func() {
		_, err := mgr.RunInteractive(ctx, sessions.RunRequest{
			StartDir: repo,
			Bee:      "scout",
			Task:     "hello session",
		})
		done <- err
	}()
	select {
	case err := <-done:
		if err != nil {
			t.Fatal(err)
		}
	case <-time.After(3 * time.Second):
		t.Fatal("timeout waiting for session")
	}

	if got := fast.lastReq.Skills; len(got) != 1 || got[0] != "paseka-release" {
		t.Fatalf("session request skills = %v", got)
	}
	if !strings.HasPrefix(fast.lastReq.SystemPrompt, skills.SectionHeading) {
		t.Fatalf("session system prompt = %q, want the section as the whole prompt", fast.lastReq.SystemPrompt)
	}
	if strings.Contains(fast.lastReq.SystemPrompt, "SESSION-SKILL-BODY") {
		t.Fatalf("session inlined the skill body: %q", fast.lastReq.SystemPrompt)
	}
	if !strings.Contains(fast.lastReq.SystemPrompt, "Run the colony release procedure.") {
		t.Fatalf("session system prompt missing the description: %q", fast.lastReq.SystemPrompt)
	}
	// The advertised path is inside the trail's frozen configuration, not the
	// live store.
	if !strings.Contains(fast.lastReq.SystemPrompt, string(filepath.Separator)+"config"+string(filepath.Separator)+"snapshots"+string(filepath.Separator)) {
		t.Fatalf("session system prompt does not advertise the frozen path: %q", fast.lastReq.SystemPrompt)
	}

	runDir := findSessionRunDir(t, repo)
	req, err := runDir.ReadRequest()
	if err != nil {
		t.Fatal(err)
	}
	if len(req.Skills) != 1 || req.Skills[0] != "paseka-release" {
		t.Fatalf("request.json skills = %v", req.Skills)
	}

	// The recorded set survives a later role-list change.
	if err := os.WriteFile(filepath.Join(repo, ".paseka", "bees", "scout.yaml"),
		[]byte("role: scout\nadapter: cursor\nprompt_template: scout.md\nskills: []\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	again, err := runDir.ReadRequest()
	if err != nil {
		t.Fatal(err)
	}
	if len(again.Skills) != 1 || again.Skills[0] != "paseka-release" {
		t.Fatalf("recorded skills changed with the role list: %v", again.Skills)
	}
}

func TestSessionFailsFastBeforeAdapterForInvalidSkills(t *testing.T) {
	repo := initSessionRepo(t)
	setupSessionHome(t, repo)
	bee := `role: scout
adapter: cursor
prompt_template: scout.md
skills:
  - paseka-missing
`
	if err := os.WriteFile(filepath.Join(repo, ".paseka", "bees", "scout.yaml"), []byte(bee), 0o644); err != nil {
		t.Fatal(err)
	}

	counter := &countingSessionAdapter{}
	mgr := sessions.NewManager()
	mgr.RegisterSessionAdapter("cursor", counter)

	_, err := mgr.RunInteractive(context.Background(), sessions.RunRequest{
		StartDir: repo,
		Bee:      "scout",
		Task:     "hello session",
	})
	if err == nil {
		t.Fatal("an invalid skill set must stop the session before the adapter starts")
	}
	if !strings.Contains(err.Error(), "paseka-missing") {
		t.Fatalf("error does not name the reference: %v", err)
	}
	if counter.calls != 0 {
		t.Fatalf("session adapter ran %d time(s) for an invalid skill set", counter.calls)
	}
}

func TestSessionResumeRecordsSkillsWithoutReinjecting(t *testing.T) {
	repo := initSessionRepo(t)
	setupSessionHome(t, repo)
	writeFinishedCursorSession(t, repo, "agent-src", "trace-src", "cursor-uuid", "")
	addSessionSkill(t, repo, "Run the colony release procedure.")

	rec := &recordingSessionAdapter{id: "cursor-uuid"}
	mgr := sessions.NewManager()
	mgr.RegisterSessionAdapter("cursor", rec)

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	res, err := mgr.ResumeDetached(ctx, sessions.ResumeRequest{
		StartDir:  repo,
		SessionID: "agent-src",
		Continue:  "keep going",
	})
	if err != nil {
		t.Fatal(err)
	}
	if rec.lastReq.SystemPrompt != "" {
		t.Fatalf("a resumed session must not re-inject the skills section: %q", rec.lastReq.SystemPrompt)
	}
	if got := rec.lastReq.Skills; len(got) != 1 || got[0] != "paseka-release" {
		t.Fatalf("resumed session request skills = %v", got)
	}
	waitSessionDone(t, mgr)

	req, err := runs.Dir{ColonyRoot: repo, TraceID: res.TraceID, AgentID: res.AgentID}.ReadRequest()
	if err != nil {
		t.Fatal(err)
	}
	if len(req.Skills) != 1 || req.Skills[0] != "paseka-release" {
		t.Fatalf("request.json skills = %v", req.Skills)
	}
}

func findSessionRunDir(t *testing.T, repo string) runs.Dir {
	t.Helper()
	entries, err := os.ReadDir(filepath.Join(repo, ".paseka", "runs"))
	if err != nil {
		t.Fatal(err)
	}
	for _, traceEntry := range entries {
		if !traceEntry.IsDir() {
			continue
		}
		agents, err := os.ReadDir(filepath.Join(repo, ".paseka", "runs", traceEntry.Name()))
		if err != nil {
			continue
		}
		for _, agentEntry := range agents {
			if !agentEntry.IsDir() {
				continue
			}
			dir := runs.Dir{ColonyRoot: repo, TraceID: traceEntry.Name(), AgentID: agentEntry.Name()}
			if _, err := os.Stat(dir.RequestPath()); err == nil {
				return dir
			}
		}
	}
	t.Fatal("no session run directory with a request.json")
	return runs.Dir{}
}

type countingSessionAdapter struct {
	calls int
}

func (c *countingSessionAdapter) Name() string { return "cursor" }

func (c *countingSessionAdapter) SessionCommand(req adapters.SessionRequest) (adapters.SessionCommand, error) {
	c.calls++
	shell, err := exec.LookPath("sh")
	if err != nil {
		return adapters.SessionCommand{}, err
	}
	return adapters.SessionCommand{
		Binary: shell,
		Args:   []string{"-c", "exit 0"},
		Env:    os.Environ(),
		Dir:    req.Workspace,
	}, nil
}
