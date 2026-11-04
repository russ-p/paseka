package opencode

import (
	"context"

	"github.com/russ-p/paseka/internal/adapters"
	"github.com/russ-p/paseka/internal/adapters/opencodev1"
)

// ResolveTranscript keeps the OpenCode generation split behind the `opencode`
// name. A session recorded on the frozen 1.x line is read from its storage JSON
// tree even when the installed binary has since moved to 2.x; the caller never
// has to know which generation wrote the session. The 2.x route is a separate
// slice and will be tried here when no 1.x store matches.
func (a *Adapter) ResolveTranscript(ctx context.Context, req adapters.TranscriptRequest) (adapters.Transcript, error) {
	return opencodev1.New().ResolveTranscript(ctx, req)
}
