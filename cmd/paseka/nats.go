package main

import (
	"fmt"
	"io"
	"os"
	"strings"

	"github.com/russ-p/paseka/internal/bus"
	"github.com/russ-p/paseka/internal/colony"
	"github.com/russ-p/paseka/internal/cues"
	"github.com/russ-p/paseka/internal/review"
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
			bees, beesErr := ctxColony.LoadAllBeesForDiagnosis()
			if beesErr != nil {
				report.Errors = append(report.Errors, beesErr.Error())
				bees = map[string]colony.Bee{}
			}
			standing := cues.DiagnoseStanding(ctxColony.ColonyRoot, bees)
			report.Warnings = append(report.Warnings, standing.Warnings...)
			profileView, profileErr := ctxColony.DoctorProfileView()
			if profileErr != nil {
				report.Errors = append(report.Errors, profileErr.Error())
			} else {
				for _, role := range profileView.SkippedScript {
					report.Warnings = append(report.Warnings, fmt.Sprintf("profile %q skipped script bee %s (global adapter not applied)", profileView.Name, role))
				}
				for _, role := range profileView.SkippedCommand {
					report.Warnings = append(report.Warnings, fmt.Sprintf("profile %q skipped command bee %s (global adapter not applied)", profileView.Name, role))
				}
			}
			printProfileDoctor(cmd.OutOrStdout(), profileView)
			printDoctorReport(cmd.OutOrStdout(), report)
			if len(report.Errors) > 0 {
				return fmt.Errorf("doctor: %d issue(s) found", len(report.Errors))
			}
			return nil
		},
	}
	cmd.Flags().StringVarP(&startDir, "path", "C", "", "directory inside the git repository")
	return cmd
}

func printProfileDoctor(w io.Writer, v colony.ProfileDoctorView) {
	name := v.Name
	if name == "" {
		name = "none"
	}
	fmt.Fprintln(w, "Profile")
	fmt.Fprintf(w, "  name:     %s\n", name)
	layers := "none"
	switch {
	case v.Layers.Colony && v.Layers.Home:
		layers = "colony+home"
	case v.Layers.Colony:
		layers = "colony"
	case v.Layers.Home:
		layers = "home"
	}
	fmt.Fprintf(w, "  layers:   %s\n", layers)
	if len(v.Remapped) > 0 {
		fmt.Fprintf(w, "  remapped: %s\n", strings.Join(v.Remapped, ", "))
	}
	fmt.Fprintf(w, "  available: %s\n", formatDoctorAvailable(v.ColonyAvailable, v.HomeAvailable))
	fmt.Fprintln(w)
}

func formatDoctorAvailable(colonyNames, homeNames []string) string {
	colonyPart := "none"
	if len(colonyNames) > 0 {
		colonyPart = strings.Join(colonyNames, ", ")
	}
	homePart := "none"
	if len(homeNames) > 0 {
		homePart = strings.Join(homeNames, ", ")
	}
	return fmt.Sprintf("colony [%s]; home [%s]", colonyPart, homePart)
}

