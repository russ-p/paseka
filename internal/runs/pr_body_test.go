package runs

import (
	"testing"
	"time"

	"github.com/russ-p/paseka/internal/protocol"
)

func TestResolvePRBodyLastWriteWins(t *testing.T) {
	root := t.TempDir()
	traceID := "trace-pr-body"
	base := time.Now().UTC()

	d := Dir{ColonyRoot: root, TraceID: traceID, AgentID: "agent-1"}
	if err := d.Prepare(); err != nil {
		t.Fatal(err)
	}
	first, err := protocol.NewEvent(traceID, "builder", 1, protocol.EventInsight, protocol.PRBodyPayload{
		Kind: protocol.InsightPRBody,
		Body: "first",
	})
	if err != nil {
		t.Fatal(err)
	}
	first.CreatedAt = base
	if err := d.AppendEvent(first); err != nil {
		t.Fatal(err)
	}
	later, err := protocol.NewEvent(traceID, "builder", 2, protocol.EventInsight, protocol.PRBodyPayload{
		Kind: protocol.InsightPRBody,
		Body: "latest body",
	})
	if err != nil {
		t.Fatal(err)
	}
	later.CreatedAt = base.Add(time.Minute)
	if err := d.AppendEvent(later); err != nil {
		t.Fatal(err)
	}

	got, err := ResolvePRBody(root, traceID)
	if err != nil {
		t.Fatal(err)
	}
	if got != "latest body" {
		t.Fatalf("body = %q", got)
	}
}
