package colony_test

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/paseka/paseka/internal/colony"
	"gopkg.in/yaml.v3"
)

func TestCommandUnmarshalString(t *testing.T) {
	var cmd colony.Command
	if err := yaml.Unmarshal([]byte(`agent -p --yolo $PROMPT`), &cmd); err != nil {
		t.Fatal(err)
	}
	got := cmd.Argv()
	want := []string{"agent", "-p", "--yolo", "$PROMPT"}
	if len(got) != len(want) {
		t.Fatalf("argv = %v, want %v", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("argv[%d] = %q, want %q", i, got[i], want[i])
		}
	}
}

func TestCommandUnmarshalList(t *testing.T) {
	var cmd colony.Command
	if err := yaml.Unmarshal([]byte(`["agent", "-p", "--model", "composer-2.5", "$PROMPT"]`), &cmd); err != nil {
		t.Fatal(err)
	}
	got := cmd.Argv()
	want := []string{"agent", "-p", "--model", "composer-2.5", "$PROMPT"}
	if len(got) != len(want) {
		t.Fatalf("argv = %v, want %v", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("argv[%d] = %q, want %q", i, got[i], want[i])
		}
	}
}

func TestCommandRenderSubstitutesVars(t *testing.T) {
	var cmd colony.Command
	if err := yaml.Unmarshal([]byte(`agent -p --workspace $WORKSPACE $PROMPT`), &cmd); err != nil {
		t.Fatal(err)
	}
	argv, err := cmd.RenderCommand(colony.CommandVars{
		Prompt:    "do the thing",
		Workspace: "/tmp/wt",
	})
	if err != nil {
		t.Fatal(err)
	}
	want := []string{"agent", "-p", "--workspace", "/tmp/wt", "do the thing"}
	if len(argv) != len(want) {
		t.Fatalf("argv = %v, want %v", argv, want)
	}
	for i := range want {
		if argv[i] != want[i] {
			t.Fatalf("argv[%d] = %q, want %q", i, argv[i], want[i])
		}
	}
}

func TestCommandRenderBraceVars(t *testing.T) {
	var cmd colony.Command
	if err := yaml.Unmarshal([]byte(`["agent", "-p", "${PROMPT}"]`), &cmd); err != nil {
		t.Fatal(err)
	}
	argv, err := cmd.RenderCommand(colony.CommandVars{Prompt: "hello world"})
	if err != nil {
		t.Fatal(err)
	}
	if argv[len(argv)-1] != "hello world" {
		t.Fatalf("last arg = %q, want %q", argv[len(argv)-1], "hello world")
	}
}

func TestBeeHasParams(t *testing.T) {
	if (colony.Bee{}).HasParams() {
		t.Fatal("empty bee should not have params")
	}
	if !(colony.Bee{Params: map[string]any{"model": "x"}}).HasParams() {
		t.Fatal("bee with params should report HasParams")
	}
}

func TestSplitCommandLineQuotes(t *testing.T) {
	var cmd colony.Command
	if err := yaml.Unmarshal([]byte(`agent -p "--model composer-2.5" $PROMPT`), &cmd); err != nil {
		t.Fatal(err)
	}
	got := cmd.Argv()
	if len(got) != 4 || got[2] != "--model composer-2.5" {
		t.Fatalf("argv = %v", got)
	}
}

func TestCommandRenderPostExecVars(t *testing.T) {
	var cmd colony.Command
	if err := yaml.Unmarshal([]byte(`notify.sh --result $RESULT --meta $META --dir $RUN_DIR`), &cmd); err != nil {
		t.Fatal(err)
	}
	argv, err := cmd.RenderCommand(colony.CommandVars{
		Prompt:     "do work",
		Workspace:  "/tmp/wt",
		Result:     "all done",
		ResultFile: "/tmp/runs/result.txt",
		Meta:       "/tmp/runs/meta.json",
		RunDir:     "/tmp/runs",
	})
	if err != nil {
		t.Fatal(err)
	}
	want := []string{"notify.sh", "--result", "all done", "--meta", "/tmp/runs/meta.json", "--dir", "/tmp/runs"}
	if len(argv) != len(want) {
		t.Fatalf("argv = %v, want %v", argv, want)
	}
	for i := range want {
		if argv[i] != want[i] {
			t.Fatalf("argv[%d] = %q, want %q", i, argv[i], want[i])
		}
	}
}

func TestLoadBeeWithPostExec(t *testing.T) {
	root := t.TempDir()
	beesDir := filepath.Join(root, ".paseka", "bees")
	if err := os.MkdirAll(beesDir, 0o755); err != nil {
		t.Fatal(err)
	}
	yamlBody := `role: builder
adapter: cursor
post_exec: notify.sh $RESULT
prompt_template: builder.md
`
	if err := os.WriteFile(filepath.Join(beesDir, "builder.yaml"), []byte(yamlBody), 0o644); err != nil {
		t.Fatal(err)
	}
	bee, _, err := colony.LoadBee(root, "builder")
	if err != nil {
		t.Fatal(err)
	}
	if !bee.PostExec.IsSet() {
		t.Fatal("expected post_exec to be set")
	}
}

func TestLoadBeeWithCommand(t *testing.T) {
	root := t.TempDir()
	beesDir := filepath.Join(root, ".paseka", "bees")
	if err := os.MkdirAll(beesDir, 0o755); err != nil {
		t.Fatal(err)
	}
	yamlBody := `role: builder
adapter: cursor
command: agent -p --yolo $PROMPT
prompt_template: builder.md
`
	if err := os.WriteFile(filepath.Join(beesDir, "builder.yaml"), []byte(yamlBody), 0o644); err != nil {
		t.Fatal(err)
	}
	bee, _, err := colony.LoadBee(root, "builder")
	if err != nil {
		t.Fatal(err)
	}
	if !bee.Command.IsSet() {
		t.Fatal("expected command to be set")
	}
}
