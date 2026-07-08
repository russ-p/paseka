package main

import (
	"fmt"
	"io"
	"os"
	"sort"
	"strings"
	"text/tabwriter"

	"github.com/paseka/paseka/internal/colony"
	"github.com/paseka/paseka/internal/runs"
	"github.com/paseka/paseka/internal/taskledger"
	"github.com/paseka/paseka/internal/tasks"
	"github.com/spf13/cobra"
)

func newTaskCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "task",
		Short: "Inspect and enqueue trace tasks",
	}
	cmd.AddCommand(newTaskListCmd())
	cmd.AddCommand(newTaskShowCmd())
	cmd.AddCommand(newTaskCreateCmd())
	cmd.AddCommand(newTaskStartCmd())
	return cmd
}

func newTaskListCmd() *cobra.Command {
	var startDir string
	cmd := &cobra.Command{
		Use:   "list",
		Short: "List tasks for a trace",
		RunE: func(cmd *cobra.Command, args []string) error {
			traceID, err := cmd.Flags().GetString("trace")
			if err != nil {
				return err
			}
			if traceID == "" {
				return fmt.Errorf("--trace is required")
			}
			ctxColony, snap, source, err := loadTaskTrace(startDir, traceID)
			if err != nil {
				return err
			}
			if len(snap.Tasks) == 0 {
				fmt.Printf("No tasks found for trace %s (%s)\n", traceID, source)
				return nil
			}
			ids := sortedTaskIDs(snap)
			w := tabwriter.NewWriter(os.Stdout, 0, 0, 2, ' ', 0)
			fmt.Fprintf(w, "TASK\tSTATUS\tBEE\tSECTOR\tTITLE\n")
			for _, id := range ids {
				task := snap.Tasks[id]
				title := task.Title
				if title == "" {
					title = task.TaskID
				}
				fmt.Fprintf(w, "%s\t%s\t%s\t%s\t%s\n", task.TaskID, task.Status, task.Bee, task.Sector, title)
			}
			_ = w.Flush()
			fmt.Printf("\nSource: %s (%s)\n", source, ctxColony.ColonyRoot)
			return nil
		},
	}
	cmd.Flags().StringVarP(&startDir, "path", "C", "", "directory inside the git repository")
	cmd.Flags().String("trace", "", "flight trail id")
	_ = cmd.MarkFlagRequired("trace")
	return cmd
}

func newTaskShowCmd() *cobra.Command {
	var startDir string
	cmd := &cobra.Command{
		Use:   "show",
		Short: "Show one task and related runs",
		RunE: func(cmd *cobra.Command, args []string) error {
			traceID, err := cmd.Flags().GetString("trace")
			if err != nil {
				return err
			}
			taskID, err := cmd.Flags().GetString("task")
			if err != nil {
				return err
			}
			if traceID == "" || taskID == "" {
				return fmt.Errorf("--trace and --task are required")
			}
			ctxColony, snap, source, err := loadTaskTrace(startDir, traceID)
			if err != nil {
				return err
			}
			task, ok := snap.Tasks[taskID]
			if !ok {
				return fmt.Errorf("task %q not found in trace %s (%s)", taskID, traceID, source)
			}

			fmt.Printf("Task %s on trace %s\n", taskID, traceID)
			fmt.Printf("  status:    %s\n", task.Status)
			if task.Bee != "" {
				fmt.Printf("  bee:       %s\n", task.Bee)
			}
			if task.Sector != "" {
				fmt.Printf("  sector:    %s\n", task.Sector)
			}
			if task.Title != "" {
				fmt.Printf("  title:     %s\n", task.Title)
			}
			if len(task.DependsOn) > 0 {
				fmt.Printf("  depends:   %s\n", strings.Join(task.DependsOn, ", "))
			}
			if task.Summary != "" {
				fmt.Printf("  summary:   %s\n", task.Summary)
			}
			if task.Commit != "" {
				fmt.Printf("  commit:    %s\n", task.Commit)
			}
			if !task.UpdatedAt.IsZero() {
				fmt.Printf("  updated:   %s\n", task.UpdatedAt.UTC().Format("2006-01-02T15:04:05Z"))
			}
			if strings.TrimSpace(task.Body) != "" {
				fmt.Printf("\nDescription:\n%s\n", strings.TrimSpace(task.Body))
			}

			taskDir, err := runs.NewTaskDir(ctxColony.ColonyRoot, traceID, taskID)
			if err != nil {
				return err
			}
			fmt.Printf("\nProjection: %s\n", taskDir.TaskPath())
			runEntries, err := taskDir.ReadTaskRuns()
			if err != nil {
				return err
			}
			if len(runEntries) == 0 {
				fmt.Println("\nRuns: none")
				return nil
			}
			fmt.Println("\nRuns:")
			for i, entry := range runEntries {
				line := fmt.Sprintf("  %d. agent=%s status=%s", i+1, entry.AgentID, entry.RunStatus)
				if entry.Bee != "" {
					line += " bee=" + entry.Bee
				}
				if entry.RunDir != "" {
					line += " dir=" + entry.RunDir
				}
				fmt.Println(line)
			}
			return nil
		},
	}
	cmd.Flags().StringVarP(&startDir, "path", "C", "", "directory inside the git repository")
	cmd.Flags().String("trace", "", "flight trail id")
	cmd.Flags().String("task", "", "task id")
	_ = cmd.MarkFlagRequired("trace")
	_ = cmd.MarkFlagRequired("task")
	return cmd
}

