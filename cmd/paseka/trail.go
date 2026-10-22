package main

import (
	"errors"
	"fmt"

	"github.com/russ-p/paseka/internal/colony"
	"github.com/russ-p/paseka/internal/trailconfig"
	"github.com/spf13/cobra"
)

func newTrailCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "trail",
		Short: "Inspect flight trails and their configuration binding",
	}
	cmd.AddCommand(newTrailShowCmd())
	cmd.AddCommand(newTrailConfigCmd())
	return cmd
}

func newTrailConfigCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "config",
		Short: "Inspect a trail's captured configuration",
	}
	cmd.AddCommand(newTrailConfigShowCmd())
	return cmd
}

func newTrailConfigShowCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "show <traceId>",
		Short: "Show initial/active ConfigRef, revision, and policy",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			return showTrailBinding(args[0])
		},
	}
}

func newTrailShowCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "show <traceId>",
		Short: "Show a trail's configuration binding",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			return showTrailBinding(args[0])
		},
	}
}

func showTrailBinding(traceID string) error {
	ctxColony, err := colony.ResolveContext("")
	if err != nil {
		return err
	}
	binding, err := trailconfig.Read(ctxColony.ColonyRoot, traceID)
	if err != nil {
		if errors.Is(err, trailconfig.ErrNoBinding) {
			return fmt.Errorf("trail %s has no configuration binding", traceID)
		}
		return err
	}
	fmt.Printf("trace: %s\n", binding.TraceID)
	fmt.Printf("policy: %s\n", binding.Policy)
	fmt.Printf("revision: %d\n", binding.ConfigRevision)
	fmt.Printf("initialConfigRef: %s\n", binding.InitialConfigRef)
	fmt.Printf("activeConfigRef: %s\n", binding.ActiveConfigRef)
	fmt.Printf("config: %s\n", binding.ConfigDir)
	return nil
}
