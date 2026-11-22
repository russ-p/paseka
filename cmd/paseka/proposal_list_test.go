package main

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/russ-p/paseka/internal/colony"
	"github.com/russ-p/paseka/internal/console"
	"github.com/russ-p/paseka/internal/protocol"
	"github.com/russ-p/paseka/internal/runs"
)

var proposalListBase = time.Date(2026, 11, 20, 9, 0, 0, 0, time.UTC)

// setupProposalListCLI builds a repository with a machine-local home, the same
// shape every CLI test uses: no NATS, so the ledger reads the filesystem
// projection under .paseka/runs.
func setupProposalListCLI(t *testing.T) string {
	t.Helper()
	t.Setenv("PASEKA_PROFILE", "")
	t.Setenv("PASEKA_NATS_URL", "")
	t.Setenv("PASEKA_COLONY_ROOT", "")

	repo := t.TempDir()
	runStatusGit(t, repo, "init")
	runStatusGit(t, repo, "config", "user.email", "test@test.com")
	runStatusGit(t, repo, "config", "user.name", "test")

	paseka := filepath.Join(repo, ".paseka")
	if err := os.MkdirAll(paseka, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(paseka, "colony.yaml"), []byte("slug: proposal-list-fixture\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	runStatusGit(t, repo, "add", ".paseka")
	runStatusGit(t, repo, "commit", "-m", "proposal list fixture")

	home := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", home)
	homeDir := filepath.Join(home, "paseka", "proposal-list-fixture")
	if err := os.MkdirAll(filepath.Join(homeDir, "adapters"), 0o755); err != nil {
		t.Fatal(err)
	}
	cfg := "colony_root: " + repo + "\nslug: proposal-list-fixture\n"
	if err := os.WriteFile(filepath.Join(homeDir, "config.yaml"), []byte(cfg), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(homeDir, "state.json"), []byte("{}\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	return repo
}

// addProposalTraceActivity gives a trace a run record, which is what the recent
// trace scan reads for last activity: a trace with only task files sorts as
// zero-time activity and cannot be placed in the recency window deliberately.
func addProposalTraceActivity(t *testing.T, repo, traceID string, at time.Time) {
	t.Helper()
	d := runs.Dir{ColonyRoot: repo, TraceID: traceID, AgentID: "builder-1"}
	if err := d.Prepare(); err != nil {
		t.Fatal(err)
	}
	if err := d.WriteRequest(protocol.Request{
		TraceID:   traceID,
		AgentID:   "builder-1",
		Bee:       "builder",
		TaskID:    "task-1",
		CreatedAt: at,
	}); err != nil {
		t.Fatal(err)
	}
}

func addProposalTask(t *testing.T, repo, traceID, taskID, title string, review protocol.TaskReviewPolicy, status protocol.TaskStatus, updatedAt time.Time) {
	t.Helper()
	taskDir, err := runs.NewTaskDir(repo, traceID, taskID)
	if err != nil {
		t.Fatal(err)
	}
	if err := taskDir.WriteTask(runs.TaskFrontmatter{
		TraceID:   traceID,
		TaskID:    taskID,
		Title:     title,
		Bee:       "builder",
		Status:    status,
		Review:    review,
		UpdatedAt: updatedAt.UTC().Format(time.RFC3339),
	}, "task body"); err != nil {
		t.Fatal(err)
	}
}

func TestProposalListTable(t *testing.T) {
	repo := setupProposalListCLI(t)
	addProposalTraceActivity(t, repo, "trace-alpha", proposalListBase)
	addProposalTask(t, repo, "trace-alpha", "task-1", "Wire the auth callback",
		protocol.TaskReviewRequired, protocol.TaskStatusWaitingReview, proposalListBase)
	addProposalTask(t, repo, "trace-alpha", "task-2", "Merge the auth work",
		protocol.TaskReviewFinal, protocol.TaskStatusWaitingReview, proposalListBase.Add(time.Minute))
	// Neither of these belongs in the queue: a waiting task with no review
	// policy is not a gate, and a review task that never reached the gate is
	// not waiting.
	addProposalTask(t, repo, "trace-alpha", "task-3", "Ordinary work",
		protocol.TaskReviewNone, protocol.TaskStatusWaitingReview, proposalListBase)
	addProposalTask(t, repo, "trace-alpha", "task-4", "Not started",
		protocol.TaskReviewRequired, protocol.TaskStatusPlanned, proposalListBase)

	out, err := runCLI(t, "proposal", "list", "-C", repo)
	if err != nil {
		t.Fatalf("proposal list: %v\n%s", err, out)
	}
	for _, header := range []string{"TRACE/TASK", "PHASE", "REVIEW", "TITLE"} {
		if !strings.Contains(out, header) {
			t.Fatalf("header missing %q:\n%s", header, out)
		}
	}
	for _, want := range []string{
		"trace-alpha/task-1  work   required  Wire the auth callback",
		"trace-alpha/task-2  merge  final     Merge the auth work",
	} {
		if !strings.Contains(out, want) {
			t.Fatalf("row missing %q:\n%s", want, out)
		}
	}
	for _, unwanted := range []string{"trace-alpha/task-3", "trace-alpha/task-4"} {
		if strings.Contains(out, unwanted) {
			t.Fatalf("queue included %q:\n%s", unwanted, out)
		}
	}
}

func TestProposalListJSON(t *testing.T) {
	repo := setupProposalListCLI(t)
	addProposalTraceActivity(t, repo, "trace-alpha", proposalListBase)
	addProposalTask(t, repo, "trace-alpha", "task-1", "Wire the auth callback",
		protocol.TaskReviewRequired, protocol.TaskStatusWaitingReview, proposalListBase)

	out, err := runCLI(t, "proposal", "list", "--json", "-C", repo)
	if err != nil {
		t.Fatalf("proposal list --json: %v\n%s", err, out)
	}
	var rows []map[string]any
	if err := json.Unmarshal([]byte(out), &rows); err != nil {
		t.Fatalf("json: %v\n%s", err, out)
	}
	if len(rows) != 1 {
		t.Fatalf("rows = %d, want 1:\n%s", len(rows), out)
	}
	row := rows[0]
	for key, want := range map[string]string{
		"traceId": "trace-alpha",
		"taskId":  "task-1",
		"title":   "Wire the auth callback",
		"phase":   string(protocol.TaskPhaseWork),
		"review":  string(protocol.TaskReviewRequired),
	} {
		if got, _ := row[key].(string); got != want {
			t.Fatalf("row[%q] = %q, want %q\n%s", key, got, want, out)
		}
	}
}

func TestProposalListEmptyState(t *testing.T) {
	repo := setupProposalListCLI(t)

	out, err := runCLI(t, "proposal", "list", "-C", repo)
	if err != nil {
		t.Fatalf("proposal list on an empty colony: %v\n%s", err, out)
	}
	if strings.TrimSpace(out) != "No reviews waiting." {
		t.Fatalf("empty table output = %q", out)
	}

	out, err = runCLI(t, "proposal", "list", "--json", "-C", repo)
	if err != nil {
		t.Fatalf("proposal list --json on an empty colony: %v\n%s", err, out)
	}
	if strings.TrimSpace(out) != "[]" {
		t.Fatalf("empty json output = %q, want []", out)
	}
}

func TestProposalListTraceNarrowing(t *testing.T) {
	repo := setupProposalListCLI(t)
	addProposalTraceActivity(t, repo, "trace-alpha", proposalListBase)
	addProposalTask(t, repo, "trace-alpha", "task-1", "Alpha review",
		protocol.TaskReviewRequired, protocol.TaskStatusWaitingReview, proposalListBase)
	addProposalTraceActivity(t, repo, "trace-beta", proposalListBase.Add(time.Minute))
	addProposalTask(t, repo, "trace-beta", "task-2", "Beta review",
		protocol.TaskReviewRequired, protocol.TaskStatusWaitingReview, proposalListBase.Add(time.Minute))

	out, err := runCLI(t, "proposal", "list", "--trace", "trace-beta", "-C", repo)
	if err != nil {
		t.Fatalf("proposal list --trace: %v\n%s", err, out)
	}
	if !strings.Contains(out, "trace-beta/task-2") {
		t.Fatalf("narrowed queue missing trace-beta:\n%s", out)
	}
	if strings.Contains(out, "trace-alpha/task-1") {
		t.Fatalf("narrowed queue leaked trace-alpha:\n%s", out)
	}

	out, err = runCLI(t, "proposal", "list", "--trace", "trace-alpha", "-C", repo)
	if err != nil {
		t.Fatalf("proposal list --trace trace-alpha: %v\n%s", err, out)
	}
	if !strings.Contains(out, "trace-alpha/task-1") || strings.Contains(out, "trace-beta/task-2") {
		t.Fatalf("narrowed queue rows are wrong:\n%s", out)
	}
}

// The queue inherits Console's review scope: the 30 most recently active
// trails. A gate on the 31st is absent, and --trace does not reach past the
// window either — it narrows what the projection returned.
func TestProposalListRecentTraceScope(t *testing.T) {
	repo := setupProposalListCLI(t)
	const traces = 31
	for i := 0; i < traces; i++ {
		traceID := fmt.Sprintf("trace-%02d", i)
		addProposalTraceActivity(t, repo, traceID, proposalListBase.Add(time.Duration(i)*time.Minute))
	}
	addProposalTask(t, repo, "trace-00", "task-1", "Oldest review",
		protocol.TaskReviewRequired, protocol.TaskStatusWaitingReview, proposalListBase)
	newest := proposalListBase.Add((traces - 1) * time.Minute)
	addProposalTask(t, repo, "trace-30", "task-2", "Newest review",
		protocol.TaskReviewRequired, protocol.TaskStatusWaitingReview, newest)

	out, err := runCLI(t, "proposal", "list", "-C", repo)
	if err != nil {
		t.Fatalf("proposal list: %v\n%s", err, out)
	}
	if !strings.Contains(out, "trace-30/task-2") {
		t.Fatalf("queue missing the newest review:\n%s", out)
	}
	if strings.Contains(out, "trace-00/task-1") {
		t.Fatalf("queue reached past the recent-trace limit:\n%s", out)
	}

	out, err = runCLI(t, "proposal", "list", "--trace", "trace-00", "-C", repo)
	if err != nil {
		t.Fatalf("proposal list --trace trace-00: %v\n%s", err, out)
	}
	if strings.TrimSpace(out) != "No reviews waiting on trace trace-00." {
		t.Fatalf("out-of-window trace output = %q", out)
	}
}

// The CLI verb and GET /api/review-queue are two entry points onto one
// projection, so one fixture must yield the same ordered rows from both.
func TestProposalListMatchesConsoleReviewQueue(t *testing.T) {
	repo := setupProposalListCLI(t)
	addProposalTraceActivity(t, repo, "trace-alpha", proposalListBase)
	addProposalTask(t, repo, "trace-alpha", "task-1", "Required review",
		protocol.TaskReviewRequired, protocol.TaskStatusWaitingReview, proposalListBase)
	addProposalTask(t, repo, "trace-alpha", "task-2", "Final merge gate",
		protocol.TaskReviewFinal, protocol.TaskStatusWaitingReview, proposalListBase.Add(time.Minute))
	addProposalTask(t, repo, "trace-alpha", "task-3", "Not waiting",
		protocol.TaskReviewRequired, protocol.TaskStatusPlanned, proposalListBase)

	out, err := runCLI(t, "proposal", "list", "--json", "-C", repo)
	if err != nil {
		t.Fatalf("proposal list --json: %v\n%s", err, out)
	}
	var fromCLI []console.ReviewQueueItem
	if err := json.Unmarshal([]byte(out), &fromCLI); err != nil {
		t.Fatalf("json: %v\n%s", err, out)
	}

	ctxColony, err := colony.ResolveContext(repo)
	if err != nil {
		t.Fatal(err)
	}
	srv := console.NewServer(console.Options{Addr: "127.0.0.1:0", Colony: ctxColony})
	req := httptest.NewRequest(http.MethodGet, "/api/review-queue", nil)
	rec := httptest.NewRecorder()
	srv.Handler().ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("review-queue status = %d body=%s", rec.Code, rec.Body.String())
	}
	var view console.ReviewQueueView
	if err := json.NewDecoder(rec.Body).Decode(&view); err != nil {
		t.Fatal(err)
	}

	if len(fromCLI) != len(view.Items) {
		t.Fatalf("cli rows = %d, console rows = %d", len(fromCLI), len(view.Items))
	}
	if view.Count != len(view.Items) {
		t.Fatalf("console count = %d, items = %d", view.Count, len(view.Items))
	}
	for i := range fromCLI {
		if fromCLI[i].TraceID != view.Items[i].TraceID || fromCLI[i].TaskID != view.Items[i].TaskID {
			t.Fatalf("row %d: cli %s/%s, console %s/%s",
				i, fromCLI[i].TraceID, fromCLI[i].TaskID, view.Items[i].TraceID, view.Items[i].TaskID)
		}
	}
}
