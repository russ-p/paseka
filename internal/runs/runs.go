package runs

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"time"
)

const (
	ResultFileName = "result.txt"
	PromptFileName = "prompt.txt"
	MetaFileName   = "meta.json"
	StatusFileName = "status.json"
)

// Dir holds paths for one spawned agent run.
type Dir struct {
	ColonyRoot string
	TraceID    string
	AgentID    string
}

// Root returns .paseka/runs/<traceId>/<agentId>/ under colony root.
func (d Dir) Root() string {
	return filepath.Join(d.ColonyRoot, ".paseka", "runs", d.TraceID, d.AgentID)
}

func (d Dir) ResultPath() string  { return filepath.Join(d.Root(), ResultFileName) }
func (d Dir) PromptPath() string  { return filepath.Join(d.Root(), PromptFileName) }
func (d Dir) MetaPath() string    { return filepath.Join(d.Root(), MetaFileName) }
func (d Dir) StatusPath() string  { return filepath.Join(d.Root(), StatusFileName) }

// Meta is written by the runtime before launching an agent.
type Meta struct {
	TraceID   string    `json:"traceId"`
	AgentID   string    `json:"agentId"`
	Bee       string    `json:"bee"`
	Adapter   string    `json:"adapter"`
	Workspace string    `json:"workspace"`
	StartedAt time.Time `json:"startedAt"`
}

// Status is written by the runtime after the agent exits.
type Status struct {
	State      string    `json:"state"` // completed | failed
	ExitCode   int       `json:"exitCode"`
	FinishedAt time.Time `json:"finishedAt"`
	Error      string    `json:"error,omitempty"`
}

// Prepare creates the run directory and removes a stale result from a previous attempt.
func (d Dir) Prepare() error {
	if d.ColonyRoot == "" || d.TraceID == "" || d.AgentID == "" {
		return fmt.Errorf("runs: colony root, traceId, and agentId are required")
	}
	root := d.Root()
	if err := os.MkdirAll(root, 0o755); err != nil {
		return fmt.Errorf("runs: mkdir %s: %w", root, err)
	}
	_ = os.Remove(d.ResultPath())
	return nil
}

func (d Dir) WritePrompt(prompt string) error {
	return os.WriteFile(d.PromptPath(), []byte(prompt), 0o644)
}

func (d Dir) WriteMeta(meta Meta) error {
	data, err := json.MarshalIndent(meta, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(d.MetaPath(), data, 0o644)
}

func (d Dir) WriteStatus(status Status) error {
	data, err := json.MarshalIndent(status, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(d.StatusPath(), data, 0o644)
}

func (d Dir) ReadResult() (string, error) {
	data, err := os.ReadFile(d.ResultPath())
	if err != nil {
		return "", err
	}
	return string(data), nil
}
