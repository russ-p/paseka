package runtime_test

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"testing"

	"github.com/paseka/paseka/internal/colony"
	"github.com/paseka/paseka/internal/runtime"
)

func TestBeeRunRoutesPiAdapterConfig(t *testing.T) {
	repo := initBeeRunRepo(t)
	setupBeeRunHomeWithPi(t, repo)

	rec := &recordingAdapter{}
	d := runtime.NewDispatcher()
	d.RegisterAdapter("pi", rec)

	piBee := `role: pi-scout
adapter: pi
prompt_template: scout.md
worktree: false
`
	if err := os.WriteFile(filepath.Join(repo, ".paseka", "bees", "pi-scout.yaml"), []byte(piBee), 0o644); err != nil {
		t.Fatal(err)
	}

	_, err := d.BeeRun(context.Background(), runtime.BeeRunRequest{
		StartDir: repo,
		Bee:      "pi-scout",
		Task:     "survey with pi",
	})
	if err != nil {
		t.Fatal(err)
	}
	if rec.lastReq.Params.Binary != "/opt/pi" {
		t.Fatalf("pi binary = %q, want /opt/pi", rec.lastReq.Params.Binary)
	}
	if rec.lastReq.Params.APIKey != "pi-secret" {
		t.Fatalf("pi api key = %q", rec.lastReq.Params.APIKey)
	}
}

func setupBeeRunHomeWithPi(t *testing.T, repo string) string {
	t.Helper()
	base := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", base)
	t.Setenv("PI_API_KEY", "pi-secret")
	slug := "test-colony"

	homeDir, err := colony.HomeDir(slug)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Join(homeDir, "adapters"), 0o755); err != nil {
		t.Fatal(err)
	}
	cfg := fmt.Sprintf("colony_root: %q\nslug: %q\n", repo, slug)
	if err := os.WriteFile(filepath.Join(homeDir, "config.yaml"), []byte(cfg), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(homeDir, "adapters", "cursor.yaml"), []byte("binary: agent\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(homeDir, "adapters", "pi.yaml"), []byte("binary: /opt/pi\napi_key_env: PI_API_KEY\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(homeDir, "state.json"), []byte("{}\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	return slug
}
