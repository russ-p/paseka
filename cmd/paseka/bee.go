package main

import (
	"fmt"
	"strings"

	"github.com/russ-p/paseka/internal/adapters"
	"github.com/russ-p/paseka/internal/colony"
	"github.com/russ-p/paseka/internal/protocol"
	"github.com/russ-p/paseka/internal/runtime"
	"github.com/russ-p/paseka/internal/sessions"
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
		body         string
		traceID      string
		intent       string
		inlinePrompt string
		noBus        bool
		raw          bool
		purpose      string
		analysisOf   string
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
			bee, _, err := ctxColony.LoadBee(args[0])
			if err != nil {
				return err
			}
			if bee.RequiresPrompt() && body == "" && inlinePrompt == "" {
				return fmt.Errorf("provide --body or --prompt")
			}
			execPurpose, execAnalysisOf, err := operatorExecutionPurpose(purpose, analysisOf)
			if err != nil {
				return err
			}
			d := runtime.NewDispatcher()
			res, err := d.BeeRun(cmd.Context(), runtime.BeeRunRequest{
				StartDir:     startDir,
				Bee:          args[0],
				TraceID:      traceID,
				Task:         body,
				Intent:       intent,
				InlinePrompt: inlinePrompt,
				NoBus:        noBus,
				Raw:          raw,
				Purpose:      execPurpose,
				AnalysisOf:   execAnalysisOf,
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
	cmd.Flags().StringVarP(&body, "body", "b", "", "task body passed to the prompt template")
	cmd.Flags().StringVar(&traceID, "trace", "", "flight trail id (generated if omitted)")
	cmd.Flags().StringVar(&intent, "intent", "", "task intent for the bee role (see bee intents or <role>-intent-* prompt partials)")
	cmd.Flags().StringVar(&inlinePrompt, "prompt", "", "inline prompt override (skips template)")
	cmd.Flags().BoolVar(&noBus, "no-bus", false, "skip NATS publish (file-only run)")
	cmd.Flags().BoolVar(&raw, "raw", false, "request plain-text adapter output (degrades usage/session parsing; debug only)")
	cmd.Flags().StringVar(&purpose, "purpose", "", "execution purpose: work (default) or analysis")
	cmd.Flags().StringVar(&analysisOf, "analysis-of", "", "work-completion milestone studied by an --purpose analysis run")
	return cmd
}

func newBeeChatCmd() *cobra.Command {
	var (
		startDir     string
		body         string
		traceID      string
		intent       string
		inlinePrompt string
		terminal     string
		purpose      string
		analysisOf   string
	)
	cmd := &cobra.Command{
		Use:   "chat <role> [prompt]",
		Short: "Start an interactive agent session (HITL)",
		Args:  cobra.MinimumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			beeRole := args[0]
			if len(args) > 1 && inlinePrompt == "" {
				inlinePrompt = strings.Join(args[1:], " ")
			}
			execPurpose, execAnalysisOf, err := operatorExecutionPurpose(purpose, analysisOf)
			if err != nil {
				return err
			}
			if body == "" && inlinePrompt == "" {
				ctxColony, err := colony.ResolveContext(startDir)
				if err != nil {
					return err
				}
				manifest, err := colony.LoadColony(ctxColony.ColonyRoot)
				if err != nil {
					return err
				}
				bee, overlay, err := ctxColony.LoadBee(beeRole)
				if err != nil {
					return err
				}
				if !colony.HasSystemTemplate(bee, overlay, manifest.Defaults) {
					return fmt.Errorf("provide a prompt argument, --body, --prompt, or configure system_template on the bee")
				}
			}

			runArgs := []string{beeRole}
			if startDir != "" {
				runArgs = append(runArgs, "--path", startDir)
			}
			if body != "" {
				runArgs = append(runArgs, "--body", body)
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
			if purpose != "" {
				runArgs = append(runArgs, "--purpose", purpose)
			}
			if analysisOf != "" {
				runArgs = append(runArgs, "--analysis-of", analysisOf)
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
				Bee:          beeRole,
				TraceID:      traceID,
				Task:         body,
				Intent:       intent,
				InlinePrompt: inlinePrompt,
				Purpose:      execPurpose,
				AnalysisOf:   execAnalysisOf,
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
	cmd.Flags().StringVarP(&body, "body", "b", "", "task body passed to the prompt template")
	cmd.Flags().StringVar(&traceID, "trace", "", "flight trail id (generated if omitted)")
	cmd.Flags().StringVar(&intent, "intent", "", "task intent for the bee role (see bee intents or <role>-intent-* prompt partials)")
	cmd.Flags().StringVar(&inlinePrompt, "prompt", "", "inline prompt override (skips template)")
	cmd.Flags().StringVar(&terminal, "terminal", "", "terminal UI: default or ghostty (overrides ~/.config/paseka/<slug>/terminal.yaml)")
	cmd.Flags().StringVar(&purpose, "purpose", "", "execution purpose: work (default) or analysis")
	cmd.Flags().StringVar(&analysisOf, "analysis-of", "", "work-completion milestone studied by an --purpose analysis session")
	return cmd
}

// operatorExecutionPurpose validates the operator-facing purpose flag. Closing
// is runtime-owned: an operator may start ordinary work or an analysis action,
// but cannot relabel a bespoke run as the trail's closing round.
func operatorExecutionPurpose(purpose, analysisOf string) (protocol.ExecutionPurpose, string, error) {
	var value protocol.ExecutionPurpose
	switch strings.TrimSpace(purpose) {
	case "", string(protocol.PurposeWork):
		value = protocol.PurposeWork
	case string(protocol.PurposeAnalysis):
		value = protocol.PurposeAnalysis
	default:
		return "", "", fmt.Errorf("--purpose must be %q or %q", protocol.PurposeWork, protocol.PurposeAnalysis)
	}
	return protocol.ResolveAdmissionAttribution(value, analysisOf)
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
