//go:build integration

package colony_test

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/paseka/paseka/internal/bus"
	"github.com/paseka/paseka/internal/colony"
	"github.com/paseka/paseka/internal/protocol"
	"github.com/paseka/paseka/internal/purge"
	"github.com/paseka/paseka/internal/taskledger"
)

func TestPurgeBusTrace(t *testing.T) {
	url := os.Getenv("PASEKA_NATS_URL")
	if url == "" {
		url = "nats://127.0.0.1:4222"
	}

	repo := initTestRepo(t)
	slug := "colony-purge-bus"
	setupPurgeHomeWithNATS(t, repo, slug, url)
	writeColonyManifest(t, repo, slug, "paseka.colony-purge-bus")

	ctx, err := colony.ResolveContext(repo)
	if err != nil {
		t.Fatal(err)
	}

	cfg := bus.Config{
		URL:           url,
		SubjectPrefix: "paseka.colony-purge-bus",
		Slug:          slug,
	}
	client, err := bus.ConnectFull(cfg)
	if err != nil {
		t.Skipf("nats unavailable: %v", err)
	}

	traceID := "trace-colony-purge-" + time.Now().Format("150405")
	otherTraceID := "trace-colony-control-" + time.Now().Format("150405")

	plan, err := protocol.NewEvent(traceID, "agent-1", 1, protocol.EventInsight, map[string]any{
		"kind": "task.plan",
		"tasks": []map[string]string{
			{"taskId": "task-1", "title": "purge me"},
		},
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
	for _, ev := range []protocol.Event{plan, other} {
		if err := client.PublishEvent(context.Background(), ev); err != nil {
			t.Fatal(err)
		}
	}

	kv, err := client.JetStream().KeyValue(bus.TaskLedgerBucket(slug))
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

	artifactName := traceID + "-agent-1.diff"
	if _, err := client.StoreArtifact(artifactName, []byte("diff bytes")); err != nil {
		t.Fatal(err)
	}
	otherArtifact := otherTraceID + "-agent-2.diff"
	if _, err := client.StoreArtifact(otherArtifact, []byte("keep")); err != nil {
		t.Fatal(err)
	}
	client.Close()

	purgePlan, err := purge.Plan(ctx, colony.PurgeTarget{Bus: true, TraceID: traceID})
	if err != nil {
		t.Fatal(err)
	}
	if purgePlan.Bus == nil || purgePlan.Bus.Empty() {
		t.Fatalf("plan bus = %#v", purgePlan.Bus)
	}

	res, err := purge.Execute(ctx, colony.PurgeTarget{Bus: true, TraceID: traceID})
	if err != nil {
		t.Fatal(err)
	}
	if res.Bus == nil {
		t.Fatal("expected bus purge result")
	}
	if len(res.Bus.KeysRemoved) != 1 || res.Bus.KeysRemoved[0] != traceID {
		t.Fatalf("keys removed = %#v", res.Bus.KeysRemoved)
	}
	if res.Bus.EventsRemoved != 1 {
		t.Fatalf("events removed = %d, want 1", res.Bus.EventsRemoved)
	}
	if len(res.Bus.ObjectsRemoved) != 1 || res.Bus.ObjectsRemoved[0] != artifactName {
		t.Fatalf("objects removed = %#v", res.Bus.ObjectsRemoved)
	}

	verifyClient, err := bus.ConnectFull(cfg)
	if err != nil {
		t.Fatal(err)
	}
	defer verifyClient.Close()

	events, err := verifyClient.ReplayTrace(traceID)
	if err != nil {
		t.Fatal(err)
	}
	if len(events) != 0 {
		t.Fatalf("replay after purge = %d events, want 0", len(events))
	}
	otherEvents, err := verifyClient.ReplayTrace(otherTraceID)
	if err != nil {
		t.Fatal(err)
	}
	if len(otherEvents) != 1 {
		t.Fatalf("other trace replay = %d events, want 1", len(otherEvents))
	}

	verifyKV, err := verifyClient.JetStream().KeyValue(bus.TaskLedgerBucket(slug))
	if err != nil {
		t.Fatal(err)
	}
	verifyLedger := taskledger.NewKVLedger(verifyKV)
	snap, err := verifyLedger.Snapshot(traceID)
	if err != nil {
		t.Fatal(err)
	}
	if len(snap.Tasks) != 0 || snap.EnergyBudget != 0 || snap.EnergyRemaining != 0 {
		t.Fatalf("snapshot after purge = %#v, want empty trace", snap)
	}

	osStore, err := verifyClient.JetStream().ObjectStore(bus.ArtifactsBucket(slug))
	if err != nil {
		t.Fatal(err)
	}
	if _, err := osStore.GetInfo(artifactName); err == nil {
		t.Fatalf("artifact %q still present after purge", artifactName)
	}
	if _, err := osStore.GetInfo(otherArtifact); err != nil {
		t.Fatalf("other artifact %q should remain: %v", otherArtifact, err)
	}
}

func setupPurgeHomeWithNATS(t *testing.T, repo, slug, natsURL string) {
	t.Helper()
	base := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", base)

	homeDir, err := colony.HomeDir(slug)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Join(homeDir, "adapters"), 0o755); err != nil {
		t.Fatal(err)
	}
	cfg := "colony_root: " + repo + "\nslug: " + slug + "\nnats:\n  url: " + natsURL + "\n"
	if err := os.WriteFile(filepath.Join(homeDir, "config.yaml"), []byte(cfg), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(homeDir, "adapters", "cursor.yaml"), []byte("binary: agent\n"), 0o644); err != nil {
		t.Fatal(err)
	}
}

func writeColonyManifest(t *testing.T, repo, slug, prefix string) {
	t.Helper()
	dir := filepath.Join(repo, ".paseka")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	manifest := "slug: " + slug + "\nnats:\n  subject_prefix: " + prefix + "\n"
	if err := os.WriteFile(filepath.Join(dir, "colony.yaml"), []byte(manifest), 0o644); err != nil {
		t.Fatal(err)
	}
}
