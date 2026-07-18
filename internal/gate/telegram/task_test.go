package telegram_test

import (
	"strings"
	"testing"

	tggate "github.com/paseka/paseka/internal/gate/telegram"
)

func TestFormatTaskPreview(t *testing.T) {
	text := tggate.FormatTaskPreview("builder", "feature", "none", "Fix the login bug", true)
	if !strings.Contains(text, "Bee: builder") {
		t.Fatalf("missing bee:\n%s", text)
	}
	if !strings.Contains(text, "Intent: feature") {
		t.Fatalf("missing intent:\n%s", text)
	}
	if !strings.Contains(text, "Review: none") {
		t.Fatalf("missing review:\n%s", text)
	}
	if !strings.Contains(text, "Autorun: yes") {
		t.Fatalf("missing autorun:\n%s", text)
	}
	if !strings.Contains(text, "Fix the login bug") {
		t.Fatalf("missing task text:\n%s", text)
	}
	if !strings.Contains(text, "task.ready") {
		t.Fatalf("missing ready hint:\n%s", text)
	}
}

func TestFormatTaskPreviewTruncatesLongText(t *testing.T) {
	long := strings.Repeat("x", 400)
	text := tggate.FormatTaskPreview("builder", "general", "none", long, false)
	if !strings.Contains(text, "...") {
		t.Fatal("expected truncation")
	}
}
