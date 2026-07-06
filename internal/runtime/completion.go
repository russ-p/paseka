package runtime

import (
	"fmt"

	"github.com/paseka/paseka/internal/adapters"
	"github.com/paseka/paseka/internal/colony"
	"github.com/paseka/paseka/internal/protocol"
)

func (d *Dispatcher) enforceCompletionContract(bee colony.Bee, runEvents []protocol.Event, result *adapters.RunResult) {
	if len(bee.CompletionContract.Required) == 0 || result == nil {
		return
	}
	domain := filterDomainEvents(runEvents)
	if err := bee.CompletionContract.ValidateRunEvents(domain); err != nil {
		msg := fmt.Sprintf("runtime: %s", err.Error())
		result.Warnings = append(result.Warnings, msg)
		result.Status = string(protocol.StatusFailed)
		if result.Err == nil {
			result.Err = fmt.Errorf("%s", msg)
		}
	}
}

func filterDomainEvents(events []protocol.Event) []protocol.Event {
	var out []protocol.Event
	for _, ev := range events {
		if protocol.IsDomainEvent(ev.Type) {
			out = append(out, ev)
		}
	}
	return out
}
