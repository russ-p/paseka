package colony_test

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/russ-p/paseka/internal/colony"
)

func TestResolveContextMissingProfileListsAvailable(t *testing.T) {
	repo, slug := setupProfileColony(t)
	writeColonyProfile(t, repo, "pi", "adapter: pi\n")
	writeHomeProfileConfig(t, slug, "nightly", "nats:\n  url: nats://nightly:4222\n")
	resetProcessProfile(t)
	colony.SetProcessProfile(colony.ProfileSelection{Name: "missing", FlagSet: true})

	_, err := colony.ResolveContext(repo)
	if err == nil {
		t.Fatal("expected missing profile error")
	}
	msg := err.Error()
	if !strings.Contains(msg, `profile "missing" not found`) {
		t.Fatalf("err = %v", err)
	}
	if !strings.Contains(msg, "pi") || !strings.Contains(msg, "nightly") {
		t.Fatalf("expected available names in %v", err)
	}
}

func TestEmptyHomeProfileDirIsNotFound(t *testing.T) {
	repo, slug := setupProfileColony(t)
	dir, err := colony.HomeDir(slug)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Join(dir, "profiles", "empty"), 0o755); err != nil {
		t.Fatal(err)
	}
	resetProcessProfile(t)
	colony.SetProcessProfile(colony.ProfileSelection{Name: "empty", FlagSet: true})
	_, err = colony.ResolveContext(repo)
	if err == nil || !strings.Contains(err.Error(), `profile "empty" not found`) {
		t.Fatalf("err = %v", err)
	}
}

func TestColonyOnlyProfileRemapsLLMBees(t *testing.T) {
	repo, _ := setupProfileColony(t)
	writeColonyProfile(t, repo, "pi", `adapter: pi
params:
  provider: google
`)
	resetProcessProfile(t)
	colony.SetProcessProfile(colony.ProfileSelection{Name: "pi", FlagSet: true})
	ctx, err := colony.ResolveContext(repo)
	if err != nil {
		t.Fatal(err)
	}
	if ctx.Profile != "pi" || !ctx.ProfileLayers.Colony || ctx.ProfileLayers.Home {
		t.Fatalf("layers = %+v name=%q", ctx.ProfileLayers, ctx.Profile)
	}
	scout, _, err := ctx.LoadBee("scout")
	if err != nil {
		t.Fatal(err)
	}
	name, err := scout.ResolveAdapter()
	if err != nil {
		t.Fatal(err)
	}
	if name != "pi" {
		t.Fatalf("scout adapter = %q", name)
	}
	if scout.Params["provider"] != "google" {
		t.Fatalf("params = %+v", scout.Params)
	}
	guard, _, err := ctx.LoadBee("oracle-guard")
	if err != nil {
		t.Fatal(err)
	}
	guardName, err := guard.ResolveAdapter()
	if err != nil {
		t.Fatal(err)
	}
	if guardName != "script" {
		t.Fatalf("script bee remapped to %q", guardName)
	}
	custom, _, err := ctx.LoadBee("custom")
	if err != nil {
		t.Fatal(err)
	}
	customName, err := custom.ResolveAdapter()
	if err != nil {
		t.Fatal(err)
	}
	if customName != "cursor" {
		t.Fatalf("command bee remapped to %q", customName)
	}
}

func TestPerBeeExceptionOverridesCommandSkip(t *testing.T) {
	repo, _ := setupProfileColony(t)
	writeColonyProfile(t, repo, "mix", `adapter: pi
bees:
  custom:
    adapter: opencode
  scout:
    params:
      model: special
`)
	resetProcessProfile(t)
	colony.SetProcessProfile(colony.ProfileSelection{Name: "mix", FlagSet: true})
	ctx, err := colony.ResolveContext(repo)
	if err != nil {
		t.Fatal(err)
	}
	custom, _, err := ctx.LoadBee("custom")
	if err != nil {
		t.Fatal(err)
	}
	name, err := custom.ResolveAdapter()
	if err != nil {
		t.Fatal(err)
	}
	if name != "opencode" {
		t.Fatalf("custom adapter = %q", name)
	}
	scout, _, err := ctx.LoadBee("scout")
	if err != nil {
		t.Fatal(err)
	}
	if scout.Params["model"] != "special" {
		t.Fatalf("scout params = %+v", scout.Params)
	}
}

func TestProfileRejectsForbiddenKeysAndScript(t *testing.T) {
	repo, _ := setupProfileColony(t)
	writeColonyProfile(t, repo, "bad", "command: echo\n")
	resetProcessProfile(t)
	colony.SetProcessProfile(colony.ProfileSelection{Name: "bad", FlagSet: true})
	_, err := colony.ResolveContext(repo)
	if err == nil || !strings.Contains(err.Error(), "forbidden key") {
		t.Fatalf("err = %v", err)
	}

	writeColonyProfile(t, repo, "scripted", "adapter: script\n")
	colony.SetProcessProfile(colony.ProfileSelection{Name: "scripted", FlagSet: true})
	_, err = colony.ResolveContext(repo)
	if err == nil || !strings.Contains(err.Error(), "not allowed") {
		t.Fatalf("err = %v", err)
	}

	writeColonyProfile(t, repo, "unknown", "adapter: nope\n")
	colony.SetProcessProfile(colony.ProfileSelection{Name: "unknown", FlagSet: true})
	_, err = colony.ResolveContext(repo)
	if err == nil || !strings.Contains(err.Error(), "unknown adapter") {
		t.Fatalf("err = %v", err)
	}

	writeColonyProfile(t, repo, "ghost", "bees:\n  missing:\n    adapter: pi\n")
	colony.SetProcessProfile(colony.ProfileSelection{Name: "ghost", FlagSet: true})
	_, err = colony.ResolveContext(repo)
	if err == nil || !strings.Contains(err.Error(), "role not found") {
		t.Fatalf("err = %v", err)
	}
}

