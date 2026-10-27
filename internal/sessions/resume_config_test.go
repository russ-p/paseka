package sessions_test

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/russ-p/paseka/internal/adapters"
	"github.com/russ-p/paseka/internal/colony"
	"github.com/russ-p/paseka/internal/protocol"
	"github.com/russ-p/paseka/internal/runs"
	"github.com/russ-p/paseka/internal/sessions"
	"github.com/russ-p/paseka/internal/trailconfig"
)

// TestManagerResumeKeepsCapturedConfiguration is the regression test for a
// resumed interactive session silently switching to a later active revision.
// The source session was admitted at revision 1; revision 2 changes the prompt
// and the bee's adapter, so resolving the live colony would refuse the resume.
func TestManagerResumeKeepsCapturedConfiguration(t *testing.T) {
	repo := initSessionRepo(t)
	setupSessionHome(t, repo)
	traceID := "trace-config-pinned"

	initial := ensureSessionTrailConfig(t, repo, traceID)
	writeFinishedSessionAt(t, repo, "agent-src", traceID, "cursor-uuid", "", "cursor", protocol.Provenance{
		ConfigRef:      initial.ActiveConfigRef,
		ConfigRevision: initial.ConfigRevision,
	})
	writeSessionRunCompleted(t, repo, traceID, "agent-src")

	writeSessionScout(t, repo, "pi")
	applied := applySessionColonyConfig(t, repo, traceID)

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
	waitSessionDone(t, mgr)

	if rec.lastReq.Bee != "scout" {
		t.Fatalf("resumed bee = %q, want scout from the captured configuration", rec.lastReq.Bee)
	}
	if rec.lastReq.InitialPrompt != "keep going" {
		t.Fatalf("continue = %q", rec.lastReq.InitialPrompt)
	}

	d := runs.Dir{ColonyRoot: repo, TraceID: traceID, AgentID: res.AgentID}
	req, err := d.ReadRequest()
	if err != nil {
		t.Fatal(err)
	}
	assertPinnedConfig(t, "request.json", req.ConfigRef, req.ConfigRevision, initial.ActiveConfigRef, initial.ConfigRevision, applied.Manifest.ActiveConfigRef)
	meta, err := d.ReadSession()
	if err != nil {
		t.Fatal(err)
	}
	assertPinnedConfig(t, "session.json", meta.ConfigRef, meta.ConfigRevision, initial.ActiveConfigRef, initial.ConfigRevision, applied.Manifest.ActiveConfigRef)

	attempts, err := trailconfig.Attempts(repo, traceID)
	if err != nil {
		t.Fatal(err)
	}
	if len(attempts) != 1 {
		t.Fatalf("attempts = %+v, want one resumed session attempt", attempts)
	}
	assertPinnedConfig(t, "attempt", attempts[0].ConfigRef, attempts[0].ConfigRevision, initial.ActiveConfigRef, initial.ConfigRevision, applied.Manifest.ActiveConfigRef)
}

// TestManagerResumeUsesCapturedBeeWhenLiveBeeIsRenamed proves the checks run
// against the bound snapshot: revision 2 renames the role, so the live colony
// cannot resolve it, while the captured revision still can.
func TestManagerResumeUsesCapturedBeeWhenLiveBeeIsRenamed(t *testing.T) {
	repo := initSessionRepo(t)
	setupSessionHome(t, repo)
	traceID := "trace-config-renamed"

	initial := ensureSessionTrailConfig(t, repo, traceID)
	writeFinishedSessionAt(t, repo, "agent-src", traceID, "cursor-uuid", "", "cursor", protocol.Provenance{
		ConfigRef:      initial.ActiveConfigRef,
		ConfigRevision: initial.ConfigRevision,
	})
	writeSessionRunCompleted(t, repo, traceID, "agent-src")

	beePath := filepath.Join(repo, ".paseka", "bees", "scout.yaml")
	bee, err := os.ReadFile(beePath)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.Remove(beePath); err != nil {
		t.Fatal(err)
	}
	renamed := strings.Replace(string(bee), "role: scout", "role: explorer", 1)
	if err := os.WriteFile(filepath.Join(repo, ".paseka", "bees", "explorer.yaml"), []byte(renamed), 0o644); err != nil {
		t.Fatal(err)
	}
	applied := applySessionColonyConfig(t, repo, traceID)

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
	waitSessionDone(t, mgr)
	if rec.lastReq.Bee != "scout" {
		t.Fatalf("resumed bee = %q, want the captured scout role", rec.lastReq.Bee)
	}

	d := runs.Dir{ColonyRoot: repo, TraceID: traceID, AgentID: res.AgentID}
	req, err := d.ReadRequest()
	if err != nil {
		t.Fatal(err)
	}
	assertPinnedConfig(t, "request.json", req.ConfigRef, req.ConfigRevision, initial.ActiveConfigRef, initial.ConfigRevision, applied.Manifest.ActiveConfigRef)
}

