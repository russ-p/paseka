package runs

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/russ-p/paseka/internal/logging"
	"github.com/russ-p/paseka/internal/protocol"
)

var traceLog = logging.Component("runs")

// TrailReadIssue classifies retained trail evidence the merged event read could
// not use. It is reported, never repaired: a projection that hid a damaged
// record would present the surviving lines as the whole history.
type TrailReadIssue struct {
	// Kind is TrailIssueUnreadable when the file could not be opened or read,
	// and TrailIssueDamaged when individual lines were skipped.
	Kind   string
	Path   string
	Detail string
}

const (
	// TrailIssueUnreadable means a whole event record could not be read.
	TrailIssueUnreadable = "unreadable"
	// TrailIssueDamaged means some lines of an event record were unparseable.
	TrailIssueDamaged = "damaged"
)

// ReadTraceEvents loads one trail's history: the union of its per-run event
// logs and its trail-level record, each event once, oldest first.
//
// This is the single reader behind the timeline, the export, prompt memory,
// trail titles and summaries, and worktree branch and pull-request body
// resolution, so none of them can drift apart. It reads the filesystem only —
// no broker is required to answer "what happened".
func ReadTraceEvents(colonyRoot, traceID string) ([]protocol.Event, error) {
	events, _, err := ReadTraceEventsReport(colonyRoot, traceID)
	return events, err
}

// ReadTraceEventsReport is ReadTraceEvents with the diagnostics a measurement
// needs: the same merged, deduplicated history plus every record it could not
// use. A damaged or unreadable record costs its own events and is reported,
// never silently treated as an empty history.
func ReadTraceEventsReport(colonyRoot, traceID string) ([]protocol.Event, []TrailReadIssue, error) {
	if colonyRoot == "" || traceID == "" {
		return nil, nil, fmt.Errorf("runs: colony root and traceId are required")
	}
	trailRoot := filepath.Join(colonyRoot, ".paseka", "runs", traceID)
	entries, err := os.ReadDir(trailRoot)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil, nil
		}
		return nil, nil, fmt.Errorf("runs: list trace %s: %w", traceID, err)
	}

	var issues []TrailReadIssue
	var perRun [][]protocol.Event
	for _, ent := range entries {
		if !ent.IsDir() || skipTraceEventDir(ent.Name()) {
			continue
		}
		agentID := ent.Name()
		runDir := Dir{ColonyRoot: colonyRoot, TraceID: traceID, AgentID: agentID}
		events, skipped, err := readEventsFileReport(runDir.EventsPath(), false)
		if err != nil {
			// One unreadable run log must not take down the colony-wide feed:
			// this reader backs the Console timeline, export, and prompt memory,
			// so a hard failure here hides every other trail as well. Skip the
			// file and say so, the way loadTraceSummary already does for a run
			// whose meta will not parse.
			issues = append(issues, TrailReadIssue{
				Kind:   TrailIssueUnreadable,
				Path:   runDir.EventsPath(),
				Detail: fmt.Sprintf("run %s event log: %v", agentID, err),
			})
			traceLog.Warn("skip unreadable run event log",
				logging.F("traceId", traceID),
				logging.F("agentId", agentID),
				logging.F("path", runDir.EventsPath()),
				logging.F("error", err.Error()))
			continue
		}
		if skipped > 0 {
			issues = append(issues, TrailReadIssue{
				Kind:   TrailIssueDamaged,
				Path:   runDir.EventsPath(),
				Detail: fmt.Sprintf("run %s event log: %d unparseable line(s) skipped", agentID, skipped),
			})
		}
		perRun = append(perRun, events)
	}

	trail := Trail{ColonyRoot: colonyRoot, TraceID: traceID}
	trailEvents, skipped, err := readEventsFileReport(trail.EventsPath(), false)
	if err != nil {
		// Same rule as a run log: a damaged file costs its own trail its
		// events, never the whole colony's read.
		issues = append(issues, TrailReadIssue{
			Kind:   TrailIssueUnreadable,
			Path:   trail.EventsPath(),
			Detail: fmt.Sprintf("trail event log: %v", err),
		})
		traceLog.Warn("skip unreadable trail event log",
			logging.F("traceId", traceID),
			logging.F("path", trail.EventsPath()),
			logging.F("error", err.Error()))
		trailEvents = nil
	} else if skipped > 0 {
		issues = append(issues, TrailReadIssue{
			Kind:   TrailIssueDamaged,
			Path:   trail.EventsPath(),
			Detail: fmt.Sprintf("trail event log: %d unparseable line(s) skipped", skipped),
		})
	}

	return mergeTrailEvents(append(perRun, trailEvents)...), issues, nil
}

// sortTraceEvents orders trail history oldest first, and totally: creation
// time, then the per-store sequence, then the producer, then the minted
// identity. The sequence still decides ties within one store — that is what the
// last-write-wins resolvers above it have always read — while identity makes the
// order total where nothing else can separate two records, so paging through a
// trail never repeats or skips a row.
func sortTraceEvents(events []protocol.Event) {
	sort.SliceStable(events, func(i, j int) bool {
		a, b := events[i], events[j]
		if !a.CreatedAt.Equal(b.CreatedAt) {
			return a.CreatedAt.Before(b.CreatedAt)
		}
		if a.Seq != b.Seq {
			return a.Seq < b.Seq
		}
		if c := strings.Compare(a.AgentID, b.AgentID); c != 0 {
			return c < 0
		}
		return a.ID < b.ID
	})
}