func printDoctorReport(w io.Writer, r bus.DoctorReport) {
	fmt.Fprintln(w, "NATS doctor")
	fmt.Fprintf(w, "  url:            %s\n", r.URL)
	fmt.Fprintf(w, "  subject prefix: %s\n", r.SubjectPrefix)
	fmt.Fprintf(w, "  connected:      %v\n", r.Connected)
	fmt.Fprintf(w, "  jetstream:      %v\n", r.JetStreamOK)
	fmt.Fprintf(w, "  event stream:   %v\n", r.StreamOK)
	fmt.Fprintf(w, "  task ledger kv: %v\n", r.KVOK)
	fmt.Fprintf(w, "  object store:   %v\n", r.ObjectStoreOK)
	if len(r.Errors) > 0 {
		fmt.Fprintln(w, "\nIssues:")
		for _, e := range r.Errors {
			fmt.Fprintf(w, "  - %s\n", e)
		}
	}
	if len(r.Warnings) > 0 {
		fmt.Fprintln(w, "\nWarnings:")
		for _, wln := range r.Warnings {
			fmt.Fprintf(w, "  - %s\n", wln)
		}
	}
	if len(r.Advisories) > 0 {
		fmt.Fprintln(w, "\nAdvisories:")
		for _, a := range r.Advisories {
			fmt.Fprintf(w, "  - %s\n", a)
		}
	}
	if len(r.Errors) == 0 && len(r.Warnings) == 0 && len(r.Advisories) == 0 {
		fmt.Fprintln(w, "\nAll checks passed.")
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
		prTitle      string
		prBody       string
		draft        bool
		runHooks     bool
	)
	cmd := &cobra.Command{
		Use:   "approve",
		Short: "Approve a review-gated task (R1 ack, local merge, or pull-request publish)",
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
			if session.Publisher == nil || session.Ledger == nil {
				return fmt.Errorf("nats url not configured")
			}
			approveRes, err := review.Approve(cmd.Context(), ctxColony, session.Publisher, session.Ledger, review.ApproveInput{
				TraceID:      traceID,
				TaskID:       taskID,
				Summary:      summary,
				MergeMessage: mergeMessage,
				PRTitle:      prTitle,
				PRBody:       prBody,
				Draft:        draft,
				RunHooks:     runHooks,
			}, review.WriteOptions{})
			if err != nil {
				return err
			}
			snap, err := session.Ledger.Snapshot(traceID)
			if err != nil {
				return err
			}
			task := snap.Tasks[taskID]
			fmt.Printf("Approved task %s on trace %s\n", taskID, traceID)
			fmt.Printf("  %s\n", review.CLIApproveMessage(review.ApproveMessageOptions{
				ProposalWorkspace: task.ProposalWorkspace,
				CommitSHA:         approveRes.CommitSHA,
				StashOutcome:      approveRes.StashOutcome,
				Published:         approveRes.Published,
				PRURL:             approveRes.PRURL,
			}))
			if approveRes.CommitSHA != "" {
				fmt.Printf("  merge commit: %s\n", approveRes.CommitSHA)
			}
			if approveRes.PRURL != "" {
				fmt.Printf("  pull request: %s\n", approveRes.PRURL)
			}
			return nil
		},
	}
	cmd.Flags().StringVarP(&startDir, "path", "C", "", "directory inside the git repository")
	cmd.Flags().StringVar(&traceID, "trace", "", "flight trail id")
	cmd.Flags().StringVar(&taskID, "task", "", "task id")
	cmd.Flags().StringVar(&summary, "summary", "approved by human", "completion summary")
	cmd.Flags().StringVar(&mergeMessage, "merge-message", "", "merge commit message (local_merge only)")
	cmd.Flags().StringVar(&prTitle, "pr-title", "", "pull request title overlay (pull_request delivery)")
	cmd.Flags().StringVar(&prBody, "pr-body", "", "pull request body overlay (pull_request delivery)")
	cmd.Flags().BoolVar(&draft, "draft", false, "open the pull request as a draft")
	cmd.Flags().BoolVar(&runHooks, "run-hooks", false, "run git hooks on the worktree-branch push")
	return cmd
}

func newProposalRejectCmd() *cobra.Command {
	var (
		startDir     string
		traceID      string
		taskID       string
		feedback     string
		commentsFile string
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
			if session.Publisher == nil || session.Ledger == nil {
				return fmt.Errorf("nats url not configured")
			}
			ctxColony, err := colony.ResolveContext(startDir)
			if err != nil {
				return err
			}
			if strings.TrimSpace(commentsFile) != "" {
				content, err := os.ReadFile(commentsFile)
				if err != nil {
					return fmt.Errorf("read comments file: %w", err)
				}
				res, err := review.DeliverReviewCommentsFile(cmd.Context(), session.Publisher, ctxColony.ColonyRoot, session.Ledger, review.DeliverCommentsInput{
					TraceID:  traceID,
					TaskID:   taskID,
					AgentID:  "human",
					Producer: "human",
					Content:  content,
					Feedback: feedback,
				})
				if err != nil {
					return err
				}
				fmt.Printf("Rejected task %s on trace %s (review comments written to comb)\n", taskID, traceID)
				if res.ReworkTaskID != "" {
					fmt.Printf("  rework task: %s\n", res.ReworkTaskID)
				}
				return nil
			}
			if err := review.Reject(cmd.Context(), session.Publisher, session.Ledger, review.RejectInput{
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
	cmd.Flags().StringVar(&feedback, "feedback", "", "human feedback for the bee (short summary when using --comments-file)")
	cmd.Flags().StringVar(&commentsFile, "comments-file", "", "path to a Markdown file to copy into the trail comb as review-comments.md before publishing feedback")
	return cmd
}
