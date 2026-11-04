package trailview_test

import (
	"testing"

	"github.com/russ-p/paseka/internal/runs"
	"github.com/russ-p/paseka/internal/trailview"
)

func TestLoadTrailExposesOriginAndUnknownIsNotManual(t *testing.T) {
	root := writeColony(t, "slug: test\nmodel_aliases:\n  builder: vendor/builder\n")
	setupHome(t, "test")
	ctx := testContext(root)

	known := prepareTrail(t, root, "trace-known", "builder")
	if _, _, err := runs.EnsureTrailOrigin(root, known, runs.TrailOrigin{
		Kind:         runs.OriginKindCue,
		Source:       "cli",
		InvocationID: "cueinv-known",
		CueID:        "feature",
	}); err != nil {
		t.Fatal(err)
	}
	trail, ok, err := trailview.LoadTrail(ctx, known)
	if err != nil || !ok {
		t.Fatalf("LoadTrail(known) = ok=%v err=%v", ok, err)
	}
	if trail.Origin == nil {
		t.Fatal("known origin was not exposed")
	}
	if trail.Origin.Kind != runs.OriginKindCue || trail.Origin.CueID != "feature" ||
		trail.Origin.InvocationID != "cueinv-known" {
		t.Fatalf("origin = %+v", trail.Origin)
	}
	if trail.OriginKind != runs.OriginKindCue {
		t.Fatalf("origin kind = %q", trail.OriginKind)
	}

	unknown := prepareTrail(t, root, "trace-unknown", "builder")
	trail, ok, err = trailview.LoadTrail(ctx, unknown)
	if err != nil || !ok {
		t.Fatalf("LoadTrail(unknown) = ok=%v err=%v", ok, err)
	}
	if trail.Origin != nil {
		t.Fatalf("trail without an origin read as %+v", trail.Origin)
	}
	if trail.OriginKind != runs.OriginKindUnknown || trail.OriginKind == runs.OriginKindManual {
		t.Fatalf("unknown origin kind = %q", trail.OriginKind)
	}
}
