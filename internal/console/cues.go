package console

import (
	"context"
	"fmt"

	"github.com/paseka/paseka/internal/colony"
	"github.com/paseka/paseka/internal/cues"
	"github.com/paseka/paseka/internal/tasks"
)

// CueView is one cue row for GET /api/cues.
type CueView struct {
	ID          string `json:"id"`
	Description string `json:"description,omitempty"`
}

// RunCueRequest is the JSON body for POST /api/cues/:id/run.
type RunCueRequest struct {
	Text    string            `json:"text"`
	TraceID string            `json:"traceId"`
	Vars    map[string]string `json:"vars"`
}

// RunCueResponse is returned after a successful cue run from Queen Console.
type RunCueResponse struct {
	TraceID   string `json:"traceId"`
	TaskID    string `json:"taskId,omitempty"`
	EventType string `json:"eventType,omitempty"`
	Kind      string `json:"kind,omitempty"`
}

// ListCues returns colony cues sorted by id (id + optional description only).
func ListCues(colonyRoot string) ([]CueView, error) {
	items, err := cues.List(colonyRoot)
	if err != nil {
		return nil, err
	}
	out := make([]CueView, 0, len(items))
	for _, item := range items {
		out = append(out, CueView{
			ID:          item.ID,
			Description: item.Description,
		})
	}
	return out, nil
}

// RunCue loads a cue, seeds optional honey, and publishes from Queen Console.
func RunCue(ctx context.Context, colonyCtx colony.Context, cueID string, req RunCueRequest) (RunCueResponse, error) {
	session, err := tasks.OpenLedger(colonyCtx)
	if err != nil {
		return RunCueResponse{}, err
	}
	defer session.Close()
	if session.Client == nil {
		return RunCueResponse{}, fmt.Errorf("nats url not configured (cue run requires NATS)")
	}

	res, err := cues.Run(ctx, session.Client, session.Ledger, cues.RunInput{
		ColonyRoot: colonyCtx.ColonyRoot,
		CueID:      cueID,
		Text:       req.Text,
		TraceID:    req.TraceID,
		Vars:       req.Vars,
		Source:     "console",
		AgentID:    "console",
	})
	if err != nil {
		return RunCueResponse{}, err
	}

	return RunCueResponse{
		TraceID:   res.TraceID,
		TaskID:    res.TaskID,
		EventType: res.EventType,
		Kind:      res.Kind,
	}, nil
}
