package trailview_test

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/russ-p/paseka/internal/colony"
	"github.com/russ-p/paseka/internal/protocol"
	"github.com/russ-p/paseka/internal/runs"
	"github.com/russ-p/paseka/internal/trailconfig"
	"github.com/russ-p/paseka/internal/trailfork"
	"github.com/russ-p/paseka/internal/trailview"
)

func TestLoadSharesConfigurationLineageAndPendingEvidence(t *testing.T) {
	root := writeColony(t, "slug: test\nmodel_aliases:\n  builder: vendor/builder\n")
	setupHome(t, "test")
	ctx := testContext(root)

	source := prepareTrail(t, root, "trace-source", "scout")
	fork := prepareTrail(t, root, "trace-fork", "builder")
	sourceManifest := ensureConfig(t, ctx, source)
	forkManifest := ensureConfig(t, ctx, fork)

	writeLineage(t, root, trailfork.Lineage{
		TraceID:              fork,
		ForkID:               "fork-1",
		SourceTrace:          source,
		SourceTask:           "task-1",
		SourceAttempt:        "agent-1",
		SourceConfigRef:      sourceManifest.ActiveConfigRef,
		SourceConfigRevision: sourceManifest.ConfigRevision,
		SourceBaseSHA:        "aaaa",
		BaseSHA:              "aaaa",
		ConfigRef:            forkManifest.ActiveConfigRef,
		Delta:                trailfork.Delta{Form: trailfork.DeltaNone},
		State:                trailview.AdmissionRunnable,
	})
	writePending(t, root, source, "req-1")

	collection, err := trailview.Load(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if len(collection.Trails) != 2 {
		t.Fatalf("trails = %+v", collection.Trails)
	}
	if len(collection.PendingForkRequests) != 1 {
		t.Fatalf("pending = %+v", collection.PendingForkRequests)
	}

	sourceRow, ok := collection.Find(source)
	if !ok {
		t.Fatal("source trail missing")
	}
	if !sourceRow.ConfigBound || sourceRow.ConfigRef != sourceManifest.ActiveConfigRef {
		t.Fatalf("source config = %+v", sourceRow)
	}
	if sourceRow.ConfigRevision != 1 || sourceRow.Policy != trailconfig.PolicyEditable {
		t.Fatalf("source revision/policy = %d/%q", sourceRow.ConfigRevision, sourceRow.Policy)
	}
	if sourceRow.AdmissionState != trailview.AdmissionOrdinary {
		t.Fatalf("source admission = %q", sourceRow.AdmissionState)
	}
	if sourceRow.PendingForkRequests != 1 {
		t.Fatalf("source pending count = %d", sourceRow.PendingForkRequests)
	}
	if sourceRow.ForkOf != "" {
		t.Fatalf("source forkOf = %q", sourceRow.ForkOf)
	}

	forkRow, ok := collection.Find(fork)
	if !ok {
		t.Fatal("fork trail missing")
	}
	if forkRow.ForkOf != source || forkRow.ForkID != "fork-1" {
		t.Fatalf("fork lineage = %+v", forkRow)
	}
	if forkRow.AdmissionState != trailview.AdmissionRunnable {
		t.Fatalf("fork admission = %q", forkRow.AdmissionState)
	}
	if forkRow.SourceTask != "task-1" || forkRow.SourceAttempt != "agent-1" {
		t.Fatalf("fork source = %+v", forkRow)
	}
	if forkRow.SourceConfigRef != sourceManifest.ActiveConfigRef ||
		forkRow.SourceConfigRevision != sourceManifest.ConfigRevision {
		t.Fatalf("fork source config = %q rev %d", forkRow.SourceConfigRef, forkRow.SourceConfigRevision)
	}
	if forkRow.CandidateConfigRef != forkManifest.ActiveConfigRef {
		t.Fatalf("fork candidate config = %q", forkRow.CandidateConfigRef)
	}
	if len(forkRow.Evidence) > 0 {
		t.Fatalf("unexpected fork evidence: %v", forkRow.Evidence)
	}

	if forks := collection.Forks(); len(forks) != 1 || forks[0].TraceID != fork {
		t.Fatalf("Forks() = %+v", forks)
	}
	if forks := collection.ForksOf(source); len(forks) != 1 || forks[0].TraceID != fork {
		t.Fatalf("ForksOf(source) = %+v", forks)
	}
	if forks := collection.ForksOf("trace-other"); len(forks) != 0 {
		t.Fatalf("ForksOf(other) = %+v", forks)
	}
}

func TestLoadTrailReportsMissingBindingAndPinnedLineageHonestly(t *testing.T) {
	root := writeColony(t, "slug: test\nmodel_aliases:\n  builder: vendor/builder\n")
	setupHome(t, "test")
	ctx := testContext(root)

	legacy := prepareTrail(t, root, "trace-legacy", "scout")
	legacyTrail, ok, err := trailview.LoadTrail(ctx, legacy)
	if err != nil || !ok {
		t.Fatalf("LoadTrail() = ok=%v err=%v", ok, err)
	}
	if legacyTrail.ConfigBound {
		t.Fatal("legacy trail was bound")
	}
	if !hasEvidence(legacyTrail.Evidence, "no configuration binding") {
		t.Fatalf("legacy evidence = %v", legacyTrail.Evidence)
	}

	pinned := prepareTrail(t, root, "trace-pinned", "builder")
	manifest := ensureConfig(t, ctx, pinned)
	manifest.Policy = trailconfig.PolicyPinned
	writeManifest(t, root, manifest)

	pinnedTrail, ok, err := trailview.LoadTrail(ctx, pinned)
	if err != nil || !ok {
		t.Fatalf("LoadTrail(pinned) = ok=%v err=%v", ok, err)
	}
	if pinnedTrail.AdmissionState != trailview.AdmissionPinned {
		t.Fatalf("pinned admission = %q", pinnedTrail.AdmissionState)
	}
	if !hasEvidence(pinnedTrail.Evidence, "no fork lineage record") {
		t.Fatalf("pinned evidence = %v", pinnedTrail.Evidence)
	}

	if err := os.Remove(filepath.Join(
		trailconfig.SnapshotDir(root, pinned), manifest.ActiveConfigRef, trailconfig.InventoryFileName)); err != nil {
		t.Fatal(err)
	}
	pinnedTrail, _, err = trailview.LoadTrail(ctx, pinned)
	if err != nil {
		t.Fatal(err)
	}
	if !hasEvidence(pinnedTrail.Evidence, "is unavailable") {
		t.Fatalf("corrupt snapshot evidence = %v", pinnedTrail.Evidence)
	}
}

func prepareTrail(t *testing.T, root, traceID, bee string) string {
	t.Helper()
	d := runs.Dir{ColonyRoot: root, TraceID: traceID, AgentID: "agent-1"}
	if err := d.Prepare(); err != nil {
		t.Fatal(err)
	}
	started := time.Now().UTC().Add(-time.Minute)
	if err := d.WriteRequest(protocol.Request{
		ProtocolVersion: protocol.Version,
		TraceID:         traceID,
		AgentID:         "agent-1",
		Bee:             bee,
		Adapter:         "cursor",
		Workspace:       root,
		ColonyRoot:      root,
		CreatedAt:       started,
	}); err != nil {
		t.Fatal(err)
	}
	if err := d.WriteStatusSnapshot(protocol.StatusSnapshot{
		ProtocolVersion: protocol.Version,
		State:           protocol.StatusCompleted,
		StartedAt:       started,
		FinishedAt:      started.Add(time.Second),
	}); err != nil {
		t.Fatal(err)
	}
	return traceID
}

func ensureConfig(t *testing.T, ctx colony.Context, traceID string) trailconfig.Manifest {
	t.Helper()
	manifest, err := trailconfig.Ensure(ctx, traceID)
	if err != nil {
		t.Fatal(err)
	}
	return manifest
}

func writeManifest(t *testing.T, root string, manifest trailconfig.Manifest) {
	t.Helper()
	data, err := json.MarshalIndent(manifest, "", "  ")
	if err != nil {
		t.Fatal(err)
	}
	path := trailconfig.ManifestPath(root, manifest.TraceID)
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, append(data, '\n'), 0o644); err != nil {
		t.Fatal(err)
	}
}

