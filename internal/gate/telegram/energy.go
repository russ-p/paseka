package telegram

import (
	"context"
	"fmt"
	"strconv"
	"strings"

	tgbotapi "github.com/go-telegram-bot-api/telegram-bot-api/v5"
	"github.com/paseka/paseka/internal/colony"
	"github.com/paseka/paseka/internal/tasks"
)

// EnergyActions handles /energy commands and inline top-up buttons.
type EnergyActions struct {
	Colony colony.Context
	Bot    BotAPI
}

// ParseEnergyCommandArgs parses /energy command arguments.
// Forms: (empty), <traceId>, add <traceId> <amount>.
func ParseEnergyCommandArgs(args string) (action string, traceID string, amount int, err error) {
	fields := strings.Fields(strings.TrimSpace(args))
	if len(fields) == 0 {
		return "show", "", 0, nil
	}
	if fields[0] == "add" {
		if len(fields) < 3 {
			return "", "", 0, fmt.Errorf("usage: /energy add <traceId> <amount>")
		}
		amount, err = strconv.Atoi(fields[2])
		if err != nil || amount <= 0 {
			return "", "", 0, fmt.Errorf("amount must be a positive integer")
		}
		return "add", fields[1], amount, nil
	}
	return "show", fields[0], 0, nil
}

// FormatEnergyShow renders honey reserve for one trace.
func FormatEnergyShow(traceID string, remaining, budget int) string {
	return fmt.Sprintf("Trace: %s\nhoney: %d/%d", traceID, remaining, budget)
}

// HandleCommand processes /energy and subcommands.
func (a *EnergyActions) HandleCommand(ctx context.Context, chatID int64, args string) {
	action, traceID, amount, err := ParseEnergyCommandArgs(args)
	if err != nil {
		a.sendText(chatID, 0, err.Error())
		return
	}
	switch action {
	case "show":
		if traceID == "" {
			a.sendText(chatID, 0, "Usage: /energy <traceId>\nOr: /energy add <traceId> <amount>")
			return
		}
		a.Show(ctx, chatID, 0, traceID)
	case "add":
		a.Add(ctx, chatID, 0, traceID, amount)
	}
}

// Show posts remaining/budget for one trace.
func (a *EnergyActions) Show(ctx context.Context, chatID int64, editMessageID int, traceID string) {
	remaining, budget, err := a.traceHoney(traceID)
	if err != nil {
		a.sendText(chatID, editMessageID, "energy unavailable: "+err.Error())
		return
	}
	a.sendText(chatID, editMessageID, FormatEnergyShow(traceID, remaining, budget))
}

// Add publishes SIGNAL/energy.add via the shared tasks package.
func (a *EnergyActions) Add(ctx context.Context, chatID int64, editMessageID int, traceID string, amount int) {
	session, err := tasks.OpenLedger(a.Colony)
	if err != nil {
		a.sendText(chatID, editMessageID, "energy add failed: "+err.Error())
		return
	}
	defer session.Close()
	if session.Client == nil || session.Ledger == nil {
		a.sendText(chatID, editMessageID, "energy add failed: nats url not configured")
		return
	}
	snap, err := tasks.AddEnergy(ctx, session, tasks.AddEnergyInput{
		TraceID: traceID,
		Amount:  amount,
		AgentID: "telegram",
	})
	if err != nil {
		a.sendText(chatID, editMessageID, "energy add failed: "+err.Error())
		return
	}
	text := fmt.Sprintf("Added %d honey to %s\nhoney: %d/%d", amount, traceID, snap.EnergyRemaining, snap.EnergyBudget)
	a.sendText(chatID, editMessageID, text)
}

func (a *EnergyActions) traceHoney(traceID string) (remaining, budget int, err error) {
	session, err := tasks.OpenLedger(a.Colony)
	if err != nil {
		return 0, 0, err
	}
	defer session.Close()
	remaining, budget = traceHoney(a.Colony, session.Ledger, traceID)
	return remaining, budget, nil
}

func (a *EnergyActions) sendText(chatID int64, editMessageID int, text string) {
	if editMessageID > 0 {
		edit := tgbotapi.NewEditMessageText(chatID, editMessageID, text)
		_, _ = a.Bot.Send(edit)
		return
	}
	msg := tgbotapi.NewMessage(chatID, text)
	_, _ = a.Bot.Send(msg)
}
