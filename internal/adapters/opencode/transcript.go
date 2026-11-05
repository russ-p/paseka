package opencode

import (
	"context"

	"github.com/russ-p/paseka/internal/adapters"
	"github.com/russ-p/paseka/internal/adapters/opencodev1"
	"github.com/russ-p/paseka/internal/adapters/opencodev2"
)

// ResolveTranscript keeps the OpenCode generation split behind the `opencode`
// name. Reads use the store itself as the generation signal: a session recorded
// on the frozen 1.x line is read from its storage JSON tree even when the
// installed binary has since moved to 2.x, and every other id is offered to the
// 2.x export/API route. The caller never has to know which generation wrote the
// session, and launch detection is unchanged.
func (a *Adapter) ResolveTranscript(ctx context.Context, req adapters.TranscriptRequest) (adapters.Transcript, error) {
	transcript, err := opencodev1.New().ResolveTranscript(ctx, req)
	if err != nil {
		return transcript, err
	}
	// Only an absent 1.x store is ambiguous: a malformed store, a timeout, or a
	// real read failure stays the 1.x answer rather than being retried as 2.x.
	if transcript.Omitted != adapters.SessionLogOmittedStoreNotFound {
		return transcript, nil
	}
	return opencodev2.New().ResolveTranscript(ctx, req)
}