func writeLineage(t *testing.T, root string, lineage trailfork.Lineage) {
	t.Helper()
	data, err := json.MarshalIndent(lineage, "", "  ")
	if err != nil {
		t.Fatal(err)
	}
	path := trailfork.LineagePath(root, lineage.TraceID)
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, append(data, '\n'), 0o644); err != nil {
		t.Fatal(err)
	}
}

func writePending(t *testing.T, root, source, requestID string) {
	t.Helper()
	now := time.Now().UTC()
	request := trailfork.Request{
		Version:   1,
		RequestID: requestID,
		State:     trailfork.StatePending,
		Origin:    "agent",
		Evidence: trailfork.Lineage{
			SourceTrace: source,
			SourceTask:  "task-1",
			Delta:       trailfork.Delta{Form: trailfork.DeltaNone},
		},
		CreatedAt: now,
		UpdatedAt: now,
		ExpiresAt: now.Add(time.Hour),
	}
	data, err := json.MarshalIndent(request, "", "  ")
	if err != nil {
		t.Fatal(err)
	}
	path := trailfork.RequestPath(root, source, requestID)
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, append(data, '\n'), 0o644); err != nil {
		t.Fatal(err)
	}
}

func hasEvidence(evidence []string, substring string) bool {
	for _, note := range evidence {
		if strings.Contains(note, substring) {
			return true
		}
	}
	return false
}

