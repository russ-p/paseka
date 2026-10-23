package console_test

import (
	"bytes"
	"encoding/json"
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
	"github.com/russ-p/paseka/internal/sessions"
	"github.com/russ-p/paseka/internal/trailconfig"
	"github.com/russ-p/paseka/internal/trailfork"
	"github.com/russ-p/paseka/internal/trailview"
)

func trailTestServer(t *testing.T, repo string, ctxColony colony.Context) *console.Server {
	t.Helper()
	return console.NewServer(console.Options{
		Addr:     "127.0.0.1:0",
		Colony:   ctxColony,
		Sessions: sessions.NewManager(),
	})
}

func postJSON(t *testing.T, srv *console.Server, target string, body any) *httptest.ResponseRecorder {
	t.Helper()
	data, err := json.Marshal(body)
	if err != nil {
		t.Fatal(err)
	}
	req := httptest.NewRequest(http.MethodPost, target, bytes.NewReader(data))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	srv.Handler().ServeHTTP(rec, req)
	return rec
}

func TestTrailConfigAPIReadsPreviewsAppliesAndCancels(t *testing.T) {
	repo := initConsoleRepo(t)
	ctxColony := setupConsoleHome(t, repo)
	writeConsoleRun(t, repo, "trace-config", "agent-config", time.Now().UTC(), protocol.StatusCompleted, "")
	if _, _, err := trailconfig.EnsureInitial(ctxColony, "trace-config"); err != nil {
		t.Fatalf("ensure initial config: %v", err)
	}
	srv := trailTestServer(t, repo, ctxColony)

	rec := httptest.NewRecorder()
	srv.Handler().ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/api/traces/trace-config/config", nil))
	if rec.Code != http.StatusOK {
		t.Fatalf("get config status = %d body=%s", rec.Code, rec.Body.String())
	}
	var manifest trailconfig.Manifest
	if err := json.NewDecoder(rec.Body).Decode(&manifest); err != nil {
		t.Fatal(err)
	}
	if manifest.ConfigRevision != 1 || manifest.ActiveConfigRef == "" || manifest.Policy != trailconfig.PolicyEditable {
		t.Fatalf("manifest = %+v", manifest)
	}

	// A colony change becomes a candidate; dry run must not advance the trail.
	if err := os.WriteFile(filepath.Join(repo, ".paseka", "bees", "scout.yaml"), []byte(`role: scout
adapter: cursor
prompt_template: scout.md
sector: intake
`), 0o644); err != nil {
		t.Fatal(err)
	}
	preview := postJSON(t, srv, "/api/traces/trace-config/config/apply", map[string]any{
		"source":           trailconfig.SourceColony,
		"expectedRevision": 1,
		"dryRun":           true,
		"reason":           "preview from console",
	})
	if preview.Code != http.StatusOK {
		t.Fatalf("preview status = %d body=%s", preview.Code, preview.Body.String())
	}
	var previewRes console.TrailConfigApplyResponse
	if err := json.NewDecoder(preview.Body).Decode(&previewRes); err != nil {
		t.Fatal(err)
	}
	if !previewRes.DryRun || previewRes.NewConfigRef == "" || previewRes.Manifest.ConfigRevision != 1 {
		t.Fatalf("preview = %+v", previewRes)
	}

	applied := postJSON(t, srv, "/api/traces/trace-config/config/apply", map[string]any{
		"source":           trailconfig.SourceColony,
		"expectedRevision": 1,
		"reason":           "adopt colony config",
	})
	if applied.Code != http.StatusOK {
		t.Fatalf("apply status = %d body=%s", applied.Code, applied.Body.String())
	}
	var appliedRes console.TrailConfigApplyResponse
	if err := json.NewDecoder(applied.Body).Decode(&appliedRes); err != nil {
		t.Fatal(err)
	}
	if appliedRes.Change.Outcome != trailconfig.OutcomeApplied || appliedRes.Manifest.ConfigRevision != 2 || appliedRes.Manifest.ActiveConfigRef != appliedRes.NewConfigRef {
		t.Fatalf("applied = %+v", appliedRes)
	}

	stale := postJSON(t, srv, "/api/traces/trace-config/config/apply", map[string]any{
		"source":           trailconfig.SourceColony,
		"expectedRevision": 1,
	})
	if stale.Code != http.StatusOK {
		t.Fatalf("stale apply status = %d body=%s", stale.Code, stale.Body.String())
	}
	var rejected console.TrailConfigApplyResponse
	if err := json.NewDecoder(stale.Body).Decode(&rejected); err != nil {
		t.Fatal(err)
	}
	if rejected.Change.Outcome != trailconfig.OutcomeRejected || !strings.Contains(rejected.Change.Detail, "expected revision") {
		t.Fatalf("stale apply = %+v", rejected)
	}

	loaded, err := trailconfig.Load(repo, "trace-config")
	if err != nil {
		t.Fatal(err)
	}
	pending := trailconfig.Change{
		Revision:         loaded.ConfigRevision,
		InitialConfigRef: loaded.InitialConfigRef,
		ActiveConfigRef:  loaded.ActiveConfigRef,
		ChangeID:         "change-pending",
		Outcome:          trailconfig.OutcomePending,
		Blockers:         []string{"task task-1 is running"},
	}
	loaded.Pending = &pending
	writeTrailManifest(t, repo, loaded)

	cancelled := postJSON(t, srv, "/api/traces/trace-config/config/cancel", map[string]any{"reason": "operator withdrew it"})
	if cancelled.Code != http.StatusOK {
		t.Fatalf("cancel status = %d body=%s", cancelled.Code, cancelled.Body.String())
	}
	afterCancel, err := trailconfig.Load(repo, "trace-config")
	if err != nil {
		t.Fatal(err)
	}
	if afterCancel.Pending != nil {
		t.Fatalf("pending change survived cancel: %+v", afterCancel.Pending)
	}
	if len(afterCancel.History) == 0 || afterCancel.History[len(afterCancel.History)-1].Outcome != trailconfig.OutcomeCancelled {
		t.Fatalf("cancel history = %+v", afterCancel.History)
	}
}

