package console

import (
	"bytes"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"unicode/utf8"
)

type procFS interface {
	ReadFile(rel string) ([]byte, error)
	ReadDir(rel string) ([]os.DirEntry, error)
}

type diskUsageFunc func(path string) (used, total uint64, err error)

type osProcFS struct {
	root string
}

func (p osProcFS) join(rel string) string {
	if rel == "" || rel == "." {
		return p.root
	}
	return filepath.Join(p.root, rel)
}

func (p osProcFS) ReadFile(rel string) ([]byte, error) {
	return os.ReadFile(p.join(rel))
}

func (p osProcFS) ReadDir(rel string) ([]os.DirEntry, error) {
	return os.ReadDir(p.join(rel))
}

type procSample struct {
	pid      int
	comm     string
	cmd      string
	rssBytes uint64
	ticks    uint64
}

func collectFromFS(fs procFS, sampler *cpuSampler, colonyRoot string, diskFn diskUsageFunc) SystemView {
	view := baseSystemIdentity()
	if fs == nil {
		return view
	}
	var errs []string

	if data, err := fs.ReadFile("sys/kernel/osrelease"); err != nil {
		errs = append(errs, "kernel: "+err.Error())
	} else {
		view.Kernel = strings.TrimSpace(string(data))
	}

	if data, err := fs.ReadFile("uptime"); err != nil {
		errs = append(errs, "uptime: "+err.Error())
	} else if up, ok := parseUptime(data); ok {
		view.UptimeSeconds = &up
	} else {
		errs = append(errs, "uptime: parse failed")
	}

	if data, err := fs.ReadFile("loadavg"); err != nil {
		errs = append(errs, "loadavg: "+err.Error())
	} else if l1, l5, l15, ok := parseLoadavg(data); ok {
		view.Load1, view.Load5, view.Load15 = &l1, &l5, &l15
	} else {
		errs = append(errs, "loadavg: parse failed")
	}

	if data, err := fs.ReadFile("meminfo"); err != nil {
		errs = append(errs, "meminfo: "+err.Error())
	} else if used, total, avail, ok := parseMeminfo(data); ok {
		view.MemUsedBytes = &used
		view.MemTotalBytes = &total
		view.MemAvailableBytes = &avail
	} else {
		errs = append(errs, "meminfo: parse failed")
	}

	var cpuTotal, cpuIdle uint64
	var haveCPU bool
	if data, err := fs.ReadFile("stat"); err != nil {
		errs = append(errs, "stat: "+err.Error())
	} else if total, idle, ok := parseProcStatCPU(data); ok {
		cpuTotal, cpuIdle, haveCPU = total, idle, true
	} else {
		errs = append(errs, "stat: parse failed")
	}

	samples, procErr := readProcSamples(fs)
	if procErr != "" {
		errs = append(errs, procErr)
	}

	current := &cpuSnapshot{
		total: cpuTotal,
		idle:  cpuIdle,
		procs: make(map[int]uint64, len(samples)),
	}
	for _, s := range samples {
		current.procs[s.pid] = s.ticks
	}

	ncpu := view.CPUs
	if ncpu < 1 {
		ncpu = 1
	}
	procCPU := map[int]float64{}
	if sampler != nil && haveCPU {
		sampler.mu.Lock()
		prev := sampler.last
		sampler.last = current
		sampler.mu.Unlock()
		if prev != nil && cpuTotal > prev.total {
			deltaTotal := cpuTotal - prev.total
			deltaIdle := uint64(0)
			if cpuIdle >= prev.idle {
				deltaIdle = cpuIdle - prev.idle
			}
			busy := 100.0 * (1.0 - float64(deltaIdle)/float64(deltaTotal))
			if busy < 0 {
				busy = 0
			}
			view.CPUPercent = &busy
			for _, s := range samples {
				prevTicks, ok := prev.procs[s.pid]
				if !ok || s.ticks < prevTicks {
					continue
				}
				procCPU[s.pid] = 100.0 * float64(s.ticks-prevTicks) * float64(ncpu) / float64(deltaTotal)
			}
		}
	}

	view.Processes = rankProcesses(samples, procCPU)

	if colonyRoot != "" && diskFn != nil {
		used, total, err := diskFn(colonyRoot)
		if err == nil && total > 0 {
			view.DiskUsedBytes = &used
			view.DiskTotalBytes = &total
		}
	}

	if len(errs) > 0 {
		view.Error = strings.Join(errs, "; ")
	}
	return view
}

func rankProcesses(samples []procSample, percents map[int]float64) []SystemProcess {
	type row struct {
		p   SystemProcess
		cpu float64
		has bool
	}
	rows := make([]row, 0, len(samples))
	for _, s := range samples {
		sp := SystemProcess{
			PID:      s.pid,
			RSSBytes: s.rssBytes,
			Comm:     s.comm,
			Cmd:      truncateRunes(s.cmd, systemCmdRunes),
		}
		r := row{p: sp}
		if pct, ok := percents[s.pid]; ok {
			cp := pct
			r.p.CPUPercent = &cp
			r.cpu = pct
			r.has = true
		}
		rows = append(rows, r)
	}
	sort.Slice(rows, func(i, j int) bool {
		if rows[i].has != rows[j].has {
			return rows[i].has
		}
		if rows[i].cpu != rows[j].cpu {
			return rows[i].cpu > rows[j].cpu
		}
		if rows[i].p.RSSBytes != rows[j].p.RSSBytes {
			return rows[i].p.RSSBytes > rows[j].p.RSSBytes
		}
		return rows[i].p.PID < rows[j].p.PID
	})
	if len(rows) > systemProcessCap {
		rows = rows[:systemProcessCap]
	}
	out := make([]SystemProcess, len(rows))
	for i, r := range rows {
		out[i] = r.p
	}
	return out
}

