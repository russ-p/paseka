// Package version reports the build stamp of a running Paseka binary: which
// release it is, which commit it was cut from, and whether that commit was a
// clean tree.
//
// The stamp has two sources, and the linker wins wherever both answer. A release
// build is stamped with -ldflags, which is the only way to learn a version that
// the build system resolved from a tag. Everything else — a `go build` in a
// worktree, `go install …@main` — leaves the linker empty, and the Go toolchain
// has already embedded the commit in the binary's own build info, so the answer
// still exists when nobody passed a flag. That is what lets an unreleased build
// from main identify itself instead of answering "dev" and nothing else.
//
// The linker writes these as -X github.com/russ-p/paseka/internal/version.version
// and friends. The full import path is required: `-X main.version` only reaches
// package main, and the console's API lives in another package entirely.
package version

import (
	"regexp"
	"runtime/debug"
	"strconv"
	"strings"
	"sync"
)

// The linker writes these at build time. They are empty in every build that did
// not ask for a stamp, which is the case the build-info fallback exists for.
var (
	version string
	commit  string
	date    string
	dirty   string
)

// repository is where this project's source lives, so a console can link a build
// to the commit behind it. It is stamped rather than derived because the module
// path is not the repository URL — a fork ships its own binary, and its console
// should not offer a link into someone else's tree.
var repository = "https://github.com/russ-p/paseka"

// Repository is the source URL for this build.
func Repository() string {
	return strings.TrimSuffix(strings.TrimSpace(repository), "/")
}

// devVersion is what an unstamped build reports. It is a name for "nobody said",
// not a version.
const devVersion = "dev"

// develVersion is what the toolchain reports for the main module of a local
// build, and means exactly as little as it looks.
const develVersion = "(devel)"

// pseudoVersionRE matches the module pseudo-version the toolchain stamps for a
// build from a checkout, and the one `go install …@main` resolves: a base
// version, a `-0.<timestamp>` separator, and the short commit it was built at.
// The trailing `+dirty` is build metadata the toolchain adds for an uncommitted
// tree, which `vcs.modified` reports more precisely.
var pseudoVersionRE = regexp.MustCompile(`^v?(.+?)-0\.\d{14}-([0-9a-f]{12})(\+.+)?$`)

// Info is the build stamp, as the CLI and the console API report it.
type Info struct {
	// Version is the release version without the `v` (`0.5.0`), a snapshot name
	// for a non-release build, or `dev` when nothing stamped it.
	Version string `json:"version"`
	// Commit is the full git sha the binary was built from, when it is known.
	Commit string `json:"commit,omitempty"`
	// Date is the commit date in RFC 3339, not the moment of the build: two
	// builds of one commit have to compare equal.
	Date string `json:"date,omitempty"`
	// Dirty reports uncommitted work in the tree the binary was built from,
	// which turns the commit into a starting point rather than a claim.
	Dirty bool `json:"dirty,omitempty"`
	// Released is true only for a build off a version tag. An operator comparing
	// two consoles needs to know whether either is a release at all.
	Released bool `json:"released"`
}

// cachedStamp reads the binary's own build info once. It cannot change while the
// process lives, and a console left open for days polls `/api/version`.
var cachedStamp = sync.OnceValue(func() Info {
	info, ok := debug.ReadBuildInfo()
	if !ok {
		return resolve(linkerStamp(), nil)
	}
	return resolve(linkerStamp(), info)
})

// Get returns the stamp of this binary.
func Get() Info {
	return cachedStamp()
}

const shortCommitLen = 7

// CommitURL is where this build's commit can be read. It is empty without a
// commit rather than pointing at the repository root: a link that is not the
// commit an operator is looking for is worse than no link at all.
func (i Info) CommitURL() string {
	if i.Commit == "" {
		return ""
	}
	return Repository() + "/commit/" + i.Commit
}

// ShortCommit is the abbreviated sha every git surface shows: seven characters,
// or the whole thing when the source gave fewer.
func (i Info) ShortCommit() string {
	if len(i.Commit) <= shortCommitLen {
		return i.Commit
	}
	return i.Commit[:shortCommitLen]
}

