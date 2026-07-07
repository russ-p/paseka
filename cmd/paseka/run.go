package main

import (
	"context"
	"os"
	"os/signal"
	"strconv"
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
			ctxColony := reactor.Colony()
			if err := runtime.RegisterSelf(ctxColony); err != nil {
				logging.Component("runtime").Warn("runtime registry failed", logging.F("error", err.Error()))
			}
			defer func() { _ = runtime.UnregisterSelf(ctxColony) }()

			ctx, stop := signal.NotifyContext(cmd.Context(), os.Interrupt, syscall.SIGTERM)
			defer stop()

			hbCtx, hbCancel := context.WithCancel(ctx)
			defer hbCancel()
			go runtime.RunHeartbeat(hbCtx, ctxColony)

			log := logging.Component("runtime")
			log.Info("hive runtime started", logging.F("pid", strconv.Itoa(os.Getpid())))
			log.Info("press Ctrl+C to stop")
			return reactor.Run(ctx)
		},
	}
	cmd.Flags().StringVarP(&startDir, "path", "C", "", "directory inside the git repository")
	return cmd
}
