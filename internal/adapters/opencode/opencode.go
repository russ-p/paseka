package opencode

import (
	"context"

	"github.com/russ-p/paseka/internal/adapters"
	"github.com/russ-p/paseka/internal/adapters/opencodev1"
	"github.com/russ-p/paseka/internal/adapters/opencodev2"
)

// Adapter is the OpenCode version proxy: it detects the installed OpenCode major
// and delegates the run to the matching implementation. It reports itself as
// `opencode`, so bee YAML, profiles, trails, and the Console keep one name.
type Adapter struct{}

func New() *Adapter {
	return &Adapter{}
}

func (a *Adapter) Name() string {
	return adapterName
}

// LaunchSpec reports the proxy's built-in binary.
func (a *Adapter) LaunchSpec() adapters.LaunchSpec {
	return adapters.LaunchSpec{DefaultBinary: defaultBinary}
}

// ResolveImplementation reports which concrete implementation the params would
// dispatch to, without running an agent.
func (a *Adapter) ResolveImplementation(params adapters.RunParams) (string, error) {
	resolved, err := resolve("", params)
	if err != nil {
		return "", err
	}
	return resolved.implementation, nil
}

// Run resolves the installed major and delegates to the matching adapter.
func (a *Adapter) Run(ctx context.Context, req adapters.RunRequest) (*adapters.RunResult, error) {
	resolved, err := resolve(req.Binary, req.Params)
	if err != nil {
		return nil, err
	}
	req.Binary = resolved.binary
	req.AdapterName = adapterName
	req.Implementation = resolved.implementation

	switch resolved.implementation {
	case implementationV2:
		return opencodev2.New().Run(ctx, req)
	default:
		return opencodev1.New().Run(ctx, req)
	}
}
