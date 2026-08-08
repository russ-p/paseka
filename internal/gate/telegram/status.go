package telegram

import (
	"fmt"
	"strings"

	"github.com/paseka/paseka/internal/colony"
	"github.com/paseka/paseka/internal/cues"
	"github.com/paseka/paseka/internal/hiveview"
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

	rt, err := hiveview.GetRuntime(ctx, sup)
	if err != nil {
		return Snapshot{}, err
	}

	agents, err := hiveview.GetAgents(ctx, sessions.NewManager())
	if err != nil {
		return Snapshot{}, err
	}

	invites, err := hiveview.ListInvites(ctx, colony.InviteStatusPending)
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

// FormatWelcome renders the gate startup greeting with a compact status line.
func FormatWelcome(s Snapshot) string {
	reactor := "stopped"
	if s.ReactorAlive {
		reactor = "alive"
	}
	status := fmt.Sprintf("Reactor: %s · bees: %d", reactor, s.LiveBeeCount)
	if s.PendingInvites > 0 {
		status += fmt.Sprintf(" · invites: %d", s.PendingInvites)
	}
	return strings.Join([]string{
		fmt.Sprintf("Welcome to Paseka · %s", s.Slug),
		"",
		status,
		"",
		"Use the buttons below or /help for commands.",
	}, "\n")
}

// FormatWelcomeFallback renders a greeting when live status is unavailable.
func FormatWelcomeFallback(slug string) string {
	return strings.Join([]string{
		fmt.Sprintf("Welcome to Paseka · %s", slug),
		"",
		"Use the buttons below or /help for commands.",
	}, "\n")
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

// FormatHelpText renders the /help response body including configured custom commands.
func FormatHelpText(commands CommandsConfig, colonyRoot string) string {
	lines := []string{
		"Paseka Human Gateway",
		"",
		"/status — colony snapshot (Refresh button)",
		"/energy <traceId> — honey reserve (remaining/budget)",
		"/energy add <traceId> <n> — top up honey",
		"/task <text> — inject task (preview + Confirm)",
		"/invites — pending session invites",
		"/traces — recent colony traces",
	}
	names := make([]string, 0, len(commands.Custom))
	for name := range commands.Custom {
		names = append(names, name)
	}
	sortCustomCommandNames(names)
	for _, name := range names {
		cmd := commands.Custom[name]
		desc := CustomCommandHelpDescription(cmd, colonyRoot)
		lines = append(lines, fmt.Sprintf("/%s <text> — %s", name, desc))
	}
	lines = append(lines, "/help — this message")
	return strings.Join(lines, "\n")
}

// CustomCommandHelpDescription prefers gate description, then cue description, then a generic fallback.
func CustomCommandHelpDescription(cmd CustomCommandConfig, colonyRoot string) string {
	if desc := strings.TrimSpace(cmd.Description); desc != "" {
		return desc
	}
	if cmd.UsesCue() {
		if colonyRoot != "" {
			if cue, err := cues.Load(colonyRoot, cmd.CueID()); err == nil {
				if desc := strings.TrimSpace(cue.Description); desc != "" {
					return desc
				}
			}
		}
		return fmt.Sprintf("run cue %s", cmd.CueID())
	}
	return fmt.Sprintf("publish SIGNAL/%s", strings.TrimSpace(cmd.Kind))
}

func sortCustomCommandNames(names []string) {
	for i := 0; i < len(names); i++ {
		for j := i + 1; j < len(names); j++ {
			if names[j] < names[i] {
				names[i], names[j] = names[j], names[i]
			}
		}
	}
}
