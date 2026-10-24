package gitroot

import (
	"path/filepath"
	"testing"
)

func TestRunUnsetsRepositoryEnvironment(t *testing.T) {
	t.Setenv("GIT_DIR", filepath.Join(t.TempDir(), "missing.git"))
	if _, err := Run(RunOpts{
		Dir:      t.TempDir(),
		Args:     []string{"init", "-q"},
		UnsetEnv: []string{"GIT_DIR"},
	}); err != nil {
		t.Fatalf("Run with UnsetEnv: %v", err)
	}
}
