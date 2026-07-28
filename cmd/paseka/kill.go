package main

import (
	"fmt"

	"github.com/paseka/paseka/internal/protocol"
	"github.com/paseka/paseka/internal/tasks"
	"github.com/spf13/cobra"
)

func newKillCmd() *cobra.Command {
	var (
		startDir string
		traceID  string
		reason   string
	)
	cmd := &cobra.Command{
		Use:   "kill",
		Short: "Hard-kill a trace (stop in-flight agents and block further dispatch)",
		RunE: func(cmd *cobra.Command, args []string) error {
			if traceID == "" {
				return fmt.Errorf("--trace is required")
			}
			session, err := openTaskSession(startDir)
			if err != nil {
				return err
			}
			defer session.Close()

			snap, err := tasks.KillTrace(cmd.Context(), session, tasks.KillTraceInput{
				TraceID: traceID,
				Reason:  reason,
			})
			if err != nil {
				return err
			}
			fmt.Printf("Killed trace %s\n", traceID)
			if snap.Killed {
				cancelled := 0
				for _, task := range snap.Tasks {
					if task.Status == protocol.TaskStatusCancelled {
						cancelled++
					}
				}
				if cancelled > 0 {
					fmt.Printf("  cancelled tasks: %d\n", cancelled)
				}
			}
			return nil
		},
	}
	cmd.Flags().StringVarP(&startDir, "path", "C", "", "directory inside the git repository")
	cmd.Flags().StringVar(&traceID, "trace", "", "flight trail id")
	cmd.Flags().StringVar(&reason, "reason", "", "optional operator reason recorded on cancelled tasks")
	return cmd
}