func TestInvalidProfileYAMLIncludesPath(t *testing.T) {
	repo, _ := setupProfileColony(t)
	path := filepath.Join(repo, ".paseka", "profiles", "broken.yaml")
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte("adapter: [\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	resetProcessProfile(t)
	colony.SetProcessProfile(colony.ProfileSelection{Name: "broken", FlagSet: true})
	_, err := colony.ResolveContext(repo)
	if err == nil || !strings.Contains(err.Error(), path) {
		t.Fatalf("err = %v", err)
	}
}

func TestAliasMergeOrderAndChainRejected(t *testing.T) {
	repo, slug := setupProfileColony(t)
	if err := os.WriteFile(filepath.Join(repo, ".paseka", "colony.yaml"), []byte("slug: "+slug+"\nmodel_aliases:\n  high: colony-id\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	writeColonyProfile(t, repo, "pi", "model_aliases:\n  high: colony-profile-id\n")
	homeDir, err := colony.HomeDir(slug)
	if err != nil {
		t.Fatal(err)
	}
	cfg := "colony_root: " + repo + "\nslug: " + slug + "\nmodel_aliases:\n  high: home-id\nnats:\n  url: nats://127.0.0.1:4222\n"
	if err := os.WriteFile(filepath.Join(homeDir, "config.yaml"), []byte(cfg), 0o644); err != nil {
		t.Fatal(err)
	}
	writeHomeProfileConfig(t, slug, "pi", "model_aliases:\n  high: home-profile-id\n")
	resetProcessProfile(t)
	colony.SetProcessProfile(colony.ProfileSelection{Name: "pi", FlagSet: true})
	ctx, err := colony.ResolveContext(repo)
	if err != nil {
		t.Fatal(err)
	}
	if ctx.ModelAliases["high"] != "home-profile-id" {
		t.Fatalf("aliases = %+v", ctx.ModelAliases)
	}

	writeHomeProfileConfig(t, slug, "pi", "model_aliases:\n  high: medium\n  medium: vendor-id\n")
	_, err = colony.ResolveContext(repo)
	if err == nil || !strings.Contains(err.Error(), "not another alias") {
		t.Fatalf("err = %v", err)
	}
}

func TestHomeOnlyProfileOverlaysNATSAndAdapter(t *testing.T) {
	repo, slug := setupProfileColony(t)
	writeHomeProfileConfig(t, slug, "lab", "nats:\n  url: nats://lab:4222\n")
	homeDir, err := colony.HomeDir(slug)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Join(homeDir, "profiles", "lab", "adapters"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(homeDir, "profiles", "lab", "adapters", "pi.yaml"), []byte("binary: /opt/pi-nightly\napi_key_env: GEMINI_API_KEY\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	resetProcessProfile(t)
	colony.SetProcessProfile(colony.ProfileSelection{Name: "lab", FlagSet: true})
	t.Setenv("PASEKA_NATS_URL", "")
	ctx, err := colony.ResolveContext(repo)
	if err != nil {
		t.Fatal(err)
	}
	if ctx.Home.NATS.EffectiveURL() != "nats://lab:4222" {
		t.Fatalf("nats = %q", ctx.Home.NATS.EffectiveURL())
	}
	if ctx.Pi.Binary != "/opt/pi-nightly" || ctx.Pi.APIKeyEnv != "GEMINI_API_KEY" {
		t.Fatalf("pi overlay = %+v", ctx.Pi)
	}
	t.Setenv("PASEKA_NATS_URL", "nats://env:4222")
	if ctx.Home.NATS.EffectiveURL() != "nats://env:4222" {
		t.Fatalf("env should still win, got %q", ctx.Home.NATS.EffectiveURL())
	}
}

func TestStickyAndEnvAndNoProfileSelection(t *testing.T) {
	repo, slug := setupProfileColony(t)
	writeColonyProfile(t, repo, "pi", "adapter: pi\n")
	homeDir, err := colony.HomeDir(slug)
	if err != nil {
		t.Fatal(err)
	}
	cfg := "colony_root: " + repo + "\nslug: " + slug + "\nprofile: pi\nnats:\n  url: nats://127.0.0.1:4222\n"
	if err := os.WriteFile(filepath.Join(homeDir, "config.yaml"), []byte(cfg), 0o644); err != nil {
		t.Fatal(err)
	}

	resetProcessProfile(t)
	ctx, err := colony.ResolveContext(repo)
	if err != nil {
		t.Fatal(err)
	}
	if ctx.Profile != "pi" {
		t.Fatalf("sticky profile = %q", ctx.Profile)
	}

	t.Setenv("PASEKA_PROFILE", "missing-env")
	_, err = colony.ResolveContext(repo)
	if err == nil || !strings.Contains(err.Error(), "missing-env") {
		t.Fatalf("env should override sticky, err=%v", err)
	}

	t.Setenv("PASEKA_PROFILE", "")
	colony.SetProcessProfile(colony.ProfileSelection{NoProfile: true})
	ctx, err = colony.ResolveContext(repo)
	if err != nil {
		t.Fatal(err)
	}
	if ctx.Profile != "" {
		t.Fatalf("--no-profile still loaded %q", ctx.Profile)
	}
}

func TestLoadColonyRejectsStickyProfile(t *testing.T) {
	dir := t.TempDir()
	pasekaDir := filepath.Join(dir, ".paseka")
	if err := os.MkdirAll(pasekaDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(pasekaDir, "colony.yaml"), []byte("slug: x\nprofile: pi\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	_, err := colony.LoadColony(dir)
	if err == nil || !strings.Contains(err.Error(), "home config.yaml") {
		t.Fatalf("err = %v", err)
	}
}

func TestCommittedLoadBeeUnchangedWhenProfileSelected(t *testing.T) {
	repo, _ := setupProfileColony(t)
	writeColonyProfile(t, repo, "pi", "adapter: pi\n")
	resetProcessProfile(t)
	colony.SetProcessProfile(colony.ProfileSelection{Name: "pi", FlagSet: true})
	if _, err := colony.ResolveContext(repo); err != nil {
		t.Fatal(err)
	}
	bee, _, err := colony.LoadBee(repo, "scout")
	if err != nil {
		t.Fatal(err)
	}
	name, err := bee.ResolveAdapter()
	if err != nil {
		t.Fatal(err)
	}
	if name != "cursor" {
		t.Fatalf("committed LoadBee should stay cursor, got %q", name)
	}
}

func resetProcessProfile(t *testing.T) {
	t.Helper()
	colony.SetProcessProfile(colony.ProfileSelection{})
	t.Cleanup(func() { colony.SetProcessProfile(colony.ProfileSelection{}) })
}

func setupProfileColony(t *testing.T) (repo, slug string) {
	t.Helper()
	resetProcessProfile(t)
	repo = t.TempDir()
	runGitProfile(t, repo, "init")
	runGitProfile(t, repo, "config", "user.email", "test@test.com")
	runGitProfile(t, repo, "config", "user.name", "test")
	runGitProfile(t, repo, "commit", "--allow-empty", "-m", "init")
	slug = "profile-test"
	paseka := filepath.Join(repo, ".paseka")
	if err := os.MkdirAll(filepath.Join(paseka, "bees"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(paseka, "colony.yaml"), []byte("slug: "+slug+"\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	mustWriteProfile(t, filepath.Join(paseka, "bees", "scout.yaml"), "role: scout\nadapter: cursor\n")
	mustWriteProfile(t, filepath.Join(paseka, "bees", "builder.yaml"), "role: builder\nadapter: cursor\n")
	mustWriteProfile(t, filepath.Join(paseka, "bees", "oracle-guard.yaml"), "role: oracle-guard\nadapter: script\ncommand: /bin/true\nrun_summary: disabled\n")
	mustWriteProfile(t, filepath.Join(paseka, "bees", "custom.yaml"), "role: custom\nadapter: cursor\ncommand: /bin/echo\n")

	xdg := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", xdg)
	t.Setenv("PASEKA_PROFILE", "")
	t.Setenv("PASEKA_NATS_URL", "")
	homeDir := filepath.Join(xdg, "paseka", slug)
	if err := os.MkdirAll(filepath.Join(homeDir, "adapters"), 0o755); err != nil {
		t.Fatal(err)
	}
	cfg := "colony_root: " + repo + "\nslug: " + slug + "\nnats:\n  url: nats://127.0.0.1:4222\n"
	if err := os.WriteFile(filepath.Join(homeDir, "config.yaml"), []byte(cfg), 0o644); err != nil {
		t.Fatal(err)
	}
	return repo, slug
}

func writeColonyProfile(t *testing.T, repo, name, body string) {
	t.Helper()
	path := filepath.Join(repo, ".paseka", "profiles", name+".yaml")
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
}

func writeHomeProfileConfig(t *testing.T, slug, name, body string) {
	t.Helper()
	homeDir, err := colony.HomeDir(slug)
	if err != nil {
		t.Fatal(err)
	}
	dir := filepath.Join(homeDir, "profiles", name)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "config.yaml"), []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
}

func mustWriteProfile(t *testing.T, path, body string) {
	t.Helper()
	if err := os.WriteFile(path, []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
}

func runGitProfile(t *testing.T, dir string, args ...string) {
	t.Helper()
	cmd := exec.Command("git", args...)
	cmd.Dir = dir
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("git %v: %v\n%s", args, err, out)
	}
}
