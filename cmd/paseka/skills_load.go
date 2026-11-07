package main

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/russ-p/paseka/internal/colony"
	"github.com/russ-p/paseka/internal/gitroot"
	"github.com/russ-p/paseka/internal/protocol"
	"github.com/russ-p/paseka/internal/runs"
	"github.com/russ-p/paseka/internal/skills"
	"github.com/russ-p/paseka/internal/trailconfig"
	"github.com/spf13/cobra"
)

// skillLoadExitRefused is the status a refusal carries: the command ran and
// answered that this run may not load this procedure. It is distinct from 1,
// which means the colony, the run, or the configuration could not be read, so
// an agent can tell "retry with the right reference" from "report a broken
// colony".
const skillLoadExitRefused = 2

func newSkillsLoadCmd() *cobra.Command {
	var (
		startDir string
		traceID  string
		agentID  string
	)
	cmd := &cobra.Command{
		Use:   "load <name> --trace <traceId> --agent <agentId>",
		Short: "Print one offered skill's path, directory, and body, and record the load",
		Long: `Serve one procedure to a run and record that the run opened it.

The reference is resolved against the configuration the run was offered, not
the live store: a store edit or configuration application after the run started
cannot make load disagree with the advertisement the run received. The command
prints the absolute SKILL.md path, the skill directory, and the body without
frontmatter.

The directory is printed so a body that refers to scripts/, references/, or
assets/ can be followed: the files are reachable under it, and an agent learns
the location only by loading.

It refuses a reference the run's role does not carry — including a pattern,
which names a set rather than one procedure — with exit 2 and
skill_not_offered. It works in either delivery mode; a path-mode run that loads
is recorded the same way.

The run's record gains one {name, contentHash, at} line in
.paseka/runs/<traceId>/<agentId>/skills-used.ndjson, where contentHash is the
SHA-256 of the served SKILL.md bytes. At finalize the runtime folds the file
into the run's result as the deduplicated used set.`,
		Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			cmd.SilenceUsage = true
			traceID = strings.TrimSpace(traceID)
			agentID = strings.TrimSpace(agentID)
			if traceID == "" || agentID == "" {
				return fmt.Errorf("skills: load: --trace and --agent are required")
			}

			colonyRoot, err := resolveSkillLoadRoot(startDir, traceID, agentID)
			if err != nil {
				return err
			}
			runDir := runs.Dir{ColonyRoot: colonyRoot, TraceID: traceID, AgentID: agentID}
			request, err := runDir.ReadRequest()
			if err != nil {
				return fmt.Errorf("skills: load: read the run record %s: %w", runDir.RequestPath(), err)
			}
			if strings.TrimSpace(request.Bee) == "" {
				return fmt.Errorf("skills: load: run record %s names no role", runDir.RequestPath())
			}

			resolved, err := resolveRunSkillSet(colonyRoot, runDir, request)
			if err != nil {
				return err
			}
			skill, err := skills.Offered(resolved, args[0])
			if err != nil {
				var refused *skills.NotOfferedError
				if errors.As(err, &refused) {
					return newExitError(skillLoadExitRefused, fmt.Errorf("skills: load: %w", err))
				}
				return err
			}

			body, contentHash, err := skills.Served(skill)
			if err != nil {
				return fmt.Errorf("skills: load: %w", err)
			}

			out := cmd.OutOrStdout()
			fmt.Fprintf(out, "path: %s\n", skill.Path)
			fmt.Fprintf(out, "dir:  %s\n\n", skill.Dir)
			body = strings.TrimLeft(body, "\n")
			if _, err := fmt.Fprint(out, body); err != nil {
				return err
			}
			if body == "" || !strings.HasSuffix(body, "\n") {
				if _, err := fmt.Fprintln(out); err != nil {
					return err
				}
			}

			// Record after a successful serve: a line exists only for a body the
			// run actually received, and one line means one load.
			return skills.AppendUsage(runDir.SkillsUsedPath(), skills.UsageRecord{
				Name:        skill.Name,
				ContentHash: contentHash,
				At:          time.Now().UTC().Format(time.RFC3339),
			})
		},
	}
	cmd.Flags().StringVarP(&startDir, "path", "C", "", "colony root, or a directory inside it")
	cmd.Flags().StringVar(&traceID, "trace", "", "flight trail id of the run (required)")
	cmd.Flags().StringVar(&agentID, "agent", "", "agent run id of the run (required)")
	_ = cmd.MarkFlagRequired("trace")
	_ = cmd.MarkFlagRequired("agent")
	return cmd
}

