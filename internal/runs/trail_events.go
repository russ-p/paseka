package runs

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/russ-p/paseka/internal/logging"
	"github.com/russ-p/paseka/internal/protocol"
)

// TrailEventsFileName is the trail-level record: every domain event the colony
// decided to publish for one flight trail, whatever produced it. It is
// deliberately not named events.ndjson — a run directory's events.ndjson is a
// per-attempt audit of what one run published, and a path fragment gets quoted
// on its own, so the two files must not read alike.
const TrailEventsFileName = "trail-events.ndjson"

// Trail holds paths for one flight trail's run root.
type Trail struct {
	ColonyRoot string
	TraceID    string
}

// Root returns .paseka/runs/<traceId>/ under colony root — beside the run
// directories rather than inside one of them, so the record survives a run
// being retried and has a place for producers that are not dispatches at all.
func (t Trail) Root() string {
	return filepath.Join(t.ColonyRoot, ".paseka", "runs", t.TraceID)
}

func (t Trail) EventsPath() string { return filepath.Join(t.Root(), TrailEventsFileName) }

// AppendEvent records one event in the trail log. The trail root is created on
// demand: an operator's intake request is a trail's first record, long before
// any run directory exists.
//
// No sequence is allocated here. A per-trail counter would be a read-modify-
// write across processes, and the trail log is ordered by creation time with
// identity as the tie-break, so a zero seq carries no loss.
func (t Trail) AppendEvent(ev protocol.Event) error {
	if t.ColonyRoot == "" || t.TraceID == "" {
		return fmt.Errorf("runs: colony root and traceId are required")
	}
	if ev.ProtocolVersion == "" {
		ev.ProtocolVersion = protocol.Version
	}
	if err := os.MkdirAll(t.Root(), 0o755); err != nil {
		return fmt.Errorf("runs: mkdir %s: %w", t.Root(), err)
	}
	data, err := json.Marshal(ev)
	if err != nil {
		return err
	}
	f, err := os.OpenFile(t.EventsPath(), os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0o644)
	if err != nil {
		return err
	}
	defer f.Close()
	if _, err := f.Write(append(data, '\n')); err != nil {
		return err
	}
	return nil
}

// ReadTrailEvents loads the trail-level record only. A colony-wide read uses
// this: one file per trail, bounded per trail instead of per run.
func ReadTrailEvents(colonyRoot, traceID string) ([]protocol.Event, error) {
	if colonyRoot == "" || traceID == "" {
		return nil, fmt.Errorf("runs: colony root and traceId are required")
	}
	trail := Trail{ColonyRoot: colonyRoot, TraceID: traceID}
	events, err := readEventsFile(trail.EventsPath())
	if err != nil {
		return nil, fmt.Errorf("runs: read trail events %s: %w", traceID, err)
	}
	return events, nil
}

// readEventsFile parses one NDJSON event log, returning nil for a missing file.
//
// A line that will not parse is skipped with a diagnostic rather than failing the
// file. Both logs are written by appending a line at a time, so a process that
// dies mid-append leaves a torn last line — and a reader that gave up there would
// throw away every event before it, which is the opposite of what an append-only
// record is for. The line is lost, the rest of the history is not, and the
// diagnostic names the file and the line so the damage is visible rather than
// silent.
func readEventsFile(path string) ([]protocol.Event, error) {
	f, err := os.Open(path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, err
	}
	defer f.Close()

	var events []protocol.Event
	scanner := newLineScanner(f)
	for line := 1; scanner.Scan(); line++ {
		raw := strings.TrimSpace(scanner.Text())
		if raw == "" {
			continue
		}
		var ev protocol.Event
		if err := json.Unmarshal([]byte(raw), &ev); err != nil {
			traceLog.Warn("skip unparseable event line",
				logging.F("path", path),
				logging.F("line", strconv.Itoa(line)),
				logging.F("error", err.Error()))
			continue
		}
		events = append(events, ev)
	}
	if err := scanner.Err(); err != nil {
		return nil, err
	}
	return events, nil
}

// DedupeTrailEvents returns one copy per event, keeping the first occurrence.
//
// The trail record is append-only and its writer is at-least-once — the seam
// records before it publishes, so a refused publish leaves the event in the
// record and the retry appends it again — which means a raw read can hold the
// same event twice. Every projection that feeds an operator deduplicates on
// identity: a repeated row is not a second event, and a repeated cursor key
// would make paging repeat the row forever.
//
// Records written before identities existed cannot be matched and are kept: they
// exist in one store only, so they cannot collide with anything.
func DedupeTrailEvents(events []protocol.Event) []protocol.Event {
	if len(events) == 0 {
		return nil
	}
	out := make([]protocol.Event, 0, len(events))
	seen := make(map[string]struct{}, len(events))
	for _, ev := range events {
		if ev.ID != "" {
			if _, dup := seen[ev.ID]; dup {
				continue
			}
			seen[ev.ID] = struct{}{}
		}
		out = append(out, ev)
	}
	return out
}

// mergeTrailEvents returns the union of a trail's per-run logs and its trail
// log: every event once, in a total order that is stable for paging. An event
// present in both stores carries one identity and appears once; an event
// retried or re-flushed appears once. Records written before identities existed
// are kept and ordered by their previous key — they exist in one store only, so
// they cannot collide with anything.
func mergeTrailEvents(groups ...[]protocol.Event) []protocol.Event {
	var all []protocol.Event
	seen := map[string]struct{}{}
	for _, group := range groups {
		for _, ev := range DedupeTrailEvents(group) {
			if ev.ID != "" {
				if _, dup := seen[ev.ID]; dup {
					continue
				}
				seen[ev.ID] = struct{}{}
			}
			all = append(all, ev)
		}
	}
	sortTraceEvents(all)
	return all
}
