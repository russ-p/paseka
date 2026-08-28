package console

import (
	"net/http"
	"os"
	"runtime"
	"sync"
)

const (
	systemProcessCap = 25
	systemCmdRunes   = 200
	procPageSize     = 4096
)

// SystemProcess is one row in the System tab process table.
type SystemProcess struct {
	PID        int      `json:"pid"`
	RSSBytes   uint64   `json:"rssBytes"`
	CPUPercent *float64 `json:"cpuPercent,omitempty"`
	Comm       string   `json:"comm"`
	Cmd        string   `json:"cmd"`
}

// SystemView is the observe-only host snapshot for GET /api/system.
type SystemView struct {
	Hostname          string          `json:"hostname,omitempty"`
	Kernel            string          `json:"kernel,omitempty"`
	OS                string          `json:"os"`
	Arch              string          `json:"arch"`
	CPUs              int             `json:"cpus"`
	UptimeSeconds     *float64        `json:"uptimeSeconds,omitempty"`
	ConsolePID        int             `json:"consolePid"`
	GoVersion         string          `json:"goVersion,omitempty"`
	Load1             *float64        `json:"load1,omitempty"`
	Load5             *float64        `json:"load5,omitempty"`
	Load15            *float64        `json:"load15,omitempty"`
	CPUPercent        *float64        `json:"cpuPercent,omitempty"`
	MemUsedBytes      *uint64         `json:"memUsedBytes,omitempty"`
	MemTotalBytes     *uint64         `json:"memTotalBytes,omitempty"`
	MemAvailableBytes *uint64         `json:"memAvailableBytes,omitempty"`
	DiskUsedBytes     *uint64         `json:"diskUsedBytes,omitempty"`
	DiskTotalBytes    *uint64         `json:"diskTotalBytes,omitempty"`
	Processes         []SystemProcess `json:"processes,omitempty"`
	Error             string          `json:"error,omitempty"`
}

type cpuSnapshot struct {
	total uint64
	idle  uint64
	procs map[int]uint64
}

type cpuSampler struct {
	mu        sync.Mutex
	collectMu sync.Mutex
	last      *cpuSnapshot
}

func newCPUSampler() *cpuSampler {
	return &cpuSampler{}
}

func baseSystemIdentity() SystemView {
	view := SystemView{
		OS:         runtime.GOOS,
		Arch:       runtime.GOARCH,
		CPUs:       runtime.NumCPU(),
		ConsolePID: os.Getpid(),
		GoVersion:  runtime.Version(),
	}
	if host, err := os.Hostname(); err == nil {
		view.Hostname = host
	}
	return view
}

func (a *api) handleSystem(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	sampler := a.sampler
	if sampler == nil {
		sampler = newCPUSampler()
		a.sampler = sampler
	}
	writeJSON(w, snapshotHost(sampler, a.ctx.ColonyRoot))
}
