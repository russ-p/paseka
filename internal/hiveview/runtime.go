package hiveview

import (
	"time"

	"github.com/paseka/paseka/internal/colony"
	"github.com/paseka/paseka/internal/runtime"
)

// RuntimeView is the hive projection of runtime status.
type RuntimeView struct {
	Status          string `json:"status"`
	PID             int    `json:"pid,omitempty"`
	StartedAt       string `json:"startedAt,omitempty"`
	LastHeartbeatAt string `json:"lastHeartbeatAt,omitempty"`
	Slug            string `json:"slug,omitempty"`
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
	view := RuntimeViewFromStatus(st)
	EnrichRuntimeView(&view, ctx)
	return view, nil
}

// RuntimeViewFromStatus maps supervisor status to a runtime view.
func RuntimeViewFromStatus(st runtime.RuntimeStatus) RuntimeView {
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

// EnrichRuntimeView fills colony context fields on a runtime view.
func EnrichRuntimeView(view *RuntimeView, ctx colony.Context) {
	view.Slug = ctx.Slug
	if ctx.ColonyRoot != "" {
		view.ColonyRoot = ctx.ColonyRoot
	}
}
