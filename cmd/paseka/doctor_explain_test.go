package main

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// TestDoctorExplainGolden pins the resolved launch plan and per-field layer
// attribution for a bee whose settings come from three different layers: a
// committed `binary`, an aliased `model`, and a defaulted `yolo`. It also pins
// that an unmapped reasoning value is reported rather than silently dropped.
func TestDoctorExplainGolden(t *testing.T) {
	repo := initExplainFixtureRepo(t)
	setupExplainHome(t, repo)

	root := newRoot()
	var out bytes.Buffer
	root.SetOut(&out)
	root.SetErr(&out)
	root.SetArgs([]string{"doctor", "-C", repo, "--explain", "builder"})
	if err := root.Execute(); err != nil {
		t.Fatalf("execute: %v\n%s", err, out.String())
	}

	got := out.String()
	want := readExplainGolden(t)
	if got != want {
		t.Fatalf("doctor --explain mismatch:\n--- want ---\n%s\n--- got ---\n%s", want, got)
	}
}

// TestDoctorExplainNamesTheLayerThatSetEachField guards the golden's intent in
// prose: the three-layer bee must not collapse every field onto one layer.
func TestDoctorExplainNamesTheLayerThatSetEachField(t *testing.T) {
	repo := initExplainFixtureRepo(t)
	setupExplainHome(t, repo)

	root := newRoot()
	var out bytes.Buffer
	root.SetOut(&out)
	root.SetErr(&out)
	root.SetArgs([]string{"doctor", "-C", repo, "--explain", "builder"})
	if err := root.Execute(); err != nil {
		t.Fatalf("execute: %v\n%s", err, out.String())
	}
	got := out.String()

	for _, want := range []string{
		"binary:", "[bee]",
		"model:", "[alias]",
		"yolo:", "[default]",
		"reasoning", "unmapped",
	} {
		if !strings.Contains(got, want) {
			t.Fatalf("doctor --explain output missing %q:\n%s", want, got)
		}
	}
	if !strings.Contains(got, "/repo/agent") {
		t.Fatalf("committed binary did not win over the machine file:\n%s", got)
	}
	if !strings.Contains(got, "openai/gpt-5.5") {
		t.Fatalf("alias was not resolved into the plan:\n%s", got)
	}
}

func initExplainFixtureRepo(t *testing.T) string {
	t.Helper()
	t.Setenv("PASEKA_PROFILE", "")
	dir := t.TempDir()
	runStatusGit(t, dir, "init")
	runStatusGit(t, dir, "config", "user.email", "test@test.com")
	runStatusGit(t, dir, "config", "user.name", "test")

	paseka := filepath.Join(dir, ".paseka")
	if err := os.MkdirAll(filepath.Join(paseka, "bees"), 0o755); err != nil {
		t.Fatal(err)
	}
	colonyYAML := "slug: explain-fixture\nmodel_aliases:\n  big: openai/gpt-5.5\n"
	if err := os.WriteFile(filepath.Join(paseka, "colony.yaml"), []byte(colonyYAML), 0o644); err != nil {
		t.Fatal(err)
	}
	// binary, model and reasoning are declared by the bee; yolo is not, so it
	// falls to the adapter default. model is an alias key, so the alias layer
	// decides the final value.
	beeYAML := "role: builder\nadapter: cursor\n" +
		"params:\n  binary: /repo/agent\n  model: big\n  reasoning: high\n"
	if err := os.WriteFile(filepath.Join(paseka, "bees", "builder.yaml"), []byte(beeYAML), 0o644); err != nil {
		t.Fatal(err)
	}
	runStatusGit(t, dir, "add", ".paseka")
	runStatusGit(t, dir, "commit", "-m", "explain fixture")
	return dir
}

func setupExplainHome(t *testing.T, repo string) {
	t.Helper()
	home := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", home)
	t.Setenv("CURSOR_API_KEY", "")
	homeDir := filepath.Join(home, "paseka", "explain-fixture")
	if err := os.MkdirAll(filepath.Join(homeDir, "adapters"), 0o755); err != nil {
		t.Fatal(err)
	}
	cfg := "colony_root: " + repo + "\nslug: explain-fixture\n"
	if err := os.WriteFile(filepath.Join(homeDir, "config.yaml"), []byte(cfg), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(homeDir, "state.json"), []byte("{}\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	// The machine file declares a binary, which proves precedence: the committed
	// `/repo/agent` must still win and be attributed to the bee.
	if err := os.WriteFile(filepath.Join(homeDir, "adapters", "cursor.yaml"), []byte("binary: /machine/agent\n"), 0o644); err != nil {
		t.Fatal(err)
	}
}

func readExplainGolden(t *testing.T) string {
	t.Helper()
	raw, err := os.ReadFile(filepath.Join("testdata", "doctor-explain.golden"))
	if err != nil {
		t.Fatalf("read golden: %v", err)
	}
	return string(raw)
}