// String is the one line a human reads: the version, the commit behind it, and
// whether the tree was dirty. The commit rides in semver build metadata
// (`0.5.0+g67730c4`), so the line still parses as the version it names.
func (i Info) String() string {
	name := i.Version
	if name == "" {
		name = devVersion
	}
	// GoReleaser already names a snapshot after its commit, and repeating it would
	// read as two builds rather than one.
	commit := ""
	if i.Commit != "" && !strings.Contains(name, i.ShortCommit()) {
		commit = "+" + i.ShortCommit()
	}
	switch {
	case commit != "" && i.Dirty:
		return name + commit + ".dirty"
	case commit != "":
		return name + commit
	case i.Dirty:
		return name + "-dirty"
	default:
		return name
	}
}

// stamp is the linker's answer, verbatim, before anything is read from it.
type stamp struct {
	version string
	commit  string
	date    string
	dirty   string
}

func linkerStamp() stamp {
	return stamp{version: version, commit: commit, date: date, dirty: dirty}
}

// resolve merges the two sources and normalizes the result. Each field falls
// back on its own, so a build that stamped only a version still reports the
// commit the toolchain knew.
func resolve(s stamp, info *debug.BuildInfo) Info {
	out := Info{
		Version: strings.TrimSpace(s.version),
		Commit:  strings.TrimSpace(s.commit),
		Date:    strings.TrimSpace(s.date),
	}
	if parsed, err := strconv.ParseBool(strings.TrimSpace(s.dirty)); err == nil {
		out.Dirty = parsed
	}
	if info != nil {
		fill(&out, info)
	}
	out.Version = normalizeVersion(out.Version)
	out.Released = isRelease(out.Version)
	return out
}

// fill takes from the embedded build info only what the linker left blank.
//
// The commit prefers the full `vcs.revision` over a pseudo-version's abbreviated
// one, so the settings are read first: they are the same commit, described
// better, and the pseudo-version's copy is the fallback for a build that has no
// checkout to read them from.
func fill(out *Info, info *debug.BuildInfo) {
	moduleVersion, moduleCommit := parseModuleVersion(info.Main.Version)
	if out.Version == "" {
		out.Version = moduleVersion
	}
	for _, setting := range info.Settings {
		switch setting.Key {
		case "vcs.revision":
			if out.Commit == "" {
				out.Commit = setting.Value
			}
		case "vcs.time":
			if out.Date == "" {
				out.Date = setting.Value
			}
		case "vcs.modified":
			// The toolchain read the tree itself, so its answer beats the
			// linker's when both exist: it cannot describe a different checkout.
			out.Dirty = setting.Value == "true"
		}
	}
	if out.Commit == "" {
		out.Commit = moduleCommit
	}
}

// parseModuleVersion splits a module version into a base version and the commit
// it resolved at. `go install …@main` builds from the module cache rather than a
// checkout, so `vcs.revision` is absent and the pseudo-version is the only place
// the commit survives.
//
// A pseudo-version yields no version at all. Its base is the version the *next*
// release would carry — `v0.5.1-0.20261001054351-67730c4` is 53 commits past
// v0.5.0, so the base `0.5.1` describes a release that does not exist. Reporting
// it would put an unreleased main build on the same line as a shipped tag, which
// is exactly the confusion the stamp exists to prevent; the commit is what
// identifies the build, and `dev` is what it honestly calls its version.
func parseModuleVersion(moduleVersion string) (version, commit string) {
	if moduleVersion == "" || moduleVersion == develVersion {
		return "", ""
	}
	if match := pseudoVersionRE.FindStringSubmatch(moduleVersion); match != nil {
		return "", match[2]
	}
	return moduleVersion, ""
}

// normalizeVersion reduces every spelling of "no version" to one word, and drops
// the `v` so the API, the CLI, and the release archive name agree.
func normalizeVersion(raw string) string {
	if raw == "" || raw == develVersion {
		return devVersion
	}
	if rest, ok := strings.CutPrefix(raw, "v"); ok && rest != "" && rest[0] >= '0' && rest[0] <= '9' {
		return rest
	}
	return raw
}

// isRelease reports whether the version names a tag rather than a moving target.
// GoReleaser writes `0.5.0` for a tag and a `SNAPSHOT` name for
// `--snapshot`; an unstamped build is `dev`. Neither of those claims to be a
// release, and an operator comparing two consoles needs the difference.
func isRelease(version string) bool {
	if version == devVersion {
		return false
	}
	return !strings.Contains(version, "SNAPSHOT")
}
