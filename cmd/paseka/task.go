package main

import (
	"fmt"
	"io"
	"os"
	"sort"
	"strings"
	"text/tabwriter"

	"github.com/paseka/paseka/internal/bus"
	"github.com/paseka/paseka/internal/colony"
	"github.com/paseka/paseka/internal/protocol"
	"github.com/paseka/paseka/internal/runs"
	"github.com/paseka/paseka/internal/taskledger"
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
			resolvedTitle := deriveTaskTitle(title, resolvedBody)
			if resolvedTitle == "" && strings.TrimSpace(resolvedBody) == "" {
				return fmt.Errorf("provide --title and/or task body via --body, --file, or --stdin")
			}

			if traceID == "" {
				id, err := colony.NewTraceID()
				if err != nil {
					return err
				}
				traceID = id
			}
			if taskID == "" {
				id, err := colony.NewTaskID()
				if err != nil {
					return err
				}
				taskID = id
			}
			if bee == "" {
				bee = "builder"
			}

			ctxColony, client, _, cleanup, err := openTaskLedger(startDir)
			if err != nil {
				return err
			}
			defer cleanup()
			if client == nil {
				return fmt.Errorf("nats url not configured (task create requires NATS)")
			}
			if sector != "" {
				manifest, err := colony.LoadColony(ctxColony.ColonyRoot)
				if err != nil {
					return err
				}
				if _, err := manifest.ResolveSector(sector); err != nil {
					return err
				}
			}

			spec := protocol.TaskSpec{
				TaskID:    taskID,
				Title:     resolvedTitle,
				Body:      strings.TrimSpace(resolvedBody),
				Bee:       bee,
				Sector:    sector,
				Intent:    intent,
				DependsOn: parseDependsOn(dependsOn),
			}
			planEv, err := taskPlanEvent(traceID, spec)
			if err != nil {
				return err
			}
			if err := client.PublishEvent(cmd.Context(), planEv); err != nil {
				return err
			}

			if autorun {
				readyEv, err := taskReadyEvent(traceID, taskledger.TaskSnapshot{
					TaskID: taskID,
					Title:  resolvedTitle,
					Body:   strings.TrimSpace(resolvedBody),
					Bee:    bee,
					Sector: sector,
					Intent: intent,
				})
				if err != nil {
					return err
				}
				if err := client.PublishEvent(cmd.Context(), readyEv); err != nil {
					return err
				}
			}

			printTaskCreateResult(traceID, taskID, bee, autorun)
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
			ctxColony, client, ledger, cleanup, err := openTaskLedger(startDir)
			if err != nil {
				return err
			}
			defer cleanup()
			if client == nil {
				return fmt.Errorf("nats url not configured (task start requires JetStream KV)")
			}

			snap, err := ledger.Snapshot(traceID)
			if err != nil {
				return err
			}
			tasks, err := tasksToStart(snap, taskID)
			if err != nil {
				return err
			}

			for _, task := range tasks {
				ev, err := taskReadyEvent(traceID, task)
				if err != nil {
					return err
				}
				if err := client.PublishEvent(cmd.Context(), ev); err != nil {
					return err
				}
				fmt.Printf("Published task.ready for %s on trace %s (bee=%s)\n", task.TaskID, traceID, task.Bee)
			}
			fmt.Println("\nEnsure paseka run is active to dispatch queued tasks.")
			_ = ctxColony
			return nil
		},
	}
	cmd.Flags().StringVarP(&startDir, "path", "C", "", "directory inside the git repository")
	cmd.Flags().StringVar(&traceID, "trace", "", "flight trail id")
	cmd.Flags().StringVar(&taskID, "task", "", "task id (optional; starts all eligible planned tasks when omitted)")
	_ = cmd.MarkFlagRequired("trace")
	return cmd
}

type taskTraceSource string

const (
	sourceKV taskTraceSource = "jetstream-kv"
	sourceFS taskTraceSource = "filesystem"
)

