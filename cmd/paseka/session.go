package main

import (
	"fmt"

	"github.com/paseka/paseka/internal/adapters"
	"github.com/paseka/paseka/internal/colony"
	"github.com/paseka/paseka/internal/sessions"
	"github.com/spf13/cobra"
)

func newSessionCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "session",
		Short: "Manage interactive agent sessions",
	}
	cmd.AddCommand(newSessionListCmd())
	cmd.AddCommand(newSessionAttachCmd())
	cmd.AddCommand(newSessionStopCmd())
	cmd.AddCommand(newSessionRunCmd())
	return cmd
}

func newSessionListCmd() *cobra.Command {
	var startDir string
	cmd := &cobra.Command{
		Use:   "list",
		Short: "List registered interactive sessions",
		RunE: func(cmd *cobra.Command, args []string) error {
			ctxColony, err := colony.ResolveContext(startDir)
			if err != nil {
				return err
			}
			entries, err := colony.ListSessions(ctxColony.Slug)
			if err != nil {
				return err
			}
			if len(entries) == 0 {
				fmt.Println("No active sessions.")
				return nil
			}
			fmt.Printf("%-12s %-12s %-8s %-6s %s\n", "SESSION", "TRACE", "BEE", "PID", "RUN DIR")
			for _, e := range entries {
				fmt.Printf("%-12s %-12s %-8s %-6d %s\n", e.SessionID, e.TraceID, e.Bee, e.PID, e.RunDir)
			}
			return nil
		},
	}
	cmd.Flags().StringVarP(&startDir, "path", "C", "", "directory inside the git repository")
	return cmd
}

func newSessionAttachCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "attach <sessionId>",
		Short: "Attach to a session running in this process",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			return sessions.DefaultManager.AttachInPlace(args[0])
		},
	}
	return cmd
}

func newSessionStopCmd() *cobra.Command {
	var startDir string
	cmd := &cobra.Command{
		Use:   "stop <sessionId>",
		Short: "Stop an interactive session by ID",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			sessionID := args[0]
			if err := sessions.DefaultManager.Stop(sessionID); err == nil {
				fmt.Printf("Session %s stopped.\n", sessionID)
				return nil
			}
			ctxColony, err := colony.ResolveContext(startDir)
			if err != nil {
				return err
			}
			if err := sessions.StopRemote(ctxColony.Slug, sessionID); err != nil {
				return err
			}
			fmt.Printf("Session %s signalled (remote PID).\n", sessionID)
			return nil
		},
	}
	cmd.Flags().StringVarP(&startDir, "path", "C", "", "directory inside the git repository")
	return cmd
}

func newSessionRunCmd() *cobra.Command {
	var (
		startDir     string
		task         string
		traceID      string
		intent       string
		inlinePrompt string
	)
	cmd := &cobra.Command{
		Use:    "run <role>",
		Short:  "Run an interactive session in the current terminal (used by Ghostty launcher)",
		Hidden: true,
		Args:   cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			if task == "" && inlinePrompt == "" {
				return fmt.Errorf("provide --task or --prompt")
			}
			res, err := sessions.DefaultManager.RunInteractive(cmd.Context(), sessions.RunRequest{
				StartDir:     startDir,
				Bee:          args[0],
				TraceID:      traceID,
				Task:         task,
				Intent:       intent,
				InlinePrompt: inlinePrompt,
			})
			if err != nil {
				return err
			}
			printSessionResult(res)
			if res.State == adapters.SessionFailed {
				return fmt.Errorf("session failed")
			}
			return nil
		},
	}
	cmd.Flags().StringVarP(&startDir, "path", "C", "", "directory inside the git repository")
	cmd.Flags().StringVarP(&task, "task", "t", "", "task body passed to the prompt template")
	cmd.Flags().StringVar(&traceID, "trace", "", "flight trail id")
	cmd.Flags().StringVar(&intent, "intent", "", "builder task intent: general, feature, bugfix, test-fix, refactor")
	cmd.Flags().StringVar(&inlinePrompt, "prompt", "", "inline prompt override")
	return cmd
}

func printSessionResult(res *sessions.RunResult) {
	fmt.Printf("\nSession %s\n", res.State)
	fmt.Printf("  session:   %s\n", res.SessionID)
	fmt.Printf("  trace:     %s\n", res.TraceID)
	fmt.Printf("  agent:     %s\n", res.AgentID)
	fmt.Printf("  workspace: %s\n", res.Workspace)
	fmt.Printf("  run dir:   %s\n", res.RunDir)
}
