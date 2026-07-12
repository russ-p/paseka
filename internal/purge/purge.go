package purge

import (
	"fmt"

	"github.com/paseka/paseka/internal/bus"
	"github.com/paseka/paseka/internal/colony"
)

// Plan lists filesystem and bus artifacts that would be removed.
func Plan(ctx colony.Context, target colony.PurgeTarget) (colony.PurgePlan, error) {
	plan, err := colony.PlanPurge(ctx, target)
	if err != nil {
		return plan, err
	}
	if !target.Bus {
		return plan, nil
	}
	if target.TraceID == "" {
		return plan, fmt.Errorf("--trace is required with --bus")
	}
	busPlan, err := planBus(ctx, target.TraceID)
	if err != nil {
		return plan, err
	}
	plan.Bus = busPlan
	return plan, nil
}

// Execute removes selected colony artifacts, including bus state when requested.
func Execute(ctx colony.Context, target colony.PurgeTarget) (colony.PurgeResult, error) {
	res, err := colony.Purge(ctx, target)
	if err != nil {
		return res, err
	}
	if !target.Bus {
		return res, nil
	}
	if target.TraceID == "" {
		return res, fmt.Errorf("--trace is required with --bus")
	}
	busRes, err := purgeBus(ctx, target.TraceID)
	if err != nil {
		return res, err
	}
	res.Bus = busRes
	return res, nil
}

func planBus(ctx colony.Context, traceID string) (*colony.BusPurgePlan, error) {
	client, err := connectBus(ctx)
	if err != nil {
		return nil, err
	}
	defer client.Close()

	busPlan, err := client.PlanPurgeTrace(traceID)
	if err != nil {
		return nil, err
	}
	return colony.BusPurgePlanFromTrace(
		busPlan.TraceID,
		busPlan.TaskLedgerKey,
		busPlan.EventCount,
		busPlan.ArtifactObjects,
	), nil
}

func purgeBus(ctx colony.Context, traceID string) (*colony.BusPurgeResult, error) {
	client, err := connectBus(ctx)
	if err != nil {
		return nil, err
	}
	defer client.Close()

	busRes, err := client.PurgeTrace(traceID)
	if err != nil {
		return nil, err
	}
	return &colony.BusPurgeResult{
		KeysRemoved:    busRes.KeysRemoved,
		EventsRemoved:  busRes.EventsRemoved,
		ObjectsRemoved: busRes.ObjectsRemoved,
	}, nil
}

func connectBus(ctx colony.Context) (*bus.Client, error) {
	client, err := bus.ConnectColony(ctx, false)
	if err != nil {
		return nil, err
	}
	if client == nil {
		return nil, fmt.Errorf("nats url not configured (--bus requires NATS)")
	}
	return client, nil
}
