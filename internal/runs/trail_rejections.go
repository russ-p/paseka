package runs

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"time"

	"github.com/russ-p/paseka/internal/logging"
)

// RejectedEventsFileName is the trail-level record of bus messages the runtime
// stopped retrying. A handler that fails deterministically used to be redelivered
// forever without a trace; this file keeps the evidence after the subscriber
// terminates the delivery.
const RejectedEventsFileName = "rejected-events.ndjson"

// RejectedEvent is one bus message whose handler failed and whose retry budget
// was exhausted. The raw payload is deliberately not stored: the event remains
// readable from the stream until retention removes it, and the record's job is
// to name what could not be applied and why.
type RejectedEvent struct {
	EventID string `json:"eventId,omitempty"`
	TraceID string `json:"traceId,omitempty"`
	Type    string `json:"type,omitempty"`
	Kind    string `json:"kind,omitempty"`
	AgentID string `json:"agentId,omitempty"`
	Subject string `json:"subject"`
	// Attempts is the retry budget the subscriber spent in this process;
	// ServerDeliveries is the broker's delivery count, kept because a message
	// can carry the attempts of a previous binary.
	Attempts         uint64    `json:"attempts"`
	ServerDeliveries uint64    `json:"serverDeliveries,omitempty"`
	Error            string    `json:"error"`
	RejectedAt       time.Time `json:"rejectedAt"`
}

// RejectionsPath returns the trail-level rejection record path.
func (t Trail) RejectionsPath() string {
	return filepath.Join(t.Root(), RejectedEventsFileName)
}

// AppendRejection appends one rejected delivery to the trail record. It is
// best-effort evidence: the delivery is terminated whether or not the record
// can be written, because retrying it would restore the loop the record exists
// to explain.
func (t Trail) AppendRejection(rec RejectedEvent) error {
	if t.ColonyRoot == "" || t.TraceID == "" {
		return fmt.Errorf("runs: colony root and traceId are required")
	}
	if rec.RejectedAt.IsZero() {
		rec.RejectedAt = time.Now().UTC()
	}
	if err := os.MkdirAll(t.Root(), 0o755); err != nil {
		return fmt.Errorf("runs: mkdir %s: %w", t.Root(), err)
	}
	data, err := json.Marshal(rec)
	if err != nil {
		return err
	}
	f, err := os.OpenFile(t.RejectionsPath(), os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0o644)
	if err != nil {
		return err
	}
	defer f.Close()
	if _, err := f.Write(append(data, '\n')); err != nil {
		return err
	}
	return nil
}

// TrailRejectionSummary is the bounded read-side view of one trail's rejected
// deliveries, used by diagnostics rather than by an event projection.
type TrailRejectionSummary struct {
	TraceID   string    `json:"traceId"`
	Count     int       `json:"count"`
	NewestAt  time.Time `json:"newestAt,omitempty"`
	LastError string    `json:"lastError,omitempty"`
}

// ListTrailRejections scans every trail for a rejection record. A missing
// directory or file is not an error; an unparseable line is skipped so one torn
// append cannot hide the rest of the evidence.
func ListTrailRejections(colonyRoot string) ([]TrailRejectionSummary, error) {
	if colonyRoot == "" {
		return nil, fmt.Errorf("runs: colony root is required")
	}
	entries, err := os.ReadDir(filepath.Join(colonyRoot, ".paseka", "runs"))
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, err
	}
	var out []TrailRejectionSummary
	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}
		traceID := entry.Name()
		path := Trail{ColonyRoot: colonyRoot, TraceID: traceID}.RejectionsPath()
		summary, ok, err := readRejectionSummary(path)
		if err != nil {
			return nil, err
		}
		if !ok {
			continue
		}
		summary.TraceID = traceID
		out = append(out, summary)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].TraceID < out[j].TraceID })
	return out, nil
}

func readRejectionSummary(path string) (TrailRejectionSummary, bool, error) {
	f, err := os.Open(path)
	if err != nil {
		if os.IsNotExist(err) {
			return TrailRejectionSummary{}, false, nil
		}
		return TrailRejectionSummary{}, false, err
	}
	defer f.Close()

	var summary TrailRejectionSummary
	scanner := newLineScanner(f)
	for scanner.Scan() {
		raw := scanner.Text()
		if len(raw) == 0 {
			continue
		}
		var rec RejectedEvent
		if err := json.Unmarshal([]byte(raw), &rec); err != nil {
			logging.Component("runs").Warn("skip unparseable rejection line",
				logging.F("path", path),
				logging.F("error", err.Error()))
			continue
		}
		summary.Count++
		if summary.NewestAt.IsZero() || rec.RejectedAt.After(summary.NewestAt) {
			summary.NewestAt = rec.RejectedAt
			summary.LastError = rec.Error
		}
	}
	if err := scanner.Err(); err != nil {
		return TrailRejectionSummary{}, false, err
	}
	return summary, summary.Count > 0, nil
}
