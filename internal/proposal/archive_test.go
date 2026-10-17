package proposal_test

import (
	"context"
	"strings"
	"testing"

	"github.com/russ-p/paseka/internal/adapters"
	"github.com/russ-p/paseka/internal/colony"
	"github.com/russ-p/paseka/internal/patches"
	"github.com/russ-p/paseka/internal/proposal"
	"github.com/russ-p/paseka/internal/protocol"
)

func autoPublishBee() colony.Bee {
	return colony.Bee{
		Role: "builder",
		Publishes: []colony.PublicationRule{
			{EventRule: colony.EventRule{Type: "MUTATION", Kind: "code.proposal.root"}},
		},
	}
}

// The early gates never reach the collector, so each one is exercised without a repository
// and each says why it declined. A skipped run is not a failure: a non-mutating bee, an
// unsuccessful run, and an explicit proposal are all ordinary.
func TestArchiveGateReasons(t *testing.T) {
	base := proposal.ArchiveRequest{
		ColonyRoot:  t.TempDir(),
		TraceID:     "trace-1",
		AgentID:     "agent-1",
		Bee:         autoPublishBee(),
		AutoPublish: true,
		Completed:   true,
	}

	t.Run("unsuccessful run", func(t *testing.T) {
		req := base
		req.Completed = false
		res, err := proposal.Archive(context.Background(), nil, req)
		if err != nil {
			t.Fatal(err)
		}
		if res.Record != nil || res.Event != nil {
			t.Fatalf("an unsuccessful run produced a record: %+v", res)
		}
		if !strings.Contains(res.Skipped, "did not complete") {
			t.Fatalf("skipped reason = %q", res.Skipped)
		}
	})

	t.Run("bee does not auto-publish", func(t *testing.T) {
		req := base
		req.AutoPublish = false
		res, err := proposal.Archive(context.Background(), nil, req)
		if err != nil {
			t.Fatal(err)
		}
		if res.Record != nil {
			t.Fatalf("a non-mutating bee produced a record: %+v", res)
		}
		if !strings.Contains(res.Skipped, "does not auto-publish") {
			t.Fatalf("skipped reason = %q", res.Skipped)
		}
		if res.Misconfigured != "" {
			t.Fatalf("a non-mutating bee is not a misconfiguration: %q", res.Misconfigured)
		}
	})

	t.Run("explicit proposal", func(t *testing.T) {
		req := base
		req.Explicit = true
		res, err := proposal.Archive(context.Background(), nil, req)
		if err != nil {
			t.Fatal(err)
		}
		if res.Record != nil || res.Event != nil {
			t.Fatalf("an explicit proposal was synthesized again: %+v", res)
		}
		if !strings.Contains(res.Skipped, "explicit") {
			t.Fatalf("skipped reason = %q", res.Skipped)
		}
	})

	t.Run("misconfigured bee is reported as such", func(t *testing.T) {
		req := base
		// worktree true but only a root proposal declared.
		req.Bee.Worktree = true
		res, err := proposal.Archive(context.Background(), nil, req)
		if err != nil {
			t.Fatal(err)
		}
		if res.Record != nil {
			t.Fatalf("a misconfigured bee produced a record: %+v", res)
		}
		if res.Misconfigured == "" {
			t.Fatal("a worktree/kind mismatch must be surfaced as a misconfiguration")
		}
	})
}

// installedProposal builds a complete record with its original event envelope, the state a
// previous finalization leaves behind.
func installedProposal(t *testing.T, root, traceID, agentID string, state string) (patches.Store, protocol.Event) {
	t.Helper()
	store := patches.NewStore(root, traceID)
	ev, err := protocol.NewEvent(traceID, agentID, 0, protocol.EventMutation, protocol.MutationPayload{
		Kind: protocol.MutationCodeProposalRoot,
		Ref:  store.Ref(agentID),
	})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := store.Write(patches.Record{
		AgentID:     agentID,
		Publication: state,
		Event:       &ev,
	}, []byte("diff --git a/x b/x\n+recorded\n")); err != nil {
		t.Fatal(err)
	}
	return store, ev
}

// A retry reuses the complete record and its original event. It must not re-collect a diff
// (the workspace here is not even a repository) and must not publish a confirmed record
// again, because that would be a second proposal for one attempt.
func TestArchiveReusesACompleteRecordWithoutRecollectingOrRepublishing(t *testing.T) {
	root := t.TempDir()
	store, original := installedProposal(t, root, "trace-1", "agent-1", patches.PublicationPublished)
	pub := &brokerPublisher{}

	res, err := proposal.Archive(context.Background(), pub, proposal.ArchiveRequest{
		ColonyRoot:  root,
		TraceID:     "trace-1",
		AgentID:     "agent-1",
		Bee:         autoPublishBee(),
		AutoPublish: true,
		Completed:   true,
	})
	if err != nil {
		t.Fatalf("a retry must not depend on the collector: %v", err)
	}
	if res.Record == nil || res.Event == nil {
		t.Fatalf("the retry returned no record: %+v", res)
	}
	if res.Event.ID != original.ID {
		t.Fatalf("retry published a new event %q, want the original %q", res.Event.ID, original.ID)
	}
	if len(pub.events) != 0 {
		t.Fatalf("a confirmed record was republished: %+v", pub.events)
	}
	rec, ok, err := store.Read("agent-1")
	if err != nil || !ok {
		t.Fatalf("record lost: ok=%v err=%v", ok, err)
	}
	if rec.Publication != patches.PublicationPublished {
		t.Fatalf("publication = %q, want the original published state", rec.Publication)
	}
}

// An uncertain attempt is retryable, but the retry publishes the event identity the record
// already holds rather than minting a second proposal.
func TestArchiveRetriesAnUnconfirmedRecordWithItsOriginalEvent(t *testing.T) {
	root := t.TempDir()
	store, original := installedProposal(t, root, "trace-1", "agent-1", patches.PublicationUnconfirmed)
	pub := &brokerPublisher{}

	if _, err := proposal.Archive(context.Background(), pub, proposal.ArchiveRequest{
		ColonyRoot:  root,
		TraceID:     "trace-1",
		AgentID:     "agent-1",
		Bee:         autoPublishBee(),
		AutoPublish: true,
		Completed:   true,
	}); err != nil {
		t.Fatal(err)
	}
	if len(pub.events) != 1 || pub.events[0].ID != original.ID {
		t.Fatalf("retry events = %+v, want the original event %q", pub.events, original.ID)
	}
	rec, _, err := store.Read("agent-1")
	if err != nil {
		t.Fatal(err)
	}
	if rec.Publication != patches.PublicationPublished {
		t.Fatalf("publication = %q, want published after the retry", rec.Publication)
	}
}

// The collector is only reached once every gate passes, so a request that passes them
// against a workspace that is not a git repository surfaces the collector error rather than
// installing a partial record.
func TestArchiveSurfacesCollectorErrors(t *testing.T) {
	root := t.TempDir()
	req := proposal.ArchiveRequest{
		ColonyRoot:  root,
		TraceID:     "trace-1",
		AgentID:     "agent-1",
		Bee:         autoPublishBee(),
		AutoPublish: true,
		Completed:   true,
		Baseline:    adapters.WorkspaceBaseline{},
	}
	res, err := proposal.Archive(context.Background(), nil, req)
	if err == nil {
		t.Fatal("expected a collector error")
	}
	if res.Record != nil {
		t.Fatal("a collector error must not install a record")
	}
}
