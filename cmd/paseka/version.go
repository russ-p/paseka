package main

import (
	"encoding/json"
	"fmt"
	"runtime"

	"github.com/russ-p/paseka/internal/version"
	"github.com/spf13/cobra"
)

func goToolchain() string {
	return runtime.Version()
}

// releaseChannel names the two kinds of build an operator has to tell apart: a
// tagged release, and anything else — a snapshot, a development build, a
// `go install` of a branch. "not a release" is the useful half of that answer.
func releaseChannel(released bool) string {
	if released {
		return "release"
	}
	return "development build (not a tagged release)"
}

func newVersionCmd() *cobra.Command {
	var jsonOut bool

	cmd := &cobra.Command{
		Use:   "version",
		Short: "Print the build stamp: version, commit, build date, Go toolchain",
		Long: "Which Paseka this binary is. A release build answers with its tag and the commit behind it;\n" +
			"a build from main answers with the commit the toolchain stamped and says dev for the version,\n" +
			"which is the honest answer when no tag exists. Use --json for the machine contract.",
		Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			cmd.SilenceUsage = true
			info := version.Get()

			out := cmd.OutOrStdout()
			if jsonOut {
				data, err := json.MarshalIndent(struct {
					version.Info
					GoVersion string `json:"goVersion"`
				}{Info: info, GoVersion: goToolchain()}, "", "  ")
				if err != nil {
					return err
				}
				if _, err := fmt.Fprintln(out, string(data)); err != nil {
					return err
				}
				return nil
			}

			if _, err := fmt.Fprintf(out, "paseka %s\n", info); err != nil {
				return err
			}
			// The commit is the whole point of asking a development build, so it is
			// printed in full: an abbreviated one does not paste into `git show`.
			if info.Commit != "" {
				if _, err := fmt.Fprintf(out, "  commit:  %s\n", info.Commit); err != nil {
					return err
				}
			}
			if info.Date != "" {
				if _, err := fmt.Fprintf(out, "  built:   %s (commit date)\n", info.Date); err != nil {
					return err
				}
			}
			if _, err := fmt.Fprintf(out, "  channel: %s\n", releaseChannel(info.Released)); err != nil {
				return err
			}
			if _, err := fmt.Fprintf(out, "  go:      %s\n", goToolchain()); err != nil {
				return err
			}
			return nil
		},
	}

	cmd.Flags().BoolVar(&jsonOut, "json", false, "emit the build stamp as JSON on stdout")
	return cmd
}
