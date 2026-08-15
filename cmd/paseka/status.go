package main

import (
	"encoding/json"
	"fmt"

	"github.com/paseka/paseka/internal/colony"
	"github.com/paseka/paseka/internal/hiveview"
	"github.com/spf13/cobra"
)

func newStatusCmd() *cobra.Command {
	var startDir string
	var jsonOut bool
	var check bool

	cmd := &cobra.Command{
		Use:   "status",
		Short: "Read-only colony snapshot (runtime, live bees, tasks, honey, attention)",
		Long:  "Observe-only index of hive health and live work. Use --json for the machine contract; --check exits non-zero when the substrate cannot choreograph.",
		RunE: func(cmd *cobra.Command, args []string) error {
			ctxColony, err := colony.ResolveContext(startDir)
			if err != nil {
				return err
			}

			snap, err := hiveview.BuildColonySnapshot(ctxColony, nil, nil)
			if err != nil {
				return err
			}
			cmd.SilenceUsage = true

			out := cmd.OutOrStdout()
			if jsonOut {
				data, err := json.MarshalIndent(snap, "", "  ")
				if err != nil {
					return err
				}
				if _, err := fmt.Fprintln(out, string(data)); err != nil {
					return err
				}
			} else {
				if _, err := fmt.Fprintln(out, hiveview.FormatColonySnapshot(snap)); err != nil {
					return err
				}
			}

			if check && !snap.SubstrateHealthy() {
				return fmt.Errorf("status: hive substrate is not ready to choreograph")
			}
			return nil
		},
	}

	cmd.Flags().StringVarP(&startDir, "path", "C", "", "directory inside the git repository")
	cmd.Flags().BoolVar(&jsonOut, "json", false, "emit snapshot as JSON on stdout")
	cmd.Flags().BoolVar(&check, "check", false, "exit non-zero when runtime is not alive or configured NATS is down")
	return cmd
}
