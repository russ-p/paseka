package main

import (
	"errors"
	"fmt"
	"os"

	"github.com/paseka/paseka/internal/bus"
	"github.com/paseka/paseka/internal/colony"
	"github.com/paseka/paseka/internal/protocol"
	"github.com/spf13/cobra"
)

func newEventCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "event",
		Short: "Validate and publish bus events",
	}
	cmd.AddCommand(newEventEmitCmd())
	cmd.AddCommand(newEventValidateCmd())
	return cmd
}

func newEventEmitCmd() *cobra.Command {
	var (
		startDir string
		useStdin bool
		agentID  string
	)
	cmd := &cobra.Command{
		Use:   "emit",
		Short: "Validate and publish a bus event",
		RunE: func(cmd *cobra.Command, args []string) error {
			if !useStdin {
				return fmt.Errorf("--stdin is required")
			}
			return runEventCommand(cmd, startDir, agentID, true)
		},
	}
	cmd.Flags().StringVarP(&startDir, "path", "C", "", "directory inside the git repository")
	cmd.Flags().BoolVar(&useStdin, "stdin", false, "read one JSON event object from stdin")
	cmd.Flags().StringVar(&agentID, "agent", "agent", "default agent id when omitted from JSON")
	return cmd
}

func newEventValidateCmd() *cobra.Command {
	var (
		startDir string
		useStdin bool
		agentID  string
	)
	cmd := &cobra.Command{
		Use:   "validate",
		Short: "Validate a bus event without publishing",
		RunE: func(cmd *cobra.Command, args []string) error {
			if !useStdin {
				return fmt.Errorf("--stdin is required")
			}
			return runEventCommand(cmd, startDir, agentID, false)
		},
	}
	cmd.Flags().StringVarP(&startDir, "path", "C", "", "directory inside the git repository")
	cmd.Flags().BoolVar(&useStdin, "stdin", false, "read one JSON event object from stdin")
	cmd.Flags().StringVar(&agentID, "agent", "agent", "default agent id when omitted from JSON")
	return cmd
}

func runEventCommand(cmd *cobra.Command, startDir, agentID string, publish bool) error {
	raw, err := bus.ReadEventInput(os.Stdin)
	if err != nil {
		return writeEventFailure(err)
	}

	var (
		client     *bus.Client
		colonyRoot string
	)
	if publish {
		ctxColony, err := colony.ResolveContext(startDir)
		if err != nil {
			return err
		}
		colonyRoot = ctxColony.ColonyRoot
		client, err = bus.ConnectColony(ctxColony, false)
		if err != nil {
			return err
		}
		if client != nil {
			defer client.Close()
		}
	}

	result, err := bus.ProcessEventInput(cmd.Context(), client, raw, agentID, publish, colonyRoot)
	if err != nil {
		return err
	}
	if err := bus.WriteEventCLIResult(os.Stdout, result); err != nil {
		return err
	}
	if !result.OK {
		return fmt.Errorf("event: %s", result.Error)
	}
	return nil
}

func writeEventFailure(err error) error {
	var verr *protocol.ValidationError
	if errors.As(err, &verr) {
		result := protocol.EventCLIResult{
			OK:      false,
			Error:   verr.Code,
			Details: verr.Details,
		}
		_ = bus.WriteEventCLIResult(os.Stdout, result)
		return fmt.Errorf("event: %s", verr.Code)
	}
	return err
}
