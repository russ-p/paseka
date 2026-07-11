package main

import (
	"fmt"
	"strings"

	"github.com/paseka/paseka/internal/adapters"
	"github.com/paseka/paseka/internal/colony"
	"github.com/paseka/paseka/internal/runtime"
	"github.com/paseka/paseka/internal/sessions"
	"github.com/spf13/cobra"
)

func newBeeCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "bee",
		Short: "Run colony bees",
	}
	cmd.AddCommand(newBeeRunCmd())
	cmd.AddCommand(newBeeChatCmd())
	return cmd
}

func newBeeRunCmd() *cobra.Command {
	var (
		startDir     string
		task         string
		traceID      string
		intent       string
		inlinePrompt string
		noBus        bool
	)
	cmd := &cobra.Command{
		Use:   "run <role>",
		Short: "Dispatch one bee (one-shot agent run)",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			ctxColony, err := colony.ResolveContext(startDir)
			if err != nil {
				return err
			}
			bee, _, err := colony.LoadBee(ctxColony.ColonyRoot, args[0])
			if err != nil {
				return err
			}
			if bee.RequiresPrompt() && task == "" && inlinePrompt == "" {
				return fmt.Errorf("provide --task or --prompt")
			}
			d := runtime.NewDispatcher()
			res, err := d.BeeRun(cmd.Context(), runtime.BeeRunRequest{
				StartDir:     startDir,
				Bee:          args[0],
				TraceID:      traceID,
				Task:         task,
				Intent:       intent,
				InlinePrompt: inlinePrompt,
				NoBus:        noBus,
			})
			if err != nil {
				return err
			}
			printBeeRunResult(res)
			if res.Result != nil && res.Result.Status == "failed" {
				if res.Result.Err != nil {
					return res.Result.Err
				}
				return fmt.Errorf("bee run failed (exit %d)", res.Result.ExitCode)
			}
			return nil
		},
	}
	cmd.Flags().StringVarP(&startDir, "path", "C", "", "directory inside the git repository (default: current directory)")
	cmd.Flags().StringVarP(&task, "task", "t", "", "task body passed to the prompt template")
	cmd.Flags().StringVar(&traceID, "trace", "", "flight trail id (generated if omitted)")
	cmd.Flags().StringVar(&intent, "intent", "", "task intent for the bee role (see bee intents or <role>-intent-* prompt partials)")
	cmd.Flags().StringVar(&inlinePrompt, "prompt", "", "inline prompt override (skips template)")
	cmd.Flags().BoolVar(&noBus, "no-bus", false, "skip NATS publish (file-only run)")
	return cmd
}

func newBeeChatCmd() *cobra.Command {
	var (
		startDir     string
		task         string
		traceID      string
		intent       string
		inlinePrompt string
		terminal     string
	)
	cmd := &cobra.Command{
		Use:   "chat <role> [prompt]",
		Short: "Start an interactive agent session (HITL)",
		Args:  cobra.MinimumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			bee := args[0]
			if len(args) > 1 && inlinePrompt == "" {
				inlinePrompt = strings.Join(args[1:], " ")
			}
			if task == "" && inlinePrompt == "" {
				return fmt.Errorf("provide a prompt argument, --task, or --prompt")
			}

			runArgs := []string{bee}
			if startDir != "" {
				runArgs = append(runArgs, "--path", startDir)
			}
			if task != "" {
				runArgs = append(runArgs, "--task", task)
			}
			if traceID != "" {
				runArgs = append(runArgs, "--trace", traceID)
			}
			if intent != "" {
				runArgs = append(runArgs, "--intent", intent)
			}
			if inlinePrompt != "" {
				runArgs = append(runArgs, "--prompt", inlinePrompt)
			}

			termKind := sessions.ResolveTerminalKind(terminal)
			if termKind == sessions.TerminalDefault {
				ctxColony, err := colony.ResolveContext(startDir)
				if err != nil {
					return err
				}
				homeCfg := colony.LoadTerminalConfig(ctxColony.Slug)
				termKind = sessions.ResolveTerminalKind(homeCfg.Terminal)
			}

			if termKind == sessions.TerminalGhostty {
				ctxColony, err := colony.ResolveContext(startDir)
				if err != nil {
					return err
				}
				homeCfg := colony.LoadTerminalConfig(ctxColony.Slug)
				if terminal != "" {
					homeCfg.Terminal = terminal
				}
				if err := sessions.LaunchInGhostty(homeCfg, runArgs); err != nil {
					return err
				}
				fmt.Println("Interactive session launched in Ghostty.")
				return nil
			}

			res, err := sessions.DefaultManager.RunInteractive(cmd.Context(), sessions.RunRequest{
				StartDir:     startDir,
				Bee:          bee,
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
	cmd.Flags().StringVar(&traceID, "trace", "", "flight trail id (generated if omitted)")
	cmd.Flags().StringVar(&intent, "intent", "", "task intent for the bee role (see bee intents or <role>-intent-* prompt partials)")
	cmd.Flags().StringVar(&inlinePrompt, "prompt", "", "inline prompt override (skips template)")
	cmd.Flags().StringVar(&terminal, "terminal", "", "terminal UI: default or ghostty (overrides ~/.config/paseka/<slug>/terminal.yaml)")
	return cmd
}

func printBeeRunResult(res *runtime.BeeRunResult) {
	status := "unknown"
	if res.Result != nil {
		status = res.Result.Status
	}
	fmt.Printf("Bee run %s\n", status)
	fmt.Printf("  trace:     %s\n", res.TraceID)
	fmt.Printf("  agent:     %s\n", res.AgentID)
	fmt.Printf("  workspace: %s\n", res.Workspace)
	fmt.Printf("  run dir:   %s\n", res.RunDir)

	if res.Result == nil {
		return
	}
	if out := strings.TrimSpace(res.Result.Output); out != "" {
		fmt.Println("\n--- output ---")
		fmt.Println(out)
	}
	for _, a := range res.Result.Artifacts {
		if a.Kind != "diff" || strings.TrimSpace(a.Content) == "" {
			continue
		}
		lines := strings.Count(a.Content, "\n")
		if a.Content != "" && !strings.HasSuffix(a.Content, "\n") {
			lines++
		}
		fmt.Printf("\n--- diff (%d lines) ---\n", lines)
		fmt.Print(a.Content)
		if !strings.HasSuffix(a.Content, "\n") {
			fmt.Println()
		}
	}
}
