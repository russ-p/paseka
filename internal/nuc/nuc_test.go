package nuc_test

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/paseka/paseka/internal/nuc"
)

func TestRoundTripExportImport(t *testing.T) {
	src := setupNucFixture(t)

	doc, err := nuc.ExportFromColony(nuc.ExportOptions{
		ColonyRoot: src,
		Name:       "fixture",
	})
	if err != nil {
		t.Fatal(err)
	}

	importRoot := t.TempDir()
	writeMinimalColony(t, importRoot)

	res, err := nuc.Import(doc, nuc.ImportOptions{ColonyRoot: importRoot})
	if err != nil {
		t.Fatal(err)
	}
	if len(res.Created) == 0 {
		t.Fatalf("expected created files, got %+v", res)
	}

	for role, wantBody := range doc.Spec.Bees {
		got, err := os.ReadFile(filepath.Join(importRoot, ".paseka", "bees", role+".yaml"))
		if err != nil {
			t.Fatalf("read bee %q: %v", role, err)
		}
		if string(got) != wantBody {
			t.Fatalf("bee %q mismatch:\nwant:\n%s\ngot:\n%s", role, wantBody, got)
		}
	}
	for ref, wantBody := range doc.Spec.Prompts {
		got, err := os.ReadFile(filepath.Join(importRoot, ".paseka", "prompts", filepath.FromSlash(ref)))
		if err != nil {
			t.Fatalf("read prompt %q: %v", ref, err)
		}
		if string(got) != wantBody {
			t.Fatalf("prompt %q mismatch", ref)
		}
	}
}

func TestImportSkipVsForce(t *testing.T) {
	root := t.TempDir()
	beesDir := filepath.Join(root, ".paseka", "bees")
	if err := os.MkdirAll(beesDir, 0o755); err != nil {
		t.Fatal(err)
	}
	existing := []byte("role: scout\nadapter: cursor\n")
	if err := os.WriteFile(filepath.Join(beesDir, "scout.yaml"), existing, 0o644); err != nil {
		t.Fatal(err)
	}

	doc := nuc.Document{
		APIVersion: nuc.APIVersion,
		Kind:       nuc.Kind,
		Metadata:   nuc.Metadata{Name: "test"},
		Spec: nuc.Spec{
			Bees: map[string]string{
				"scout": "role: scout\nadapter: pi\n",
			},
		},
	}

	skipRes, err := nuc.Import(doc, nuc.ImportOptions{ColonyRoot: root})
	if err != nil {
		t.Fatal(err)
	}
	if len(skipRes.Skipped) != 1 || len(skipRes.Created) != 0 {
		t.Fatalf("skip: %+v", skipRes)
	}
	got, _ := os.ReadFile(filepath.Join(beesDir, "scout.yaml"))
	if string(got) != string(existing) {
		t.Fatal("skip should not change file")
	}

	forceRes, err := nuc.Import(doc, nuc.ImportOptions{ColonyRoot: root, Force: true})
	if err != nil {
		t.Fatal(err)
	}
	if len(forceRes.Overwritten) != 1 {
		t.Fatalf("force: %+v", forceRes)
	}
	got, _ = os.ReadFile(filepath.Join(beesDir, "scout.yaml"))
	if string(got) != doc.Spec.Bees["scout"] {
		t.Fatal("force should overwrite file")
	}
}

func TestRejectPromptPathTraversal(t *testing.T) {
	doc := nuc.Document{
		APIVersion: nuc.APIVersion,
		Kind:       nuc.Kind,
		Metadata:   nuc.Metadata{Name: "bad"},
		Spec: nuc.Spec{
			Bees: map[string]string{
				"scout": "role: scout\nadapter: cursor\n",
			},
			Prompts: map[string]string{
				"../escape.md": "nope",
			},
		},
	}
	if err := doc.Validate(); err == nil {
		t.Fatal("expected validation error for path traversal")
	}
}

func TestRejectInvalidKind(t *testing.T) {
	_, err := nuc.ParseDocument([]byte(`apiVersion: paseka/v1
kind: Hive
metadata:
  name: x
spec:
  bees:
    scout: |
      role: scout
`))
	if err == nil || !strings.Contains(err.Error(), "kind") {
		t.Fatalf("expected kind error, got %v", err)
	}
}

func TestExportBeesFilter(t *testing.T) {
	src := setupNucFixture(t)

	doc, err := nuc.ExportFromColony(nuc.ExportOptions{
		ColonyRoot: src,
		Name:       "filtered",
		Bees:       []string{"scout"},
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(doc.Spec.Bees) != 1 {
		t.Fatalf("bees: %+v", doc.Spec.Bees)
	}
	if _, ok := doc.Spec.Bees["scout"]; !ok {
		t.Fatal("expected scout only")
	}
	if _, ok := doc.Spec.Prompts["scout.md"]; !ok {
		t.Fatal("expected scout prompt in export")
	}
}

func setupNucFixture(t *testing.T) string {
	t.Helper()
	root := t.TempDir()
	writeMinimalColony(t, root)
	mustWrite(t, filepath.Join(root, ".paseka", "bees", "scout.yaml"), "role: scout\nadapter: cursor\nprompt_template: scout.md\n")
	mustWrite(t, filepath.Join(root, ".paseka", "bees", "builder.yaml"), "role: builder\nadapter: cursor\nprompt_template: builder.md\n")
	mustWrite(t, filepath.Join(root, ".paseka", "prompts", "scout.md"), "scout prompt\n")
	mustWrite(t, filepath.Join(root, ".paseka", "prompts", "builder.md"), "builder prompt\n")
	mustWrite(t, filepath.Join(root, ".paseka", "prompts", "_partials", "emit-howto.md"), "emit howto\n")
	return root
}

func writeMinimalColony(t *testing.T, root string) {
	t.Helper()
	mustMkdir(t, filepath.Join(root, ".paseka", "bees"))
	mustMkdir(t, filepath.Join(root, ".paseka", "prompts", "_partials"))
	mustWrite(t, filepath.Join(root, ".paseka", "colony.yaml"), "slug: test-colony\n")
}

func mustMkdir(t *testing.T, path string) {
	t.Helper()
	if err := os.MkdirAll(path, 0o755); err != nil {
		t.Fatal(err)
	}
}

func mustWrite(t *testing.T, path, body string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
}
