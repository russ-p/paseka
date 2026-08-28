//go:build !linux

package console

func snapshotHost(_ *cpuSampler, _ string) SystemView {
	return baseSystemIdentity()
}
