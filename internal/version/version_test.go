package version

import (
	"encoding/json"
	"runtime/debug"
	"strings"
	"testing"
)

func buildSetting(key, value string) debug.BuildSetting {
	return debug.BuildSetting{Key: key, Value: value}
}

func TestResolvePrefersTheLinker(t *testing.T) {
	// A release build answers from -ldflags even though the toolchain also
	// stamped the tree it was built from: the tag is knowledge the build system
	// has and the toolchain does not.
	got := resolve(
		stamp{version: "0.6.0", commit: "abc123def456", date: "2026-10-01T09:00:00Z", dirty: "false"},
		&debug.BuildInfo{
			Main: debug.Module{Version: develVersion},
			Settings: []debug.BuildSetting{
				buildSetting("vcs.revision", "9999999999999999999999999999999999999999"),
				buildSetting("vcs.time", "2020-01-01T00:00:00Z"),
				buildSetting("vcs.modified", "true"),
			},
		},
	)

	if got.Version != "0.6.0" {
		t.Fatalf("version = %q", got.Version)
	}
	if got.Commit != "abc123def456" {
		t.Fatalf("commit = %q", got.Commit)
	}
	if got.Date != "2026-10-01T09:00:00Z" {
		t.Fatalf("date = %q", got.Date)
	}
	// The toolchain read the tree the linker described, so it has the last word
	// on whether that tree was dirty.
	if !got.Dirty {
		t.Fatal("dirty = false, want the toolchain's vcs.modified to win")
	}
	if !got.Released {
		t.Fatal("released = false for a tagged build")
	}
}

func TestResolveFallsBackToBuildInfo(t *testing.T) {
	// `go build` in a worktree passes no flags at all, and that build still has
	// to name its commit — this is the unreleased-from-main case.
	got := resolve(stamp{}, &debug.BuildInfo{
		Main: debug.Module{Version: develVersion},
		Settings: []debug.BuildSetting{
			buildSetting("vcs.revision", "67730c4a1b2c3d4e5f60718293a4b5c6d7e8f901"),
			buildSetting("vcs.time", "2026-09-30T12:04:22Z"),
			buildSetting("vcs.modified", "false"),
		},
	})

	if got.Version != devVersion {
		t.Fatalf("version = %q, want %q", got.Version, devVersion)
	}
	if got.Commit != "67730c4a1b2c3d4e5f60718293a4b5c6d7e8f901" {
		t.Fatalf("commit = %q", got.Commit)
	}
	if got.Date != "2026-09-30T12:04:22Z" {
		t.Fatalf("date = %q", got.Date)
	}
	if got.Dirty {
		t.Fatal("dirty = true")
	}
	if got.Released {
		t.Fatal("released = true for an unstamped build")
	}
}

func TestResolveFillsEachFieldOnItsOwn(t *testing.T) {
	// A build that stamped only a version still learns its commit, which is what
	// keeps one missing -X from hiding the most useful half of the answer.
	got := resolve(stamp{version: "0.6.1"}, &debug.BuildInfo{
		Main: debug.Module{Version: develVersion},
		Settings: []debug.BuildSetting{
			buildSetting("vcs.revision", "abcdef1234567890"),
			buildSetting("vcs.time", "2026-10-02T08:00:00Z"),
		},
	})

	if got.Version != "0.6.1" {
		t.Fatalf("version = %q", got.Version)
	}
	if got.Commit != "abcdef1234567890" {
		t.Fatalf("commit = %q", got.Commit)
	}
	if got.Date != "2026-10-02T08:00:00Z" {
		t.Fatalf("date = %q", got.Date)
	}
}

func TestResolveReadsThePseudoVersionOfAGoInstall(t *testing.T) {
	// `go install github.com/russ-p/paseka/cmd/paseka@main` builds from the
	// module cache, where there is no checkout to stamp: the commit survives only
	// inside the pseudo-version. Its base is the version the *next* release would
	// carry, so it must not be reported as this build's version — that would put a
	// development build on the same line as a shipped tag.
	got := resolve(stamp{}, &debug.BuildInfo{
		Main: debug.Module{Version: "v0.6.1-0.20261002081533-67730c4a1b2c"},
	})

	if got.Version != devVersion {
		t.Fatalf("version = %q, want %q", got.Version, devVersion)
	}
	if got.Commit != "67730c4a1b2c" {
		t.Fatalf("commit = %q", got.Commit)
	}
	if got.Released {
		t.Fatal("released = true for a pseudo-version")
	}
	if got.String() != "dev+67730c4" {
		t.Fatalf("string = %q", got.String())
	}
}