// TestManagerResumeRefusesUnavailableConfiguration asserts the pin never falls
// back to the active revision: a lost snapshot refuses the continuation before
// any run record exists.
func TestManagerResumeRefusesUnavailableConfiguration(t *testing.T) {
	repo := initSessionRepo(t)
	setupSessionHome(t, repo)
	traceID := "trace-config-lost"

	initial := ensureSessionTrailConfig(t, repo, traceID)
	writeFinishedSessionAt(t, repo, "agent-src", traceID, "cursor-uuid", "", "cursor", protocol.Provenance{
		ConfigRef:      initial.ActiveConfigRef,
		ConfigRevision: initial.ConfigRevision,
	})
	writeSessionRunCompleted(t, repo, traceID, "agent-src")

	writeSessionPrompt(t, repo, "NEW-LOST {{.Task}}")
	applySessionColonyConfig(t, repo, traceID)

	// The retained snapshot disappears while the source session still claims it.
	if err := os.RemoveAll(filepath.Join(trailconfig.SnapshotDir(repo, traceID), initial.ActiveConfigRef)); err != nil {
		t.Fatal(err)
	}
	before := sessionRunDirs(t, repo, traceID)

	mgr := sessions.NewManager()
	mgr.RegisterSessionAdapter("cursor", &recordingSessionAdapter{})
	_, err := mgr.ResumeDetached(context.Background(), sessions.ResumeRequest{
		StartDir:  repo,
		SessionID: "agent-src",
		Continue:  "keep going",
	})
	assertResumeCode(t, err, sessions.ResumeErrConfigUnavailable, 409)
	if !strings.Contains(err.Error(), initial.ActiveConfigRef) {
		t.Fatalf("error %q does not name the missing configuration", err)
	}
	after := sessionRunDirs(t, repo, traceID)
	if len(after) != len(before) {
		t.Fatalf("resume created run directories: before=%v after=%v", before, after)
	}
}

// TestManagerResumeLegacySessionUsesActiveConfiguration keeps the compatibility
// path: a session with no recorded configuration resumes on the active version.
func TestManagerResumeLegacySessionUsesActiveConfiguration(t *testing.T) {
	repo := initSessionRepo(t)
	setupSessionHome(t, repo)
	traceID := "trace-config-legacy"

	ensureSessionTrailConfig(t, repo, traceID)
	writeFinishedSessionAt(t, repo, "agent-src", traceID, "cursor-uuid", "", "cursor", protocol.Provenance{})
	writeSessionRunCompleted(t, repo, traceID, "agent-src")

	writeSessionPrompt(t, repo, "NEW-LEGACY {{.Task}}")
	applied := applySessionColonyConfig(t, repo, traceID)

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
	waitSessionDone(t, mgr)

	req, err := runs.Dir{ColonyRoot: repo, TraceID: traceID, AgentID: res.AgentID}.ReadRequest()
	if err != nil {
		t.Fatal(err)
	}
	if req.ConfigRef != applied.Manifest.ActiveConfigRef || req.ConfigRevision != applied.Manifest.ConfigRevision {
		t.Fatalf("legacy resume ran %q rev %d, want active %q rev %d",
			req.ConfigRef, req.ConfigRevision, applied.Manifest.ActiveConfigRef, applied.Manifest.ConfigRevision)
	}
}

