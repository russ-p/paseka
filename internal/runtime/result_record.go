package runtime

import (
	"time"

	"github.com/russ-p/paseka/internal/adapters"
	"github.com/russ-p/paseka/internal/protocol"
)

// resultRecord renders the adapter's normalized outcome as the run's
// result.json. It is only used when the adapter returned without persisting a
// record of its own: finalization owns the persisted outcome, so a verdict with
// no file to land in is not acceptable. Fields the adapter did persist are
// preserved by the reconciliation itself, which reads and rewrites the record
// rather than replacing it.
func (p *preparedDispatch) resultRecord(result *adapters.RunResult) protocol.Result {
	res := protocol.Result{
		ProtocolVersion:   protocol.Version,
		TraceID:           p.req.TraceID,
		AgentID:           p.agentID,
		Summary:           result.Summary,
		Diagnostics:       protocol.Diagnostics{ExitCode: result.ExitCode},
		Usage:             result.Usage,
		ProviderSessionID: result.ProviderSessionID,
		FinishedAt:        time.Now().UTC(),
		Gate:              result.Gate,
		Contract:          result.Contract,
		Provenance:        p.req.provenance(),
	}
	if result.Status != "" {
		res.Status = protocol.RunStatus(result.Status)
	}
	res.Reason = result.Reason
	if result.Err != nil {
		res.Diagnostics.Error = result.Err.Error()
	}
	for _, artifact := range result.Artifacts {
		res.Artifacts = append(res.Artifacts, protocol.ArtifactRef{Kind: artifact.Kind, Path: artifact.Path})
	}
	return res
}
