package artifacts

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestShouldSkipName(t *testing.T) {
	cases := map[string]bool{
		".hidden":    true,
		"notes.md":   false,
		"foo~":       true,
		"foo.swp":    true,
		"foo.tmp":    true,
		"draft.md":   false,
		"risks.json": false,
	}
	for name, want := range cases {
		if got := ShouldSkipName(name); got != want {
			t.Fatalf("ShouldSkipName(%q) = %v, want %v", name, got, want)
		}
	}
}

func TestArtifactKindAndTitleHeuristics(t *testing.T) {
	if got := ArtifactKindFromRef("research/api.md"); got != "api" {
		t.Fatalf("kind = %q", got)
	}
	if got := ArtifactKindFromRef(".paseka/runs/t/artifacts/draft-spec.md"); got != "draft-spec" {
		t.Fatalf("kind = %q", got)
	}
	title := TitleFromMarkdown([]byte("# Research brief\n\nbody"))
	if title != "Research brief" {
		t.Fatalf("title = %q", title)
	}
	if got := TitleFromMarkdown([]byte("plain\n")); got != "plain" {
		t.Fatalf("title = %q", got)
	}
}

func TestScanSkipHiddenAndTemp(t *testing.T) {
	root := t.TempDir()
	traceID := "trace-scan"
	comb := Root(root, traceID)
	if err := os.MkdirAll(filepath.Join(comb, "nested"), 0o755); err != nil {
		t.Fatal(err)
	}
	files := map[string]string{
		"research.md":       "# Research\n",
		".hidden.md":        "x",
		"notes~":            "x",
		"nested/draft.md":   "body",
		"nested/.secret.md": "x",
		"skip.swp":          "x",
		"skip.tmp":          "x",
	}
	for name, body := range files {
		path := filepath.Join(comb, filepath.FromSlash(name))
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, []byte(body), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	hashes, err := Scan(root, traceID)
	if err != nil {
		t.Fatal(err)
	}
	if len(hashes) != 2 {
		t.Fatalf("hashes = %d, want 2: %v", len(hashes), hashes)
	}
	for ref := range hashes {
		if strings.Contains(ref, ".hidden") || strings.Contains(ref, "~") || strings.Contains(ref, ".swp") {
			t.Fatalf("unexpected ref %q", ref)
		}
	}
}

func TestResolveUnderCombRejectsEscape(t *testing.T) {
	root := t.TempDir()
	traceID := "trace-escape"
	comb := Root(root, traceID)
	if err := os.MkdirAll(comb, 0o755); err != nil {
		t.Fatal(err)
	}
	if _, err := ResolveUnderComb(root, traceID, "../../../etc/passwd"); err == nil {
		t.Fatal("expected escape error")
	}
}

func TestBaselineAndDelta(t *testing.T) {
	root := t.TempDir()
	traceID := "trace-delta"
	agentID := "agent-1"
	comb := Root(root, traceID)
	if err := os.MkdirAll(comb, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(comb, "research.md"), []byte("v1"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := CaptureBaseline(root, traceID, agentID); err != nil {
		t.Fatal(err)
	}
	baseline, ok, err := LoadBaseline(root, traceID, agentID)
	if err != nil || !ok {
		t.Fatalf("baseline load: ok=%v err=%v", ok, err)
	}
	delta, err := ComputeDelta(root, traceID, baseline)
	if err != nil {
		t.Fatal(err)
	}
	if len(delta.Added)+len(delta.Changed) != 0 {
		t.Fatalf("expected empty delta, got %+v", delta)
	}
	if err := os.WriteFile(filepath.Join(comb, "research.md"), []byte("v2"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(comb, "notes.md"), []byte("# Notes"), 0o644); err != nil {
		t.Fatal(err)
	}
	delta, err = ComputeDelta(root, traceID, baseline)
	if err != nil {
		t.Fatal(err)
	}
	if len(delta.Changed) != 1 || len(delta.Added) != 1 {
		t.Fatalf("delta = %+v", delta)
	}
}

func TestLoadBaselineMissingSkipsFlush(t *testing.T) {
	root := t.TempDir()
	_, ok, err := LoadBaseline(root, "trace-missing", "agent-1")
	if err != nil {
		t.Fatal(err)
	}
	if ok {
		t.Fatal("missing baseline must not be treated as empty map")
	}
}

func TestWriteFileRejectsSymlink(t *testing.T) {
	root := t.TempDir()
	traceID := "trace-symlink"
	comb := Root(root, traceID)
	if err := os.MkdirAll(comb, 0o755); err != nil {
		t.Fatal(err)
	}
	outside := filepath.Join(root, "outside.txt")
	if err := os.WriteFile(outside, []byte("secret"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(outside, filepath.Join(comb, "notes.md")); err != nil {
		t.Fatal(err)
	}
	if _, err := WriteFile(root, traceID, "notes.md", []byte("overwrite")); err == nil {
		t.Fatal("expected symlink write to fail")
	}
	got, err := os.ReadFile(outside)
	if err != nil {
		t.Fatal(err)
	}
	if string(got) != "secret" {
		t.Fatalf("outside file mutated: %q", got)
	}
}

func TestBaselineCaptureFailureSkipsFlush(t *testing.T) {
	root := t.TempDir()
	traceID := "trace-fail"
	agentID := "agent-1"
	// comb is a file, not a directory
	if err := os.MkdirAll(filepath.Dir(Root(root, traceID)), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(Root(root, traceID), []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
	_ = CaptureBaseline(root, traceID, agentID)
	_, ok, err := LoadBaseline(root, traceID, agentID)
	if err != nil {
		t.Fatal(err)
	}
	if ok {
		t.Fatal("expected baseline not ok after failed capture")
	}
}