func readProcSamples(fs procFS) ([]procSample, string) {
	entries, err := fs.ReadDir(".")
	if err != nil {
		return nil, "processes: " + err.Error()
	}
	var samples []procSample
	for _, ent := range entries {
		name := ent.Name()
		pid, err := strconv.Atoi(name)
		if err != nil || pid <= 0 {
			continue
		}
		cmdline, err := fs.ReadFile(name + "/cmdline")
		if err != nil {
			continue // vanished PID, EACCES, etc. — best-effort table
		}
		cmd := decodeCmdline(cmdline)
		if cmd == "" {
			continue
		}
		statData, err := fs.ReadFile(name + "/stat")
		if err != nil {
			continue
		}
		parsedPID, comm, utime, stime, rssPages, ok := parsePIDStat(statData)
		if !ok {
			continue
		}
		if parsedPID != 0 {
			pid = parsedPID
		}
		if commFile, err := fs.ReadFile(name + "/comm"); err == nil {
			if c := strings.TrimSpace(string(commFile)); c != "" {
				comm = c
			}
		}
		samples = append(samples, procSample{
			pid:      pid,
			comm:     comm,
			cmd:      cmd,
			rssBytes: rssPages * procPageSize,
			ticks:    utime + stime,
		})
	}
	return samples, ""
}

func decodeCmdline(raw []byte) string {
	raw = bytes.TrimRight(raw, "\x00")
	if len(raw) == 0 {
		return ""
	}
	parts := bytes.Split(raw, []byte{0})
	fields := make([]string, 0, len(parts))
	for _, p := range parts {
		if len(p) == 0 {
			continue
		}
		fields = append(fields, string(p))
	}
	return strings.Join(fields, " ")
}

func truncateRunes(s string, limit int) string {
	if limit <= 0 || utf8.RuneCountInString(s) <= limit {
		return s
	}
	return string([]rune(s)[:limit])
}

func parseUptime(data []byte) (float64, bool) {
	fields := strings.Fields(string(data))
	if len(fields) < 1 {
		return 0, false
	}
	v, err := strconv.ParseFloat(fields[0], 64)
	if err != nil {
		return 0, false
	}
	return v, true
}

func parseLoadavg(data []byte) (l1, l5, l15 float64, ok bool) {
	fields := strings.Fields(string(data))
	if len(fields) < 3 {
		return 0, 0, 0, false
	}
	var err error
	if l1, err = strconv.ParseFloat(fields[0], 64); err != nil {
		return 0, 0, 0, false
	}
	if l5, err = strconv.ParseFloat(fields[1], 64); err != nil {
		return 0, 0, 0, false
	}
	if l15, err = strconv.ParseFloat(fields[2], 64); err != nil {
		return 0, 0, 0, false
	}
	return l1, l5, l15, true
}

func parseMeminfo(data []byte) (used, total, avail uint64, ok bool) {
	var haveTotal, haveAvail bool
	for _, line := range strings.Split(string(data), "\n") {
		key, rest, found := strings.Cut(line, ":")
		if !found {
			continue
		}
		fields := strings.Fields(rest)
		if len(fields) < 1 {
			continue
		}
		kb, err := strconv.ParseUint(fields[0], 10, 64)
		if err != nil {
			continue
		}
		switch key {
		case "MemTotal":
			total = kb * 1024
			haveTotal = true
		case "MemAvailable":
			avail = kb * 1024
			haveAvail = true
		}
	}
	if !haveTotal || !haveAvail || total == 0 {
		return 0, 0, 0, false
	}
	if total >= avail {
		used = total - avail
	}
	return used, total, avail, true
}

func parseProcStatCPU(data []byte) (total, idle uint64, ok bool) {
	for _, line := range strings.Split(string(data), "\n") {
		if !strings.HasPrefix(line, "cpu ") {
			continue
		}
		fields := strings.Fields(line)
		if len(fields) < 5 {
			return 0, 0, false
		}
		n := 8
		if len(fields)-1 < n {
			n = len(fields) - 1
		}
		var idleVal, iowait uint64
		for i := 0; i < n; i++ {
			v, err := strconv.ParseUint(fields[i+1], 10, 64)
			if err != nil {
				return 0, 0, false
			}
			total += v
			switch i {
			case 3:
				idleVal = v
			case 4:
				iowait = v
			}
		}
		return total, idleVal + iowait, true
	}
	return 0, 0, false
}

func parsePIDStat(data []byte) (pid int, comm string, utime, stime, rssPages uint64, ok bool) {
	s := strings.TrimSpace(string(data))
	start := strings.Index(s, "(")
	end := strings.LastIndex(s, ")")
	if start < 0 || end < 0 || end <= start {
		return 0, "", 0, 0, 0, false
	}
	pid, err := strconv.Atoi(strings.TrimSpace(s[:start]))
	if err != nil {
		return 0, "", 0, 0, 0, false
	}
	comm = s[start+1 : end]
	rest := strings.Fields(s[end+1:])
	if len(rest) < 22 {
		return 0, "", 0, 0, 0, false
	}
	utime, err = strconv.ParseUint(rest[11], 10, 64)
	if err != nil {
		return 0, "", 0, 0, 0, false
	}
	stime, err = strconv.ParseUint(rest[12], 10, 64)
	if err != nil {
		return 0, "", 0, 0, 0, false
	}
	rssPages, err = strconv.ParseUint(rest[21], 10, 64)
	if err != nil {
		return 0, "", 0, 0, 0, false
	}
	return pid, comm, utime, stime, rssPages, true
}
