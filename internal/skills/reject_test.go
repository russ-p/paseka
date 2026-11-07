package skills_test

import (
	"fmt"
	"sync"
	"testing"
	"time"

	"github.com/russ-p/paseka/internal/skills"
)

// TestAppendProposalDecisionSerializesConcurrentWrites drives the shared
// decision writer from several goroutines. The lock is what keeps a line whole;
// the assertion is that every write is present and every line parses.
func TestAppendProposalDecisionSerializesConcurrentWrites(t *testing.T) {
	root := t.TempDir()
	traceID := "trace-alpha"
	const writers = 6

	var wg sync.WaitGroup
	errs := make(chan error, writers)
	for i := 0; i < writers; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			errs <- skills.AppendProposalDecision(root, traceID, skills.ProposalDecision{
				Skill:         fmt.Sprintf("skill-%d", i),
				Trail:         traceID,
				ProcedureHash: fmt.Sprintf("hash-%d", i),
				Destination:   skills.ProposalStoreDestination,
				Decision:      skills.ProposalRefused,
				At:            time.Date(2026, 11, 7, 10, 0, i, 0, time.UTC),
			})
		}(i)
	}
	wg.Wait()
	close(errs)
	for err := range errs {
		if err != nil {
			t.Fatalf("append decision: %v", err)
		}
	}

	decisions, issues, err := skills.ReadProposalDecisions(
		skills.ProposalDecisionsPath(root, traceID), traceID,
	)
	if err != nil {
		t.Fatalf("read decisions: %v", err)
	}
	if len(issues) != 0 {
		t.Fatalf("decision issues: %v", issues)
	}
	if len(decisions) != writers {
		t.Fatalf("decisions = %d, want %d", len(decisions), writers)
	}
}
