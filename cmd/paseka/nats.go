package main

import (
	"fmt"
	"strings"

	"github.com/paseka/paseka/internal/bus"
	"github.com/paseka/paseka/internal/colony"
	"github.com/spf13/cobra"
)

func newDoctorCmd() *cobra.Command {
	var startDir string
	cmd := &cobra.Command{
		Use:   "doctor",
		Short: "Check NATS connectivity and JetStream resources",
		RunE: func(cmd *cobra.Command, args []string) error {
			ctxColony, err := colony.ResolveContext(startDir)
			if err != nil {
				return err
			}
			report, err := bus.Diagnose(ctxColony)
			if err != nil {
				return err
			}
			printDoctorReport(report)
			if len(report.Errors) > 0 {
				return fmt.Errorf("doctor: %d issue(s) found", len(report.Errors))
			}
			return nil
		},
	}
	cmd.Flags().StringVarP(&startDir, "path", "C", "", "directory inside the git repository")
	return cmd
}

func printDoctorReport(r bus.DoctorReport) {
	fmt.Println("NATS doctor")
	fmt.Printf("  url:            %s\n", r.URL)
	fmt.Printf("  subject prefix: %s\n", r.SubjectPrefix)
	fmt.Printf("  connected:      %v\n", r.Connected)
	fmt.Printf("  jetstream:      %v\n", r.JetStreamOK)
	fmt.Printf("  event stream:   %v\n", r.StreamOK)
	fmt.Printf("  task ledger kv: %v\n", r.KVOK)
	fmt.Printf("  object store:   %v\n", r.ObjectStoreOK)
	if len(r.Errors) > 0 {
		fmt.Println("\nIssues:")
		for _, e := range r.Errors {
			fmt.Printf("  - %s\n", e)
		}
	} else {
		fmt.Println("\nAll checks passed.")
	}
}

func newReplayCmd() *cobra.Command {
	var startDir string
	cmd := &cobra.Command{
		Use:   "replay <traceId>",
		Short: "Replay domain events for a trace from JetStream",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			ctxColony, err := colony.ResolveContext(startDir)
			if err != nil {
				return err
			}
			client, err := bus.ConnectColony(ctxColony, false)
			if err != nil {
				return err
			}
			if client == nil {
				return fmt.Errorf("nats url not configured")
			}
			defer client.Close()

			events, err := client.ReplayTrace(args[0])
			if err != nil {
				return err
			}
			if len(events) == 0 {
				fmt.Printf("No domain events found for trace %s\n", args[0])
				return nil
			}
			fmt.Printf("Replay %s (%d events)\n", args[0], len(events))
			for i, ev := range events {
				kind := ""
				if len(ev.Payload) > 0 {
					kind = bus.PayloadKind(ev.Payload)
				}
				line := fmt.Sprintf("%d. %s", i+1, ev.Type)
				if kind != "" {
					line += " (" + kind + ")"
				}
				if ev.AgentID != "" {
					line += " agent=" + ev.AgentID
				}
				fmt.Println(line)
			}
			return nil
		},
	}
	cmd.Flags().StringVarP(&startDir, "path", "C", "", "directory inside the git repository")
	return cmd
}

