package systeminject_test

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/paseka/paseka/internal/adapters/systeminject"
	"github.com/paseka/paseka/internal/runs"
)

func TestCursorPluginPath(t *testing.T) {
	root := t.TempDir()
	runDir := runs.Dir{ColonyRoot: root, TraceID: "trace-1", AgentID: "agent-1"}
	if err := runDir.Prepare(); err != nil {
		t.Fatal(err)
	}
	want := filepath.Join(runDir.Root(), systeminject.CursorPluginDirName)
	if got := systeminject.CursorPluginPath(runDir); got != want {
		t.Fatalf("CursorPluginPath = %q, want %q", got, want)
	}
	pluginDir, err := systeminject.WriteCursorPlugin(runDir, "You are Scout.")
	if err != nil {
		t.Fatal(err)
	}
	if pluginDir != want {
		t.Fatalf("WriteCursorPlugin dir = %q, want %q", pluginDir, want)
	}
}

func TestWriteCursorPlugin(t *testing.T) {
	root := t.TempDir()
	runDir := runs.Dir{ColonyRoot: root, TraceID: "trace-1", AgentID: "agent-1"}
	if err := runDir.Prepare(); err != nil {
		t.Fatal(err)
	}

	pluginDir, err := systeminject.WriteCursorPlugin(runDir, "You are Scout.")
	if err != nil {
		t.Fatal(err)
	}
	if pluginDir == "" {
		t.Fatal("expected plugin dir")
	}

	rulePath := filepath.Join(pluginDir, "rules", "bee-system.mdc")
	data, err := os.ReadFile(rulePath)
	if err != nil {
		t.Fatal(err)
	}
	body := string(data)
	if !strings.Contains(body, "alwaysApply: true") {
		t.Fatalf("rule missing alwaysApply: %q", body)
	}
	if !strings.Contains(body, "You are Scout.") {
		t.Fatalf("rule missing system text: %q", body)
	}
}
