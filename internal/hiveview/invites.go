package hiveview

import (
	"sort"
	"time"

	"github.com/paseka/paseka/internal/colony"
	"github.com/paseka/paseka/internal/homestate"
	"github.com/paseka/paseka/internal/protocol"
)

// InviteView is a projection of one Human Gateway invite.
type InviteView struct {
	InviteID    string    `json:"inviteId"`
	TraceID     string    `json:"traceId"`
	Bee         string    `json:"bee"`
	Intent      string    `json:"intent,omitempty"`
	Task        string    `json:"task"`
	Status      string    `json:"status"`
	ArtifactRef string    `json:"artifactRef,omitempty"`
	SessionID   string    `json:"sessionId,omitempty"`
	CreatedAt   time.Time `json:"createdAt"`
	UpdatedAt   time.Time `json:"updatedAt"`
}

// ListInvites returns Human Gateway invites for the colony.
func ListInvites(ctx colony.Context, status protocol.InviteStatus) ([]InviteView, error) {
	if status == "" {
		status = protocol.InviteStatusPending
	}
	entries, err := homestate.ListInvites(ctx.Slug, status, "")
	if err != nil {
		return nil, err
	}
	out := make([]InviteView, 0, len(entries))
	for _, e := range entries {
		out = append(out, InviteView{
			InviteID:    e.InviteID,
			TraceID:     e.TraceID,
			Bee:         e.Bee,
			Intent:      e.Intent,
			Task:        e.Task,
			Status:      string(e.Status),
			ArtifactRef: e.ArtifactRef,
			SessionID:   e.SessionID,
			CreatedAt:   e.CreatedAt,
			UpdatedAt:   e.UpdatedAt,
		})
	}
	sort.Slice(out, func(i, j int) bool {
		return out[i].CreatedAt.After(out[j].CreatedAt)
	})
	return out, nil
}
