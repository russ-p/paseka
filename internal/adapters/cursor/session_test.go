package cursor_test

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/paseka/paseka/internal/adapters"
	"github.com/paseka/paseka/internal/adapters/cursor"
)

const testChatUUID = "c6b62c6f-7ead-4fd6-9922-e952131177ff"

func TestSessionCommandInteractiveNoPrintFlag(t *testing.T) {
	a := cursor.NewSession()
	cmd, err := a.SessionCommand(adapters.SessionRequest{
		Workspace:     "/tmp/ws",
		InitialPrompt: "discuss feature",
		Params:        adapters.RunParams{Binary: writeFakeCreateChatAgent(t, testChatUUID), Trust: true, Force: true, Model: "composer-2.5"},
	})
	if err != nil {
		t.Fatal(err)
	}

	for _, arg := range cmd.Args {
		switch arg {
		case "-p":
			t.Fatalf("interactive session must not include -p, args=%v", cmd.Args)
		case "--trust":
			t.Fatalf("interactive session must not include --trust (headless-only), args=%v", cmd.Args)
		case "--plugin-dir":
			t.Fatalf("interactive session must not include --plugin-dir, args=%v", cmd.Args)
		}
	}
	if cmd.Dir != "/tmp/ws" {
		t.Fatalf("dir = %q", cmd.Dir)
	}
	last := cmd.Args[len(cmd.Args)-1]
	if last != "discuss feature" {
		t.Fatalf("prompt arg = %q", last)
	}
	if cmd.ProviderSessionID != testChatUUID {
		t.Fatalf("provider session id = %q", cmd.ProviderSessionID)
	}
	if adapters.FlagValue(cmd.Args, "--resume") != testChatUUID {
		t.Fatalf("args = %v", cmd.Args)
	}
}

func TestSessionCommandDetachedStillInteractive(t *testing.T) {
	a := cursor.NewSession()
	cmd, err := a.SessionCommand(adapters.SessionRequest{
		Workspace:     "/tmp/ws",
		InitialPrompt: "implement feature",
		Detached:      true,
		Params:        adapters.RunParams{Binary: writeFakeCreateChatAgent(t, testChatUUID), Trust: true, Force: true, Model: "composer-2.5"},
	})
	if err != nil {
		t.Fatal(err)
	}

	for _, arg := range cmd.Args {
		switch arg {
		case "-p", "--trust", "--output-format", "--plugin-dir":
			t.Fatalf("detached session must stay interactive, args=%v", cmd.Args)
		}
	}
	want := []string{
		"--workspace", "/tmp/ws",
		"--force",
		"--model", "composer-2.5",
		"--resume", testChatUUID,
		"implement feature",
	}
	if len(cmd.Args) != len(want) {
		t.Fatalf("got %d args, want %d: %v", len(cmd.Args), len(want), cmd.Args)
	}
	for i := range want {
		if cmd.Args[i] != want[i] {
			t.Fatalf("args[%d] = %q, want %q (full: %v)", i, cmd.Args[i], want[i], cmd.Args)
		}
	}
	if cmd.ProviderSessionID != testChatUUID {
		t.Fatalf("provider session id = %q", cmd.ProviderSessionID)
	}
}

func TestSessionCommandGluesSystemIntoPositional(t *testing.T) {
	a := cursor.NewSession()
	cmd, err := a.SessionCommand(adapters.SessionRequest{
		Workspace:     "/tmp/ws",
		InitialPrompt: "discuss feature",
		SystemPrompt:  "You are Scout.",
		Params:        adapters.RunParams{Binary: writeFakeCreateChatAgent(t, testChatUUID), Model: "composer-2.5"},
	})
	if err != nil {
		t.Fatal(err)
	}

	want := "You are Scout.\ndiscuss feature"
	last := cmd.Args[len(cmd.Args)-1]
	if last != want {
		t.Fatalf("prompt arg = %q, want %q", last, want)
	}
}

