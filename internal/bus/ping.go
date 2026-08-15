package bus

import (
	"time"

	"github.com/nats-io/nats.go"
	"github.com/paseka/paseka/internal/colony"
)

// PingResult summarizes light NATS connectivity for colony status snapshots.
// Unlike Diagnose, Ping only dials the server — no JetStream, KV, or object store checks.
type PingResult struct {
	Configured bool
	Connected  bool
}

// Ping checks whether NATS is configured and reachable with a short dial.
func Ping(ctx colony.Context) (PingResult, error) {
	manifest, err := colony.LoadColony(ctx.ColonyRoot)
	if err != nil {
		return PingResult{}, err
	}
	cfg := ConfigFromContext(ctx, manifest)
	if !cfg.Enabled() {
		return PingResult{}, nil
	}
	result := PingResult{Configured: true}
	nc, err := nats.Connect(cfg.URL,
		nats.Name("paseka-"+cfg.Slug+"-ping"),
		nats.Timeout(5*time.Second),
		nats.NoReconnect(),
	)
	if err != nil {
		return result, nil
	}
	nc.Close()
	result.Connected = true
	return result, nil
}
