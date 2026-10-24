package trailconfig_test

import (
	"errors"
	"os"
	"path/filepath"
	"testing"

	"github.com/russ-p/paseka/internal/protocol"
	"github.com/russ-p/paseka/internal/runs"
	"github.com/russ-p/paseka/internal/taskledger"
	"github.com/russ-p/paseka/internal/trailconfig"
)

func markLegacyExecution(t *testing.T, root, traceID string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Join(root, ".paseka", "runs", traceID, "tasks"), 0o755); err != nil {
		t.Fatal(err)
	}
}

func TestEnsureInitialExpectedCapturesLegacyAtRevisionZero(t *testing.T) {
	root := writeColony(t, "slug: test\n")
	setupHome(t, "test")
	ctx := testContext(root)
	traceID := "trace-legacy-capture"
	markLegacyExecution(t, root, traceID)

	zero := 0
	manifest, created, err := trailconfig.EnsureInitialExpected(ctx, traceID, &zero)
	if err != nil {
		t.Fatal(err)
	}
	if !created {
		t.Fatal("expected a new revision 1")
	}
	if manifest.ConfigRevision != 1 || manifest.InitialConfigRef == "" || manifest.ActiveConfigRef != manifest.InitialConfigRef {
		t.Fatalf("manifest = %+v", manifest)
	}
	if _, err := os.Stat(trailconfig.AttemptsPath(root, traceID)); !os.IsNotExist(err) {
		t.Fatalf("legacy capture must not claim earlier attempts: %v", err)
	}

	again, created, err := trailconfig.EnsureInitial(ctx, traceID)
	if err != nil {
		t.Fatal(err)
	}
	if created || again.InitialConfigRef != manifest.InitialConfigRef {
		t.Fatalf("idempotent capture = %+v created=%v", again, created)
	}

	if _, _, err := trailconfig.EnsureInitialExpected(ctx, traceID, &zero); err == nil {
		t.Fatal("expected revision 0 to refuse an already captured trail")
	}
	one := 1
	if _, _, err := trailconfig.EnsureInitialExpected(ctx, "trace-other", &one); err == nil {
		t.Fatal("expected a non-zero revision to refuse initial capture")
	}
}

func TestEnsureInitialExpectedRefusesUnsafeLegacyBoundary(t *testing.T) {
	root := writeColony(t, "slug: test\n")
	setupHome(t, "test")
	ctx := testContext(root)
	traceID := "trace-legacy-busy"
	if err := runs.WriteTaskSnapshot(root, traceID, taskledger.TaskSnapshot{
		TaskID: "task-1",
		Title:  "running work",
		Status: protocol.TaskStatusRunning,
	}); err != nil {
		t.Fatal(err)
	}

	zero := 0
	if _, _, err := trailconfig.EnsureInitialExpected(ctx, traceID, &zero); err == nil {
		t.Fatal("legacy capture ignored the safe boundary")
	}
}

func TestBindDoesNotCaptureLegacyTrail(t *testing.T) {
	root := writeColony(t, "slug: test\n")
	setupHome(t, "test")
	ctx := testContext(root)
	traceID := "trace-legacy-unbound"
	markLegacyExecution(t, root, traceID)

	if _, _, err := trailconfig.Bind(ctx, traceID); err == nil {
		t.Fatal("expected Bind to leave a legacy trail unbound")
	}
	if _, err := trailconfig.Load(root, traceID); !errors.Is(err, trailconfig.ErrNoBinding) {
		t.Fatalf("Load error = %v, want ErrNoBinding", err)
	}

	bound, manifest, isBound, err := trailconfig.BindNewWork(ctx, traceID)
	if err != nil {
		t.Fatal(err)
	}
	if isBound || bound.BoundConfig() || manifest.ConfigRevision != 0 {
		t.Fatalf("legacy binding = %+v manifest=%+v isBound=%v", bound, manifest, isBound)
	}
	if _, err := trailconfig.Load(root, traceID); !errors.Is(err, trailconfig.ErrNoBinding) {
		t.Fatalf("legacy work captured a binding: %v", err)
	}
}

func TestBindNewWorkCapturesBrandNewTrail(t *testing.T) {
	root := writeColony(t, "slug: test\n")
	setupHome(t, "test")
	ctx := testContext(root)
	traceID := "trace-new-work"

	if !trailconfig.IsNewTrail(root, traceID) {
		t.Fatal("a missing trail must be new")
	}
	bound, manifest, isBound, err := trailconfig.BindNewWork(ctx, traceID)
	if err != nil {
		t.Fatal(err)
	}
	if !isBound || !bound.BoundConfig() || manifest.ConfigRevision != 1 {
		t.Fatalf("new work binding = %+v manifest=%+v isBound=%v", bound, manifest, isBound)
	}
	firstEventOnly := "trace-first-event"
	trail := runs.Trail{ColonyRoot: root, TraceID: firstEventOnly}
	if err := os.MkdirAll(trail.Root(), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(trail.EventsPath(), []byte("{}\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if !trailconfig.IsNewTrail(root, firstEventOnly) {
		t.Fatal("a trail with only its event log is still new")
	}
}
