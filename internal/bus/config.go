package bus

import (
	"strings"

	"github.com/paseka/paseka/internal/colony"
)

// Config holds NATS connection and subject prefix settings.
type Config struct {
	URL           string
	SubjectPrefix string
	Slug          string
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
	}
}

// Enabled reports whether a NATS URL is configured.
func (c Config) Enabled() bool {
	return c.URL != ""
}