func TestTrailConfigAPIPinnedPolicyIsClear(t *testing.T) {
	repo := initConsoleRepo(t)
	ctxColony := setupConsoleHome(t, repo)
	writeConsoleRun(t, repo, "trace-pinned", "agent-pinned", time.Now().UTC(), protocol.StatusCompleted, "")
	manifest, _, err := trailconfig.EnsureInitial(ctxColony, "trace-pinned")
	if err != nil {
		t.Fatal(err)
	}
	manifest.Policy = trailconfig.PolicyPinned
	writeTrailManifest(t, repo, manifest)
	srv := trailTestServer(t, repo, ctxColony)

	rec := postJSON(t, srv, "/api/traces/trace-pinned/config/apply", map[string]any{
		"source": trailconfig.SourceColony,
	})
	if rec.Code != http.StatusConflict {
		t.Fatalf("pinned apply status = %d body=%s", rec.Code, rec.Body.String())
	}
	if !strings.Contains(strings.ToLower(rec.Body.String()), "pinned") {
		t.Fatalf("pinned apply body = %q, want a pinned explanation", rec.Body.String())
	}
}

func TestTrailCollectionAndForkLineageAPIs(t *testing.T) {
	repo := initConsoleRepo(t)
	ctxColony := setupConsoleHome(t, repo)
	now := time.Now().UTC()
	writeConsoleRun(t, repo, "trace-source", "agent-source", now.Add(-time.Hour), protocol.StatusCompleted, "")
	writeConsoleRun(t, repo, "trace-fork", "agent-fork", now, protocol.StatusRunning, "")
	lineage := trailfork.Lineage{
		Version:              1,
		ForkID:               "fork-1",
		TraceID:              "trace-fork",
		SourceTrace:          "trace-source",
		SourceTask:           "task-1",
		SourceAttempt:        "attempt-1",
		SourceBaseSHA:        "aaaa",
		BaseSHA:              "aaaa",
		SourceConfigRef:      "sha256:source",
		SourceConfigRevision: 3,
		ConfigRef:            "sha256:candidate",
		Policy:               trailconfig.PolicyPinned,
		Delta:                trailfork.Delta{Form: trailfork.DeltaNone},
		State:                "runnable",
		CreatedAt:            now,
	}
	data, err := json.Marshal(lineage)
	if err != nil {
		t.Fatal(err)
	}
	path := trailfork.LineagePath(repo, "trace-fork")
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, data, 0o644); err != nil {
		t.Fatal(err)
	}
	srv := trailTestServer(t, repo, ctxColony)

	rec := httptest.NewRecorder()
	srv.Handler().ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/api/trails", nil))
	if rec.Code != http.StatusOK {
		t.Fatalf("trails status = %d body=%s", rec.Code, rec.Body.String())
	}
	var collection trailview.Collection
	if err := json.NewDecoder(rec.Body).Decode(&collection); err != nil {
		t.Fatal(err)
	}
	fork, ok := collection.Find("trace-fork")
	if !ok {
		t.Fatalf("collection did not carry the fork: %+v", collection.Trails)
	}
	if fork.ForkOf != "trace-source" || fork.ForkID != "fork-1" || fork.SourceConfigRevision != 3 || fork.Policy != trailconfig.PolicyPinned {
		t.Fatalf("fork row = %+v", fork)
	}

	rec = httptest.NewRecorder()
	srv.Handler().ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/api/traces/trace-source/forks", nil))
	if rec.Code != http.StatusOK {
		t.Fatalf("forks status = %d body=%s", rec.Code, rec.Body.String())
	}
	var forks []trailview.Trail
	if err := json.NewDecoder(rec.Body).Decode(&forks); err != nil {
		t.Fatal(err)
	}
	if len(forks) != 1 || forks[0].TraceID != "trace-fork" {
		t.Fatalf("forks of source = %+v", forks)
	}

	detail := httptest.NewRecorder()
	srv.Handler().ServeHTTP(detail, httptest.NewRequest(http.MethodGet, "/api/traces/trace-fork", nil))
	if detail.Code != http.StatusOK {
		t.Fatalf("fork detail status = %d body=%s", detail.Code, detail.Body.String())
	}
	if !strings.Contains(detail.Body.String(), `"forkOf":"trace-source"`) {
		t.Fatalf("fork detail body = %s", detail.Body.String())
	}
}

