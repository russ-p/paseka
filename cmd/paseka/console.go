package main

import (
	"fmt"
	"os"
	"os/signal"
	"syscall"

	"github.com/paseka/paseka/internal/colony"
	"github.com/paseka/paseka/internal/console"
	"github.com/paseka/paseka/internal/sessions"
	"github.com/spf13/cobra"
)

func newConsoleCmd() *cobra.Command {
	var (
		addr     string
		startDir string
	)
	cmd := &cobra.Command{
		Use:   "console",
		Short: "Start Queen Console web UI (localhost sessions surface)",
		RunE: func(cmd *cobra.Command, args []string) error {
			ctxColony, err := colony.ResolveContext(startDir)
			if err != nil {
				return err
			}
			srv := console.NewServer(console.Options{
				Addr:     addr,
				Colony:   ctxColony,
				Sessions: sessions.NewManager(),
			})

			ctx, stop := signal.NotifyContext(cmd.Context(), os.Interrupt, syscall.SIGTERM)
			defer stop()
			if err := srv.Run(ctx); err != nil {
				return err
			}
			fmt.Println("Queen Console stopped.")
			return nil
		},
	}
	cmd.Flags().StringVar(&addr, "addr", "127.0.0.1:8787", "listen address (localhost only)")
	cmd.Flags().StringVarP(&startDir, "path", "C", "", "directory inside the git repository")
	return cmd
}
