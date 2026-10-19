package main

import (
	"fmt"
	"strings"

	"github.com/russ-p/paseka/internal/colony"
	"github.com/russ-p/paseka/internal/export"
	"github.com/spf13/cobra"
)

func newExportCmd() *cobra.Command {
	var (
		startDir    string
		traceID     string
		formatFlag  string
		includeFlag []string
	)
	cmd := &cobra.Command{
		Use:   "export [traceId]",
		Short: "Export a flight trail as a self-contained HTML or Markdown report",
		Args:  cobra.MaximumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			id, err := resolveExportTraceID(args, traceID)
			if err != nil {
				return err
			}
			format, err := export.ParseFormat(formatFlag)
			if err != nil {
				return err
			}
			include, err := export.ParseInclude(includeFlag)
			if err != nil {
				return err
			}
			ctx, err := colony.ResolveContext(startDir)
			if err != nil {
				return err
			}
			path, err := export.ExportTrace(ctx, export.Options{
				TraceID: id,
				Format:  format,
				Include: include,
			})
			if err != nil {
				return err
			}
			fmt.Println(path)
			return nil
		},
	}
	cmd.Flags().StringVarP(&startDir, "path", "C", "", "directory inside the git repository")
	cmd.Flags().StringVar(&traceID, "trace", "", "flight trail id (alternative to the positional argument)")
	cmd.Flags().StringVar(&formatFlag, "format", "html", "export renderer (html, md)")
	cmd.Flags().StringSliceVar(&includeFlag, "include", nil, "optional payload slices: usage, durations, bees, colony, cues, artifacts, agent-logs, patches (repeatable or comma-separated)")
	return cmd
}

func resolveExportTraceID(args []string, flagTraceID string) (string, error) {
	positional := ""
	if len(args) > 0 {
		positional = strings.TrimSpace(args[0])
	}
	fromFlag := strings.TrimSpace(flagTraceID)
	switch {
	case positional == "" && fromFlag == "":
		return "", fmt.Errorf("flight trail id is required: pass it as an argument (paseka export <traceId>) or with --trace <traceId>")
	case positional == "":
		return fromFlag, nil
	case fromFlag == "" || positional == fromFlag:
		return positional, nil
	default:
		return "", fmt.Errorf("conflicting flight trail ids: argument %q and --trace %q; pass only one", positional, fromFlag)
	}
}