// TestManagerStartDetachedRunsPinnedConfigurationPrompt drives the launch-time
// pin directly: the rendered prompt must come from the retained snapshot, not
// from the live colony that revision 2 changed.
func TestManagerStartDetachedRunsPinnedConfigurationPrompt(t *testing.T) {
	repo := initSessionRepo(t)
	setupSessionHome(t, repo)
	traceID := "trace-config-pinned-launch"

	promptPath := filepath.Join(repo, ".paseka", "prompts", "scout.md")
	if err := os.WriteFile(promptPath, []byte("OLD-SNAPSHOT {{.Task}}"), 0o644); err != nil {
		t.Fatal(err)
	}
	initial := ensureSessionTrailConfig(t, repo, traceID)
	if err := os.WriteFile(promptPath, []byte("NEW-LIVE {{.Task}}"), 0o644); err != nil {
		t.Fatal(err)
	}
	writeSessionScout(t, repo, "pi")
	applied := applySessionColonyConfig(t, repo, traceID)
	rec := &recordingSessionAdapter{}
	mgr := sessions.NewManager()
	mgr.RegisterSessionAdapter("cursor", rec)

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	res, err := mgr.StartDetached(ctx, sessions.RunRequest{
		StartDir:       repo,
		Bee:            "scout",
		TraceID:        traceID,
		Task:           "hello pinned",
		ConfigRef:      initial.ActiveConfigRef,
		ConfigRevision: initial.ConfigRevision,
	})
	if err != nil {
		t.Fatal(err)
	}
	waitSessionDone(t, mgr)

	if !strings.Contains(rec.lastReq.InitialPrompt, "OLD-SNAPSHOT") || !strings.Contains(rec.lastReq.InitialPrompt, "hello pinned") {
		t.Fatalf("pinned prompt = %q, want the retained revision-1 template", rec.lastReq.InitialPrompt)
	}
	if strings.Contains(rec.lastReq.InitialPrompt, "NEW-LIVE") {
		t.Fatalf("pinned prompt came from the live colony: %q", rec.lastReq.InitialPrompt)
	}
	if rec.lastReq.Bee != "scout" {
		t.Fatalf("pinned bee = %q", rec.lastReq.Bee)
	}

	req, err := runs.Dir{ColonyRoot: repo, TraceID: traceID, AgentID: res.AgentID}.ReadRequest()
	if err != nil {
		t.Fatal(err)
	}
	assertPinnedConfig(t, "request.json", req.ConfigRef, req.ConfigRevision, initial.ActiveConfigRef, initial.ConfigRevision, applied.Manifest.ActiveConfigRef)
	attempts, err := trailconfig.Attempts(repo, traceID)
	if err != nil {
		t.Fatal(err)
	}
	if len(attempts) != 1 {
		t.Fatalf("attempts = %+v, want the pinned session attempt", attempts)
	}
	assertPinnedConfig(t, "attempt", attempts[0].ConfigRef, attempts[0].ConfigRevision, initial.ActiveConfigRef, initial.ConfigRevision, applied.Manifest.ActiveConfigRef)
}

func ensureSessionTrailConfig(t *testing.T, repo, traceID string) trailconfig.Manifest {
	t.Helper()
	initial, err := trailconfig.Ensure(colony.Context{ColonyRoot: repo, Slug: "session-test"}, traceID)
	if err != nil {
		t.Fatal(err)
	}
	return initial
}

func writeSessionPrompt(t *testing.T, repo, template string) {
	t.Helper()
	if err := os.WriteFile(filepath.Join(repo, ".paseka", "prompts", "scout.md"), []byte(template), 0o644); err != nil {
		t.Fatal(err)
	}
}

