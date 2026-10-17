package bus

import (
	"strings"

	"github.com/russ-p/paseka/internal/colony"
)

// Config holds NATS connection and subject prefix settings.
type Config struct {
	URL           string
	SubjectPrefix string
	Slug          string
	// ColonyRoot locates the trail records this client writes on the way to the
	// bus. It reaches the publisher through the composition point every root
	// already calls, rather than being threaded through callers that know
	// nothing about the run tree. Empty means bus-only: no trail record.
	ColonyRoot string
}

// ConfigFromContext builds bus config from colony context and manifest.
func ConfigFromContext(ctx colony.Context, manifest colony.Colony) Config {
	prefix := strings.TrimSpace(manifest.NATS.SubjectPrefix)
	if prefix == "" {
		prefix = "paseka." + ctx.Slug
	}
	return Config{
		URL:           ctx.Home.NATS.EffectiveURL(),
		SubjectPrefix: prefix,
		Slug:          ctx.Slug,
		ColonyRoot:    ctx.ColonyRoot,
	}
}

// Enabled reports whether a NATS URL is configured.
func (c Config) Enabled() bool {
	return c.URL != ""
}
