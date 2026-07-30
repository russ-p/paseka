package main

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
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
	cmd.AddCommand(newEventFlushCmd())
	cmd.AddCommand(newEventPendingCmd())
	return cmd
}

func newEventEmitCmd() *cobra.Command {
	var (
		startDir string
		useStdin bool
		agentID  string
		deferIt  bool
	)
	cmd := &cobra.Command{
		Use:   "emit",
		Short: "Validate and publish a bus event",
		RunE: func(cmd *cobra.Command, args []string) error {
			if !useStdin {
				return fmt.Errorf("--stdin is required")
			}
			return runEventCommand(cmd, startDir, agentID, true, deferIt)
		},
	}
	cmd.Flags().StringVarP(&startDir, "path", "C", "", "directory inside the git repository")
	cmd.Flags().BoolVar(&useStdin, "stdin", false, "read one JSON event object from stdin")
	cmd.Flags().StringVar(&agentID, "agent", "agent", "default agent id when omitted from JSON")
	cmd.Flags().BoolVar(&deferIt, "defer", false, "queue event for flush on successful run completion instead of publishing now")
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
			return runEventCommand(cmd, startDir, agentID, false, false)
		},
	}
	cmd.Flags().StringVarP(&startDir, "path", "C", "", "directory inside the git repository")
	cmd.Flags().BoolVar(&useStdin, "stdin", false, "read one JSON event object from stdin")
	cmd.Flags().StringVar(&agentID, "agent", "agent", "default agent id when omitted from JSON")
	return cmd
}

func newEventFlushCmd() *cobra.Command {
	var (
		startDir string
		traceID  string
		agentID  string
		discard  bool
	)
	cmd := &cobra.Command{
		Use:   "flush",
		Short: "Flush or discard deferred events for a run",
		RunE: func(cmd *cobra.Command, args []string) error {
			if traceID == "" || agentID == "" {
				return fmt.Errorf("--trace and --agent are required")
			}
			ctxColony, err := colony.ResolveContext(startDir)
			if err != nil {
				return err
			}
			client, err := bus.ConnectColony(ctxColony, false)
			if err != nil {
				return err
			}
			if client != nil {
				defer client.Close()
			}
			var pub bus.Publisher = bus.NopPublisher{}
			if client != nil {
				pub = client
			}
			result, err := bus.FlushPending(cmd.Context(), pub, ctxColony.ColonyRoot, traceID, agentID, discard)
			if err != nil {
				return err
			}
			if err := writeFlushResult(os.Stdout, result); err != nil {
				return err
			}
			if !result.OK {
				return fmt.Errorf("event flush: %s", result.Error)
			}
			return nil
		},
	}
	cmd.Flags().StringVarP(&startDir, "path", "C", "", "directory inside the git repository")
	cmd.Flags().StringVar(&traceID, "trace", "", "flight trail id")
	cmd.Flags().StringVar(&agentID, "agent", "", "agent run id")
	cmd.Flags().BoolVar(&discard, "discard", false, "clear pending queue without publishing")
	return cmd
}

func newEventPendingCmd() *cobra.Command {
	var (
		startDir string
		traceID  string
		agentID  string
	)
	cmd := &cobra.Command{
		Use:   "pending",
		Short: "Show deferred event count and kinds for a run",
		RunE: func(cmd *cobra.Command, args []string) error {
			if traceID == "" || agentID == "" {
				return fmt.Errorf("--trace and --agent are required")
			}
			ctxColony, err := colony.ResolveContext(startDir)
			if err != nil {
				return err
			}
			result, err := bus.InspectPending(ctxColony.ColonyRoot, traceID, agentID)
			if err != nil {
				return err
			}
			return writePendingInspectResult(os.Stdout, result)
		},
	}
	cmd.Flags().StringVarP(&startDir, "path", "C", "", "directory inside the git repository")
	cmd.Flags().StringVar(&traceID, "trace", "", "flight trail id")
	cmd.Flags().StringVar(&agentID, "agent", "", "agent run id")
	return cmd
}

func runEventCommand(cmd *cobra.Command, startDir, agentID string, publish bool, deferEmit bool) error {
	raw, err := bus.ReadEventInput(os.Stdin)
	if err != nil {
		return writeEventFailure(err)
	}

	var (
		client     *bus.Client
		colonyRoot string
	)
	if publish || deferEmit {
		ctxColony, err := colony.ResolveContext(startDir)
		if err != nil {
			return err
		}
		colonyRoot = ctxColony.ColonyRoot
		if publish && !deferEmit {
			client, err = bus.ConnectColony(ctxColony, false)
			if err != nil {
				return err
			}
			if client != nil {
				defer client.Close()
			}
		}
	}

	result, err := bus.ProcessEventInput(cmd.Context(), client, raw, agentID, publish && !deferEmit, deferEmit, colonyRoot)
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

func writeFlushResult(w io.Writer, result bus.FlushResult) error {
	data, err := json.Marshal(result)
	if err != nil {
		return err
	}
	_, err = fmt.Fprintln(w, string(data))
	return err
}

func writePendingInspectResult(w io.Writer, result bus.PendingInspectResult) error {
	data, err := json.Marshal(result)
	if err != nil {
		return err
	}
	_, err = fmt.Fprintln(w, string(data))
	return err
}
