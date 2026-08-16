package sessions

import (
	"context"
	"strings"

	"github.com/paseka/paseka/internal/artifacts"
	"github.com/paseka/paseka/internal/bus"
	"github.com/paseka/paseka/internal/colony"
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

func flushSessionArtifacts(ctx context.Context, colonyRoot, traceID, agentID string, pub bus.Publisher) error {
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
