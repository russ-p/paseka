package telegram

import (
	"fmt"
	"strings"

	"github.com/paseka/paseka/internal/colony"
	"github.com/paseka/paseka/internal/console"
	"github.com/paseka/paseka/internal/runtime"
	"github.com/paseka/paseka/internal/sessions"
)

// Snapshot is the Telegram /status projection for one colony.
type Snapshot struct {
	Slug           string
	SubjectPrefix  string
	ReactorAlive   bool
	LiveBeeCount   int
	PendingInvites int
}

// BuildSnapshot aggregates status from the same sources Queen Console uses.
func BuildSnapshot(ctx colony.Context, sup *runtime.Supervisor) (Snapshot, error) {
	if sup == nil {
		sup = runtime.DefaultSupervisor()
	}

	rt, err := console.GetRuntime(ctx, sup)
	if err != nil {
		return Snapshot{}, err
	}

	agents, err := console.GetAgents(ctx, sessions.NewManager())
	if err != nil {
		return Snapshot{}, err
	}

	invites, err := console.ListInvites(ctx, colony.InviteStatusPending)
	if err != nil {
		return Snapshot{}, err
	}

	return Snapshot{
		Slug:           ctx.Slug,
		SubjectPrefix:  rt.SubjectPrefix,
		ReactorAlive:   rt.Alive,
		LiveBeeCount:   agents.Count,
		PendingInvites: len(invites),
	}, nil
}

// FormatStatus renders the /status message body.
func FormatStatus(s Snapshot) string {
	reactor := "stopped"
	if s.ReactorAlive {
		reactor = "alive"
	}
	lines := []string{
		fmt.Sprintf("Paseka · %s", s.Slug),
		"",
		fmt.Sprintf("Reactor: %s", reactor),
	}
	if s.SubjectPrefix != "" {
		lines = append(lines, fmt.Sprintf("Subject: %s", s.SubjectPrefix))
	}
	lines = append(lines,
		fmt.Sprintf("Live bees: %d", s.LiveBeeCount),
		fmt.Sprintf("Pending invites: %d", s.PendingInvites),
	)
	return strings.Join(lines, "\n")
}

// HelpText is the /help response body.
const HelpText = `Paseka Human Gateway

/status — colony snapshot (Refresh button)
/help — this message`
