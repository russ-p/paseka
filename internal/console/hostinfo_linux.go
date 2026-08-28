//go:build linux

package console

import "syscall"

func snapshotHost(sampler *cpuSampler, colonyRoot string) SystemView {
	if sampler != nil {
		sampler.collectMu.Lock()
		defer sampler.collectMu.Unlock()
	}
	return collectFromFS(osProcFS{root: "/proc"}, sampler, colonyRoot, linuxDiskUsage)
}

func linuxDiskUsage(path string) (used, total uint64, err error) {
	var st syscall.Statfs_t
	if err := syscall.Statfs(path, &st); err != nil {
		return 0, 0, err
	}
	bsize := uint64(st.Frsize)
	if bsize == 0 {
		bsize = uint64(st.Bsize)
	}
	total = st.Blocks * bsize
	avail := st.Bavail * bsize
	if total >= avail {
		used = total - avail
	}
	return used, total, nil
}
