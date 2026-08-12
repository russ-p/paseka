package main

import (
	"fmt"
	"os"
	"strings"
	"text/tabwriter"

	"github.com/paseka/paseka/internal/colony"
	"github.com/paseka/paseka/internal/cues"
	"github.com/paseka/paseka/internal/tasks"
	"github.com/spf13/cobra"
)

func newCueCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "cue",
		Short: "Run colony Forage Cue shortcuts",
	}
	cmd.AddCommand(newCueListCmd())
	cmd.AddCommand(newCueRunCmd())
	return cmd
}

func newCueListCmd() *cobra.Command {
	var startDir string
	cmd := &cobra.Command{
		Use:   "list",
		Short: "List colony cues sorted by id",
		RunE: func(cmd *cobra.Command, args []string) error {
			ctxColony, err := colony.ResolveContext(startDir)
			if err != nil {
				return err
			}
			items, err := cues.List(ctxColony.ColonyRoot)
			if err != nil {
				return err
			}
			if len(items) == 0 {
				fmt.Printf("No cues found under %s\n", cues.Dir(ctxColony.ColonyRoot))
				return nil
			}
			w := tabwriter.NewWriter(os.Stdout, 0, 0, 2, ' ', 0)
			fmt.Fprintf(w, "CUE\tDESCRIPTION\n")
			for _, item := range items {
				fmt.Fprintf(w, "%s\t%s\n", item.ID, item.Description)
			}
			_ = w.Flush()
			return nil
		},
	}
	cmd.Flags().StringVarP(&startDir, "path", "C", "", "directory inside the git repository")
	return cmd
}

func newCueRunCmd() *cobra.Command {
	var (
		startDir string
		traceID  string
		setFlags []string
	)
	cmd := &cobra.Command{
		Use:   "run <id> <text>",
		Short: "Run a cue and publish immediately",
		Args:  cobra.MinimumNArgs(2),
		RunE: func(cmd *cobra.Command, args []string) error {
			vars, err := cues.ParseSetFlags(setFlags)
			if err != nil {
				return err
			}

			ctxColony, err := colony.ResolveContext(startDir)
			if err != nil {
				return err
			}
			session, err := tasks.OpenLedger(ctxColony)
			if err != nil {
				return err
			}
			defer session.Close()

			res, err := cues.Run(cmd.Context(), session.Publisher, session.Ledger, cues.RunInput{
				ColonyRoot: ctxColony.ColonyRoot,
				CueID:      args[0],
				Text:       strings.Join(args[1:], " "),
				TraceID:    traceID,
				Vars:       vars,
				Source:     "cli",
				AgentID:    "cli",
			})
			if err != nil {
				return err
			}

			fmt.Printf("Published %s/%s\n", res.EventType, res.Kind)
			fmt.Printf("Trace: %s\n", res.TraceID)
			if res.TaskID != "" {
				fmt.Printf("Task: %s\n", res.TaskID)
			}
			return nil
		},
	}
	cmd.Flags().StringVarP(&startDir, "path", "C", "", "directory inside the git repository")
	cmd.Flags().StringVar(&traceID, "trace", "", "attach publish to an existing flight trail (new trace when omitted; cue energy_budget is ignored when the trail is already seeded)")
	cmd.Flags().StringArrayVar(&setFlags, "set", nil, "template variable override (key=val, repeatable)")
	return cmd
}
