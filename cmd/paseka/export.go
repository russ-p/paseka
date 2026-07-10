package main

import (
	"fmt"

	"github.com/paseka/paseka/internal/colony"
	"github.com/paseka/paseka/internal/export"
	"github.com/spf13/cobra"
)

func newExportCmd() *cobra.Command {
	var (
		startDir string
		traceID  string
	)
	cmd := &cobra.Command{
		Use:   "export",
		Short: "Export a flight trail as a self-contained HTML report",
		RunE: func(cmd *cobra.Command, args []string) error {
			if traceID == "" {
				return fmt.Errorf("--trace is required")
			}
			ctx, err := colony.ResolveContext(startDir)
			if err != nil {
				return err
			}
			path, err := export.ExportTrace(ctx, export.Options{TraceID: traceID})
			if err != nil {
				return err
			}
			fmt.Println(path)
			return nil
		},
	}
	cmd.Flags().StringVarP(&startDir, "path", "C", "", "directory inside the git repository")
	cmd.Flags().StringVar(&traceID, "trace", "", "flight trail id")
	_ = cmd.MarkFlagRequired("trace")
	return cmd
}
