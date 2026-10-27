package opencode

import (
	"context"
	"strings"

	"github.com/russ-p/paseka/internal/adapters"
	"github.com/russ-p/paseka/internal/adapters/opencodev1"
	"github.com/russ-p/paseka/internal/adapters/opencodev2"
	"github.com/russ-p/paseka/internal/protocol"
)

// SessionAdapter is the interactive half of the OpenCode version proxy.
type SessionAdapter struct{}

func NewSession() *SessionAdapter {
	return &SessionAdapter{}
}

func (a *SessionAdapter) Name() string {
	return adapterName
}

// ResolveImplementation reports which concrete implementation a continuation
// would run on. Resume uses it to refuse cross-major sessions.
func (a *SessionAdapter) ResolveImplementation(params adapters.RunParams) (string, error) {
	resolved, err := resolve("", params)
	if err != nil {
		return "", err
	}
	return resolved.implementation, nil
}

// SessionCommand resolves the installed major and delegates the interactive
// command to the matching adapter.
func (a *SessionAdapter) SessionCommand(req adapters.SessionRequest) (adapters.SessionCommand, error) {
	resolved, err := resolve(req.Binary, req.Params)
	if err != nil {
		return adapters.SessionCommand{}, err
	}
	req.Binary = resolved.binary
	req.AdapterName = adapterName
	req.Implementation = resolved.implementation

	var cmd adapters.SessionCommand
	switch resolved.implementation {
	case implementationV2:
		cmd, err = opencodev2.NewSession().SessionCommand(req)
	default:
		cmd, err = opencodev1.NewSession().SessionCommand(req)
	}
	if err != nil {
		return adapters.SessionCommand{}, err
	}
	cmd.Implementation = resolved.implementation
	return cmd, nil
}

// SessionUsage delegates post-exit accounting to the implementation that ran the
// session, using the resolved binary the TUI actually used.
func (a *SessionAdapter) SessionUsage(ctx context.Context, handle adapters.SessionHandle) *protocol.Usage {
	switch resolveHandleImplementation(handle) {
	case implementationV2:
		return opencodev2.NewSession().SessionUsage(ctx, handle)
	case implementationV1:
		return opencodev1.NewSession().SessionUsage(ctx, handle)
	default:
		return nil
	}
}

func resolveHandleImplementation(handle adapters.SessionHandle) string {
	if name := implementationName(handle.Implementation); name != "" {
		return name
	}
	if strings.TrimSpace(handle.Binary) == "" {
		return ""
	}
	resolved, err := resolve(handle.Binary, adapters.RunParams{})
	if err != nil {
		return ""
	}
	return resolved.implementation
}
