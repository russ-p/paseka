package telegram_test

import (
	"strings"
	"testing"

	tggate "github.com/paseka/paseka/internal/gate/telegram"
)

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
