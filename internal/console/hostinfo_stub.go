//go:build !linux

package console

func snapshotHost(_ *cpuSampler, _ string) SystemView {
	return baseSystemIdentity()
}

func snapshotHostPlaque(sampler *cpuSampler, colonyRoot string) SystemView {
	return snapshotHost(sampler, colonyRoot)
}
