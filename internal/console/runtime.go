package console

import (
	"github.com/paseka/paseka/internal/colony"
	"github.com/paseka/paseka/internal/hiveview"
	"github.com/paseka/paseka/internal/runtime"
)

// StartRuntime launches an external `paseka run` when none is alive.
func StartRuntime(ctx colony.Context, sup *runtime.Supervisor) (hiveview.RuntimeView, error) {
	if sup == nil {
		sup = runtime.DefaultSupervisor()
	}
	st, err := sup.Start(ctx)
	if err != nil {
		return hiveview.RuntimeView{}, err
	}
	view := hiveview.RuntimeViewFromStatus(st)
	hiveview.EnrichRuntimeView(&view, ctx)
	return view, nil
}

// StopRuntime stops the registered hive runtime process.
func StopRuntime(ctx colony.Context, sup *runtime.Supervisor) (hiveview.RuntimeView, error) {
	if sup == nil {
		sup = runtime.DefaultSupervisor()
	}
	st, err := sup.Stop(ctx)
	if err != nil {
		return hiveview.RuntimeView{}, err
	}
	view := hiveview.RuntimeViewFromStatus(st)
	hiveview.EnrichRuntimeView(&view, ctx)
	return view, nil
}
