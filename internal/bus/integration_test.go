//go:build integration

package bus_test

import (
	"context"
	"os"
	"testing"
	"time"

	"github.com/paseka/paseka/internal/bus"
	"github.com/paseka/paseka/internal/protocol"
	"github.com/paseka/paseka/internal/taskledger"
)

func TestIntegrationPublishAndReplay(t *testing.T) {
	url := os.Getenv("PASEKA_NATS_URL")
	if url == "" {
		url = "nats://127.0.0.1:4222"
	}
	cfg := bus.Config{
		URL:           url,
		SubjectPrefix: "paseka.integration-test",
		Slug:          "integration-test",
	}
	client, err := bus.Connect(cfg)
	if err != nil {
		t.Skipf("nats unavailable: %v", err)
	}
	defer client.Close()

	traceID := "trace-integration-" + time.Now().Format("150405")
	ev, err := protocol.NewEvent(traceID, "test", 1, protocol.EventInsight, map[string]any{
		"kind": "task.plan",
		"tasks": []map[string]string{
			{"taskId": "task-1", "title": "test"},
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	if err := client.PublishEvent(context.Background(), ev); err != nil {
		t.Fatal(err)
	}

	events, err := client.ReplayTrace(traceID)
	if err != nil {
		t.Fatal(err)
	}
	if len(events) == 0 {
		t.Fatal("expected at least one replayed event")
	}
}

func TestIntegrationPurgeTrace(t *testing.T) {
	url := os.Getenv("PASEKA_NATS_URL")
	if url == "" {
		url = "nats://127.0.0.1:4222"
	}
	cfg := bus.Config{
		URL:           url,
		SubjectPrefix: "paseka.integration-purge",
		Slug:          "integration-purge",
	}
	client, err := bus.ConnectFull(cfg)
	if err != nil {
		t.Skipf("nats unavailable: %v", err)
	}
	defer client.Close()

	traceID := "trace-purge-" + time.Now().Format("150405")
	otherTraceID := "trace-purge-control-" + time.Now().Format("150405")

	plan, err := protocol.NewEvent(traceID, "agent-1", 1, protocol.EventInsight, map[string]any{
		"kind": "task.plan",
		"tasks": []map[string]string{
			{"taskId": "task-1", "title": "purge me"},
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	signal, err := protocol.NewEvent(traceID, "agent-1", 2, protocol.EventSignal, map[string]any{
		"kind":   "energy.add",
		"amount": 3,
	})
	if err != nil {
		t.Fatal(err)
	}
	other, err := protocol.NewEvent(otherTraceID, "agent-2", 1, protocol.EventInsight, map[string]any{
		"kind":    "context.note",
		"summary": "keep this event",
	})
	if err != nil {
		t.Fatal(err)
	}
	for _, ev := range []protocol.Event{plan, signal, other} {
		if err := client.PublishEvent(context.Background(), ev); err != nil {
			t.Fatal(err)
		}
	}

	kv, err := client.JetStream().KeyValue(bus.TaskLedgerBucket(cfg.Slug))
	if err != nil {
		t.Fatal(err)
	}
	ledger := taskledger.NewKVLedger(kv)
	if err := ledger.SeedEnergy(traceID, 10); err != nil {
		t.Fatal(err)
	}
	if _, err := ledger.Apply(plan); err != nil {
		t.Fatal(err)
	}
	snap, err := ledger.Snapshot(traceID)
	if err != nil {
		t.Fatal(err)
	}
	if len(snap.Tasks) == 0 {
		t.Fatal("expected seeded task ledger before purge")
	}

	artifactName := traceID + "-agent-1.diff"
	if _, err := client.StoreArtifact(artifactName, []byte("diff bytes")); err != nil {
		t.Fatal(err)
	}
	otherArtifact := otherTraceID + "-agent-2.diff"
	if _, err := client.StoreArtifact(otherArtifact, []byte("keep")); err != nil {
		t.Fatal(err)
	}

	result, err := client.PurgeTrace(traceID)
	if err != nil {
		t.Fatal(err)
	}
	if len(result.KeysRemoved) != 1 || result.KeysRemoved[0] != traceID {
		t.Fatalf("keys removed = %#v, want [%q]", result.KeysRemoved, traceID)
	}
	if result.EventsRemoved != 2 {
		t.Fatalf("events removed = %d, want 2", result.EventsRemoved)
	}
	if len(result.ObjectsRemoved) != 1 || result.ObjectsRemoved[0] != artifactName {
		t.Fatalf("objects removed = %#v, want [%q]", result.ObjectsRemoved, artifactName)
	}

	snap, err = ledger.Snapshot(traceID)
	if err != nil {
		t.Fatal(err)
	}
	if len(snap.Tasks) != 0 || snap.EnergyBudget != 0 || snap.EnergyRemaining != 0 {
		t.Fatalf("snapshot after purge = %#v, want empty trace", snap)
	}

	events, err := client.ReplayTrace(traceID)
	if err != nil {
		t.Fatal(err)
	}
	if len(events) != 0 {
		t.Fatalf("replay after purge = %d events, want 0", len(events))
	}

	otherEvents, err := client.ReplayTrace(otherTraceID)
	if err != nil {
		t.Fatal(err)
	}
	if len(otherEvents) != 1 {
		t.Fatalf("other trace replay = %d events, want 1", len(otherEvents))
	}

	os, err := client.JetStream().ObjectStore(bus.ArtifactsBucket(cfg.Slug))
	if err != nil {
		t.Fatal(err)
	}
	if _, err := os.GetInfo(artifactName); err == nil {
		t.Fatalf("artifact %q still present after purge", artifactName)
	}
	if _, err := os.GetInfo(otherArtifact); err != nil {
		t.Fatalf("other artifact %q should remain: %v", otherArtifact, err)
	}
}
