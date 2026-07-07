package console

import (
	"time"

	"github.com/paseka/paseka/internal/colony"
	"github.com/paseka/paseka/internal/runtime"
)

// RuntimeView is the Queen Console projection of hive runtime status.
type RuntimeView struct {
	Status          string `json:"status"`
	PID             int    `json:"pid,omitempty"`
	StartedAt       string `json:"startedAt,omitempty"`
	LastHeartbeatAt string `json:"lastHeartbeatAt,omitempty"`
	ColonyRoot      string `json:"colonyRoot,omitempty"`
	SubjectPrefix   string `json:"subjectPrefix,omitempty"`
	Alive           bool   `json:"alive"`
}

// GetRuntime returns the current hive runtime status for the colony.
func GetRuntime(ctx colony.Context, sup *runtime.Supervisor) (RuntimeView, error) {
	if sup == nil {
		sup = runtime.DefaultSupervisor()
	}
	st, err := sup.Status(ctx)
	if err != nil {
		return RuntimeView{}, err
	}
	return runtimeViewFromStatus(st), nil
}

// StartRuntime launches an external `paseka run` when none is alive.
func StartRuntime(ctx colony.Context, sup *runtime.Supervisor) (RuntimeView, error) {
	if sup == nil {
		sup = runtime.DefaultSupervisor()
	}
	st, err := sup.Start(ctx)
	if err != nil {
		return RuntimeView{}, err
	}
	return runtimeViewFromStatus(st), nil
}

// StopRuntime stops the registered hive runtime process.
func StopRuntime(ctx colony.Context, sup *runtime.Supervisor) (RuntimeView, error) {
	if sup == nil {
		sup = runtime.DefaultSupervisor()
	}
	st, err := sup.Stop(ctx)
	if err != nil {
		return RuntimeView{}, err
	}
	return runtimeViewFromStatus(st), nil
}

func runtimeViewFromStatus(st runtime.RuntimeStatus) RuntimeView {
	view := RuntimeView{
		Status:        st.Status,
		PID:           st.PID,
		ColonyRoot:    st.ColonyRoot,
		SubjectPrefix: st.SubjectPrefix,
		Alive:         st.Alive,
	}
	if !st.StartedAt.IsZero() {
		view.StartedAt = st.StartedAt.UTC().Format(time.RFC3339)
	}
	if !st.LastHeartbeatAt.IsZero() {
		view.LastHeartbeatAt = st.LastHeartbeatAt.UTC().Format(time.RFC3339)
	}
	return view
}
