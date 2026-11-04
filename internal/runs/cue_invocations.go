package runs

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/russ-p/paseka/internal/protocol"
)

// CueInvocationsDirName is the reserved trace subdirectory holding one durable
// record per validated cue invocation. It lives with the trail's other retained
// evidence rather than inside a run directory, so pruning a worktree does not
// erase where the trail's work came from.
const CueInvocationsDirName = "cue-invocations"

// CueInvocation is the durable identity of one validated cue invocation.
//
// The record is written before the invocation publishes anything, and it holds
// the exact ingress observation and normal ingress events that invocation owns,
// including their identities. A publication retry replays those records instead
// of minting a second invocation or a second set of events.
type CueInvocation struct {
	InvocationID string    `json:"invocationId"`
	TraceID      string    `json:"traceId"`
	AgentID      string    `json:"agentId,omitempty"`
	CueID        string    `json:"cueId"`
	Source       string    `json:"source,omitempty"`
	Mode         string    `json:"mode"`
	InvokedAt    time.Time `json:"invokedAt"`
	// TaskIDs are the tasks this invocation creates, if any.
	TaskIDs []string `json:"taskIds,omitempty"`
	// Ingress is the SIGNAL/cue.ingress observation for this invocation. Its
	// payload names the normal event identities and created task ids.
	Ingress          protocol.Event `json:"ingress"`
	IngressPublished bool           `json:"ingressPublished,omitempty"`
	// Events are the invocation's normal ingress events in publish order.
	Events []CueInvocationEvent `json:"events,omitempty"`
}

// CueInvocationEvent is one normal ingress event owned by an invocation.
type CueInvocationEvent struct {
	Event     protocol.Event `json:"event"`
	Published bool           `json:"published,omitempty"`
}

// EventIDs returns the recorded identities of the invocation's normal ingress
// events, in publish order.
func (inv CueInvocation) EventIDs() []string {
	ids := make([]string, 0, len(inv.Events))
	for _, event := range inv.Events {
		if event.Event.ID != "" {
			ids = append(ids, event.Event.ID)
		}
	}
	return ids
}

// Published reports whether every recorded event has been confirmed.
func (inv CueInvocation) Published() bool {
	if !inv.IngressPublished {
		return false
	}
	for _, event := range inv.Events {
		if !event.Published {
			return false
		}
	}
	return true
}

// CueInvocationsRoot returns .paseka/runs/<traceId>/cue-invocations/.
func CueInvocationsRoot(colonyRoot, traceID string) string {
	return filepath.Join(colonyRoot, ".paseka", "runs", traceID, CueInvocationsDirName)
}

// CueInvocationPath returns the durable record path for one invocation.
func CueInvocationPath(colonyRoot, traceID, invocationID string) string {
	return filepath.Join(CueInvocationsRoot(colonyRoot, traceID), invocationID+".json")
}

// WriteCueInvocation atomically records (or updates) one invocation identity.
func WriteCueInvocation(colonyRoot, traceID string, inv CueInvocation) error {
	if err := validateTrailRecordIDs(colonyRoot, traceID, inv.InvocationID); err != nil {
		return err
	}
	if inv.TraceID == "" {
		inv.TraceID = traceID
	}
	if inv.TraceID != traceID {
		return fmt.Errorf("runs: cue invocation %q belongs to trace %q, not %q", inv.InvocationID, inv.TraceID, traceID)
	}
	data, err := json.MarshalIndent(inv, "", "  ")
	if err != nil {
		return err
	}
	return writeFileAtomic(CueInvocationPath(colonyRoot, traceID, inv.InvocationID), append(data, '\n'), 0o644)
}

// ReadCueInvocation loads one durable invocation record. A missing record is not
// an error: ok is false and the caller decides whether that is a fresh
// invocation or unavailable provenance.
func ReadCueInvocation(colonyRoot, traceID, invocationID string) (CueInvocation, bool, error) {
	if err := validateTrailRecordIDs(colonyRoot, traceID, invocationID); err != nil {
		return CueInvocation{}, false, err
	}
	data, err := os.ReadFile(CueInvocationPath(colonyRoot, traceID, invocationID))
	if err != nil {
		if os.IsNotExist(err) {
			return CueInvocation{}, false, nil
		}
		return CueInvocation{}, false, err
	}
	var inv CueInvocation
	if err := json.Unmarshal(data, &inv); err != nil {
		return CueInvocation{}, false, fmt.Errorf("runs: parse cue invocation %q: %w", invocationID, err)
	}
	return inv, true, nil
}

// ListCueInvocations returns every retained invocation for one trace, ordered by
// invocation time and then identity. A missing directory is an empty history,
// not an error.
func ListCueInvocations(colonyRoot, traceID string) ([]CueInvocation, error) {
	if strings.TrimSpace(colonyRoot) == "" || strings.TrimSpace(traceID) == "" {
		return nil, fmt.Errorf("runs: colony root and traceId are required")
	}
	entries, err := os.ReadDir(CueInvocationsRoot(colonyRoot, traceID))
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, err
	}
	out := make([]CueInvocation, 0, len(entries))
	for _, entry := range entries {
		if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".json") {
			continue
		}
		invocationID := strings.TrimSuffix(entry.Name(), ".json")
		inv, ok, err := ReadCueInvocation(colonyRoot, traceID, invocationID)
		if err != nil {
			return nil, err
		}
		if ok {
			out = append(out, inv)
		}
	}
	sort.SliceStable(out, func(i, j int) bool {
		a, b := out[i], out[j]
		if !a.InvokedAt.Equal(b.InvokedAt) {
			return a.InvokedAt.Before(b.InvokedAt)
		}
		return a.InvocationID < b.InvocationID
	})
	return out, nil
}

// validateTrailRecordIDs refuses ids that could escape the trail directory. The
// record path is built from caller-supplied identities, so a separator or a
// parent component must fail here rather than write outside the trail.
func validateTrailRecordIDs(colonyRoot, traceID, invocationID string) error {
	if strings.TrimSpace(colonyRoot) == "" {
		return fmt.Errorf("runs: colony root is required")
	}
	if err := validateTrailRecordID("traceId", traceID); err != nil {
		return err
	}
	return validateTrailRecordID("invocationId", invocationID)
}

func validateTrailRecordID(label, id string) error {
	id = strings.TrimSpace(id)
	if id == "" {
		return fmt.Errorf("runs: %s is required", label)
	}
	if id == "." || id == ".." || id != filepath.Base(id) || strings.ContainsAny(id, `/\`) {
		return fmt.Errorf("runs: invalid %s %q", label, id)
	}
	return nil
}
