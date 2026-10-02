package main

import (
	"bytes"
	"encoding/json"
	"strings"
	"testing"

	"github.com/russ-p/paseka/internal/version"
)

func runVersionCLI(t *testing.T, args ...string) string {
	t.Helper()
	root := newRoot()
	var out bytes.Buffer
	root.SetOut(&out)
	root.SetErr(&out)
	root.SetArgs(append([]string{"version"}, args...))
	if err := root.Execute(); err != nil {
		t.Fatalf("execute: %v\n%s", err, out.String())
	}
	return out.String()
}

func TestVersionCLIReportsTheBuild(t *testing.T) {
	out := runVersionCLI(t)

	if !strings.Contains(out, "paseka ") {
		t.Fatalf("output = %q", out)
	}
	// The lines an operator needs before filing a report: the build, whether it is
	// a release, and the toolchain.
	for _, line := range []string{"channel:", "go:"} {
		if !strings.Contains(out, line) {
			t.Fatalf("output %q has no %q line", out, line)
		}
	}
	info := version.Get()
	// A commit and a date are printed only when something knows them: `go test`
	// binaries carry no VCS stamp, and a line reading `commit:` with nothing after
	// it is worse than the line's absence.
	if info.Commit != "" && !strings.Contains(out, "commit:  "+info.Commit) {
		t.Fatalf("output %q does not carry the commit %q", out, info.Commit)
	}
	if info.Date != "" && !strings.Contains(out, info.Date) {
		t.Fatalf("output %q does not carry the date %q", out, info.Date)
	}
	if !strings.Contains(out, "paseka "+info.String()) {
		t.Fatalf("output %q does not carry the stamp %q", out, info.String())
	}
	if want := "channel: " + releaseChannel(info.Released); !strings.Contains(out, want) {
		t.Fatalf("output %q has no %q line", out, want)
	}
}

func TestReleaseChannelNamesTheTwoBuilds(t *testing.T) {
	// An operator has to be able to tell a shipped tag from anything built after
	// it, and "not a release" is the half of the answer that matters.
	if got := releaseChannel(true); got != "release" {
		t.Fatalf("releaseChannel(true) = %q", got)
	}
	if got := releaseChannel(false); !strings.Contains(got, "not a tagged release") {
		t.Fatalf("releaseChannel(false) = %q", got)
	}
}

func TestVersionCLIJSONCarriesTheStamp(t *testing.T) {
	out := runVersionCLI(t, "--json")

	var got struct {
		Version   string `json:"version"`
		Commit    string `json:"commit"`
		Date      string `json:"date"`
		Dirty     bool   `json:"dirty"`
		Released  bool   `json:"released"`
		GoVersion string `json:"goVersion"`
	}
	if err := json.Unmarshal([]byte(out), &got); err != nil {
		t.Fatalf("parse %q: %v", out, err)
	}
	info := version.Get()
	if got.Version != info.Version {
		t.Fatalf("version = %q, want %q", got.Version, info.Version)
	}
	if got.Commit != info.Commit {
		t.Fatalf("commit = %q, want %q", got.Commit, info.Commit)
	}
	if got.Released != info.Released {
		t.Fatalf("released = %v, want %v", got.Released, info.Released)
	}
	if got.GoVersion == "" {
		t.Fatal("goVersion is empty")
	}
	// The JSON is a machine contract, so it must not carry the prose the text
	// form wraps the same value in.
	if strings.Contains(out, "channel:") {
		t.Fatalf("json output has the text channel line: %q", out)
	}
}

func TestVersionCLINeedsNoColony(t *testing.T) {
	// `paseka version` must answer outside a repository: the case that needs it is
	// an operator whose checkout is broken or missing.
	t.Chdir(t.TempDir())
	t.Setenv("PASEKA_PROFILE", "")

	out := runVersionCLI(t)

	if !strings.Contains(out, "paseka ") {
		t.Fatalf("output = %q", out)
	}
}