// writeSessionScout rewrites the live scout bee definition so the next captured
// revision differs from the retained one.
func writeSessionScout(t *testing.T, repo, adapter string) {
	t.Helper()
	body := "role: scout\nadapter: " + adapter + "\nprompt_template: scout.md\n"
	if err := os.WriteFile(filepath.Join(repo, ".paseka", "bees", "scout.yaml"), []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
}

func applySessionColonyConfig(t *testing.T, repo, traceID string) trailconfig.ApplyResult {
	t.Helper()
	manifest, err := trailconfig.Load(repo, traceID)
	if err != nil {
		t.Fatal(err)
	}
	expected := manifest.ConfigRevision
	applied, err := trailconfig.Apply(trailconfig.ApplyOptions{
		Context:          colony.Context{ColonyRoot: repo, Slug: "session-test"},
		TraceID:          traceID,
		Source:           trailconfig.SourceColony,
		Actor:            "operator",
		ExpectedRevision: &expected,
	})
	if err != nil {
		t.Fatal(err)
	}
	if applied.Change.Outcome != trailconfig.OutcomeApplied {
		t.Fatalf("apply outcome = %q (%s), want applied", applied.Change.Outcome, applied.Change.Detail)
	}
	return applied
}

func writeSessionRunCompleted(t *testing.T, repo, traceID, agentID string) {
	t.Helper()
	started := time.Now().UTC().Add(-time.Hour)
	if err := (runs.Dir{ColonyRoot: repo, TraceID: traceID, AgentID: agentID}).WriteStatusSnapshot(protocol.StatusSnapshot{
		ProtocolVersion: protocol.Version,
		State:           protocol.StatusCompleted,
		StartedAt:       started,
		FinishedAt:      started.Add(time.Minute),
	}); err != nil {
		t.Fatal(err)
	}
}

// writeFinishedSessionAt writes a completed session record with the given
// recorded provenance.
func writeFinishedSessionAt(t *testing.T, repo, sessionID, traceID, providerID, resumedFrom, adapter string, provenance protocol.Provenance) runs.Dir {
	t.Helper()
	d := runs.Dir{ColonyRoot: repo, TraceID: traceID, AgentID: sessionID}
	if err := d.Prepare(); err != nil {
		t.Fatal(err)
	}
	started := time.Now().UTC().Add(-time.Hour)
	if err := d.WriteSession(runs.SessionMeta{
		SessionID:         sessionID,
		TraceID:           traceID,
		AgentID:           sessionID,
		Bee:               "scout",
		Adapter:           adapter,
		Workspace:         repo,
		ColonyRoot:        repo,
		State:             string(adapters.SessionCompleted),
		ProviderSessionID: providerID,
		ResumedFrom:       resumedFrom,
		StartedAt:         started,
		FinishedAt:        started.Add(time.Minute),
		Provenance:        provenance,
	}); err != nil {
		t.Fatal(err)
	}
	return d
}

// sessionRunDirs lists agent run directories, excluding reserved trace
// subdirectories such as config, tasks, and artifacts.
func sessionRunDirs(t *testing.T, repo, traceID string) []string {
	t.Helper()
	entries, err := os.ReadDir(filepath.Join(repo, ".paseka", "runs", traceID))
	if err != nil {
		t.Fatal(err)
	}
	var dirs []string
	for _, entry := range entries {
		if !entry.IsDir() || runs.IsReservedTraceSubdir(entry.Name()) {
			continue
		}
		dirs = append(dirs, entry.Name())
	}
	return dirs
}

func assertPinnedConfig(t *testing.T, label, gotRef string, gotRevision int, wantRef string, wantRevision int, activeRef string) {
	t.Helper()
	if gotRef != wantRef || gotRevision != wantRevision {
		t.Fatalf("%s config = %q rev %d, want captured %q rev %d (active %q)", label, gotRef, gotRevision, wantRef, wantRevision, activeRef)
	}
}
