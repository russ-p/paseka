package trailview_test

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/russ-p/paseka/internal/trailconfig"
	"github.com/russ-p/paseka/internal/trailfork"
	"github.com/russ-p/paseka/internal/trailview"
)

func TestResolveScopeDistinguishesOrdinaryAndExperimentalTrails(t *testing.T) {
	root := writeColony(t, "slug: test\nmodel_aliases:\n  builder: vendor/builder\n")
	setupHome(t, "test")
	ctx := testContext(root)

	source := prepareTrail(t, root, "trace-source", "scout")
	fork := prepareTrail(t, root, "trace-fork", "builder")
	sourceManifest := ensureConfig(t, ctx, source)
	forkManifest := ensureConfig(t, ctx, fork)
	forkManifest.Policy = trailconfig.PolicyPinned
	writeManifest(t, root, forkManifest)
	writeLineage(t, root, trailfork.Lineage{
		TraceID:              fork,
		ForkID:               "fork-1",
		SourceTrace:          source,
		SourceTask:           "task-1",
		SourceAttempt:        "agent-1",
		SourceConfigRef:      sourceManifest.ActiveConfigRef,
		SourceConfigRevision: sourceManifest.ConfigRevision,
		ConfigRef:            forkManifest.ActiveConfigRef,
		Policy:               trailconfig.PolicyPinned,
		State:                trailview.AdmissionRunnable,
		Delta:                trailfork.Delta{Form: trailfork.DeltaNone},
	})

	ordinary, err := trailview.ResolveScope(ctx, source)
	if err != nil {
		t.Fatal(err)
	}
	if ordinary.Experimental || ordinary.ForkOf != "" || ordinary.AdmissionState != trailview.AdmissionOrdinary {
		t.Fatalf("source scope = %+v", ordinary)
	}

	experimental, err := trailview.ResolveScope(ctx, fork)
	if err != nil {
		t.Fatal(err)
	}
	if !experimental.Experimental {
		t.Fatalf("fork scope is not experimental: %+v", experimental)
	}
	if experimental.ForkOf != source || experimental.ForkID != "fork-1" ||
		experimental.SourceTask != "task-1" || experimental.SourceAttempt != "agent-1" ||
		experimental.SourceConfigRef != sourceManifest.ActiveConfigRef {
		t.Fatalf("fork lineage = %+v", experimental)
	}
	if experimental.Policy != trailconfig.PolicyPinned || experimental.ConfigRef != forkManifest.ActiveConfigRef {
		t.Fatalf("fork binding = %+v", experimental)
	}
	if len(experimental.Evidence) > 0 {
		t.Fatalf("unexpected fork evidence: %v", experimental.Evidence)
	}
}

func TestResolveScopeReportsPinnedWithoutLineageAndUnboundLineage(t *testing.T) {
	root := writeColony(t, "slug: test\nmodel_aliases:\n  builder: vendor/builder\n")
	setupHome(t, "test")
	ctx := testContext(root)

	pinned := prepareTrail(t, root, "trace-pinned", "builder")
	manifest := ensureConfig(t, ctx, pinned)
	manifest.Policy = trailconfig.PolicyPinned
	writeManifest(t, root, manifest)

	scope, err := trailview.ResolveScope(ctx, pinned)
	if err != nil {
		t.Fatal(err)
	}
	if !scope.Experimental || scope.AdmissionState != trailview.AdmissionPinned {
		t.Fatalf("pinned scope = %+v", scope)
	}
	if !hasEvidence(scope.Evidence, "no fork lineage record") {
		t.Fatalf("pinned evidence = %v", scope.Evidence)
	}

	unbound := prepareTrail(t, root, "trace-unbound", "builder")
	writeLineage(t, root, trailfork.Lineage{
		TraceID:     unbound,
		ForkID:      "fork-2",
		SourceTrace: "trace-source",
		Delta:       trailfork.Delta{Form: trailfork.DeltaNone},
	})
	scope, err = trailview.ResolveScope(ctx, unbound)
	if err != nil {
		t.Fatal(err)
	}
	if !scope.Experimental || scope.ForkOf != "trace-source" {
		t.Fatalf("unbound scope = %+v", scope)
	}
	if !hasEvidence(scope.Evidence, "no configuration binding") {
		t.Fatalf("unbound evidence = %v", scope.Evidence)
	}
}

func TestResolveScopeRefusesUnreadableLineage(t *testing.T) {
	root := writeColony(t, "slug: test\n")
	setupHome(t, "test")
	ctx := testContext(root)
	traceID := prepareTrail(t, root, "trace-fork", "builder")

	path := trailfork.LineagePath(root, traceID)
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte("{not json"), 0o644); err != nil {
		t.Fatal(err)
	}

	if _, err := trailview.ResolveScope(ctx, traceID); err == nil || !strings.Contains(err.Error(), "fork lineage") {
		t.Fatalf("err = %v", err)
	}
}
