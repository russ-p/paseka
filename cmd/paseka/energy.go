package main

import (
	"fmt"

	"github.com/paseka/paseka/internal/tasks"
	"github.com/spf13/cobra"
)

func newEnergyCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "energy",
		Short: "Manage per-trace honey reserve (energyToken)",
	}
	cmd.AddCommand(newEnergyShowCmd())
	cmd.AddCommand(newEnergyAddCmd())
	return cmd
}

func newEnergyShowCmd() *cobra.Command {
	var startDir string
	var traceID string
	cmd := &cobra.Command{
		Use:   "show",
		Short: "Show honey reserve for a trace",
		RunE: func(cmd *cobra.Command, args []string) error {
			if traceID == "" {
				return fmt.Errorf("--trace is required")
			}
			session, err := openTaskSession(startDir)
			if err != nil {
				return err
			}
			defer session.Close()
			if session.Ledger == nil {
				return fmt.Errorf("nats url not configured")
			}
			snap, err := session.Ledger.Snapshot(traceID)
			if err != nil {
				return err
			}
			fmt.Printf("Trace: %s\n", traceID)
			fmt.Printf("  budget:    %d\n", snap.EnergyBudget)
			fmt.Printf("  remaining: %d\n", snap.EnergyRemaining)
			return nil
		},
	}
	cmd.Flags().StringVarP(&startDir, "path", "C", "", "directory inside the git repository")
	cmd.Flags().StringVar(&traceID, "trace", "", "flight trail id")
	return cmd
}

func newEnergyAddCmd() *cobra.Command {
	var (
		startDir string
		traceID  string
		amount   int
	)
	cmd := &cobra.Command{
		Use:   "add",
		Short: "Add honey reserve to a trace",
		RunE: func(cmd *cobra.Command, args []string) error {
			if traceID == "" {
				return fmt.Errorf("--trace is required")
			}
			session, err := openTaskSession(startDir)
			if err != nil {
				return err
			}
			defer session.Close()

			snap, err := tasks.AddEnergy(cmd.Context(), session, tasks.AddEnergyInput{
				TraceID: traceID,
				Amount:  amount,
			})
			if err != nil {
				return err
			}
			fmt.Printf("Added %d honey to trace %s\n", amount, traceID)
			fmt.Printf("  remaining: %d / %d\n", snap.EnergyRemaining, snap.EnergyBudget)
			return nil
		},
	}
	cmd.Flags().StringVarP(&startDir, "path", "C", "", "directory inside the git repository")
	cmd.Flags().StringVar(&traceID, "trace", "", "flight trail id")
	cmd.Flags().IntVar(&amount, "amount", 0, "honey tokens to add")
	return cmd
}