func loadTaskTrace(startDir, traceID string) (colony.Context, taskledger.TraceSnapshot, taskTraceSource, error) {
	ctxColony, err := colony.ResolveContext(startDir)
	if err != nil {
		return colony.Context{}, taskledger.TraceSnapshot{}, "", err
	}
	_, client, ledger, cleanup, err := openTaskLedger(startDir)
	if err != nil {
		return colony.Context{}, taskledger.TraceSnapshot{}, "", err
	}
	defer cleanup()
	if ledger != nil {
		snap, err := ledger.Snapshot(traceID)
		if err != nil {
			return colony.Context{}, taskledger.TraceSnapshot{}, "", err
		}
		if len(snap.Tasks) > 0 {
			_ = client
			return ctxColony, snap, sourceKV, nil
		}
	}
	snap, err := runs.LoadTraceTasksFromFS(ctxColony.ColonyRoot, traceID)
	if err != nil {
		return colony.Context{}, taskledger.TraceSnapshot{}, "", err
	}
	return ctxColony, snap, sourceFS, nil
}

func openTaskLedger(startDir string) (colony.Context, *bus.Client, taskledger.Ledger, func(), error) {
	ctxColony, err := colony.ResolveContext(startDir)
	if err != nil {
		return colony.Context{}, nil, nil, func() {}, err
	}
	client, err := bus.ConnectColony(ctxColony, false)
	if err != nil {
		return colony.Context{}, nil, nil, func() {}, err
	}
	if client == nil {
		return ctxColony, nil, nil, func() {}, nil
	}
	kv, err := client.JetStream().KeyValue(bus.TaskLedgerBucket(ctxColony.Slug))
	if err != nil {
		client.Close()
		return colony.Context{}, nil, nil, func() {}, fmt.Errorf("task ledger kv: %w", err)
	}
	return ctxColony, client, taskledger.NewKVLedger(kv), func() { client.Close() }, nil
}

func tasksToStart(snap taskledger.TraceSnapshot, taskID string) ([]taskledger.TaskSnapshot, error) {
	if taskID != "" {
		task, err := taskledger.CanStart(snap, taskID)
		if err == taskledger.ErrTaskAlreadyReady {
			return nil, fmt.Errorf("task %q is already ready", taskID)
		}
		if err != nil {
			return nil, err
		}
		return []taskledger.TaskSnapshot{task}, nil
	}
	eligible := taskledger.EligiblePlanned(snap)
	if len(eligible) == 0 {
		return nil, taskledger.ErrNoEligibleTasks
	}
	return []taskledger.TaskSnapshot{eligible[0]}, nil
}

func taskReadyEvent(traceID string, task taskledger.TaskSnapshot) (protocol.Event, error) {
	bee := task.Bee
	if bee == "" {
		bee = "builder"
	}
	return protocol.NewEvent(traceID, "cli", 0, protocol.EventSignal, protocol.TaskReadyPayload{
		Kind:   protocol.TaskEventReady,
		TaskID: task.TaskID,
		Title:  task.Title,
		Body:   task.Body,
		Bee:    bee,
		Sector: task.Sector,
		Intent: task.Intent,
	})
}

func taskPlanEvent(traceID string, spec protocol.TaskSpec) (protocol.Event, error) {
	return protocol.NewEvent(traceID, "cli", 0, protocol.EventInsight, protocol.TaskPlanPayload{
		Kind:  protocol.TaskEventPlan,
		Tasks: []protocol.TaskSpec{spec},
	})
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

func deriveTaskTitle(title, body string) string {
	if strings.TrimSpace(title) != "" {
		return strings.TrimSpace(title)
	}
	for _, line := range strings.Split(body, "\n") {
		line = strings.TrimSpace(line)
		if line != "" {
			if len(line) > 120 {
				return line[:120]
			}
			return line
		}
	}
	return ""
}

func parseDependsOn(values []string) []string {
	var out []string
	for _, value := range values {
		for _, part := range strings.Split(value, ",") {
			part = strings.TrimSpace(part)
			if part != "" {
				out = append(out, part)
			}
		}
	}
	return out
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
