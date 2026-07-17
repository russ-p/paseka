package main

import (
	"fmt"
	"strings"

	"github.com/paseka/paseka/internal/bus"
	"github.com/paseka/paseka/internal/colony"
	"github.com/paseka/paseka/internal/review"
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
	}
	if len(r.Warnings) > 0 {
		fmt.Println("\nWarnings:")
		for _, w := range r.Warnings {
			fmt.Printf("  - %s\n", w)
		}
	}
	if len(r.Advisories) > 0 {
		fmt.Println("\nAdvisories:")
		for _, a := range r.Advisories {
			fmt.Printf("  - %s\n", a)
		}
	}
	if len(r.Errors) == 0 && len(r.Warnings) == 0 && len(r.Advisories) == 0 {
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
		startDir     string
		traceID      string
		taskID       string
		summary      string
		mergeMessage string
	)
	cmd := &cobra.Command{
		Use:   "approve",
		Short: "Approve a review-gated task (R1 ack for root proposals; merge for isolated final gate)",
		RunE: func(cmd *cobra.Command, args []string) error {
			if traceID == "" || taskID == "" {
				return fmt.Errorf("--trace and --task are required")
			}
			ctxColony, err := colony.ResolveContext(startDir)
			if err != nil {
				return err
			}
			session, err := openTaskSession(startDir)
			if err != nil {
				return err
			}
			defer session.Close()
			if session.Client == nil || session.Ledger == nil {
				return fmt.Errorf("nats url not configured")
			}
			commit, err := review.Approve(cmd.Context(), ctxColony, session.Client, session.Ledger, review.ApproveInput{
				TraceID:      traceID,
				TaskID:       taskID,
				Summary:      summary,
				MergeMessage: mergeMessage,
			})
			if err != nil {
				return err
			}
			fmt.Printf("Approved task %s on trace %s\n", taskID, traceID)
			if commit != "" {
				fmt.Printf("  merge commit: %s\n", commit)
			}
			return nil
		},
	}
	cmd.Flags().StringVarP(&startDir, "path", "C", "", "directory inside the git repository")
	cmd.Flags().StringVar(&traceID, "trace", "", "flight trail id")
	cmd.Flags().StringVar(&taskID, "task", "", "task id")
	cmd.Flags().StringVar(&summary, "summary", "approved by human", "completion summary")
	cmd.Flags().StringVar(&mergeMessage, "merge-message", "", "merge commit message")
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
		Short: "Reject a review-gated task (publishes human INSIGHT feedback)",
		RunE: func(cmd *cobra.Command, args []string) error {
			if traceID == "" || taskID == "" {
				return fmt.Errorf("--trace and --task are required")
			}
			session, err := openTaskSession(startDir)
			if err != nil {
				return err
			}
			defer session.Close()
			if session.Client == nil || session.Ledger == nil {
				return fmt.Errorf("nats url not configured")
			}
			if err := review.Reject(cmd.Context(), session.Client, session.Ledger, review.RejectInput{
				TraceID:  traceID,
				TaskID:   taskID,
				Feedback: feedback,
			}); err != nil {
				return err
			}
			fmt.Printf("Rejected task %s on trace %s\n", taskID, traceID)
			return nil
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