func testContext(root string) colony.Context {
	return colony.Context{
		ColonyRoot:   root,
		Slug:         "test",
		Profile:      "pi",
		ModelAliases: map[string]string{"builder": "vendor/builder"},
	}
}

func setupHome(t *testing.T, slug string) string {
	t.Helper()
	base := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", base)
	home := filepath.Join(base, "paseka", slug)
	if err := os.MkdirAll(home, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(home, "config.yaml"), []byte("profile: pi\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	for _, rel := range []string{
		"profiles/pi/adapters/cursor.yaml",
		"skills/private/SKILL.md",
	} {
		path := filepath.Join(home, filepath.FromSlash(rel))
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			t.Fatal(err)
		}
		content := "private: true\n"
		if strings.HasSuffix(rel, "cursor.yaml") {
			content = "binary: agent\napi_key_env: CURSOR_API_KEY\n"
		}
		if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	return home
}

func writeColony(t *testing.T, manifest string) string {
	t.Helper()
	root := t.TempDir()
	files := map[string]string{
		".paseka/colony.yaml":                 manifest,
		".paseka/bees/builder.yaml":           "role: builder\nadapter: script\nprompt_template: builder.md\ncommand: [echo]\n",
		".paseka/bees/builder.local.yaml":     "prompt_template: builder.md\n",
		".paseka/prompts/builder.md":          "build the thing\n",
		".paseka/prompts/_partials/shared.md": "shared partial\n",
		".paseka/cues/feature.yaml":           "emit: signal\ntype: SIGNAL\nkind: feature.requested\n",
		".paseka/profiles/pi.yaml":            "adapter: pi\n",
		".agents/skills/example/SKILL.md":     "---\nname: example\ndescription: example skill\n---\nbody\n",
	}
	for rel, content := range files {
		path := filepath.Join(root, filepath.FromSlash(rel))
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	return root
}
