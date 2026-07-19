package telegram_test

import (
	"encoding/json"
	"strings"
	"testing"

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
	})
	if !strings.Contains(text, "/feature <text> — Intake idea/bug via Scout") {
		t.Fatalf("missing custom help line:\n%s", text)
	}
}
