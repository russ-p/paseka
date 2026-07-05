package main

import (
	"fmt"
	"os"

	"github.com/paseka/paseka/internal/colony"
	"github.com/spf13/cobra"
)

func main() {
	if err := newRoot().Execute(); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

func newRoot() *cobra.Command {
	root := &cobra.Command{
		Use:   "paseka",
		Short: "Queen Shell — manage your hive",
	}
	root.AddCommand(newInitCmd())
	root.AddCommand(newBeeCmd())
	root.AddCommand(newSessionCmd())
	root.AddCommand(newPurgeCmd())
	return root
}

func newInitCmd() *cobra.Command {
	var startDir string
	cmd := &cobra.Command{
		Use:   "init",
		Short: "Initialize .paseka colony config in the current git repository",
		RunE: func(cmd *cobra.Command, args []string) error {
			res, err := colony.Init(colony.InitOptions{StartDir: startDir})
			if err != nil {
				return err
			}
			printInitResult(res)
			return nil
		},
	}
	cmd.Flags().StringVarP(&startDir, "path", "C", "", "directory inside the git repository (default: current directory)")
	return cmd
}

func printInitResult(res colony.InitResult) {
	fmt.Printf("Colony initialized at %s\n", res.ColonyRoot)
	fmt.Printf("Slug: %s\n", res.Slug)
	fmt.Printf("Home config: %s\n", res.HomeDir)
	if len(res.Created) > 0 {
		fmt.Println("\nCreated:")
		for _, p := range res.Created {
			fmt.Printf("  + %s\n", p)
		}
	}
	if len(res.Skipped) > 0 {
		fmt.Println("\nSkipped (already exists):")
		for _, p := range res.Skipped {
			fmt.Printf("  · %s\n", p)
		}
	}
	fmt.Println("\nNext steps:")
	fmt.Println("  1. agent login   # or set CURSOR_API_KEY")
	fmt.Println("  2. paseka bee run scout --task \"your task\"")
	fmt.Println("  3. paseka bee chat scout \"discuss a feature\"  # interactive HITL")
	fmt.Println("  4. paseka run    # start hive runtime (coming soon)")
}
