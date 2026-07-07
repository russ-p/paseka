package runtime

import (
	"fmt"
	"strings"

	"github.com/paseka/paseka/internal/logging"
	"github.com/paseka/paseka/internal/protocol"
	"github.com/paseka/paseka/internal/taskledger"
)

var runtimeLog = logging.Component("runtime")

func logEventReceived(ev protocol.Event) {
	kind := protocol.PayloadKind(ev.Payload)
	agent := ev.AgentID
	if agent == "" {
		agent = "-"
	}
	runtimeLog.Debug("event received",
		logging.F("trace", ev.TraceID),
		logging.F("type", string(ev.Type)),
		logging.F("kind", kindOrDash(kind)),
		logging.F("agent", agent),
	)
}

func logLedgerOutcome(traceID string, ready int) {
	if ready == 0 {
		return
	}
	runtimeLog.Info("ledger ready tasks",
		logging.F("trace", traceID),
		logging.F("ready_tasks", fmt.Sprintf("%d", ready)),
	)
}

func logTaskDispatchPlan(traceID, taskID, bee string) {
	runtimeLog.Info("will dispatch task",
		logging.F("trace", traceID),
		logging.F("task", taskID),
		logging.F("bee", bee),
	)
}

func logDirectDispatchPlan(traceID string, ev protocol.Event, bees []string) {
	kind := protocol.PayloadKind(ev.Payload)
	runtimeLog.Info("will dispatch direct",
		logging.F("trace", traceID),
		logging.F("type", string(ev.Type)),
		logging.F("kind", kindOrDash(kind)),
		logging.F("bees", strings.Join(bees, ",")),
	)
}

func logDispatchDone(mode DispatchMode, bee, traceID, taskID, agentID, status string) {
	fields := []logging.Field{
		logging.F("mode", string(mode)),
		logging.F("bee", bee),
		logging.F("trace", traceID),
		logging.F("agent", agentID),
		logging.F("status", status),
	}
	if taskID != "" {
		fields = append(fields, logging.F("task", taskID))
	}
	runtimeLog.Info("dispatch done", fields...)
}

func logDispatchSkip(reason, traceID, taskID, bee string) {
	runtimeLog.Info("skip dispatch",
		logging.F("reason", reason),
		logging.F("trace", traceID),
		logging.F("task", taskID),
		logging.F("bee", bee),
	)
}

func logNoDispatch(ev protocol.Event) {
	kind := protocol.PayloadKind(ev.Payload)
	runtimeLog.Debug("no dispatch",
		logging.F("trace", ev.TraceID),
		logging.F("type", string(ev.Type)),
		logging.F("kind", kindOrDash(kind)),
	)
}

func taskBeeName(task taskledger.TaskSnapshot) string {
	if task.Bee != "" {
		return task.Bee
	}
	return "builder"
}

func kindOrDash(kind string) string {
	if kind == "" {
		return "-"
	}
	return kind
}
