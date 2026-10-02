package console

import (
	"strings"
	"testing"

	"github.com/russ-p/paseka/internal/version"
)

func TestBuildLineSpellsOutTheChannel(t *testing.T) {
	// A banner answers "am I about to open a release or something built after
	// one", and a version string alone does not say that: `dev+67730c4.dirty`
	// reads as a name until the words beside it say otherwise.
	line := buildLine()

	if !strings.HasPrefix(line, "Build: ") {
		t.Fatalf("buildLine() = %q", line)
	}
	if !strings.Contains(line, version.Get().String()) {
		t.Fatalf("buildLine() = %q, want the stamp %q", line, version.Get().String())
	}
	want := "(development build)"
	if version.Get().Released {
		want = "(release)"
	}
	if !strings.Contains(line, want) {
		t.Fatalf("buildLine() = %q, want %q", line, want)
	}
}

func TestStartupBannerCarriesTheBuild(t *testing.T) {
	lines := startupBanner("127.0.0.1:8787")

	if len(lines) != 3 {
		t.Fatalf("banner = %q", lines)
	}
	if !strings.Contains(lines[0], "Queen Console") || !strings.Contains(lines[0], "http://127.0.0.1:8787") {
		t.Fatalf("first line = %q", lines[0])
	}
	// The build is on the banner and not only in the UI: the process about to hold
	// a port is the one an operator has to name in a bug report an hour later, and
	// scrollback is the last place anybody looks for it.
	if !strings.Contains(lines[1], buildLine()) {
		t.Fatalf("second line = %q, want the build line", lines[1])
	}
	if !strings.HasPrefix(lines[1], "  ") {
		t.Fatalf("second line = %q, want it indented under the header", lines[1])
	}
	if lines[2] != "  Redesign preview: http://127.0.0.1:8787/next/" {
		t.Fatalf("third line = %q", lines[2])
	}
}
