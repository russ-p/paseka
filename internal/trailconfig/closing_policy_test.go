package trailconfig_test

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/russ-p/paseka/internal/trailconfig"
)

const closingColonyManifest = `slug: test
closing:
  enabled: true
  when:
    min_work_tasks: 1
    has_isolated_proposal: true
  task:
    bee: builder
    intent: closing
    title: Review the trail
    review: none
    params:
      model: strong
  max_rounds: 1
merge:
  task:
    title: "Review and merge: {{.TraceTitle}}"
    body: |
      {{range .Tasks}}- {{.TaskID}} ({{.Intent}})
      {{end}}
`

func TestClosingPolicyIsPinnedAtTrailStart(t *testing.T) {
	root := writeColony(t, closingColonyManifest)
	setupHome(t, "test")
	ctx := testContext(root)

	manifest, err := trailconfig.Ensure(ctx, "trace-policy")
	if err != nil {
		t.Fatal(err)
	}
	pinned := manifest.ClosingPolicy()
	if !pinned.Enabled || pinned.ResolvedMaxRounds() != 1 || pinned.Task.Bee != "builder" {
		t.Fatalf("pinned closing policy = %+v", pinned)
	}
	if !manifest.MergePolicy().Configured() {
		t.Fatal("merge copy was not pinned")
	}

	inventory, _, err := trailconfig.ReadSnapshot(root, "trace-policy", manifest.ActiveConfigRef)
	if err != nil {
		t.Fatal(err)
	}
	if inventory.Resolved.Closing == nil || !inventory.Resolved.Closing.Enabled {
		t.Fatalf("snapshot resolved closing = %+v", inventory.Resolved.Closing)
	}
	if inventory.Resolved.Merge == nil || inventory.Resolved.Merge.Task.Title == "" {
		t.Fatalf("snapshot resolved merge = %+v", inventory.Resolved.Merge)
	}

	// Edit the live colony after the trail started.
	edited := `slug: test
closing:
  enabled: false
  max_rounds: 3
merge:
  task:
    title: changed copy
`
	if err := os.WriteFile(filepath.Join(root, ".paseka", "colony.yaml"), []byte(edited), 0o644); err != nil {
		t.Fatal(err)
	}

	reloaded, err := trailconfig.Load(root, "trace-policy")
	if err != nil {
		t.Fatal(err)
	}
	if got := reloaded.ClosingPolicy(); !got.Enabled || got.ResolvedMaxRounds() != 1 {
		t.Fatalf("live colony edit changed the pinned policy: %+v", got)
	}
	if got := reloaded.MergePolicy().Task.Title; got != "Review and merge: {{.TraceTitle}}" {
		t.Fatalf("live colony edit changed the pinned merge copy: %q", got)
	}

	// The pinned snapshot keeps the original resolved view too.
	inventory, _, err = trailconfig.ReadSnapshot(root, "trace-policy", reloaded.ActiveConfigRef)
	if err != nil {
		t.Fatal(err)
	}
	if inventory.Resolved.Closing == nil || inventory.Resolved.Closing.ResolvedMaxRounds() != 1 {
		t.Fatalf("snapshot closing changed after live edit: %+v", inventory.Resolved.Closing)
	}

	// Only the existing safe cutover may adopt the edited colony policy.
	applied, err := trailconfig.Apply(trailconfig.ApplyOptions{
		Context: ctx,
		TraceID: "trace-policy",
		Source:  trailconfig.SourceColony,
		Actor:   "test",
	})
	if err != nil {
		t.Fatal(err)
	}
	if applied.Manifest.ClosingPolicy().Enabled {
		t.Fatal("cutover did not adopt the edited closing policy")
	}
	if got := applied.Manifest.MergePolicy().Task.Title; got != "changed copy" {
		t.Fatalf("cutover merge copy = %q", got)
	}
}

func TestDefaultPolicyLeavesSnapshotResolvedUnset(t *testing.T) {
	root := writeColony(t, "slug: test\n")
	setupHome(t, "test")
	ctx := testContext(root)

	manifest, err := trailconfig.Ensure(ctx, "trace-default-policy")
	if err != nil {
		t.Fatal(err)
	}
	if manifest.Closing != nil || manifest.Merge != nil {
		t.Fatalf("default colony pinned a policy: closing=%+v merge=%+v", manifest.Closing, manifest.Merge)
	}
	inventory, _, err := trailconfig.ReadSnapshot(root, "trace-default-policy", manifest.ActiveConfigRef)
	if err != nil {
		t.Fatal(err)
	}
	if inventory.Resolved.Closing != nil || inventory.Resolved.Merge != nil {
		t.Fatalf("default colony snapshot carries a policy: %+v", inventory.Resolved)
	}
}

func TestClosingPolicyPinsResolvedDefaults(t *testing.T) {
	root := writeColony(t, `slug: test
closing:
  enabled: true
  task:
    intent: closing
`)
	setupHome(t, "test")
	ctx := testContext(root)

	manifest, err := trailconfig.Ensure(ctx, "trace-resolved-policy")
	if err != nil {
		t.Fatal(err)
	}
	pinned := manifest.ClosingPolicy()
	if pinned.Task.Bee != "builder" {
		t.Fatalf("pinned closing bee = %q, want the effective default", pinned.Task.Bee)
	}
	if pinned.ResolvedMaxRounds() != 2 || pinned.MaxRounds == nil || *pinned.MaxRounds != 2 {
		t.Fatalf("pinned max rounds = %+v, want the resolved default", pinned.MaxRounds)
	}
	inventory, _, err := trailconfig.ReadSnapshot(root, "trace-resolved-policy", manifest.ActiveConfigRef)
	if err != nil {
		t.Fatal(err)
	}
	if inventory.Resolved.Closing == nil || inventory.Resolved.Closing.Task.Bee != "builder" {
		t.Fatalf("snapshot resolved closing = %+v", inventory.Resolved.Closing)
	}
}
