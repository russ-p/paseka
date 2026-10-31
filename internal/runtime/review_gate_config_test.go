package runtime

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/russ-p/paseka/internal/taskledger"
)

// The gate boundary may only open with policies this trail actually pinned. An
// unreadable binding therefore has to surface as an error: swallowing it and
// falling back to the zero policy opens the merge gate over work whose wrap-up
// and review the trail claimed at start, and says nothing.
func TestGateBoundaryFailsClosedWhenTrailConfigIsUnreadable(t *testing.T) {
	root := t.TempDir()
	if err := os.MkdirAll(filepath.Join(root, ".paseka"), 0o755); err != nil {
		t.Fatal(err)
	}
	// A colony manifest that cannot be parsed stands in for any broken trail
	// configuration the boundary used to shrug off.
	if err := os.WriteFile(filepath.Join(root, ".paseka", "colony.yaml"), []byte("slug: [unterminated\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	ledger := taskledger.NewMemoryLedger()
	r := NewTestReactor(TestReactorOptions{
		ColonyRoot: root,
		Ledger:     ledger,
	})

	err := r.maybeActivateFinalReview(context.Background(), "trace-broken-config", nil)
	if err == nil {
		t.Fatal("gate boundary opened without a readable trail config")
	}
	if !strings.Contains(err.Error(), "trail config") {
		t.Fatalf("error %q does not name the trail config", err)
	}
	snap, snapErr := ledger.Snapshot("trace-broken-config")
	if snapErr != nil {
		t.Fatal(snapErr)
	}
	if len(snap.Tasks) != 0 {
		t.Fatalf("gate boundary seeded tasks from a failed config read: %+v", snap.Tasks)
	}
}
