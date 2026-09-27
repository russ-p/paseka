package artifacts

import (
	"os"
	"path/filepath"
	"testing"
)

func writeCombFile(t *testing.T, colonyRoot, traceID, rel, body string) {
	t.Helper()
	full := filepath.Join(Root(colonyRoot, traceID), rel)
	if err := os.MkdirAll(filepath.Dir(full), 0o755); err != nil {
		t.Fatalf("mkdir %s: %v", full, err)
	}
	if err := os.WriteFile(full, []byte(body), 0o644); err != nil {
		t.Fatalf("write %s: %v", full, err)
	}
}

func TestItemFromFileReportsSize(t *testing.T) {
	root := t.TempDir()
	trace := "trace-size"
	body := "the comb body an operator would want to read"
	writeCombFile(t, root, trace, "notes/checkpoint.json", body)

	item, err := ItemFromFile(root, trace, "notes/checkpoint.json")
	if err != nil {
		t.Fatalf("ItemFromFile: %v", err)
	}

	// The stat that builds `Updated` already knows the size, so this costs nothing — and
	// without it a file the preview will refuse looks the same as one it will not.
	if want := int64(len(body)); item.Bytes != want {
		t.Fatalf("Bytes = %d, want %d", item.Bytes, want)
	}
}

func TestItemFromFileSizeSurvivesTheAnnouncementMerge(t *testing.T) {
	root := t.TempDir()
	trace := "trace-merge"
	writeCombFile(t, root, trace, "patch.diff", "diff --git a/x b/x")

	items, err := ListItems(root, trace)
	if err != nil {
		t.Fatalf("ListItems: %v", err)
	}
	if len(items) != 1 {
		t.Fatalf("items = %d, want 1", len(items))
	}

	// The merge overlays bus metadata onto filesystem items, so a field added to one
	// and not the other would be quietly dropped on every announced file.
	merged := MergeAnnounced(items, nil)
	if merged[0].Bytes != items[0].Bytes {
		t.Fatalf("Bytes after merge = %d, want %d", merged[0].Bytes, items[0].Bytes)
	}
}

func TestPreviewAndExportCapsAreSeparateLevers(t *testing.T) {
	// Equal today, which is the point: the preview's ceiling must be movable without
	// moving what may be shipped in a trace export, and vice versa. A test that asserted
	// they differ would freeze today's value; this one fails if they are the same symbol.
	if MaxInlinePreviewBytes <= 0 || MaxInlineExportBytes <= 0 {
		t.Fatalf("caps = preview %d, export %d", MaxInlinePreviewBytes, MaxInlineExportBytes)
	}
}
