package console

import (
	"os"
	"strconv"
	"strings"
	"testing"
	"unicode/utf8"
)

type mapProcFS struct {
	files map[string]string
	dirs  map[string][]string
}

func (m mapProcFS) ReadFile(rel string) ([]byte, error) {
	data, ok := m.files[rel]
	if !ok {
		return nil, os.ErrNotExist
	}
	return []byte(data), nil
}

func (m mapProcFS) ReadDir(rel string) ([]os.DirEntry, error) {
	names, ok := m.dirs[rel]
	if !ok {
		return nil, os.ErrNotExist
	}
	out := make([]os.DirEntry, 0, len(names))
	for _, n := range names {
		out = append(out, fakeDirEntry{name: n})
	}
	return out, nil
}

type fakeDirEntry struct {
	name string
}

func (e fakeDirEntry) Name() string               { return e.name }
func (e fakeDirEntry) IsDir() bool                { return true }
func (e fakeDirEntry) Type() os.FileMode          { return os.ModeDir }
func (e fakeDirEntry) Info() (os.FileInfo, error) { return nil, os.ErrNotExist }

func pidStatLine(pid int, comm string, utime, stime, rssPages uint64) string {
	return strings.Join([]string{
		strconv.Itoa(pid), "(" + comm + ")",
		"S", "0", "1", "1", "0", "-1", "0", "0", "0", "0", "0",
		strconv.FormatUint(utime, 10), strconv.FormatUint(stime, 10),
		"0", "0", "20", "0", "1", "0", "0", "0",
		strconv.FormatUint(rssPages, 10),
	}, " ")
}

func fixtureFS() mapProcFS {
	longCmd := strings.Repeat("あ", 250)
	return mapProcFS{
		files: map[string]string{
			"sys/kernel/osrelease": "6.17.0-test",
			"uptime":               "12345.67 890.12",
			"loadavg":              "0.50 0.40 0.30 1/200 99",
			"meminfo":              "MemTotal: 16384000 kB\nMemAvailable: 8192000 kB\n",
			"stat":                 "cpu  100 0 100 800 0 0 0 0 0 0\ncpu0 100 0 100 800 0 0 0 0 0 0\n",
			"10/stat":              pidStatLine(10, "java", 10, 10, 400),
			"10/cmdline":           "java\x00-jar\x00app.jar",
			"10/comm":              "java\n",
			"11/stat":              pidStatLine(11, "node", 1, 1, 800),
			"11/cmdline":           "node\x00--test\x00" + longCmd,
			"11/comm":              "node\n",
			"2/stat":               pidStatLine(2, "kthreadd", 0, 0, 9999),
			"2/cmdline":            "",
			"2/comm":               "kthreadd\n",
		},
		dirs: map[string][]string{
			".": {"10", "11", "2", "stat", "meminfo"},
		},
	}
}

func TestCollectFromFSFirstSampleOmitsCPUPercent(t *testing.T) {
	fs := fixtureFS()
	sampler := newCPUSampler()
	view := collectFromFS(fs, sampler, "/colony", func(string) (uint64, uint64, error) {
		return 100, 1000, nil
	})
	if view.CPUPercent != nil {
		t.Fatalf("first sample cpuPercent = %v, want omitted", *view.CPUPercent)
	}
	if view.Kernel != "6.17.0-test" {
		t.Fatalf("kernel = %q", view.Kernel)
	}
	if view.UptimeSeconds == nil || *view.UptimeSeconds != 12345.67 {
		t.Fatalf("uptime = %v", view.UptimeSeconds)
	}
	if view.Load1 == nil || *view.Load1 != 0.5 {
		t.Fatalf("load1 = %v", view.Load1)
	}
	if view.MemTotalBytes == nil || *view.MemTotalBytes != 16384000*1024 {
		t.Fatalf("memTotal = %v", view.MemTotalBytes)
	}
	if view.DiskUsedBytes == nil || *view.DiskUsedBytes != 100 {
		t.Fatalf("disk used = %v", view.DiskUsedBytes)
	}
	if view.Error != "" {
		t.Fatalf("error = %q", view.Error)
	}
	if len(view.Processes) != 2 {
		t.Fatalf("processes = %+v (kernel thread should be skipped)", view.Processes)
	}
	for _, p := range view.Processes {
		if p.PID == 2 {
			t.Fatal("kernel thread pid 2 should be omitted")
		}
		if p.CPUPercent != nil {
			t.Fatalf("pid %d first-sample cpuPercent should be omitted", p.PID)
		}
		if p.PID == 11 {
			if utf8.RuneCountInString(p.Cmd) != systemCmdRunes {
				t.Fatalf("cmd rune count = %d want %d", utf8.RuneCountInString(p.Cmd), systemCmdRunes)
			}
		}
	}
}

