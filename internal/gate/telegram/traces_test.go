package telegram_test

import (
	"context"
	"strings"
	"testing"
	"time"

	tgbotapi "github.com/go-telegram-bot-api/telegram-bot-api/v5"
	"github.com/paseka/paseka/internal/colony"
	"github.com/paseka/paseka/internal/console"
	tggate "github.com/paseka/paseka/internal/gate/telegram"
	"github.com/paseka/paseka/internal/protocol"
	"github.com/paseka/paseka/internal/runs"
)

func TestFormatTracesListEmpty(t *testing.T) {
	text := tggate.FormatTracesList(tggate.Config{}, nil, time.Now())
	if text != "No traces yet." {
		t.Fatalf("got %q", text)
	}
}

func TestFormatTracesListRendersStatusHintsAndTimes(t *testing.T) {
	now := time.Date(2026, 7, 19, 15, 0, 0, 0, time.UTC)
	text := tggate.FormatTracesList(tggate.Config{}, []console.TraceSummaryView{
		{
			TraceID:        "trace-active",
			LastActivityAt: now.Add(-30 * time.Minute),
			RunCount:       2,
			HasActive:      true,
		},
		{
			TraceID:        "trace-failed",
			LastActivityAt: now.Add(-2 * time.Hour),
			RunCount:       1,
			HasFailures:    true,
		},
		{
			TraceID:        "trace-idle",
			LastActivityAt: now.Add(-48 * time.Hour),
			RunCount:       4,
		},
	}, now)

	for _, want := range []string{
		"Recent traces",
		"trace-active · 30m ago · active",
		"trace-failed · 2h ago · failed",
		"trace-idle · 2d ago · 4 runs",
	} {
		if !strings.Contains(text, want) {
			t.Fatalf("missing %q in:\n%s", want, text)
		}
	}
}

func TestFormatTracesListIncludesConsoleDeepLinks(t *testing.T) {
	now := time.Now().UTC()
	text := tggate.FormatTracesList(tggate.Config{
		ConsoleBaseURL: "https://console.example/",
	}, []console.TraceSummaryView{
		{
			TraceID:        "trace-abc",
			LastActivityAt: now,
			RunCount:       1,
		},
	}, now)
	if !strings.Contains(text, "https://console.example/#traces/trace-abc") {
		t.Fatalf("missing console deep link:\n%s", text)
	}
}

func TestTraceConsoleURL(t *testing.T) {
	got := tggate.TraceConsoleURL(tggate.Config{
		ConsoleBaseURL: "https://console.example",
	}, "trace-1")
	if got != "https://console.example/#traces/trace-1" {
		t.Fatalf("got %q", got)
	}
	if tggate.TraceConsoleURL(tggate.Config{}, "trace-1") != "" {
		t.Fatal("expected empty url without console base")
	}
}

func TestHandlerTracesEmptyColony(t *testing.T) {
	home := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", home)
	repo := initTestRepo(t)
	ctxColony, err := colony.ResolveContext(repo)
	if err != nil {
		t.Fatal(err)
	}

	bot := &mockBot{}
	h := &tggate.Handler{
		Colony: ctxColony,
		Config: tggate.Config{
			AllowFrom: []int64{1},
			ChatIDs:   []int64{-100},
		},
		Bot: bot,
	}
	update := tgbotapi.Update{
		Message: &tgbotapi.Message{
			Entities: []tgbotapi.MessageEntity{{Type: "bot_command", Offset: 0, Length: 7}},
			Text:     "/traces",
			Chat:     &tgbotapi.Chat{ID: -100},
			From:     &tgbotapi.User{ID: 1},
		},
	}
	h.HandleUpdate(context.Background(), update)
	if len(bot.sent) != 1 {
		t.Fatalf("expected 1 reply, got %d", len(bot.sent))
	}
	msg, ok := bot.sent[0].(tgbotapi.MessageConfig)
	if !ok {
		t.Fatalf("expected MessageConfig, got %T", bot.sent[0])
	}
	if msg.Text != "No traces yet." {
		t.Fatalf("unexpected text: %q", msg.Text)
	}
}

func TestHandlerTracesListsRecentTrace(t *testing.T) {
	home := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", home)
	repo := initTestRepo(t)
	ctxColony, err := colony.ResolveContext(repo)
	if err != nil {
		t.Fatal(err)
	}

	traceID := "trace-tg"
	agentID := "agent-tg"
	started := time.Now().UTC().Add(-15 * time.Minute)
	d := runs.Dir{ColonyRoot: repo, TraceID: traceID, AgentID: agentID}
	if err := d.Prepare(); err != nil {
		t.Fatal(err)
	}
	if err := d.WriteRequest(protocol.Request{
		ProtocolVersion: protocol.Version,
		TraceID:         traceID,
		AgentID:         agentID,
		Bee:             "builder",
		Adapter:         "cursor",
		Workspace:       repo,
		ColonyRoot:      repo,
		CreatedAt:       started,
	}); err != nil {
		t.Fatal(err)
	}
	if err := d.WriteStatusSnapshot(protocol.StatusSnapshot{
		ProtocolVersion: protocol.Version,
		State:           protocol.StatusRunning,
		StartedAt:       started,
	}); err != nil {
		t.Fatal(err)
	}

	bot := &mockBot{}
	h := &tggate.Handler{
		Colony: ctxColony,
		Config: tggate.Config{
			AllowFrom:      []int64{1},
			ChatIDs:        []int64{-100},
			ConsoleBaseURL: "https://console.example",
		},
		Bot: bot,
	}
	update := tgbotapi.Update{
		Message: &tgbotapi.Message{
			Entities: []tgbotapi.MessageEntity{{Type: "bot_command", Offset: 0, Length: 7}},
			Text:     "/traces",
			Chat:     &tgbotapi.Chat{ID: -100},
			From:     &tgbotapi.User{ID: 1},
		},
	}
	h.HandleUpdate(context.Background(), update)
	if len(bot.sent) != 1 {
		t.Fatalf("expected 1 reply, got %d", len(bot.sent))
	}
	msg, ok := bot.sent[0].(tgbotapi.MessageConfig)
	if !ok {
		t.Fatalf("expected MessageConfig, got %T", bot.sent[0])
	}
	if !strings.Contains(msg.Text, traceID) {
		t.Fatalf("missing trace id in:\n%s", msg.Text)
	}
	if !strings.Contains(msg.Text, "active") {
		t.Fatalf("missing active hint in:\n%s", msg.Text)
	}
	if !strings.Contains(msg.Text, "https://console.example/#traces/"+traceID) {
		t.Fatalf("missing console deep link in:\n%s", msg.Text)
	}
}

func TestFormatHelpTextIncludesTraces(t *testing.T) {
	text := tggate.FormatHelpText(tggate.CommandsConfig{})
	if !strings.Contains(text, "/traces — recent colony traces") {
		t.Fatalf("missing /traces help line:\n%s", text)
	}
}
