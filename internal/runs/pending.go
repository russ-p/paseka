package runs

import (
	"bufio"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/paseka/paseka/internal/protocol"
)

const PendingFileName = "pending.ndjson"

func (d Dir) PendingPath() string { return filepath.Join(d.Root(), PendingFileName) }

// PendingSummary describes queued events for a run.
type PendingSummary struct {
	Count int
	Kinds []string
}

// AppendPending appends one validated event envelope to the per-run pending queue.
func (d Dir) AppendPending(ev protocol.Event) error {
	if d.ColonyRoot == "" || d.TraceID == "" || d.AgentID == "" {
		return fmt.Errorf("runs: colony root, traceId, and agentId are required")
	}
	if _, err := os.Stat(d.Root()); err != nil {
		return fmt.Errorf("runs: run directory not found: %w", err)
	}
	if ev.ProtocolVersion == "" {
		ev.ProtocolVersion = protocol.Version
	}
	data, err := json.Marshal(ev)
	if err != nil {
		return err
	}
	f, err := os.OpenFile(d.PendingPath(), os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0o644)
	if err != nil {
		return err
	}
	defer f.Close()
	_, err = f.Write(append(data, '\n'))
	return err
}

// ReadPending returns queued events in FIFO order.
func (d Dir) ReadPending() ([]protocol.Event, error) {
	f, err := os.Open(d.PendingPath())
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, err
	}
	defer f.Close()

	var events []protocol.Event
	scanner := bufio.NewScanner(f)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" {
			continue
		}
		var ev protocol.Event
		if err := json.Unmarshal([]byte(line), &ev); err != nil {
			return nil, fmt.Errorf("runs: parse pending event: %w", err)
		}
		events = append(events, ev)
	}
	if err := scanner.Err(); err != nil {
		return nil, err
	}
	return events, nil
}

// WritePending replaces the pending queue with the given events.
func (d Dir) WritePending(events []protocol.Event) error {
	path := d.PendingPath()
	if len(events) == 0 {
		if err := os.Remove(path); err != nil && !os.IsNotExist(err) {
			return err
		}
		return nil
	}
	f, err := os.OpenFile(path, os.O_CREATE|os.O_TRUNC|os.O_WRONLY, 0o644)
	if err != nil {
		return err
	}
	defer f.Close()
	for _, ev := range events {
		if ev.ProtocolVersion == "" {
			ev.ProtocolVersion = protocol.Version
		}
		data, err := json.Marshal(ev)
		if err != nil {
			return err
		}
		if _, err := f.Write(append(data, '\n')); err != nil {
			return err
		}
	}
	return nil
}

// ClearPending removes the pending queue file.
func (d Dir) ClearPending() error {
	if err := os.Remove(d.PendingPath()); err != nil && !os.IsNotExist(err) {
		return err
	}
	return nil
}

// PendingSummary returns pending count and payload kinds in FIFO order.
func (d Dir) PendingSummary() (PendingSummary, error) {
	events, err := d.ReadPending()
	if err != nil {
		return PendingSummary{}, err
	}
	kinds := make([]string, 0, len(events))
	for _, ev := range events {
		kinds = append(kinds, protocol.PayloadKind(ev.Payload))
	}
	return PendingSummary{Count: len(events), Kinds: kinds}, nil
}
