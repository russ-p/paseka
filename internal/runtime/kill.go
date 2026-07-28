package runtime

import (
	"context"
	"strings"

	"github.com/paseka/paseka/internal/protocol"
)

func systemKillDetected(ev protocol.Event) bool {
	return ev.Type == protocol.EventSignal && protocol.PayloadKind(ev.Payload) == string(protocol.SignalSystemKill)
}

func (r *Reactor) cancelInflightForTrace(traceID string) {
	if traceID == "" {
		return
	}
	taskPrefix := traceID + ":"
	directPrefix := traceID + "|"

	r.mu.Lock()
	var cancels []context.CancelFunc
	for key, cancel := range r.inflight {
		if strings.HasPrefix(key, taskPrefix) {
			cancels = append(cancels, cancel)
		}
	}
	for key, cancel := range r.directInflight {
		if strings.HasPrefix(key, directPrefix) {
			cancels = append(cancels, cancel)
		}
	}
	r.mu.Unlock()

	for _, cancel := range cancels {
		cancel()
	}
}

func (r *Reactor) traceKilled(traceID string) bool {
	if traceID == "" || r.ledger == nil {
		return false
	}
	snap, err := r.ledger.Snapshot(traceID)
	if err != nil {
		return false
	}
	return snap.Killed
}

func (r *Reactor) beginTaskInflight(traceID, taskID string) (context.Context, func()) {
	key := traceID + ":" + taskID
	ctx, cancel := context.WithCancel(context.Background())

	r.mu.Lock()
	r.inflight[key] = cancel
	r.mu.Unlock()

	end := func() {
		r.mu.Lock()
		delete(r.inflight, key)
		r.mu.Unlock()
		cancel()
	}
	return ctx, end
}

func (r *Reactor) beginDirectInflight(key string) (context.Context, func()) {
	ctx, cancel := context.WithCancel(context.Background())

	r.mu.Lock()
	r.directInflight[key] = cancel
	r.mu.Unlock()

	end := func() {
		r.mu.Lock()
		delete(r.directInflight, key)
		r.mu.Unlock()
		cancel()
	}
	return ctx, end
}
