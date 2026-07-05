//go:build integration

package bus_test

import (
	"context"
	"os"
	"testing"
	"time"

	"github.com/paseka/paseka/internal/bus"
	"github.com/paseka/paseka/internal/protocol"
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
