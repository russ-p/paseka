package main

import (
	"encoding/json"
	"fmt"
	"io"
	"text/tabwriter"

	"github.com/russ-p/paseka/internal/colony"
	"github.com/russ-p/paseka/internal/console"
	"github.com/spf13/cobra"
)

func newProposalListCmd() *cobra.Command {
	var (
		startDir string
		traceID  string
		jsonOut  bool
	)
	cmd := &cobra.Command{
		Use:   "list",
		Short: "List tasks waiting for human review",
		Long: `List the tasks that stopped at a review gate — the ids proposal approve,
reject, and discard act on.

The queue is Queen Console's Reviews projection, read from the same code
(internal/console.ListReviewQueue): every task in waiting_review whose review
policy is required or final, across the same 30 most recently active trails
(hiveview.TaskBoardTraceLimit). A gate on an older trail is outside that window
and does not appear here, with or without --trace; inspect it directly with
paseka task show --trace <id>.

Each row prints <traceId>/<taskId>, so the pair can be passed verbatim to the
decision verbs. --json prints the shared queue item as a JSON array, with the
same keys as the Console API. The command only reads: it publishes no event and
writes no ledger entry.`,
		RunE: func(cmd *cobra.Command, args []string) error {
			ctxColony, err := colony.ResolveContext(startDir)
			if err != nil {
				return err
			}
			queue, err := console.ListReviewQueue(ctxColony)
			if err != nil {
				return err
			}
			items := filterReviewQueue(queue.Items, traceID)

			out := cmd.OutOrStdout()
			if jsonOut {
				// An empty queue is `[]`, not `null`: a consumer ranges over the
				// array without a nil check.
				if items == nil {
					items = []console.ReviewQueueItem{}
				}
				encoder := json.NewEncoder(out)
				encoder.SetIndent("", "  ")
				return encoder.Encode(items)
			}

			if len(items) == 0 {
				if traceID != "" {
					fmt.Fprintf(out, "No reviews waiting on trace %s.\n", traceID)
					return nil
				}
				fmt.Fprintln(out, "No reviews waiting.")
				return nil
			}
			writeReviewQueue(out, items)
			return nil
		},
	}
	cmd.Flags().StringVarP(&startDir, "path", "C", "", "directory inside the git repository")
	cmd.Flags().StringVar(&traceID, "trace", "", "narrow the queue to one flight trail id")
	cmd.Flags().BoolVar(&jsonOut, "json", false, "emit the queue as JSON on stdout")
	return cmd
}

// filterReviewQueue narrows the shared projection to one trail. The narrowing
// runs after the projection rather than inside it: the queue's scope is the same
// recent-traces window Console reads, and --trace must not change which trails
// that window contains.
func filterReviewQueue(items []console.ReviewQueueItem, traceID string) []console.ReviewQueueItem {
	if traceID == "" {
		return items
	}
	narrowed := make([]console.ReviewQueueItem, 0, len(items))
	for _, item := range items {
		if item.TraceID == traceID {
			narrowed = append(narrowed, item)
		}
	}
	return narrowed
}

// writeReviewQueue prints one row per review gate in the shared queue order —
// newest activity first. The first column is the trace/task pair the decision
// verbs take, so a row is usable without copying two separate ids.
func writeReviewQueue(w io.Writer, items []console.ReviewQueueItem) {
	tab := tabwriter.NewWriter(w, 0, 0, 2, ' ', 0)
	fmt.Fprintf(tab, "TRACE/TASK\tPHASE\tREVIEW\tTITLE\n")
	for _, item := range items {
		review := item.Review
		if review == "" {
			review = "-"
		}
		title := item.Title
		if title == "" {
			title = item.TaskID
		}
		fmt.Fprintf(tab, "%s/%s\t%s\t%s\t%s\n", item.TraceID, item.TaskID, item.Phase, review, title)
	}
	_ = tab.Flush()
}
