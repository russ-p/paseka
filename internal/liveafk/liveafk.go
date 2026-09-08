// Package liveafk scans .paseka/runs for live headless (AFK) adapter runs.
// Interactive sessions are never reported here; they are session records.
package liveafk

import (
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/russ-p/paseka/internal/colony"
	"github.com/russ-p/paseka/internal/protocol"
	"github.com/russ-p/paseka/internal/runs"
)

// Run is one live AFK adapter process.
type Run struct {
	Bee       string
	PID       int
	TraceID   string
	AgentID   string
	StartedAt time.Time
	RunDir    string
}

// ScanColony returns live AFK runs across all Flight Trails.
// Unreadable trails are skipped so one broken run dir cannot hide the rest.
func ScanColony(colonyRoot string) ([]Run, error) {
	runsRoot := filepath.Join(colonyRoot, ".paseka", "runs")
	traceDirs, err := os.ReadDir(runsRoot)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, err
	}

	var out []Run
	for _, entry := range traceDirs {
		if !entry.IsDir() {
			continue
		}
		found, err := scanTrace(colonyRoot, filepath.Join(runsRoot, entry.Name()), entry.Name())
		if err != nil {
			continue
		}
		out = append(out, found...)
	}
	return out, nil
}

// OnTrace returns live AFK runs for one Flight Trail.
func OnTrace(colonyRoot, traceID string) ([]Run, error) {
	traceID = strings.TrimSpace(traceID)
	if colonyRoot == "" || traceID == "" {
		return nil, nil
	}
	return scanTrace(colonyRoot, filepath.Join(colonyRoot, ".paseka", "runs", traceID), traceID)
}

func scanTrace(colonyRoot, tracePath, traceID string) ([]Run, error) {
	agentDirs, err := os.ReadDir(tracePath)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, err
	}
	var out []Run
	for _, entry := range agentDirs {
		if !entry.IsDir() || runs.IsReservedTraceSubdir(entry.Name()) {
			continue
		}
		agentID := entry.Name()
		d := runs.Dir{ColonyRoot: colonyRoot, TraceID: traceID, AgentID: agentID}
		if !fileExists(d.RequestPath()) {
			continue
		}
		if fileExists(d.SessionPath()) {
			continue
		}
		snap, err := d.ReadStatus()
		if err != nil {
			continue
		}
		if snap.State != protocol.StatusRunning || snap.PID <= 0 || !colony.ProcessAlive(snap.PID) {
			continue
		}
		req, err := d.ReadRequest()
		if err != nil {
			continue
		}
		startedAt := snap.StartedAt
		if startedAt.IsZero() {
			startedAt = req.CreatedAt
		}
		out = append(out, Run{
			Bee:       req.Bee,
			PID:       snap.PID,
			TraceID:   traceID,
			AgentID:   agentID,
			StartedAt: startedAt,
			RunDir:    d.Root(),
		})
	}
	return out, nil
}

func fileExists(path string) bool {
	_, err := os.Stat(path)
	return err == nil
}
