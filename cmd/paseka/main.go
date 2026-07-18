package main

import (
	"fmt"
	"os"

	"github.com/paseka/paseka/internal/colony"
	"github.com/paseka/paseka/internal/logging"
	"github.com/spf13/cobra"
)

func main() {
	if err := newRoot().Execute(); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

func newRoot() *cobra.Command {
	var logLevel string
	var noColor bool

	root := &cobra.Command{
		Use:   "paseka",
		Short: "Queen Shell — manage your hive",
		PersistentPreRunE: func(cmd *cobra.Command, args []string) error {
			level, err := logging.ParseLevel(logLevel)
			if err != nil {
				return err
			}
			logging.SetDefault(logging.New(logging.Options{
				Level:   level,
				NoColor: noColor,
			}))
			return nil
		},
	}
	root.PersistentFlags().StringVar(&logLevel, "log-level", "info", "log level: error, warn, info, debug")
	root.PersistentFlags().BoolVar(&noColor, "no-color", false, "disable ANSI colors in logs")
	root.AddCommand(newInitCmd())
	root.AddCommand(newBeeCmd())
	root.AddCommand(newSessionCmd())
	root.AddCommand(newInviteCmd())
	root.AddCommand(newRunCmd())
	root.AddCommand(newTaskCmd())
	root.AddCommand(newDoctorCmd())
	root.AddCommand(newReplayCmd())
	root.AddCommand(newSignalCmd())
	root.AddCommand(newEventCmd())
	root.AddCommand(newProposalCmd())
	root.AddCommand(newEnergyCmd())
	root.AddCommand(newPurgeCmd())
	root.AddCommand(newConsoleCmd())
	root.AddCommand(newExportCmd())
	root.AddCommand(newColonyCmd())
	root.AddCommand(newNucCmd())
	return root
}

func newInitCmd() *cobra.Command {
	var startDir string
	var adapter string
	cmd := &cobra.Command{
		Use:   "init",
		Short: "Initialize .paseka colony config in the current git repository",
		RunE: func(cmd *cobra.Command, args []string) error {
			res, err := colony.Init(colony.InitOptions{StartDir: startDir, Adapter: adapter})
			if err != nil {
				return err
			}
			printInitResult(res)
			return nil
		},
	}
	cmd.Flags().StringVarP(&startDir, "path", "C", "", "directory inside the git repository (default: current directory)")
	cmd.Flags().StringVar(&adapter, "adapter", "cursor", "scaffold bees and home config for this adapter (cursor or pi; unknown values use cursor)")
	return cmd
}

func printInitResult(res colony.InitResult) {
	fmt.Printf("Colony initialized at %s\n", res.ColonyRoot)
	fmt.Printf("Slug: %s\n", res.Slug)
	fmt.Printf("Adapter: %s\n", res.Adapter)
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
	switch res.Adapter {
	case "pi":
		fmt.Println("  1. Ensure `pi` is in PATH (install Pi CLI)")
		fmt.Println("  2. Optionally set api_key_env in ~/.config/paseka/<slug>/adapters/pi.yaml")
		fmt.Println("  3. paseka bee run scout --task \"your task\"")
		fmt.Println("  4. paseka bee chat scout \"discuss a feature\"  # interactive HITL")
		fmt.Println("  5. paseka run    # start hive runtime (NATS reactor)")
	default:
		fmt.Println("  1. agent login   # or set CURSOR_API_KEY")
		fmt.Println("  2. paseka bee run scout --task \"your task\"")
		fmt.Println("  3. paseka bee chat scout \"discuss a feature\"  # interactive HITL")
		fmt.Println("  4. paseka run    # start hive runtime (NATS reactor)")
	}
}
