package gitroot

import "testing"

func TestIsolatedWorktreeBranch(t *testing.T) {
	got := IsolatedWorktreeBranch("trace/abc def")
	if got != "paseka/trace-abc-def" {
		t.Fatalf("got %q", got)
	}
}

func TestCheckRefFormatRejectsHelpFlag(t *testing.T) {
	if err := CheckRefFormat("--help"); err == nil {
		t.Fatal("expected --help to fail as a ref, not print git help")
	}
}

func TestCheckRefFormatAcceptsFeature(t *testing.T) {
	if err := CheckRefFormat("feature/live-bees"); err != nil {
		t.Fatal(err)
	}
}

func TestLocalBranchExists(t *testing.T) {
	if LocalBranchExists(t.TempDir(), "main") {
		t.Fatal("non-repo must not report a local branch")
	}
}
