package runtime

import (
	"encoding/json"
	"os"
	"os/exec"
	"testing"
	"time"

	"github.com/paseka/paseka/internal/colony"
	"github.com/paseka/paseka/internal/protocol"
)

func TestHandleInviteProjection(t *testing.T) {
	repo := initInviteTestRepo(t)
	res, err := colony.Init(colony.InitOptions{StartDir: repo})
	if err != nil {
		t.Fatal(err)
	}
	r := &Reactor{colony: colony.Context{Slug: res.Slug, ColonyRoot: res.ColonyRoot}}
	payload, _ := json.Marshal(protocol.SessionInvitePayload{
		Kind:     protocol.SignalSessionInvite,
		InviteID: "inv-proj",
		Bee:      "drone",
		Intent:   "grilling",
		Task:     "Grill feature",
		Status:   protocol.InviteStatusPending,
	})
	ev := protocol.Event{
		TraceID:   "trace-proj",
		Type:      protocol.EventSignal,
		CreatedAt: time.Now().UTC(),
		Payload:   payload,
	}
	if err := r.handleInviteProjection(ev); err != nil {
		t.Fatal(err)
	}
	invites, err := colony.ListInvites(res.Slug, colony.InviteStatusPending, "trace-proj")
	if err != nil {
		t.Fatal(err)
	}
	if len(invites) != 1 || invites[0].InviteID != "inv-proj" {
		t.Fatalf("invites = %#v", invites)
	}
}

func initInviteTestRepo(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	cmd := exec.Command("git", "init")
	cmd.Dir = dir
	cmd.Env = append(os.Environ(), "GIT_CONFIG_GLOBAL=/dev/null", "GIT_CONFIG_SYSTEM=/dev/null")
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("git init: %v\n%s", err, out)
	}
	for _, args := range [][]string{
		{"config", "user.email", "test@test.com"},
		{"config", "user.name", "test"},
	} {
		cmd := exec.Command("git", args...)
		cmd.Dir = dir
		cmd.Env = append(os.Environ(), "GIT_CONFIG_GLOBAL=/dev/null", "GIT_CONFIG_SYSTEM=/dev/null")
		if out, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("git %v: %v\n%s", args, err, out)
		}
	}
	return dir
}