func newSignalCmd() *cobra.Command {
	var (
		startDir string
		traceID  string
		agentID  string
		typ      string
		payload  string
	)
	cmd := &cobra.Command{
		Use:   "signal",
		Short: "Publish a domain event to the NATS bus",
		RunE: func(cmd *cobra.Command, args []string) error {
			if traceID == "" {
				id, err := colony.NewTraceID()
				if err != nil {
					return err
				}
				traceID = id
			}
			if agentID == "" {
				agentID = "cli"
			}
			if typ == "" {
				return fmt.Errorf("--type is required (SIGNAL, INSIGHT, MUTATION, VERIFICATION)")
			}
			if strings.TrimSpace(payload) == "" {
				return fmt.Errorf("--payload is required (JSON object)")
			}

			ctxColony, err := colony.ResolveContext(startDir)
			if err != nil {
				return err
			}
			client, err := bus.ConnectColony(ctxColony, false)
			if err != nil {
				return err
			}
			if client == nil {
				return fmt.Errorf("nats url not configured")
			}
			defer client.Close()

			ev, err := bus.NewEventFromCLI(traceID, agentID, typ, payload)
			if err != nil {
				return err
			}
			if err := client.PublishEvent(cmd.Context(), ev); err != nil {
				return err
			}
			fmt.Printf("Published %s on trace %s\n", ev.Type, traceID)
			return nil
		},
	}
	cmd.Flags().StringVarP(&startDir, "path", "C", "", "directory inside the git repository")
	cmd.Flags().StringVar(&traceID, "trace", "", "flight trail id")
	cmd.Flags().StringVar(&agentID, "agent", "", "agent id (default: cli)")
	cmd.Flags().StringVar(&typ, "type", "", "event type: SIGNAL, INSIGHT, MUTATION, VERIFICATION")
	cmd.Flags().StringVar(&payload, "payload", "", "JSON payload object")
	return cmd
}

func newProposalCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "proposal",
		Short: "Human-in-the-loop code proposal actions",
	}
	cmd.AddCommand(newProposalApproveCmd())
	cmd.AddCommand(newProposalRejectCmd())
	return cmd
}

func newProposalApproveCmd() *cobra.Command {
	var (
		startDir string
		traceID  string
		taskID   string
		summary  string
	)
	cmd := &cobra.Command{
		Use:   "approve",
		Short: "Approve a code proposal (publishes task.completed)",
		RunE: func(cmd *cobra.Command, args []string) error {
			if traceID == "" || taskID == "" {
				return fmt.Errorf("--trace and --task are required")
			}
			payload := fmt.Sprintf(`{"kind":"task.completed","taskId":%q,"status":"completed","summary":%q}`,
				taskID, summary)
			return runSignal(cmd, startDir, traceID, "VERIFICATION", payload)
		},
	}
	cmd.Flags().StringVarP(&startDir, "path", "C", "", "directory inside the git repository")
	cmd.Flags().StringVar(&traceID, "trace", "", "flight trail id")
	cmd.Flags().StringVar(&taskID, "task", "", "task id")
	cmd.Flags().StringVar(&summary, "summary", "approved by human", "completion summary")
	return cmd
}

func newProposalRejectCmd() *cobra.Command {
	var (
		startDir string
		traceID  string
		taskID   string
		feedback string
	)
	cmd := &cobra.Command{
		Use:   "reject",
		Short: "Reject a code proposal (publishes human INSIGHT feedback)",
		RunE: func(cmd *cobra.Command, args []string) error {
			if traceID == "" || taskID == "" {
				return fmt.Errorf("--trace and --task are required")
			}
			if feedback == "" {
				feedback = "Please revise the proposal."
			}
			payload := fmt.Sprintf(`{"kind":"human.feedback","taskId":%q,"message":%q}`, taskID, feedback)
			return runSignal(cmd, startDir, traceID, "INSIGHT", payload)
		},
	}
	cmd.Flags().StringVarP(&startDir, "path", "C", "", "directory inside the git repository")
	cmd.Flags().StringVar(&traceID, "trace", "", "flight trail id")
	cmd.Flags().StringVar(&taskID, "task", "", "task id")
	cmd.Flags().StringVar(&feedback, "feedback", "", "human feedback for the bee")
	return cmd
}

func runSignal(cmd *cobra.Command, startDir, traceID, typ, payload string) error {
	ctxColony, err := colony.ResolveContext(startDir)
	if err != nil {
		return err
	}
	client, err := bus.ConnectColony(ctxColony, false)
	if err != nil {
		return err
	}
	if client == nil {
		return fmt.Errorf("nats url not configured")
	}
	defer client.Close()

	ev, err := bus.NewEventFromCLI(traceID, "human", typ, payload)
	if err != nil {
		return err
	}
	if err := client.PublishEvent(cmd.Context(), ev); err != nil {
		return err
	}
	fmt.Printf("Published %s on trace %s\n", ev.Type, traceID)
	return nil
}
