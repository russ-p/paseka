package telegram

import (
	"context"
	"fmt"

	tgbotapi "github.com/go-telegram-bot-api/telegram-bot-api/v5"
	"github.com/paseka/paseka/internal/bus"
	"github.com/paseka/paseka/internal/colony"
	"github.com/paseka/paseka/internal/logging"
	"github.com/paseka/paseka/internal/runtime"
)

// Gate is the Telegram Human Gateway long-poll process for one colony.
type Gate struct {
	Colony     colony.Context
	Config     Config
	Supervisor *runtime.Supervisor
	Bot        *tgbotapi.BotAPI
}

// NewGate connects to Telegram and verifies NATS for the colony.
func NewGate(ctx colony.Context, cfg Config) (*Gate, error) {
	if !cfg.LongPoll() {
		return nil, fmt.Errorf("telegram gate: webhook mode is not implemented yet (use mode: longpoll)")
	}
	client, err := bus.ConnectColony(ctx, true)
	if err != nil {
		return nil, fmt.Errorf("telegram gate: nats: %w", err)
	}
	if client == nil {
		return nil, fmt.Errorf("telegram gate: nats url not configured in home config")
	}
	client.Close()

	bot, err := tgbotapi.NewBotAPI(cfg.BotToken())
	if err != nil {
		return nil, fmt.Errorf("telegram gate: telegram api: %w", err)
	}
	bot.Debug = false

	return &Gate{
		Colony:     ctx,
		Config:     cfg,
		Supervisor: runtime.DefaultSupervisor(),
		Bot:        bot,
	}, nil
}

// Run long-polls Telegram until ctx is cancelled.
func (g *Gate) Run(ctx context.Context) error {
	log := logging.Component("gate.telegram")
	log.Info("telegram gate started",
		logging.F("slug", g.Colony.Slug),
		logging.F("username", g.Bot.Self.UserName),
	)

	notifier, err := NewNotifier(g.Colony, g.Config, g.Bot)
	if err != nil {
		return err
	}
	defer notifier.Close()

	notifyCtx, cancelNotify := context.WithCancel(ctx)
	defer cancelNotify()
	go func() {
		if err := notifier.Run(notifyCtx); err != nil && notifyCtx.Err() == nil {
			log.Error("telegram notify pipeline stopped", logging.F("error", err.Error()))
		}
	}()

	handler := &Handler{
		Colony:     g.Colony,
		Config:     g.Config,
		Supervisor: g.Supervisor,
		Bot:        g.Bot,
		Invites: &InviteActions{
			Colony: g.Colony,
			Config: g.Config,
			Bot:    g.Bot,
		},
		Energy: &EnergyActions{
			Colony: g.Colony,
			Bot:    g.Bot,
		},
		Tasks: &TaskActions{
			Colony:  g.Colony,
			Config:  g.Config,
			Bot:     g.Bot,
			Pending: NewPendingTasks(),
		},
		Proposals: &ProposalActions{
			Colony: g.Colony,
			Config: g.Config,
			Bot:    g.Bot,
		},
	}

	u := tgbotapi.NewUpdate(0)
	u.Timeout = 60
	updates := g.Bot.GetUpdatesChan(u)

	for {
		select {
		case <-ctx.Done():
			g.Bot.StopReceivingUpdates()
			cancelNotify()
			return ctx.Err()
		case update, ok := <-updates:
			if !ok {
				cancelNotify()
				return ctx.Err()
			}
			handler.HandleUpdate(ctx, update)
		}
	}
}
