package console

import (
	"fmt"
	"net/http"
	"runtime"

	"github.com/russ-p/paseka/internal/version"
)

// BuildView is the build stamp behind `GET /api/version`. It answers a question
// only this process can: which Paseka is answering. The commit matters because
// an operator runs several — a release, a build from main, a `go install` — and
// a console that does not name its own build cannot be told apart from another
// one in a bug report.
type BuildView struct {
	// Version is the release version, or `dev` for a build nothing stamped.
	Version string `json:"version"`
	// Commit is the full sha behind the version; absent when unknown.
	Commit string `json:"commit,omitempty"`
	// ShortCommit is the abbreviated form an operator reads and pastes.
	ShortCommit string `json:"shortCommit,omitempty"`
	// Date is the commit date in RFC 3339, not the build time, so two builds of
	// one commit compare equal.
	Date string `json:"date,omitempty"`
	// Dirty marks a tree with uncommitted work, which makes the commit a
	// starting point rather than a claim about what shipped.
	Dirty bool `json:"dirty,omitempty"`
	// Released separates a tagged build from a snapshot or a development one.
	Released bool `json:"released"`
	// Display is the one line to show: version, commit, and dirty state.
	Display string `json:"display"`
	// Repository is where this build's source lives, so the console can offer a
	// link instead of a bare sha.
	Repository string `json:"repository"`
	// CommitURL is this build's commit on that repository, absent when there is no
	// commit to point at.
	CommitURL string `json:"commitUrl,omitempty"`
	// GoVersion is the toolchain that compiled this binary, which is the other
	// half of "works on my box".
	GoVersion string `json:"goVersion"`
}

// buildView is the answer the console serves. It is derived from the binary
// itself rather than from the colony, so it stays correct after the working
// directory has moved on: what is running is what is stamped, not what the
// checkout the operator happens to be standing in says.
func buildView() BuildView {
	info := version.Get()
	return BuildView{
		Version:     info.Version,
		Commit:      info.Commit,
		ShortCommit: info.ShortCommit(),
		Date:        info.Date,
		Dirty:       info.Dirty,
		Released:    info.Released,
		Display:     info.String(),
		Repository:  version.Repository(),
		CommitURL:   info.CommitURL(),
		GoVersion:   runtime.Version(),
	}
}

// buildLine is the stamp as the startup banner carries it. The channel is spelled
// out rather than left to the reader of a version string, because the question a
// banner answers is "am I about to open a release or something built after one",
// and `dev+67730c4.dirty` does not say that on its own.
func buildLine() string {
	info := version.Get()
	channel := "development build"
	if info.Released {
		channel = "release"
	}
	return fmt.Sprintf("Build: %s (%s)", info, channel)
}

func (a *api) handleVersion(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	writeJSON(w, buildView())
}
