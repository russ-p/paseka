package invites

import (
	"os"
	"os/exec"
	"testing"

	"github.com/paseka/paseka/internal/colony"
	"github.com/paseka/paseka/internal/protocol"
)

func TestRecordValidatesPayload(t *testing.T) {
	repo := initTestRepo(t)
	res, err := colony.Init(colony.InitOptions{StartDir: repo})
	if err != nil {
		t.Fatal(err)
	}
	svc := &Service{Colony: colony.Context{Slug: res.Slug, ColonyRoot: res.ColonyRoot}}
	_, err = svc.Record(RecordInput{
		TraceID: "trace-1",
		Payload: protocol.SessionInvitePayload{
			Kind:   protocol.SignalSessionInvite,
			Bee:    "drone",
			Task:   "Grill feature",
			Status: protocol.InviteStatusPending,
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	invites, err := colony.ListInvites(res.Slug, colony.InviteStatusPending, "")
	if err != nil {
		t.Fatal(err)
	}
	if len(invites) != 1 {
		t.Fatalf("invites = %#v", invites)
	}
	if invites[0].TraceID != "trace-1" || invites[0].Bee != "drone" {
		t.Fatalf("invite = %#v", invites[0])
	}
	if invites[0].InviteID == "" {
		t.Fatal("expected generated inviteId")
	}
}

func TestRejectPendingInviteRequiresBus(t *testing.T) {
	repo := initTestRepo(t)
	res, err := colony.Init(colony.InitOptions{StartDir: repo})
	if err != nil {
		t.Fatal(err)
	}
	entry := colony.InviteEntry{
		InviteID: "inv-test",
		TraceID:  "trace-1",
		Bee:      "drone",
		Task:     "Grill",
		Status:   colony.InviteStatusPending,
	}
	if err := colony.UpsertInvite(res.Slug, entry); err != nil {
		t.Fatal(err)
	}
	svc := &Service{Colony: colony.Context{Slug: res.Slug, ColonyRoot: res.ColonyRoot}}
	_, err = svc.Reject(t.Context(), "inv-test", false)
	if err == nil {
		t.Fatal("expected error without bus")
	}
}

func initTestRepo(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	runGit(t, dir, "init")
	runGit(t, dir, "config", "user.email", "test@test.com")
	runGit(t, dir, "config", "user.name", "test")
	return dir
}

func runGit(t *testing.T, dir string, args ...string) {
	t.Helper()
	cmd := exec.Command("git", args...)
	cmd.Dir = dir
	cmd.Env = append(os.Environ(), "GIT_CONFIG_GLOBAL=/dev/null", "GIT_CONFIG_SYSTEM=/dev/null")
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("git %v in %s: %v\n%s", args, dir, err, out)
	}
}
