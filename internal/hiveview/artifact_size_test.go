package hiveview

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/russ-p/paseka/internal/artifacts"
	"github.com/russ-p/paseka/internal/colony"
)

func writeTraceCombFile(t *testing.T, root, traceID, rel, body string) {
	t.Helper()
	full := filepath.Join(artifacts.Root(root, traceID), rel)
	if err := os.MkdirAll(filepath.Dir(full), 0o755); err != nil {
		t.Fatalf("mkdir %s: %v", full, err)
	}
	if err := os.WriteFile(full, []byte(body), 0o644); err != nil {
		t.Fatalf("write %s: %v", full, err)
	}
}

func TestListTraceArtifactsCarriesSize(t *testing.T) {
	root := t.TempDir()
	body := `{"cursor":"abc"}`
	writeTraceCombFile(t, root, "trace-01", "checkpoints/state.json", body)

	views, err := ListTraceArtifacts(colony.Context{ColonyRoot: root, Slug: "test"}, "trace-01")
	if err != nil {
		t.Fatalf("ListTraceArtifacts: %v", err)
	}
	if len(views) != 1 {
		t.Fatalf("views = %d, want 1", len(views))
	}
	if want := int64(len(body)); views[0].Bytes != want {
		t.Fatalf("Bytes = %d, want %d", views[0].Bytes, want)
	}
}

func TestGetTraceArtifactContentReportsSizeWhenItOmitsTheBody(t *testing.T) {
	root := t.TempDir()
	// One byte over the preview cap, so the refusal is exercised without writing half a
	// megabyte into a temp dir.
	oversize := strings.Repeat("x", artifacts.MaxInlinePreviewBytes+1)
	writeTraceCombFile(t, root, "trace-01", "notes.md", oversize)

	view, err := GetTraceArtifactContent(
		colony.Context{ColonyRoot: root, Slug: "test"},
		"trace-01",
		"notes.md",
	)
	if err != nil {
		t.Fatalf("GetTraceArtifactContent: %v", err)
	}

	if view.Omitted == "" {
		t.Fatal("omitted = empty, want a refusal")
	}
	if view.Content != "" {
		t.Fatalf("content = %d bytes, want none", len(view.Content))
	}
	// "Too large" on its own leaves the operator unable to tell a 600 KiB file from a
	// 600 GiB one, and therefore unable to choose between raising the cap and paging it.
	if want := int64(len(oversize)); view.Bytes != want {
		t.Fatalf("Bytes = %d, want %d", view.Bytes, want)
	}
}

func TestGetTraceArtifactContentReportsSizeForABinaryFile(t *testing.T) {
	root := t.TempDir()
	writeTraceCombFile(t, root, "trace-01", "logo.png", "\x89PNG\r\n\x1a\n")

	view, err := GetTraceArtifactContent(
		colony.Context{ColonyRoot: root, Slug: "test"},
		"trace-01",
		"logo.png",
	)
	if err != nil {
		t.Fatalf("GetTraceArtifactContent: %v", err)
	}
	if view.Omitted == "" {
		t.Fatal("omitted = empty, want a refusal")
	}
	// It is a small file the preview will never read, and the size is what says whether
	// that is worth reporting at all.
	if view.Bytes != 8 {
		t.Fatalf("Bytes = %d, want 8", view.Bytes)
	}
}