func newTaskCreateCmd() *cobra.Command {
	var (
		startDir  string
		traceID   string
		taskID    string
		title     string
		body      string
		bodyFile  string
		fromStdin bool
		bee       string
		sector    string
		intent    string
		dependsOn []string
		autorun   bool
	)
	cmd := &cobra.Command{
		Use:   "create",
		Short: "Create a new task and publish task.plan",
		Long:  "Creates a trace and task when omitted, publishes task.plan, and optionally publishes task.ready with --autorun. Execution requires a separate paseka run process.",
		RunE: func(cmd *cobra.Command, args []string) error {
			resolvedBody, err := resolveTaskCreateBody(body, bodyFile, fromStdin, os.Stdin)
			if err != nil {
				return err
			}

			session, err := openTaskSession(startDir)
			if err != nil {
				return err
			}
			defer session.Close()

			res, err := tasks.Create(cmd.Context(), session, tasks.CreateInput{
				TraceID:   traceID,
				TaskID:    taskID,
				Title:     title,
				Body:      resolvedBody,
				Bee:       bee,
				Sector:    sector,
				Intent:    intent,
				DependsOn: dependsOn,
				Autorun:   autorun,
				AgentID:   "cli",
			})
			if err != nil {
				return err
			}

			printTaskCreateResult(res.TraceID, res.TaskID, res.Bee, res.Autorun)
			return nil
		},
	}
	cmd.Flags().StringVarP(&startDir, "path", "C", "", "directory inside the git repository")
	cmd.Flags().StringVar(&traceID, "trace", "", "flight trail id (generated when omitted)")
	cmd.Flags().StringVar(&taskID, "task", "", "task id (generated when omitted)")
	cmd.Flags().StringVar(&title, "title", "", "task title")
	cmd.Flags().StringVar(&body, "body", "", "task body text")
	cmd.Flags().StringVar(&bodyFile, "file", "", "read task body from file")
	cmd.Flags().BoolVar(&fromStdin, "stdin", false, "read task body from stdin")
	cmd.Flags().StringVar(&bee, "bee", "", "bee role (default: builder)")
	cmd.Flags().StringVar(&sector, "sector", "", "colony sector name (from colony.yaml sectors)")
	cmd.Flags().StringVar(&intent, "intent", "", "builder task intent: general, feature, bugfix, test-fix, refactor")
	cmd.Flags().StringSliceVar(&dependsOn, "depends-on", nil, "task dependencies (repeatable or comma-separated)")
	cmd.Flags().BoolVar(&autorun, "autorun", false, "publish task.ready immediately after task.plan")
	return cmd
}

