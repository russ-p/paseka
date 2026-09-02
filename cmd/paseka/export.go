package main

import (
	"fmt"

	"github.com/paseka/paseka/internal/colony"
	"github.com/paseka/paseka/internal/export"
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
		Use:   "export",
		Short: "Export a flight trail as a self-contained HTML or Markdown report",
		RunE: func(cmd *cobra.Command, args []string) error {
			if traceID == "" {
				return fmt.Errorf("--trace is required")
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
				TraceID: traceID,
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
	cmd.Flags().StringVar(&traceID, "trace", "", "flight trail id")
	cmd.Flags().StringVar(&formatFlag, "format", "html", "export renderer (html, md)")
	cmd.Flags().StringSliceVar(&includeFlag, "include", nil, "optional payload slices: usage, durations, bees, colony, cues, artifacts, agent-logs (repeatable or comma-separated)")
	_ = cmd.MarkFlagRequired("trace")
	return cmd
}