func TestForkRequestAPIListsAndRejects(t *testing.T) {
	repo := initConsoleRepo(t)
	ctxColony := setupConsoleHome(t, repo)
	now := time.Now().UTC()
	writeConsoleRun(t, repo, "trace-source", "agent-source", now, protocol.StatusCompleted, "")
	req := trailfork.Request{
		Version:       1,
		RequestID:     "req-1",
		State:         trailfork.StatePending,
		Origin:        "agent",
		Evidence:      trailfork.Lineage{SourceTrace: "trace-source", SourceTask: "task-1", Delta: trailfork.Delta{Form: trailfork.DeltaNone}},
		ContentDigest: "digest-1",
		Validation:    "ok",
		CreatedAt:     now,
		UpdatedAt:     now,
		ExpiresAt:     now.Add(time.Hour),
	}
	data, err := json.Marshal(req)
	if err != nil {
		t.Fatal(err)
	}
	path := trailfork.RequestPath(repo, "trace-source", req.RequestID)
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, data, 0o644); err != nil {
		t.Fatal(err)
	}
	srv := trailTestServer(t, repo, ctxColony)

	rec := httptest.NewRecorder()
	srv.Handler().ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/api/fork-requests", nil))
	if rec.Code != http.StatusOK {
		t.Fatalf("pending requests status = %d body=%s", rec.Code, rec.Body.String())
	}
	var pending []trailfork.Request
	if err := json.NewDecoder(rec.Body).Decode(&pending); err != nil {
		t.Fatal(err)
	}
	if len(pending) != 1 || pending[0].RequestID != "req-1" || pending[0].Evidence.SourceTrace != "trace-source" {
		t.Fatalf("pending = %+v", pending)
	}

	scoped := httptest.NewRecorder()
	srv.Handler().ServeHTTP(scoped, httptest.NewRequest(http.MethodGet, "/api/traces/trace-source/fork-requests", nil))
	if scoped.Code != http.StatusOK || !strings.Contains(scoped.Body.String(), "req-1") {
		t.Fatalf("trace fork requests = %d body=%s", scoped.Code, scoped.Body.String())
	}

	rejected := postJSON(t, srv, "/api/fork-requests/req-1/reject", map[string]any{"reason": "not worth a fork"})
	if rejected.Code != http.StatusOK {
		t.Fatalf("reject status = %d body=%s", rejected.Code, rejected.Body.String())
	}
	var decision console.ForkDecision
	if err := json.NewDecoder(rejected.Body).Decode(&decision); err != nil {
		t.Fatal(err)
	}
	if decision.Request.State != trailfork.StateRejected || decision.Request.Reason != "not worth a fork" {
		t.Fatalf("decision = %+v", decision)
	}

	rec = httptest.NewRecorder()
	srv.Handler().ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/api/fork-requests", nil))
	var afterReject []trailfork.Request
	if err := json.NewDecoder(rec.Body).Decode(&afterReject); err != nil {
		t.Fatal(err)
	}
	if len(afterReject) != 0 {
		t.Fatalf("rejected request still pending: %+v", afterReject)
	}
	rec = httptest.NewRecorder()
	srv.Handler().ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/api/fork-requests?all=1", nil))
	var all []trailfork.Request
	if err := json.NewDecoder(rec.Body).Decode(&all); err != nil {
		t.Fatal(err)
	}
	if len(all) != 1 || all[0].State != trailfork.StateRejected {
		t.Fatalf("all requests = %+v", all)
	}
}

