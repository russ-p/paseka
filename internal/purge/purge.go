package purge

import (
	"fmt"

	"github.com/paseka/paseka/internal/bus"
	"github.com/paseka/paseka/internal/colony"
	"github.com/paseka/paseka/internal/energy"
	"github.com/paseka/paseka/internal/tasks"
)

func validateReseedEnergy(target PurgeTarget) error {
	if !target.ReseedEnergy {
		return nil
	}
	if !target.Bus {
		return fmt.Errorf("--reseed-energy requires --bus")
	}
	if target.TraceID == "" {
		return fmt.Errorf("--reseed-energy requires --trace")
	}
	return nil
}

// Plan lists filesystem and bus artifacts that would be removed.
func Plan(ctx colony.Context, target PurgeTarget) (PurgePlan, error) {
	if err := validateReseedEnergy(target); err != nil {
		return PurgePlan{}, err
	}
	plan, err := planFS(ctx, target)
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
func Execute(ctx colony.Context, target PurgeTarget) (PurgeResult, error) {
	if err := validateReseedEnergy(target); err != nil {
		return PurgeResult{}, err
	}
	res, err := purgeFS(ctx, target)
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
	if target.ReseedEnergy {
		budget, err := reseedEnergy(ctx, target.TraceID)
		if err != nil {
			return res, err
		}
		res.EnergyReseeded = budget
	}
	return res, nil
}

func reseedEnergy(ctx colony.Context, traceID string) (int, error) {
	session, err := tasks.OpenLedger(ctx)
	if err != nil {
		return 0, err
	}
	defer session.Close()
	if session.Ledger == nil {
		return 0, fmt.Errorf("nats url not configured (--reseed-energy requires NATS)")
	}
	if err := energy.EnsureSeeded(session.Ledger, ctx.ColonyRoot, traceID); err != nil {
		return 0, err
	}
	snap, err := session.Ledger.Snapshot(traceID)
	if err != nil {
		return 0, err
	}
	return snap.EnergyBudget, nil
}

func planBus(ctx colony.Context, traceID string) (*BusPurgePlan, error) {
	client, err := connectBus(ctx)
	if err != nil {
		return nil, err
	}
	defer client.Close()

	busPlan, err := client.PlanPurgeTrace(traceID)
	if err != nil {
		return nil, err
	}
	return BusPurgePlanFromTrace(
		busPlan.TraceID,
		busPlan.TaskLedgerKey,
		busPlan.EventCount,
		busPlan.ArtifactObjects,
	), nil
}

func purgeBus(ctx colony.Context, traceID string) (*BusPurgeResult, error) {
	client, err := connectBus(ctx)
	if err != nil {
		return nil, err
	}
	defer client.Close()

	busRes, err := client.PurgeTrace(traceID)
	if err != nil {
		return nil, err
	}
	return &BusPurgeResult{
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