func TestSessionCommandCreateChatParsesUUIDFromBanner(t *testing.T) {
	a := cursor.NewSession()
	cmd, err := a.SessionCommand(adapters.SessionRequest{
		Workspace:     "/tmp/ws",
		InitialPrompt: "discuss feature",
		Params:        adapters.RunParams{Binary: writeFakeCreateChatAgent(t, "Created chat "+testChatUUID)},
	})
	if err != nil {
		t.Fatal(err)
	}
	if cmd.ProviderSessionID != testChatUUID {
		t.Fatalf("provider session id = %q", cmd.ProviderSessionID)
	}
}

func TestSessionCommandCreateChatFailureStillLaunches(t *testing.T) {
	a := cursor.NewSession()
	cmd, err := a.SessionCommand(adapters.SessionRequest{
		Workspace:     "/tmp/ws",
		InitialPrompt: "discuss feature",
		Params:        adapters.RunParams{Binary: writeFailingCreateChatAgent(t), Model: "composer-2.5"},
	})
	if err != nil {
		t.Fatal(err)
	}
	if cmd.ProviderSessionID != "" {
		t.Fatalf("provider session id = %q, want empty", cmd.ProviderSessionID)
	}
	for _, arg := range cmd.Args {
		if arg == "--resume" || strings.HasPrefix(arg, "--resume=") {
			t.Fatalf("must not inject --resume after create-chat failure, args=%v", cmd.Args)
		}
	}
}

func TestSessionCommandOverrideDoesNotCreateChat(t *testing.T) {
	marker := filepath.Join(t.TempDir(), "create-chat-called")
	fake := writeCreateChatMarkerAgent(t, marker)
	a := cursor.NewSession()
	cmd, err := a.SessionCommand(adapters.SessionRequest{
		Workspace:     "/tmp/ws",
		InitialPrompt: "discuss feature",
		Command:       []string{fake, "--workspace", "/tmp/ws", "--resume", testChatUUID, "discuss feature"},
	})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(marker); err == nil {
		t.Fatal("command override must not call create-chat")
	}
	if cmd.ProviderSessionID != testChatUUID {
		t.Fatalf("provider session id = %q", cmd.ProviderSessionID)
	}
	if adapters.FlagValue(cmd.Args, "--resume") != testChatUUID {
		t.Fatalf("args = %v", cmd.Args)
	}
}

func TestSessionCommandOverrideOmitsIDWithoutResume(t *testing.T) {
	fake := writeFailingCreateChatAgent(t)
	a := cursor.NewSession()
	cmd, err := a.SessionCommand(adapters.SessionRequest{
		Workspace:     "/tmp/ws",
		InitialPrompt: "discuss feature",
		Command:       []string{fake, "--workspace", "/tmp/ws", "discuss feature"},
	})
	if err != nil {
		t.Fatal(err)
	}
	if cmd.ProviderSessionID != "" {
		t.Fatalf("provider session id = %q, want empty", cmd.ProviderSessionID)
	}
}

func writeFakeCreateChatAgent(t *testing.T, id string) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "fake-agent")
	script := "#!/bin/sh\n" +
		"if [ \"$1\" = \"create-chat\" ]; then\n" +
		"  printf '%s\\n' '" + id + "'\n" +
		"  exit 0\n" +
		"fi\n" +
		"exit 0\n"
	if err := os.WriteFile(path, []byte(script), 0o755); err != nil {
		t.Fatal(err)
	}
	return path
}

func writeFailingCreateChatAgent(t *testing.T) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "fake-agent")
	script := "#!/bin/sh\n" +
		"if [ \"$1\" = \"create-chat\" ]; then\n" +
		"  printf 'denied\\n' >&2\n" +
		"  exit 1\n" +
		"fi\n" +
		"exit 0\n"
	if err := os.WriteFile(path, []byte(script), 0o755); err != nil {
		t.Fatal(err)
	}
	return path
}

func writeCreateChatMarkerAgent(t *testing.T, marker string) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "fake-agent")
	script := "#!/bin/sh\n" +
		"if [ \"$1\" = \"create-chat\" ]; then\n" +
		"  printf x > " + shellSingleQuote(marker) + "\n" +
		"  exit 0\n" +
		"fi\n" +
		"exit 0\n"
	if err := os.WriteFile(path, []byte(script), 0o755); err != nil {
		t.Fatal(err)
	}
	return path
}

func shellSingleQuote(s string) string {
	return "'" + strings.ReplaceAll(s, "'", `'"'"'`) + "'"
}
