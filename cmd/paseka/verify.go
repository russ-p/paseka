package main

import (
	"encoding/json"
	"fmt"

	"github.com/russ-p/paseka/internal/bus"
	"github.com/russ-p/paseka/internal/colony"
	"github.com/spf13/cobra"
)

func newVerifyCmd() *cobra.Command {
	var (
		startDir string
		asJSON   bool
	)
	cmd := &cobra.Command{
		Use:   "verify <traceId>",
		Short: "Compare a trail's local event record against what reached the bus",
		Long: "Compare a trail's local event record against what reached the bus.\n\n" +
			"The trail log is what the colony decided to do; the bus is what was broadcast.\n" +
			"This reports events that are only local, only on the bus, or in both. With no\n" +
			"broker reachable it reports that verification was not possible — never that a\n" +
			"divergence exists.",
		Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			// A divergence is a finding, not a usage error: the command ran and
			// reported. Only the flag layer should print usage.
			cmd.SilenceUsage = true
			ctxColony, err := colony.ResolveContext(startDir)
			if err != nil {
				return err
			}
			client, err := bus.ConnectColony(ctxColony, false)
			if err != nil {
				return notVerifiable(cmd, args[0], err, asJSON)
			}
			if client == nil {
				return notVerifiable(cmd, args[0], fmt.Errorf("nats url not configured"), asJSON)
			}
			defer client.Close()

			report, err := bus.VerifyTrace(ctxColony.ColonyRoot, args[0], client)
			if err != nil {
				return notVerifiable(cmd, args[0], err, asJSON)
			}
			if asJSON {
				data, err := json.Marshal(report)
				if err != nil {
					return err
				}
				fmt.Fprintln(cmd.OutOrStdout(), string(data))
			} else {
				fmt.Fprint(cmd.OutOrStdout(), bus.FormatVerifyReport(report))
			}
			if !report.StreamComplete {
				return fmt.Errorf("trail %s: verification inconclusive, the bus read did not finish", args[0])
			}
			if report.Diverged() {
				return fmt.Errorf("trail %s: local record and bus diverge", args[0])
			}
			return nil
		},
	}
	cmd.Flags().StringVarP(&startDir, "path", "C", "", "directory inside the git repository")
	cmd.Flags().BoolVar(&asJSON, "json", false, "emit the comparison as JSON")
	return cmd
}

// notVerifiable says verification could not run. An unreachable broker is not
// evidence of divergence, so the command refuses to imply one.
func notVerifiable(cmd *cobra.Command, traceID string, cause error, asJSON bool) error {
	if asJSON {
		data, err := json.Marshal(map[string]any{
			"traceId":  traceID,
			"verified": false,
			"reason":   cause.Error(),
		})
		if err == nil {
			fmt.Fprintln(cmd.OutOrStdout(), string(data))
		}
	} else {
		fmt.Fprintf(cmd.OutOrStdout(), "Trail %s: verification not possible: %v\n", traceID, cause)
	}
	return fmt.Errorf("trail %s: verification not possible: %w", traceID, cause)
}