func TestResolveReadsThePseudoVersionOfALocalBuild(t *testing.T) {
	// Go 1.24 and later stamp a plain `go build` with a pseudo-version too, and
	// add `+dirty` build metadata for an uncommitted tree. The full `vcs.revision`
	// is the better description of the same commit, so it is the one that wins.
	got := resolve(stamp{}, &debug.BuildInfo{
		Main: debug.Module{Version: "v0.5.1-0.20261001054351-67730c4da70f+dirty"},
		Settings: []debug.BuildSetting{
			buildSetting("vcs.revision", "67730c4da70fbd912a575fe614e5e248c402fdf0"),
			buildSetting("vcs.time", "2026-10-01T05:43:51Z"),
			buildSetting("vcs.modified", "true"),
		},
	})

	if got.Version != devVersion {
		t.Fatalf("version = %q, want %q", got.Version, devVersion)
	}
	if got.Commit != "67730c4da70fbd912a575fe614e5e248c402fdf0" {
		t.Fatalf("commit = %q", got.Commit)
	}
	if !got.Dirty {
		t.Fatal("dirty = false")
	}
	if got.Released {
		t.Fatal("released = true")
	}
}

func TestResolveKeepsATaggedModuleVersion(t *testing.T) {
	// `go install …@v0.6.0` resolves a real tag, so the module version is the
	// release and says so — the same answer the linker would give.
	got := resolve(stamp{}, &debug.BuildInfo{
		Main: debug.Module{Version: "v0.6.0"},
	})

	if got.Version != "0.6.0" {
		t.Fatalf("version = %q", got.Version)
	}
	if !got.Released {
		t.Fatal("released = false for a tagged module version")
	}
	if got.Commit != "" {
		t.Fatalf("commit = %q, want none: a module version names no commit", got.Commit)
	}
}

func TestResolveNamesAnUnstampedBinary(t *testing.T) {
	// No linker, no build info: a `CGO_ENABLED=0 go build` with VCS stamping off,
	// or a binary built outside the module. It must still answer, because a
	// console endpoint that fails here is worse than one that says "dev".
	got := resolve(stamp{}, nil)

	if got.Version != devVersion {
		t.Fatalf("version = %q, want %q", got.Version, devVersion)
	}
	if got.String() != devVersion {
		t.Fatalf("string = %q", got.String())
	}
	if got.Released {
		t.Fatal("released = true")
	}
}

func TestResolveReadsTheLinkerDirtyFlag(t *testing.T) {
	if got := resolve(stamp{version: "0.6.0", dirty: "true"}, nil); !got.Dirty {
		t.Fatal("dirty = false, want the linker's own flag")
	}
	if got := resolve(stamp{version: "0.6.0", dirty: "false"}, nil); got.Dirty {
		t.Fatal("dirty = true")
	}
	// An unparseable flag is not a dirty tree, and guessing would be worse than
	// saying nothing.
	if got := resolve(stamp{version: "0.6.0", dirty: "yes please"}, nil); got.Dirty {
		t.Fatal("dirty = true for an unparseable flag")
	}
}

func TestResolveTreatsASnapshotAsUnreleased(t *testing.T) {
	// `goreleaser build --snapshot` names the build after the commit instead of a
	// tag. The UI has to be able to tell it apart from a release.
	got := resolve(stamp{version: "0.6.0-SNAPSHOT-67730c4"}, nil)

	if got.Released {
		t.Fatal("released = true for a snapshot")
	}
	if got.String() != "0.6.0-SNAPSHOT-67730c4" {
		t.Fatalf("string = %q", got.String())
	}
}

