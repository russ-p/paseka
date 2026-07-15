package main

import (
	"fmt"
	"os"
	"strings"

	"github.com/paseka/paseka/internal/colony"
	"github.com/spf13/cobra"
)

func newColonyCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "colony",
		Short: "Colony configuration and projections",
	}
	cmd.AddCommand(newColonyTopologyCmd())
	return cmd
}

func newColonyTopologyCmd() *cobra.Command {
	var (
		startDir string
		outFile  string
	)
	cmd := &cobra.Command{
		Use:   "topology",
		Short: "Print colony EDA topology as Mermaid flowchart",
		Long:  "Build a static, config-derived EDA topology from bee YAML and colony auto_invites. Prints the same Mermaid string as GET /api/colony/topology. Does not require NATS.",
		RunE: func(cmd *cobra.Command, args []string) error {
			ctxColony, err := colony.ResolveContext(startDir)
			if err != nil {
				return err
			}
			topo, err := colony.BuildTopology(ctxColony.ColonyRoot)
			if err != nil {
				return err
			}
			mermaid := strings.TrimSpace(topo.Mermaid)
			if mermaid == "" {
				return fmt.Errorf("colony topology: empty mermaid projection")
			}
			if outFile != "" {
				return os.WriteFile(outFile, []byte(mermaid+"\n"), 0o644)
			}
			_, err = fmt.Fprintln(cmd.OutOrStdout(), mermaid)
			return err
		},
	}
	cmd.Flags().StringVarP(&startDir, "path", "C", "", "directory inside the git repository")
	cmd.Flags().StringVar(&outFile, "out", "", "write Mermaid to file instead of stdout")
	return cmd
}
