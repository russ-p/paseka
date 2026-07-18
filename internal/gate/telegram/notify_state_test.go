package telegram_test

import (
	"os"
	"path/filepath"
	"testing"

	tggate "github.com/paseka/paseka/internal/gate/telegram"
)

func TestNotifyStateDedup(t *testing.T) {
	home := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", home)
	slug := "dedup-test"

	st, err := tggate.LoadNotifyState(slug)
	if err != nil {
		t.Fatal(err)
	}
	key := "invite:inv-1:pending"
	if !st.ShouldNotify(key) {
		t.Fatal("expected first sight to notify")
	}
	if err := st.MarkNotified(key); err != nil {
		t.Fatal(err)
	}
	if st.ShouldNotify(key) {
		t.Fatal("expected dedup after mark")
	}

	st2, err := tggate.LoadNotifyState(slug)
	if err != nil {
		t.Fatal(err)
	}
	if st2.ShouldNotify(key) {
		t.Fatal("expected persisted dedup after reload")
	}
	path, err := tggate.NotifyStatePath(slug)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(path); err != nil {
		t.Fatalf("state file missing: %v", err)
	}
	if filepath.Base(path) != "telegram-notify-state.json" {
		t.Fatalf("path = %s", path)
	}
}

func TestNotifyConfigInvitesDefaultEnabled(t *testing.T) {
	var cfg tggate.NotifyConfig
	if !cfg.InvitesEnabled() {
		t.Fatal("invites notify should default to enabled")
	}
	disabled := false
	cfg.Invites = &disabled
	if cfg.InvitesEnabled() {
		t.Fatal("expected explicit false to disable invites notify")
	}
}

func TestNotifyConfigBlockedDefaultEnabled(t *testing.T) {
	var cfg tggate.NotifyConfig
	if !cfg.BlockedEnabled() {
		t.Fatal("blocked notify should default to enabled")
	}
}
