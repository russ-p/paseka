package adapters_test

import (
	"testing"

	"github.com/paseka/paseka/internal/adapters"
)

func TestResolveExecCustomCommand(t *testing.T) {
	binary, args := adapters.ResolveExec(
		[]string{"agent", "-p", "--yolo", "hello"},
		func() (string, []string) { t.Fatal("fallback should not run"); return "", nil },
	)
	if binary != "agent" {
		t.Fatalf("binary = %q", binary)
	}
	want := []string{"-p", "--yolo", "hello"}
	if len(args) != len(want) {
		t.Fatalf("args = %v", args)
	}
	for i := range want {
		if args[i] != want[i] {
			t.Fatalf("args[%d] = %q, want %q", i, args[i], want[i])
		}
	}
}

func TestResolveExecFallback(t *testing.T) {
	binary, args := adapters.ResolveExec(nil, func() (string, []string) {
		return "pi", []string{"-p", "task"}
	})
	if binary != "pi" || len(args) != 2 || args[1] != "task" {
		t.Fatalf("got %q %v", binary, args)
	}
}

func TestFlagValue(t *testing.T) {
	argv := []string{"agent", "-p", "--output-format", "json", "--model", "x"}
	if got := adapters.FlagValue(argv, "--output-format"); got != "json" {
		t.Fatalf("got %q", got)
	}
	if got := adapters.FlagValue([]string{"--mode=text"}, "--mode"); got != "text" {
		t.Fatalf("got %q", got)
	}
}
