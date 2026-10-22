package bus

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/russ-p/paseka/internal/trailconfig"
)

func TestProcessEventInputBindsTrailConfiguration(t *testing.T) {
	root := t.TempDir()
	if err := os.MkdirAll(filepath.Join(root, ".paseka", "prompts"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, ".paseka", "colony.yaml"), []byte("slug: test\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, ".paseka", "prompts", "default.md"), []byte("hello\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())

	raw := []byte(`{"traceId":"trace-config","type":"SIGNAL","payload":{"kind":"feature.requested"}}`)
	result, err := ProcessEventInput(context.Background(), LocalPublisher{ColonyRoot: root}, "", raw, "cli", true, false, root)
	if err != nil {
		t.Fatal(err)
	}
	if result.OK {
		t.Fatalf("result = %#v, want the run audit log to remain required", result)
	}
	binding, err := trailconfig.Read(root, "trace-config")
	if err != nil {
		t.Fatal(err)
	}
	if binding.ConfigRevision != 1 || binding.InitialConfigRef == "" {
		t.Fatalf("binding = %+v", binding)
	}
}
