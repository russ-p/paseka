package runs_test

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/paseka/paseka/internal/runs"
)

func TestDirPaths(t *testing.T) {
	d := runs.Dir{ColonyRoot: "/colony", TraceID: "trace-1", AgentID: "agent-a"}

	wantRoot := filepath.Join("/colony", ".paseka", "runs", "trace-1", "agent-a")
	if d.Root() != wantRoot {
		t.Fatalf("Root() = %q, want %q", d.Root(), wantRoot)
	}
	if d.ResultPath() != filepath.Join(wantRoot, "result.txt") {
		t.Fatal("unexpected ResultPath")
	}
}

func TestPrepareAndResult(t *testing.T) {
	root := t.TempDir()
	d := runs.Dir{ColonyRoot: root, TraceID: "t1", AgentID: "a1"}

	if err := d.Prepare(); err != nil {
		t.Fatal(err)
	}
	if err := d.WritePrompt("hello"); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(d.ResultPath(), []byte("done"), 0o644); err != nil {
		t.Fatal(err)
	}
	got, err := d.ReadResult()
	if err != nil || got != "done" {
		t.Fatalf("ReadResult() = %q, %v", got, err)
	}
}
