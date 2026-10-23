package bus

import (
	"github.com/russ-p/paseka/internal/protocol"
	"github.com/russ-p/paseka/internal/runs"
)

// stampRunProvenance makes the run's recorded ownership authoritative for an
// emitted event. Fields supplied by an event producer are ignored because the
// CLI input shape does not carry them; this lookup is what binds live and
// deferred events to the attempt and configuration that actually ran.
func stampRunProvenance(colonyRoot string, ev *protocol.Event) {
	if colonyRoot == "" || ev == nil || ev.AgentID == "" || ev.TraceID == "" {
		return
	}
	meta, ok, err := runs.FindRun(colonyRoot, ev.TraceID, ev.AgentID)
	if err != nil || !ok {
		return
	}
	ev.Provenance = meta.Provenance
}