func TestForkRequestApproveWithoutNATSIsUnavailable(t *testing.T) {
	repo := initConsoleRepo(t)
	ctxColony := setupConsoleHome(t, repo)
	now := time.Now().UTC()
	writeConsoleRun(t, repo, "trace-source", "agent-source", now, protocol.StatusCompleted, "")
	req := trailfork.Request{
		Version:       1,
		RequestID:     "req-approve",
		State:         trailfork.StatePending,
		Origin:        "agent",
		Evidence:      trailfork.Lineage{SourceTrace: "trace-source", Delta: trailfork.Delta{Form: trailfork.DeltaNone}},
		ContentDigest: "digest-1",
		CreatedAt:     now,
		UpdatedAt:     now,
		ExpiresAt:     now.Add(time.Hour),
	}
	data, err := json.Marshal(req)
	if err != nil {
		t.Fatal(err)
	}
	path := trailfork.RequestPath(repo, "trace-source", req.RequestID)
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, data, 0o644); err != nil {
		t.Fatal(err)
	}
	srv := trailTestServer(t, repo, ctxColony)

	rec := postJSON(t, srv, "/api/fork-requests/req-approve/approve", map[string]any{})
	if rec.Code != http.StatusServiceUnavailable || !strings.Contains(rec.Body.String(), "nats url not configured") {
		t.Fatalf("approve without nats = %d body=%s", rec.Code, rec.Body.String())
	}
}

func writeTrailManifest(t *testing.T, repo string, manifest trailconfig.Manifest) {
	t.Helper()
	data, err := json.MarshalIndent(manifest, "", "  ")
	if err != nil {
		t.Fatal(err)
	}
	data = append(data, '\n')
	if err := os.WriteFile(trailconfig.ManifestPath(repo, manifest.TraceID), data, 0o644); err != nil {
		t.Fatal(err)
	}
}
