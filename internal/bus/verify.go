package bus

import (
	"fmt"
	"sort"
	"strings"

	"github.com/russ-p/paseka/internal/protocol"
	"github.com/russ-p/paseka/internal/runs"
)

// VerifiedEvent is one event's presence in each record.
type VerifiedEvent struct {
	ID          string             `json:"id,omitempty"`
	Type        protocol.EventType `json:"type"`
	PayloadKind string             `json:"payloadKind,omitempty"`
	AgentID     string             `json:"agentId,omitempty"`
	CreatedAt   string             `json:"createdAt"`
}

// VerifyReport is the comparison of one trail's local record against what
// reached the bus. Where an event was found is the section it sits in rather
// than a field on it: `Both` for an event in both records, `LocalOnly` for one
// the broker never took, `BusOnly` for one missing locally, and `Unverifiable`
// for a record written before identities existed — which cannot be matched
// across the two, and guessing would invent a divergence that may not exist.
type VerifyReport struct {
	TraceID string `json:"traceId"`
	// StreamComplete says the broker side was read to the end. False means the
	// comparison is inconclusive, not that the records agree: an event still in
	// flight would arrive as LocalOnly.
	StreamComplete bool            `json:"streamComplete"`
	LocalCount     int             `json:"localCount"`
	BusCount       int             `json:"busCount"`
	Both           []VerifiedEvent `json:"both"`
	LocalOnly      []VerifiedEvent `json:"localOnly"`
	BusOnly        []VerifiedEvent `json:"busOnly"`
	Unverifiable   []VerifiedEvent `json:"unverifiable"`
}

// Diverged reports whether the two records disagree about anything. An
// incomplete broker read never counts as divergence: the comparison is unknown,
// and an unknown must not be printed as a finding.
func (r VerifyReport) Diverged() bool {
	return r.StreamComplete && (len(r.LocalOnly) > 0 || len(r.BusOnly) > 0)
}

// TraceStream is what verification reads: the broker side of a comparison, and
// whether the read reached the end of the stream.
type TraceStream interface {
	ReplayTraceExhaustive(traceID string) (events []protocol.Event, complete bool, err error)
}

// VerifyTrace compares a trail's local record with what the broker holds.
//
// It is deliberately not an export section — export must keep working with no
// broker reachable — and not a flag on `paseka replay`, which answers a
// different question and whose output is intentionally payload-free. The local
// record is what the colony decided to do; the stream is what was broadcast,
// and only this comparison says whether they agree.
func VerifyTrace(colonyRoot, traceID string, stream TraceStream) (VerifyReport, error) {
	report := VerifyReport{TraceID: traceID}
	if strings.TrimSpace(colonyRoot) == "" || strings.TrimSpace(traceID) == "" {
		return report, fmt.Errorf("bus: colony root and traceId are required")
	}
	if !streamAvailable(stream) {
		return report, fmt.Errorf("bus: trace stream is required")
	}

	local, err := runs.ReadTrailEvents(colonyRoot, traceID)
	if err != nil {
		return report, fmt.Errorf("bus: read trail record: %w", err)
	}
	remote, complete, err := stream.ReplayTraceExhaustive(traceID)
	if err != nil {
		return report, err
	}
	report.StreamComplete = complete

	// The record is append-only and its writer is at-least-once, so the same
	// event can hold several lines — exactly what a merged read hides. Compare
	// distinct events by identity, or the report would count one event as a
	// divergence from itself.
	localIDs := map[string]struct{}{}
	var localUnique []protocol.Event
	for _, ev := range local {
		if ev.ID == "" {
			report.Unverifiable = append(report.Unverifiable, verifiedEvent(ev))
			continue
		}
		if _, dup := localIDs[ev.ID]; dup {
			continue
		}
		localIDs[ev.ID] = struct{}{}
		localUnique = append(localUnique, ev)
	}

	seenOnBus := map[string]struct{}{}
	for _, ev := range remote {
		if ev.ID == "" {
			report.Unverifiable = append(report.Unverifiable, verifiedEvent(ev))
			continue
		}
		if _, dup := seenOnBus[ev.ID]; dup {
			continue
		}
		seenOnBus[ev.ID] = struct{}{}
		if _, ok := localIDs[ev.ID]; ok {
			report.Both = append(report.Both, verifiedEvent(ev))
			continue
		}
		report.BusOnly = append(report.BusOnly, verifiedEvent(ev))
	}
	for _, ev := range localUnique {
		if _, ok := seenOnBus[ev.ID]; ok {
			continue
		}
		report.LocalOnly = append(report.LocalOnly, verifiedEvent(ev))
	}

	// Counted in distinct events, so the headline numbers add up to the rows
	// below them and a re-flushed event does not read as a second one.
	report.LocalCount = len(localUnique)
	report.BusCount = len(seenOnBus)

	sortVerifiedEvents(report.Both)
	sortVerifiedEvents(report.LocalOnly)
	sortVerifiedEvents(report.BusOnly)
	sortVerifiedEvents(report.Unverifiable)
	return report, nil
}

