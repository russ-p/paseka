package runtime

import (
	"context"
	"fmt"

	"github.com/paseka/paseka/internal/bus"
	"github.com/paseka/paseka/internal/colony"
)

func (d *Dispatcher) flushDeferredEvents(ctx context.Context, colonyRoot, traceID, agentID string) error {
	pub, closer, err := d.flushPublisher(colonyRoot)
	if err != nil {
		return err
	}
	if closer != nil {
		defer closer()
	}
	result, err := bus.FlushPending(ctx, pub, colonyRoot, traceID, agentID, false)
	if err != nil {
		return err
	}
	if !result.OK {
		if result.Error != "" {
			return fmt.Errorf("flush pending: %s", result.Error)
		}
		return fmt.Errorf("flush pending failed")
	}
	return nil
}

func (d *Dispatcher) flushPublisher(colonyRoot string) (bus.Publisher, func(), error) {
	if d.publisher != nil {
		return d.publisher, nil, nil
	}
	client, err := bus.ConnectColony(colony.Context{ColonyRoot: colonyRoot}, false)
	if err != nil {
		return nil, nil, err
	}
	if client == nil {
		return bus.NopPublisher{}, nil, nil
	}
	return client, func() { client.Close() }, nil
}
