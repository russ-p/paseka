package bus_test

import (
	"github.com/paseka/paseka/internal/colony"
)

func colonyCtx(slug, natsURL string) colony.Context {
	return colony.Context{
		Slug: slug,
		Home: colony.HomeConfig{
			Slug: slug,
			NATS: colony.NATSConfig{URL: natsURL},
		},
	}
}

func colonyManifest(prefix string) colony.Colony {
	return colony.Colony{
		NATS: colony.ColonyNATS{SubjectPrefix: prefix},
	}
}
