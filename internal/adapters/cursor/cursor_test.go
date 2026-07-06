package cursor

import (
	"path/filepath"
	"strings"
	"testing"

	"github.com/paseka/paseka/internal/adapters"
)

func TestAdapterName(t *testing.T) {
	if New().Name() != "cursor" {
		t.Fatal("expected adapter name cursor")
	}
}

func TestBuildArgs(t *testing.T) {
	req := adapters.RunRequest{
		Workspace: "/colony/worktree",
		Params: adapters.RunParams{
			Model:        "composer-2.5",
			OutputFormat: "stream-json",
			Trust:        true,
			Force:        true,
		},
	}
	args := buildArgs(req, "implement feature")

	want := []string{
		"-p", "--workspace", "/colony/worktree",
		"--output-format", "stream-json",
		"--trust", "--force",
		"--model", "composer-2.5",
		"implement feature",
	}
	if len(args) != len(want) {
		t.Fatalf("got %d args, want %d: %v", len(args), len(want), args)
	}
	for i := range want {
		if args[i] != want[i] {
			t.Fatalf("args[%d] = %q, want %q (full: %v)", i, args[i], want[i], args)
		}
	}
}

func TestAugmentPromptUsesAbsolutePath(t *testing.T) {
	resultPath := filepath.Join("/colony", ".paseka", "runs", "t1", "a1", "result.txt")
	got := augmentPrompt("Do work.", resultPath)
	if !strings.Contains(got, "/colony/.paseka/runs/t1/a1/result.txt") {
		t.Fatalf("expected absolute result path in prompt, got: %q", got)
	}
	if strings.Contains(got, "events.ndjson") {
		t.Fatalf("prompt should not mention events.ndjson, got: %q", got)
	}

	unchanged := augmentPrompt(
		"Write summary to /colony/.paseka/runs/t1/a1/result.txt.",
		resultPath,
	)
	if unchanged != "Write summary to /colony/.paseka/runs/t1/a1/result.txt." {
		t.Fatalf("prompt should not duplicate paths: %q", unchanged)
	}
}
