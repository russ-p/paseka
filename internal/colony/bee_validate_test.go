package colony_test

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/paseka/paseka/internal/colony"
	"gopkg.in/yaml.v3"
)

func TestResolveAdapterScript(t *testing.T) {
	bee := colony.Bee{Adapter: "script", Command: mustCommand(t, "echo ok")}
	name, err := bee.ResolveAdapter()
	if err != nil {
		t.Fatal(err)
	}
	if name != "script" {
		t.Fatalf("adapter = %q, want script", name)
	}
}

func TestLoadBeeScriptRequiresCommand(t *testing.T) {
	root := t.TempDir()
	beesDir := filepath.Join(root, ".paseka", "bees")
	if err := os.MkdirAll(beesDir, 0o755); err != nil {
		t.Fatal(err)
	}
	yamlBody := `role: oracle-guard
adapter: script
run_summary: disabled
`
	if err := os.WriteFile(filepath.Join(beesDir, "oracle-guard.yaml"), []byte(yamlBody), 0o644); err != nil {
		t.Fatal(err)
	}
	_, _, err := colony.LoadBee(root, "oracle-guard")
	if err == nil {
		t.Fatal("expected error for script bee without command")
	}
	if !strings.Contains(err.Error(), "requires command") {
		t.Fatalf("err = %v", err)
	}
}

func TestRequiresPrompt(t *testing.T) {
	scriptBee := colony.Bee{Adapter: "script", Command: mustCommand(t, "echo ok")}
	if scriptBee.RequiresPrompt() {
		t.Fatal("script bee should not require prompt")
	}
	cursorBee := colony.Bee{Adapter: "cursor"}
	if !cursorBee.RequiresPrompt() {
		t.Fatal("cursor bee should require prompt")
	}
}

func TestLoadBeeScriptWithCommand(t *testing.T) {
	root := t.TempDir()
	beesDir := filepath.Join(root, ".paseka", "bees")
	if err := os.MkdirAll(beesDir, 0o755); err != nil {
		t.Fatal(err)
	}
	yamlBody := `role: oracle-guard
adapter: script
command: ./scripts/guard.sh
run_summary: disabled
`
	if err := os.WriteFile(filepath.Join(beesDir, "oracle-guard.yaml"), []byte(yamlBody), 0o644); err != nil {
		t.Fatal(err)
	}
	bee, _, err := colony.LoadBee(root, "oracle-guard")
	if err != nil {
		t.Fatal(err)
	}
	if !bee.Command.IsSet() {
		t.Fatal("expected command to be set")
	}
}

func TestCommandRenderDispatchVars(t *testing.T) {
	var cmd colony.Command
	if err := yaml.Unmarshal([]byte(`./run.sh $TRACE_ID $AGENT_ID $TASK_ID $COLONY_ROOT $RUN_DIR`), &cmd); err != nil {
		t.Fatal(err)
	}
	argv, err := cmd.RenderCommand(colony.CommandVars{
		TraceID:    "trace-1",
		AgentID:    "agent-1",
		TaskID:     "task-1",
		ColonyRoot: "/colony",
		RunDir:     "/colony/.paseka/runs/trace-1/agent-1",
	})
	if err != nil {
		t.Fatal(err)
	}
	want := []string{
		"./run.sh", "trace-1", "agent-1", "task-1", "/colony",
		"/colony/.paseka/runs/trace-1/agent-1",
	}
	if len(argv) != len(want) {
		t.Fatalf("argv = %v, want %v", argv, want)
	}
	for i := range want {
		if argv[i] != want[i] {
			t.Fatalf("argv[%d] = %q, want %q", i, argv[i], want[i])
		}
	}
}

func mustCommand(t *testing.T, s string) colony.Command {
	t.Helper()
	var cmd colony.Command
	if err := yaml.Unmarshal([]byte(s), &cmd); err != nil {
		t.Fatal(err)
	}
	return cmd
}
