package runtime

import (
	"context"
	"strings"

	"github.com/paseka/paseka/internal/artifacts"
	"github.com/paseka/paseka/internal/bus"
	"github.com/paseka/paseka/internal/colony"
	"github.com/paseka/paseka/internal/logging"
)

func subjectPrefixForColonyRoot(colonyRoot string) string {
	c, err := colony.LoadColony(colonyRoot)
	if err != nil {
		return ""
	}
	prefix := strings.TrimSpace(c.NATS.SubjectPrefix)
	if prefix == "" && strings.TrimSpace(c.Slug) != "" {
		prefix = "paseka." + c.Slug
	}
	return prefix
}

func (d *Dispatcher) flushArtifactDelta(ctx context.Context, colonyRoot, traceID, agentID string) error {
	has, err := artifacts.RunHasArtifactWritten(colonyRoot, traceID, agentID)
	if err != nil {
		return err
	}
	if has {
		return nil
	}
	pub, closer, err := d.flushPublisher(colonyRoot)
	if err != nil {
		return err
	}
	if closer != nil {
		defer closer()
	}
	return artifacts.FlushRunDelta(ctx, pub, colonyRoot, subjectPrefixForColonyRoot(colonyRoot), traceID, agentID)
}

func captureArtifactsBaseline(colonyRoot, traceID, agentID string) {
	if err := artifacts.CaptureBaseline(colonyRoot, traceID, agentID); err != nil {
		logging.Component("runtime").Warn("artifacts baseline capture failed",
			logging.F("trace", traceID),
			logging.F("agent", agentID),
			logging.F("error", err.Error()),
		)
	}
}

// FlushSessionArtifacts flushes comb delta after a successful interactive session.
func FlushSessionArtifacts(ctx context.Context, colonyRoot, traceID, agentID string, pub bus.Publisher) error {
	has, err := artifacts.RunHasArtifactWritten(colonyRoot, traceID, agentID)
	if err != nil {
		return err
	}
	if has {
		return nil
	}
	if pub == nil {
		pub = bus.NopPublisher{}
	}
	return artifacts.FlushRunDelta(ctx, pub, colonyRoot, subjectPrefixForColonyRoot(colonyRoot), traceID, agentID)
}