func newTaskStartCmd() *cobra.Command {
	var (
		startDir string
		traceID  string
		taskID   string
	)
	cmd := &cobra.Command{
		Use:   "start",
		Short: "Enqueue eligible task(s) by publishing task.ready",
		Long:  "Publishes task.ready for eligible planned tasks. Actual execution requires a separate paseka run process.",
		RunE: func(cmd *cobra.Command, args []string) error {
			if traceID == "" {
				return fmt.Errorf("--trace is required")
			}
			session, err := openTaskSession(startDir)
			if err != nil {
				return err
			}
			defer session.Close()

			started, err := tasks.Start(cmd.Context(), session, traceID, taskID, "cli")
			if err != nil {
				return err
			}

			for _, task := range started {
				fmt.Printf("Published task.ready for %s on trace %s (bee=%s)\n", task.TaskID, traceID, task.Bee)
			}
			fmt.Println("\nEnsure paseka run is active to dispatch queued tasks.")
			return nil
		},
	}
	cmd.Flags().StringVarP(&startDir, "path", "C", "", "directory inside the git repository")
	cmd.Flags().StringVar(&traceID, "trace", "", "flight trail id")
	cmd.Flags().StringVar(&taskID, "task", "", "task id (optional; starts all eligible planned tasks when omitted)")
	_ = cmd.MarkFlagRequired("trace")
	return cmd
}

func loadTaskTrace(startDir, traceID string) (colony.Context, taskledger.TraceSnapshot, tasks.Source, error) {
	ctxColony, err := colony.ResolveContext(startDir)
	if err != nil {
		return colony.Context{}, taskledger.TraceSnapshot{}, "", err
	}
	session, err := openTaskSession(startDir)
	if err != nil {
		return colony.Context{}, taskledger.TraceSnapshot{}, "", err
	}
	defer session.Close()
	snap, source, err := tasks.LoadTrace(ctxColony, session.Ledger, traceID)
	if err != nil {
		return colony.Context{}, taskledger.TraceSnapshot{}, "", err
	}
	return ctxColony, snap, source, nil
}

func openTaskSession(startDir string) (*tasks.LedgerSession, error) {
	ctxColony, err := colony.ResolveContext(startDir)
	if err != nil {
		return nil, err
	}
	return tasks.OpenLedger(ctxColony)
}

func resolveTaskCreateBody(body, filePath string, fromStdin bool, stdin io.Reader) (string, error) {
	sources := 0
	if strings.TrimSpace(body) != "" {
		sources++
	}
	if filePath != "" {
		sources++
	}
	if fromStdin {
		sources++
	}
	if sources > 1 {
		return "", fmt.Errorf("use only one body source: --body, --file, or --stdin")
	}
	if body != "" {
		return body, nil
	}
	if filePath != "" {
		data, err := os.ReadFile(filePath)
		if err != nil {
			return "", fmt.Errorf("read task body file: %w", err)
		}
		return string(data), nil
	}
	if fromStdin {
		data, err := io.ReadAll(stdin)
		if err != nil {
			return "", fmt.Errorf("read task body from stdin: %w", err)
		}
		return string(data), nil
	}
	return "", nil
}

func printTaskCreateResult(traceID, taskID, bee string, autorun bool) {
	fmt.Println("Created task")
	fmt.Printf("  trace: %s\n", traceID)
	fmt.Printf("  task:  %s\n", taskID)
	fmt.Printf("  bee:   %s\n", bee)
	if autorun {
		fmt.Println("\nPublished task.ready. Ensure paseka run is active to dispatch the task.")
	} else {
		fmt.Printf("\nStart:\n  paseka task start --trace %s --task %s\n", traceID, taskID)
	}
	fmt.Printf("\nInspect:\n  paseka task show --trace %s --task %s\n", traceID, taskID)
}

func sortedTaskIDs(snap taskledger.TraceSnapshot) []string {
	ids := make([]string, 0, len(snap.Tasks))
	for id := range snap.Tasks {
		ids = append(ids, id)
	}
	sort.Strings(ids)
	return ids
}
