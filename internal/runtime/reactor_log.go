package runtime

import (
	"fmt"
	"log"
	"strings"

	"github.com/paseka/paseka/internal/protocol"
	"github.com/paseka/paseka/internal/taskledger"
)

func logEventReceived(ev protocol.Event) {
	kind := protocol.PayloadKind(ev.Payload)
	agent := ev.AgentID
	if agent == "" {
		agent = "-"
	}
	log.Printf("runtime: event received trace=%s type=%s kind=%s agent=%s",
		ev.TraceID, ev.Type, kindOrDash(kind), agent)
}

func logLedgerOutcome(traceID string, ready int) {
	if ready == 0 {
		return
	}
	log.Printf("runtime: ledger trace=%s ready_tasks=%d", traceID, ready)
}

func logTaskDispatchPlan(traceID, taskID, bee string) {
	log.Printf("runtime: will dispatch task trace=%s task=%s bee=%s", traceID, taskID, bee)
}

func logDirectDispatchPlan(traceID string, ev protocol.Event, bees []string) {
	kind := protocol.PayloadKind(ev.Payload)
	log.Printf("runtime: will dispatch direct trace=%s type=%s kind=%s bees=%s",
		traceID, ev.Type, kindOrDash(kind), strings.Join(bees, ","))
}

func logDispatchDone(mode DispatchMode, bee, traceID, taskID, agentID, status string) {
	taskPart := ""
	if taskID != "" {
		taskPart = fmt.Sprintf(" task=%s", taskID)
	}
	log.Printf("runtime: dispatch %s done bee=%s trace=%s%s agent=%s status=%s",
		mode, bee, traceID, taskPart, agentID, status)
}

func logDispatchSkip(reason, traceID, taskID, bee string) {
	log.Printf("runtime: skip dispatch (%s) trace=%s task=%s bee=%s", reason, traceID, taskID, bee)
}

func logNoDispatch(ev protocol.Event) {
	kind := protocol.PayloadKind(ev.Payload)
	log.Printf("runtime: no dispatch trace=%s type=%s kind=%s", ev.TraceID, ev.Type, kindOrDash(kind))
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