func TestCollectFromFSSecondSampleFillsCPUPercent(t *testing.T) {
	fs := fixtureFS()
	sampler := newCPUSampler()
	_ = collectFromFS(fs, sampler, "", nil)

	fs.files["stat"] = "cpu  200 0 200 1400 0 0 0 0 0 0\n"
	fs.files["10/stat"] = pidStatLine(10, "java", 20, 20, 400)
	fs.files["11/stat"] = pidStatLine(11, "node", 2, 1, 800)

	view := collectFromFS(fs, sampler, "", nil)
	if view.CPUPercent == nil {
		t.Fatal("second sample missing cpuPercent")
	}
	// delta total=800, delta idle=600 → 25%
	if *view.CPUPercent < 24.9 || *view.CPUPercent > 25.1 {
		t.Fatalf("cpuPercent = %v want 25", *view.CPUPercent)
	}
	var java *SystemProcess
	for i := range view.Processes {
		if view.Processes[i].PID == 10 {
			java = &view.Processes[i]
		}
	}
	if java == nil || java.CPUPercent == nil {
		t.Fatalf("java process missing cpu: %+v", view.Processes)
	}
	if view.Processes[0].PID != 10 {
		t.Fatalf("expected java (higher CPU) first, got %+v", view.Processes)
	}
}

func TestCollectFromFSMissingProcSetsError(t *testing.T) {
	fs := mapProcFS{
		files: map[string]string{},
		dirs:  map[string][]string{".": {}},
	}
	view := collectFromFS(fs, newCPUSampler(), "", nil)
	if view.Error == "" {
		t.Fatal("expected error when /proc files are missing")
	}
	if view.OS == "" || view.Arch == "" || view.ConsolePID == 0 {
		t.Fatalf("identity missing: %+v", view)
	}
	if view.CPUPercent != nil || view.MemTotalBytes != nil {
		t.Fatalf("metrics should be empty on missing proc: %+v", view)
	}
}

func TestCollectFromFSVanishedPIDDoesNotSetError(t *testing.T) {
	fs := fixtureFS()
	fs.dirs["."] = append(fs.dirs["."], "99")
	view := collectFromFS(fs, newCPUSampler(), "", nil)
	if view.Error != "" {
		t.Fatalf("vanished pid must not set snapshot error, got %q", view.Error)
	}
	if len(view.Processes) != 2 {
		t.Fatalf("want java+node only, got %+v", view.Processes)
	}
}

func TestCollectFromFSProcessDirFailureSetsError(t *testing.T) {
	fs := fixtureFS()
	fs.dirs = map[string][]string{}
	view := collectFromFS(fs, newCPUSampler(), "", nil)
	if view.Error == "" || !strings.Contains(view.Error, "processes:") {
		t.Fatalf("ReadDir failure should set processes error, got %q", view.Error)
	}
	if view.MemTotalBytes == nil {
		t.Fatal("memory should still be present")
	}
}

func TestCollectFromFSDiskFailureOmitsDisk(t *testing.T) {
	fs := fixtureFS()
	view := collectFromFS(fs, newCPUSampler(), "/mnt/broken", func(string) (uint64, uint64, error) {
		return 0, 0, os.ErrPermission
	})
	if view.DiskUsedBytes != nil || view.DiskTotalBytes != nil {
		t.Fatalf("disk fields should be omitted: %+v", view)
	}
	if view.Error != "" {
		t.Fatalf("disk failure must not set snapshot error, got %q", view.Error)
	}
	if view.MemTotalBytes == nil {
		t.Fatal("memory should still be present")
	}
}

func TestTruncateRunes(t *testing.T) {
	s := strings.Repeat("й", 210)
	got := truncateRunes(s, 200)
	if utf8.RuneCountInString(got) != 200 {
		t.Fatalf("rune count = %d", utf8.RuneCountInString(got))
	}
	if truncateRunes("short", 200) != "short" {
		t.Fatal("short string should be unchanged")
	}
}

func TestRankProcessesCapsAt25(t *testing.T) {
	samples := make([]procSample, 40)
	for i := range samples {
		samples[i] = procSample{pid: i + 1, comm: "p", cmd: "cmd", rssBytes: uint64(i), ticks: 1}
	}
	out := rankProcesses(samples, nil)
	if len(out) != systemProcessCap {
		t.Fatalf("len = %d want %d", len(out), systemProcessCap)
	}
	if out[0].RSSBytes < out[len(out)-1].RSSBytes {
		t.Fatalf("expected RSS descending, first=%d last=%d", out[0].RSSBytes, out[len(out)-1].RSSBytes)
	}
}
