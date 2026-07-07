package main

import (
	"os"
	"os/signal"
	"syscall"

	"github.com/paseka/paseka/internal/logging"
	"github.com/paseka/paseka/internal/runtime"
	"github.com/spf13/cobra"
)

func newRunCmd() *cobra.Command {
	var startDir string
	cmd := &cobra.Command{
		Use:   "run",
		Short: "Start the hive runtime (NATS reactor)",
		RunE: func(cmd *cobra.Command, args []string) error {
			reactor, err := runtime.NewReactor(runtime.ReactorOptions{StartDir: startDir})
			if err != nil {
				return err
			}
			ctx, stop := signal.NotifyContext(cmd.Context(), os.Interrupt, syscall.SIGTERM)
			defer stop()

			log := logging.Component("runtime")
			log.Info("hive runtime started")
			log.Info("press Ctrl+C to stop")
			return reactor.Run(ctx)
		},
	}
	cmd.Flags().StringVarP(&startDir, "path", "C", "", "directory inside the git repository")
	return cmd
}
