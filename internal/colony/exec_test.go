package colony_test

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/paseka/paseka/internal/colony"
)

func TestExecArgvWritesFile(t *testing.T) {
	dir := t.TempDir()
	out := filepath.Join(dir, "hook.out")
	argv := []string{"sh", "-c", "echo hooked > " + out}
	if err := colony.ExecArgv(context.Background(), argv, dir); err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(out)
	if err != nil {
		t.Fatal(err)
	}
	if string(data) != "hooked\n" {
		t.Fatalf("hook output = %q", data)
	}
}

func TestRunPostExecSkipsWhenUnset(t *testing.T) {
	if err := colony.RunPostExec(context.Background(), colony.Command{}, colony.CommandVars{}, ""); err != nil {
		t.Fatal(err)
	}
}