func TestNormalizeVersion(t *testing.T) {
	// The linker values arrive already trimmed by `resolve`, so this is about the
	// semantic normalization only.
	cases := []struct {
		in   string
		want string
	}{
		{"", devVersion},
		{develVersion, devVersion},
		{"0.6.0", "0.6.0"},
		{"v0.6.0", "0.6.0"},
		{"vault", "vault"},
	}
	for _, c := range cases {
		if got := normalizeVersion(c.in); got != c.want {
			t.Errorf("normalizeVersion(%q) = %q, want %q", c.in, got, c.want)
		}
	}
}

func TestInfoString(t *testing.T) {
	cases := []struct {
		name string
		info Info
		want string
	}{
		{"release", Info{Version: "0.6.0", Commit: "67730c4a1b2c"}, "0.6.0+67730c4"},
		{"release dirty", Info{Version: "0.6.0", Commit: "67730c4a1b2c", Dirty: true}, "0.6.0+67730c4.dirty"},
		{"unstamped", Info{Version: devVersion}, "dev"},
		{"unstamped dirty", Info{Version: devVersion, Dirty: true}, "dev-dirty"},
		{"empty", Info{}, "dev"},
		{"short commit", Info{Version: "0.6.0", Commit: "67730c4"}, "0.6.0+67730c4"},
		// GoReleaser names a snapshot after its own commit, so the line must not
		// print it twice and read as two builds.
		{"snapshot", Info{Version: "0.6.0-SNAPSHOT-67730c4", Commit: "67730c4a1b2c"}, "0.6.0-SNAPSHOT-67730c4"},
	}
	for _, c := range cases {
		if got := c.info.String(); got != c.want {
			t.Errorf("%s: String() = %q, want %q", c.name, got, c.want)
		}
	}
}

func TestShortCommit(t *testing.T) {
	if got := (Info{Commit: "67730c4a1b2c3d4e5f60718293a4b5c6d7e8f901"}).ShortCommit(); got != "67730c4" {
		t.Fatalf("ShortCommit() = %q", got)
	}
	// A pseudo-version's sha is twelve characters and shows as those twelve:
	// shortening it again would only lose what nothing shortened.
	if got := (Info{Commit: "67730c4a1b2c"}).ShortCommit(); got != "67730c4" {
		t.Fatalf("ShortCommit() = %q", got)
	}
	if got := (Info{}).ShortCommit(); got != "" {
		t.Fatalf("ShortCommit() = %q", got)
	}
}

func TestCommitURL(t *testing.T) {
	// The console offers a link rather than a bare sha, and a link to the
	// repository root is not the commit somebody is looking for — so an unstamped
	// build gets no link at all.
	if got := (Info{Commit: "67730c4da70fbd912a575fe614e5e248c402fdf0"}).CommitURL(); got != "https://github.com/russ-p/paseka/commit/67730c4da70fbd912a575fe614e5e248c402fdf0" {
		t.Fatalf("CommitURL() = %q", got)
	}
	if got := (Info{}).CommitURL(); got != "" {
		t.Fatalf("CommitURL() = %q, want none", got)
	}
}

func TestRepository(t *testing.T) {
	if got := Repository(); got != "https://github.com/russ-p/paseka" {
		t.Fatalf("Repository() = %q", got)
	}
	// A trailing slash would double up against the `/commit/` path.
	if got := Repository(); strings.HasSuffix(got, "/") {
		t.Fatalf("Repository() = %q, want no trailing slash", got)
	}
}

func TestGetAnswersSomething(t *testing.T) {
	// The test binary carries its own build info, so this asserts the shape of a
	// real answer rather than a fixed value — the commit under test is not the one
	// the assertions above were written against.
	got := Get()
	if got.Version == "" {
		t.Fatal("version is empty")
	}
	if got.String() == "" {
		t.Fatal("String() is empty")
	}
	if got.Released && got.Version == devVersion {
		t.Fatal("released is true for an unstamped build")
	}
}

func TestInfoJSONShape(t *testing.T) {
	// The console's TS type mirrors these names, and `omitempty` on the commit is
	// what keeps an unstamped build from advertising an empty sha.
	data, err := json.Marshal(Info{Version: "0.6.0"})
	if err != nil {
		t.Fatal(err)
	}
	const want = `{"version":"0.6.0","released":false}`
	if string(data) != want {
		t.Fatalf("json = %s, want %s", data, want)
	}
}
