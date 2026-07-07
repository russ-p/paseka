package bus

import (
	"bytes"
	"strings"
	"testing"

	"github.com/paseka/paseka/internal/logging"
	"github.com/paseka/paseka/internal/protocol"
)

func TestLogDomainEventPublish(t *testing.T) {
	var buf bytes.Buffer
	logging.SetDefault(logging.New(logging.Options{
		Level:   logging.LevelInfo,
		Writer:  &buf,
		NoColor: true,
	}))
	t.Cleanup(func() {
		logging.SetDefault(logging.New(logging.Options{Level: logging.LevelInfo}))
	})

	ev, err := protocol.NewEvent("trace-1", "agent-1", 0, protocol.EventSignal, protocol.TaskReadyPayload{
		Kind:   protocol.TaskEventReady,
		TaskID: "task-1",
	})
	if err != nil {
		t.Fatal(err)
	}

	logDomainEvent("publish", "paseka.events.SIGNAL.task.ready", ev)
	out := buf.String()
	for _, want := range []string{"bus", "domain event", "direction=publish", "trace=trace-1", "kind=task.ready"} {
		if !strings.Contains(out, want) {
			t.Fatalf("output %q missing %q", out, want)
		}
	}
}
