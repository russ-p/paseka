package main

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/russ-p/paseka/internal/colony"
	"github.com/russ-p/paseka/internal/hiveview"
)

func TestCLIProfileFlagOverridesSticky(t *testing.T) {
	repo, _ := setupProfileStatus(t, "opencode")
	writeStatusProfile(t, repo, "pi", "adapter: pi\n")
	writeStatusProfile(t, repo, "opencode", "adapter: opencode\n")

	snap := executeStatusJSON(t, "--profile", "pi", "status", "-C", repo, "--json")
	if snap.Profile != "pi" {
		t.Fatalf("profile = %q, want pi", snap.Profile)
	}
}

func TestCLIProfileFlagAfterSubcommand(t *testing.T) {
	repo, _ := setupProfileStatus(t, "opencode")
	writeStatusProfile(t, repo, "pi", "adapter: pi\n")
	writeStatusProfile(t, repo, "opencode", "adapter: opencode\n")

	snap := executeStatusJSON(t, "status", "-C", repo, "--json", "--profile", "pi")
	if snap.Profile != "pi" {
		t.Fatalf("profile after subcommand = %q, want pi", snap.Profile)
	}
}

func TestCLIEnvProfileAndFlagWins(t *testing.T) {
	repo, _ := setupProfileStatus(t, "")
	writeStatusProfile(t, repo, "pi", "adapter: pi\n")
	writeStatusProfile(t, repo, "opencode", "adapter: opencode\n")
	t.Setenv("PASEKA_PROFILE", "opencode")

	snap := executeStatusJSON(t, "status", "-C", repo, "--json")
	if snap.Profile != "opencode" {
		t.Fatalf("env profile = %q, want opencode", snap.Profile)
	}

	snap = executeStatusJSON(t, "status", "-C", repo, "--json", "--profile", "pi")
	if snap.Profile != "pi" {
		t.Fatalf("flag should win over env, got %q", snap.Profile)
	}
}

func TestCLINoProfileIgnoresSticky(t *testing.T) {
	repo, _ := setupProfileStatus(t, "pi")
	writeStatusProfile(t, repo, "pi", "adapter: pi\n")

	snap := executeStatusJSON(t, "--no-profile", "status", "-C", repo, "--json")
	if snap.Profile != "" {
		t.Fatalf("profile = %q, want empty", snap.Profile)
	}

	snap = executeStatusJSON(t, "status", "-C", repo, "--json", "--no-profile")
	if snap.Profile != "" {
		t.Fatalf("no-profile after subcommand = %q, want empty", snap.Profile)
	}
}

func TestCLIProfileConflictAndEmpty(t *testing.T) {
	repo, _ := setupProfileStatus(t, "")

	err := executeStatusErr(t, "--no-profile", "--profile", "pi", "status", "-C", repo)
	if err == nil || !strings.Contains(err.Error(), "cannot be combined") {
		t.Fatalf("conflict err = %v", err)
	}

	err = executeStatusErr(t, "status", "-C", repo, "--no-profile", "--profile", "pi")
	if err == nil || !strings.Contains(err.Error(), "cannot be combined") {
		t.Fatalf("conflict after subcommand err = %v", err)
	}

	err = executeStatusErr(t, "--profile", "", "status", "-C", repo)
	if err == nil || !strings.Contains(err.Error(), "--profile requires a name") {
		t.Fatalf("empty profile err = %v", err)
	}
}

func TestCLIUnknownProfileFailsClosed(t *testing.T) {
	repo, _ := setupProfileStatus(t, "")
	writeStatusProfile(t, repo, "pi", "adapter: pi\n")

	err := executeStatusErr(t, "status", "-C", repo, "--profile", "nope")
	if err == nil || !strings.Contains(err.Error(), `profile "nope" not found`) {
		t.Fatalf("err = %v", err)
	}
	if !strings.Contains(err.Error(), "pi") {
		t.Fatalf("expected available names, got %v", err)
	}
}

func TestDoctorPrintsProfileSection(t *testing.T) {
	repo, _ := setupProfileStatus(t, "")
	writeStatusProfile(t, repo, "pi", "adapter: pi\n")

	root := newRoot()
	var out bytes.Buffer
	root.SetOut(&out)
	root.SetErr(&out)
	root.SetArgs([]string{"doctor", "-C", repo, "--profile", "pi"})
	_ = root.Execute()
	got := out.String()
	if !strings.Contains(got, "Profile") || !strings.Contains(got, "pi") {
		t.Fatalf("doctor output missing profile:\n%s", got)
	}
}

func setupProfileStatus(t *testing.T, sticky string) (repo, homeDir string) {
	t.Helper()
	t.Cleanup(func() { colony.SetProcessProfile(colony.ProfileSelection{}) })
	repo = initStatusFixtureRepo(t)
	setupStatusCLIHome(t, repo, "")
	t.Setenv("PASEKA_PROFILE", "")
	homeDir = filepath.Join(os.Getenv("XDG_CONFIG_HOME"), "paseka", "topology-fixture")
	if sticky != "" {
		cfg, err := os.ReadFile(filepath.Join(homeDir, "config.yaml"))
		if err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(homeDir, "config.yaml"), append(cfg, []byte("profile: "+sticky+"\n")...), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	return repo, homeDir
}

func executeStatusJSON(t *testing.T, args ...string) hiveview.ColonySnapshot {
	t.Helper()
	root := newRoot()
	var out bytes.Buffer
	root.SetOut(&out)
	root.SetErr(&out)
	root.SetArgs(args)
	if err := root.Execute(); err != nil {
		t.Fatalf("execute %v: %v\n%s", args, err, out.String())
	}
	var snap hiveview.ColonySnapshot
	if err := json.Unmarshal(out.Bytes(), &snap); err != nil {
		t.Fatalf("json %v: %v\n%s", args, err, out.String())
	}
	return snap
}

func executeStatusErr(t *testing.T, args ...string) error {
	t.Helper()
	root := newRoot()
	var out bytes.Buffer
	root.SetOut(&out)
	root.SetErr(&out)
	root.SetArgs(args)
	return root.Execute()
}

func writeStatusProfile(t *testing.T, repo, name, body string) {
	t.Helper()
	path := filepath.Join(repo, ".paseka", "profiles", name+".yaml")
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
}
