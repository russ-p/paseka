//go:build integration

package purge_test

import (
	"context"
	"os"
	"testing"
	"time"

	"github.com/russ-p/paseka/internal/bus"
	"github.com/russ-p/paseka/internal/colony"
	"github.com/russ-p/paseka/internal/protocol"
	"github.com/russ-p/paseka/internal/purge"
	"github.com/russ-p/paseka/internal/taskledger"
)

func TestPruneBusByLedgerAge(t *testing.T) {
	url := os.Getenv("PASEKA_NATS_URL")
	if url == "" {
		url = "nats://127.0.0.1:4222"
	}

	repo := initTestRepo(t)
	slug := "colony-prune-bus"
	setupPurgeHomeWithNATS(t, repo, slug, url)
	writeColonyManifest(t, repo, slug, "paseka.colony-prune-bus", 0)

	ctx, err := colony.ResolveContext(repo)
	if err != nil {
		t.Fatal(err)
	}

	cfg := bus.Config{URL: url, SubjectPrefix: "paseka.colony-prune-bus", Slug: slug}
	client, err := bus.ConnectFull(cfg)
	if err != nil {
		t.Skipf("nats unavailable: %v", err)
	}
	defer client.Close()

	oldTrace := "trace-prune-old-" + time.Now().Format("150405")
	newTrace := "trace-prune-new-" + time.Now().Format("150405")

	kv, err := client.JetStream().KeyValue(bus.TaskLedgerBucket(slug))
	if err != nil {
		t.Fatal(err)
	}
	ledger := taskledger.NewKVLedger(kv)

	for _, tc := range []struct {
		trace string
		at    time.Time
	}{
		{oldTrace, time.Now().Add(-30 * 24 * time.Hour)},
		{newTrace, time.Now()},
	} {
		ev, err := protocol.NewEvent(tc.trace, "agent-1", 1, protocol.EventInsight, map[string]any{
			"kind":  "task.plan",
			"tasks": []map[string]string{{"taskId": "task-1", "title": "prune me"}},
		})
		if err != nil {
			t.Fatal(err)
		}
		ev.CreatedAt = tc.at
		if err := client.PublishEvent(context.Background(), ev); err != nil {
			t.Fatal(err)
		}
		if _, err := ledger.Apply(ev); err != nil {
			t.Fatal(err)
		}
	}

	plan, err := purge.Prune(ctx, purge.PruneTarget{
		Bus:       true,
		OlderThan: 14 * 24 * time.Hour,
		Now:       time.Now(),
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(plan.Bus) != 1 || plan.Bus[0] != oldTrace {
		t.Fatalf("plan bus = %v, want [%s]", plan.Bus, oldTrace)
	}

	res, err := purge.ExecutePrune(ctx, purge.PruneTarget{Bus: true}, plan)
	if err != nil {
		t.Fatal(err)
	}
	if len(res.Bus) != 1 || res.Bus[0].TraceID != oldTrace {
		t.Fatalf("result bus = %+v", res.Bus)
	}
	if res.Bus[0].EventsRemoved != 1 {
		t.Fatalf("events removed = %d, want 1", res.Bus[0].EventsRemoved)
	}

	snap, err := ledger.Snapshot(oldTrace)
	if err != nil {
		t.Fatal(err)
	}
	if len(snap.Tasks) != 0 {
		t.Fatalf("old trace still has tasks: %+v", snap.Tasks)
	}
	snap, err = ledger.Snapshot(newTrace)
	if err != nil {
		t.Fatal(err)
	}
	if len(snap.Tasks) != 1 {
		t.Fatalf("new trace tasks = %+v, want 1", snap.Tasks)
	}
}