// resolveRunSkillSet resolves the gate the run was offered. A run bound to a
// trail reads its frozen configuration snapshot and never falls back to the
// live checkout; an unbound run resolves the live colony the way its dispatch
// did.
func resolveRunSkillSet(colonyRoot string, runDir runs.Dir, request protocol.Request) (skills.RunResolution, error) {
	ctxColony := colony.Context{ColonyRoot: colonyRoot}
	configRef := strings.TrimSpace(request.ConfigRef)
	if configRef != "" {
		bound, err := trailconfig.BindRef(ctxColony, runDir.TraceID, configRef, request.ConfigRevision)
		if err != nil {
			return skills.RunResolution{}, fmt.Errorf("skills: load: bind the run's configuration: %w", err)
		}
		resolved, err := skills.ResolveRun(skills.RunInput{
			ConfigRoot: bound.ConfigRoot,
			Frozen:     true,
			Role:       request.Bee,
			Profile:    bound.Profile,
		})
		if err != nil {
			return skills.RunResolution{}, fmt.Errorf("skills: load: %w", err)
		}
		return resolved, nil
	}

	manifest, err := colony.LoadColony(colonyRoot)
	if err != nil {
		return skills.RunResolution{}, fmt.Errorf("skills: load: read colony configuration at %s: %w", colonyRoot, err)
	}
	origin, _ := gitroot.OriginURL(colonyRoot)
	slug := colony.ResolveSlug(colonyRoot, manifest, origin)
	resolved, err := skills.ResolveRun(skills.RunInput{
		ConfigRoot: colonyRoot,
		Slug:       slug,
		Role:       request.Bee,
		Profile:    request.Profile,
	})
	if err != nil {
		return skills.RunResolution{}, fmt.Errorf("skills: load: %w", err)
	}
	return resolved, nil
}

// resolveSkillLoadRoot finds the colony that holds this run. The order is -C,
// then PASEKA_COLONY_ROOT, then the nearest ancestor of the working directory
// holding both .paseka/colony.yaml and the run directory. The run directory is
// part of the test because a git worktree carries the committed `.paseka/` but
// not the gitignored runs, so walking up from `.paseka/worktrees/<traceId>/`
// reaches the primary checkout instead of stopping at the worktree.
func resolveSkillLoadRoot(startDir, traceID, agentID string) (string, error) {
	var tried []string
	seen := map[string]bool{}
	check := func(dir string) (string, bool) {
		if strings.TrimSpace(dir) == "" {
			return "", false
		}
		abs, err := filepath.Abs(dir)
		if err != nil {
			return "", false
		}
		if !seen[abs] {
			seen[abs] = true
			tried = append(tried, abs)
		}
		if !holdsColonyManifest(abs) {
			return "", false
		}
		runRoot := filepath.Join(abs, ".paseka", "runs", traceID, agentID)
		if info, err := os.Stat(runRoot); err != nil || !info.IsDir() {
			return "", false
		}
		return abs, true
	}

	if root, ok := check(startDir); ok {
		return root, nil
	}
	if root, ok := check(os.Getenv("PASEKA_COLONY_ROOT")); ok {
		return root, nil
	}
	wd, err := os.Getwd()
	if err != nil {
		return "", fmt.Errorf("skills: load: resolve working directory: %w", err)
	}
	for dir := wd; ; {
		if root, ok := check(dir); ok {
			return root, nil
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			break
		}
		dir = parent
	}

	return "", fmt.Errorf(
		"skills: load: no colony root holds trace %q and agent %q; tried: %s; pass -C <colonyRoot> to name the colony that holds the run",
		traceID, agentID, strings.Join(tried, ", "),
	)
}

// holdsColonyManifest reports whether dir is a colony root. It is a file test
// rather than a git lookup because load must work from a worktree, where the
// nearest ancestor with a manifest and the run is the primary checkout.
func holdsColonyManifest(dir string) bool {
	info, err := os.Stat(filepath.Join(dir, ".paseka", "colony.yaml"))
	return err == nil && !info.IsDir()
}
