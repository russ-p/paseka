package telegram_test

import (
	"fmt"
	"strings"
	"testing"

	"github.com/paseka/paseka/internal/colony"
	tggate "github.com/paseka/paseka/internal/gate/telegram"
	"github.com/paseka/paseka/internal/protocol"
)

func TestFormatInviteCardIncludesHoneyLine(t *testing.T) {
	ctx := colony.Context{Slug: "test", ColonyRoot: ""}
	invite := colony.InviteEntry{
		InviteID: "inv-abc",
		TraceID:  "trace-1",
		Bee:      "drone",
		Intent:   "grilling",
		Task:     "Review feature",
		Status:   colony.InviteStatusPending,
	}
	text := tggate.FormatInviteCard(ctx, nil, invite)
	if !strings.Contains(text, fmt.Sprintf("honey: 0/%d", protocol.DefaultEnergyBudget)) {
		t.Fatalf("missing honey line:\n%s", text)
	}
	if !strings.Contains(text, "Intent: grilling") {
		t.Fatalf("missing intent:\n%s", text)
	}
	if !strings.Contains(text, "inv-abc") {
		t.Fatalf("missing invite id:\n%s", text)
	}
}

func TestFormatInviteCardTruncatesLongTask(t *testing.T) {
	longTask := strings.Repeat("x", 300)
	invite := colony.InviteEntry{InviteID: "inv-1", TraceID: "trace-1", Bee: "b", Task: longTask, Status: colony.InviteStatusPending}
	text := tggate.FormatInviteCard(colony.Context{}, nil, invite)
	if len(text) > 500 {
		t.Fatalf("card too long: %d chars", len(text))
	}
	if !strings.Contains(text, "...") {
		t.Fatal("expected truncation ellipsis")
	}
}

func TestGateConsumerNameSanitizesSlug(t *testing.T) {
	if got := tggate.GateConsumerName("My.Colony!"); got != "paseka-gate-telegram-my_colony" {
		t.Fatalf("consumer = %q", got)
	}
}

func TestParseEnergyCallback(t *testing.T) {
	traceID, amount, ok := tggate.ParseEnergyCallback("trace-019f762e449b7cd4:12")
	if !ok || traceID != "trace-019f762e449b7cd4" || amount != 12 {
		t.Fatalf("got trace=%q amount=%d ok=%v", traceID, amount, ok)
	}
	_, _, ok = tggate.ParseEnergyCallback("bad")
	if ok {
		t.Fatal("expected invalid callback")
	}
}
