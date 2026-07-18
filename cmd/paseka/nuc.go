package main

import (
	"fmt"
	"io"
	"net/http"
	"os"
	"strings"
	"time"

	"github.com/paseka/paseka/internal/colony"
	"github.com/paseka/paseka/internal/nuc"
	"github.com/spf13/cobra"
)

func newNucCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "nuc",
		Short: "Export and import portable bee packs (Nuc)",
	}
	cmd.AddCommand(newNucExportCmd())
	cmd.AddCommand(newNucImportCmd())
	return cmd
}

func newNucExportCmd() *cobra.Command {
	var (
		startDir    string
		outPath     string
		beesFilter  string
		name        string
		description string
	)
	cmd := &cobra.Command{
		Use:   "export",
		Short: "Export colony bees and prompts into a nuc file",
		RunE: func(cmd *cobra.Command, args []string) error {
			ctx, err := colony.ResolveContext(startDir)
			if err != nil {
				return err
			}
			var bees []string
			if beesFilter != "" {
				for _, part := range strings.Split(beesFilter, ",") {
					part = strings.TrimSpace(part)
					if part != "" {
						bees = append(bees, part)
					}
				}
			}
			doc, err := nuc.ExportFromColony(nuc.ExportOptions{
				ColonyRoot:  ctx.ColonyRoot,
				Name:        name,
				Description: description,
				Bees:        bees,
			})
			if err != nil {
				return err
			}
			data, err := nuc.MarshalDocument(doc)
			if err != nil {
				return err
			}
			if outPath == "" {
				_, err = cmd.OutOrStdout().Write(data)
				return err
			}
			if err := os.WriteFile(outPath, data, 0o644); err != nil {
				return err
			}
			fmt.Fprintln(cmd.OutOrStdout(), outPath)
			return nil
		},
	}
	cmd.Flags().StringVarP(&startDir, "path", "C", "", "directory inside the git repository")
	cmd.Flags().StringVarP(&outPath, "output", "o", "", "write nuc file (default: stdout)")
	cmd.Flags().StringVar(&beesFilter, "bees", "", "comma-separated bee roles to export (default: all)")
	cmd.Flags().StringVar(&name, "name", "", "nuc metadata.name (default: colony slug)")
	cmd.Flags().StringVar(&description, "description", "", "nuc metadata.description")
	return cmd
}

func newNucImportCmd() *cobra.Command {
	var (
		startDir string
		force    bool
		dryRun   bool
		verbose  bool
	)
	cmd := &cobra.Command{
		Use:   "import <file|url|->",
		Short: "Import a nuc file into the current colony",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			ctx, err := colony.ResolveContext(startDir)
			if err != nil {
				return err
			}
			data, err := readNucSource(args[0])
			if err != nil {
				return err
			}
			doc, err := nuc.ParseDocument(data)
			if err != nil {
				return err
			}
			res, err := nuc.Import(doc, nuc.ImportOptions{
				ColonyRoot: ctx.ColonyRoot,
				Force:      force,
				DryRun:     dryRun,
			})
			if err != nil {
				return err
			}
			_, err = fmt.Fprint(cmd.OutOrStdout(), nuc.FormatImportSummary(res, verbose))
			return err
		},
	}
	cmd.Flags().StringVarP(&startDir, "path", "C", "", "directory inside the git repository")
	cmd.Flags().BoolVar(&force, "force", false, "overwrite existing bee and prompt files")
	cmd.Flags().BoolVar(&dryRun, "dry-run", false, "show import plan without writing files")
	cmd.Flags().BoolVarP(&verbose, "verbose", "v", false, "list created, skipped, and overwritten paths")
	return cmd
}

func readNucSource(source string) ([]byte, error) {
	switch {
	case source == "-":
		return io.ReadAll(os.Stdin)
	case strings.HasPrefix(source, "http://") || strings.HasPrefix(source, "https://"):
		client := &http.Client{Timeout: 30 * time.Second}
		resp, err := client.Get(source)
		if err != nil {
			return nil, fmt.Errorf("nuc: fetch %s: %w", source, err)
		}
		defer resp.Body.Close()
		if resp.StatusCode < 200 || resp.StatusCode >= 300 {
			return nil, fmt.Errorf("nuc: fetch %s: HTTP %s", source, resp.Status)
		}
		return io.ReadAll(resp.Body)
	default:
		return os.ReadFile(source)
	}
}