func verifiedEvent(ev protocol.Event) VerifiedEvent {
	return VerifiedEvent{
		ID:          ev.ID,
		Type:        ev.Type,
		PayloadKind: PayloadKind(ev.Payload),
		AgentID:     ev.AgentID,
		CreatedAt:   ev.CreatedAt.UTC().Format("2006-01-02T15:04:05.000Z"),
	}
}

func sortVerifiedEvents(events []VerifiedEvent) {
	sort.SliceStable(events, func(i, j int) bool {
		a, b := events[i], events[j]
		if a.CreatedAt != b.CreatedAt {
			return a.CreatedAt < b.CreatedAt
		}
		return a.ID < b.ID
	})
}

// FormatVerifyReport renders a verification as an operator reads it: a verdict
// first, then the rows that disagree.
func FormatVerifyReport(report VerifyReport) string {
	var b strings.Builder
	fmt.Fprintf(&b, "Trail %s\n", report.TraceID)
	fmt.Fprintf(&b, "  local record: %d event(s)\n", report.LocalCount)
	fmt.Fprintf(&b, "  bus stream:   %d event(s)\n", report.BusCount)
	fmt.Fprintf(&b, "  in both:      %d\n", len(report.Both))

	switch {
	case !report.StreamComplete:
		// The broker side stopped before the end of the stream, so every row
		// below is a lower bound. Saying "agree" here would be the same lie as
		// saying "diverged": both claim to know something the read cannot.
		fmt.Fprintf(&b, "  INCONCLUSIVE: the bus read did not reach the end of the stream\n")
	case report.Diverged():
		fmt.Fprintf(&b, "  DIVERGED\n")
	default:
		fmt.Fprintf(&b, "  agree\n")
	}
	writeVerifiedRows(&b, "recorded locally but never broadcast", report.LocalOnly)
	writeVerifiedRows(&b, "broadcast but not in the local record", report.BusOnly)
	if len(report.Unverifiable) > 0 {
		fmt.Fprintf(&b, "  %d event(s) predate event identity and cannot be compared\n", len(report.Unverifiable))
	}
	return b.String()
}

func writeVerifiedRows(b *strings.Builder, heading string, events []VerifiedEvent) {
	if len(events) == 0 {
		return
	}
	fmt.Fprintf(b, "\n%s:\n", heading)
	for _, ev := range events {
		kind := string(ev.Type)
		if ev.PayloadKind != "" {
			kind += " (" + ev.PayloadKind + ")"
		}
		fmt.Fprintf(b, "  %s  %s", ev.CreatedAt, kind)
		if ev.AgentID != "" {
			fmt.Fprintf(b, "  agent=%s", ev.AgentID)
		}
		if ev.ID != "" {
			fmt.Fprintf(b, "  id=%s", ev.ID)
		}
		b.WriteString("\n")
	}
}
