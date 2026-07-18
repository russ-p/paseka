package main

import (
	"fmt"
	"os"
	"os/signal"
	"syscall"

	"github.com/paseka/paseka/internal/colony"
	tggate "github.com/paseka/paseka/internal/gate/telegram"
	"github.com/spf13/cobra"
)

func newGateCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "gate",
		Short: "Human Gateway surfaces (Telegram, …)",
	}
	cmd.AddCommand(newGateTelegramCmd())
	return cmd
}

func newGateTelegramCmd() *cobra.Command {
	var startDir string
	cmd := &cobra.Command{
		Use:   "telegram",
		Short: "Start Telegram Human Gateway (long-poll)",
		RunE: func(cmd *cobra.Command, args []string) error {
			ctxColony, err := colony.ResolveContext(startDir)
			if err != nil {
				return err
			}
			cfg, err := tggate.Load(ctxColony.Slug)
			if err != nil {
				return err
			}
			gate, err := tggate.NewGate(ctxColony, cfg)
			if err != nil {
				return err
			}

			ctx, stop := signal.NotifyContext(cmd.Context(), os.Interrupt, syscall.SIGTERM)
			defer stop()
			if err := gate.Run(ctx); err != nil && ctx.Err() == nil {
				return err
			}
			fmt.Println("Telegram gate stopped.")
			return nil
		},
	}
	cmd.Flags().StringVarP(&startDir, "path", "C", "", "directory inside the git repository")
	return cmd
}
