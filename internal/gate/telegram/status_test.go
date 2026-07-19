package telegram_test

import (
	"strings"
	"testing"

	tggate "github.com/paseka/paseka/internal/gate/telegram"
)

func TestFormatWelcome(t *testing.T) {
	text := tggate.FormatWelcome(tggate.Snapshot{
		Slug:           "acme-api",
		ReactorAlive:   true,
		LiveBeeCount:   2,
		PendingInvites: 1,
	})
	for _, want := range []string{
		"Welcome to Paseka · acme-api",
		"Reactor: alive · bees: 2 · invites: 1",
		"Use the buttons below or /help for commands.",
	} {
		if !strings.Contains(text, want) {
			t.Fatalf("missing %q in:\n%s", want, text)
		}
	}
}

func TestFormatWelcomeOmitsZeroInvites(t *testing.T) {
	text := tggate.FormatWelcome(tggate.Snapshot{
		Slug:         "acme-api",
		ReactorAlive: false,
		LiveBeeCount: 0,
	})
	if strings.Contains(text, "invites:") {
		t.Fatalf("expected no invites line when zero, got:\n%s", text)
	}
	if !strings.Contains(text, "Reactor: stopped · bees: 0") {
		t.Fatalf("missing compact status in:\n%s", text)
	}
}

func TestFormatWelcomeFallback(t *testing.T) {
	text := tggate.FormatWelcomeFallback("acme-api")
	if !strings.Contains(text, "Welcome to Paseka · acme-api") {
		t.Fatalf("unexpected fallback:\n%s", text)
	}
	if strings.Contains(text, "Reactor:") {
		t.Fatalf("fallback should not include status:\n%s", text)
	}
}

func TestFormatStatus(t *testing.T) {
	text := tggate.FormatStatus(tggate.Snapshot{
		Slug:           "acme-api",
		SubjectPrefix:  "paseka.acme-api",
		ReactorAlive:   true,
		LiveBeeCount:   2,
		PendingInvites: 1,
	})
	for _, want := range []string{
		"Paseka · acme-api",
		"Reactor: alive",
		"Subject: paseka.acme-api",
		"Live bees: 2",
		"Pending invites: 1",
	} {
		if !strings.Contains(text, want) {
			t.Fatalf("missing %q in:\n%s", want, text)
		}
	}
}
