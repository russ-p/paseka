package telegram_test

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/paseka/paseka/internal/cues"
	tggate "github.com/paseka/paseka/internal/gate/telegram"
)

func TestBuildCustomSignalPayload(t *testing.T) {
	raw, err := tggate.BuildCustomSignalPayload(tggate.CustomCommandConfig{
		Kind: "feature.requested",
		Static: map[string]string{
			"priority": "medium",
		},
	}, "Live bees\nShow active bees in header.")
	if err != nil {
		t.Fatal(err)
	}
	var payload map[string]string
	if err := json.Unmarshal(raw, &payload); err != nil {
		t.Fatal(err)
	}
	if payload["kind"] != "feature.requested" {
		t.Fatalf("kind = %q", payload["kind"])
	}
	if payload["title"] != "Live bees" {
		t.Fatalf("title = %q", payload["title"])
	}
	if payload["body"] != "Live bees\nShow active bees in header." {
		t.Fatalf("body = %q", payload["body"])
	}
	if payload["source"] != "telegram" {
		t.Fatalf("source = %q", payload["source"])
	}
	if payload["priority"] != "medium" {
		t.Fatalf("priority = %q", payload["priority"])
	}
}

func TestFormatSignalPreview(t *testing.T) {
	text := tggate.FormatSignalPreview(tggate.CustomCommandConfig{
		Type: "SIGNAL",
		Kind: "feature.requested",
	}, "OAuth callback for API")
	if !strings.Contains(text, "Kind: feature.requested") {
		t.Fatalf("missing kind:\n%s", text)
	}
	if !strings.Contains(text, "OAuth callback for API") {
		t.Fatalf("missing text:\n%s", text)
	}
	if !strings.Contains(text, "Confirm to publish") {
		t.Fatalf("missing confirm hint:\n%s", text)
	}
}

func TestFormatHelpTextIncludesCustomCommands(t *testing.T) {
	text := tggate.FormatHelpText(tggate.CommandsConfig{
		Custom: map[string]tggate.CustomCommandConfig{
			"feature": {
				Description: "Intake idea/bug via Scout",
				Kind:        "feature.requested",
			},
		},
	}, "")
	if !strings.Contains(text, "/feature <text> — Intake idea/bug via Scout") {
		t.Fatalf("missing custom help line:\n%s", text)
	}
}

func writeCueFile(t *testing.T, root, name, body string) {
	t.Helper()
	dir := filepath.Join(root, ".paseka", "cues")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, name), []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
}

func TestFormatCuePreviewSignal(t *testing.T) {
	text := tggate.FormatCuePreview(cues.Cue{
		ID:          "feature",
		Description: "Intake idea",
		Emit:        cues.EmitSignal,
		SignalType:  "SIGNAL",
		SignalKind:  "feature.requested",
	}, "OAuth callback for API")
	if !strings.Contains(text, "Cue: feature") {
		t.Fatalf("missing cue id:\n%s", text)
	}
	if !strings.Contains(text, "Kind: feature.requested") {
		t.Fatalf("missing kind:\n%s", text)
	}
	if !strings.Contains(text, "OAuth callback for API") {
		t.Fatalf("missing text:\n%s", text)
	}
}

func TestFormatCuePreviewTask(t *testing.T) {
	text := tggate.FormatCuePreview(cues.Cue{
		ID:          "hotfix",
		Description: "Urgent fix",
		Emit:        cues.EmitTask,
		Bee:         "builder",
		Intent:      "bugfix",
		Review:      "none",
		Autorun:     true,
	}, "Fix login redirect")
	if !strings.Contains(text, "Bee: builder") {
		t.Fatalf("missing bee:\n%s", text)
	}
	if !strings.Contains(text, "Autorun: yes") {
		t.Fatalf("missing autorun:\n%s", text)
	}
}

func TestCustomCommandHelpDescriptionFallsBackToCue(t *testing.T) {
	root := t.TempDir()
	writeCueFile(t, root, "feature.yaml", `description: Intake idea
emit: signal
type: SIGNAL
kind: feature.requested
title: "{{.Title}}"
body: "{{.Body}}"
`)
	desc := tggate.CustomCommandHelpDescription(tggate.CustomCommandConfig{
		Cue: "feature",
	}, root)
	if desc != "Intake idea" {
		t.Fatalf("description = %q", desc)
	}
}

func TestCustomCommandHelpDescriptionPrefersGate(t *testing.T) {
	root := t.TempDir()
	writeCueFile(t, root, "feature.yaml", `description: Intake idea
emit: signal
type: SIGNAL
kind: feature.requested
title: "{{.Title}}"
body: "{{.Body}}"
`)
	desc := tggate.CustomCommandHelpDescription(tggate.CustomCommandConfig{
		Cue:         "feature",
		Description: "Gate wording",
	}, root)
	if desc != "Gate wording" {
		t.Fatalf("description = %q", desc)
	}
}
