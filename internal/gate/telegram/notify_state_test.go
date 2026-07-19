package telegram_test

import (
	"os"
	"path/filepath"
	"testing"

	tggate "github.com/paseka/paseka/internal/gate/telegram"
	"gopkg.in/yaml.v3"
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

func TestNotifyConfigDefaults(t *testing.T) {
	var cfg tggate.NotifyConfig
	if !cfg.Mode(tggate.NotifyCategoryInvites).Enabled() {
		t.Fatal("invites should default to sound")
	}
	if cfg.Mode(tggate.NotifyCategoryInvites).Silent() {
		t.Fatal("invites default should not be silent")
	}
	if !cfg.Mode(tggate.NotifyCategoryBlocked).Enabled() {
		t.Fatal("blocked should default to sound")
	}
	if !cfg.Mode(tggate.NotifyCategoryReviewRequired).Enabled() {
		t.Fatal("review_required should default to sound")
	}
	if !cfg.Mode(tggate.NotifyCategoryReviewFinal).Enabled() {
		t.Fatal("review_final should default to sound")
	}
	if cfg.Mode(tggate.NotifyCategoryCommitGate).Enabled() {
		t.Fatal("commit_gate should default to off")
	}
	if !cfg.Mode(tggate.NotifyCategoryCompleted).Enabled() {
		t.Fatal("completed should default to silent (enabled)")
	}
	if !cfg.Mode(tggate.NotifyCategoryCompleted).Silent() {
		t.Fatal("completed should default to silent mode")
	}
}

func TestNotifyModeUnmarshalYAML(t *testing.T) {
	tests := []struct {
		in   string
		want tggate.NotifyMode
	}{
		{"true", tggate.NotifySound},
		{"false", tggate.NotifyOff},
		{"sound", tggate.NotifySound},
		{"silent", tggate.NotifySilent},
		{"off", tggate.NotifyOff},
		{"SOUND", tggate.NotifySound},
	}
	for _, tc := range tests {
		var m tggate.NotifyMode
		if err := yaml.Unmarshal([]byte(tc.in), &m); err != nil {
			t.Fatalf("unmarshal %q: %v", tc.in, err)
		}
		if m != tc.want {
			t.Fatalf("unmarshal %q = %q, want %q", tc.in, m, tc.want)
		}
	}
}

func TestLoadNotifyModesFromYAML(t *testing.T) {
	writeTelegramYAML(t, "tg-notify", `enabled: true
bot_token: "tok"
allow_from: [1]
chat_ids: [-1]
notify:
  invites: silent
  blocked: sound
  failed: off
  review_required: silent
  review_final: sound
  commit_gate: off
  completed: silent
`)
	cfg, err := tggate.Load("tg-notify")
	if err != nil {
		t.Fatal(err)
	}
	if !cfg.Notify.Mode(tggate.NotifyCategoryInvites).Silent() {
		t.Fatal("invites should be silent")
	}
	if cfg.Notify.Mode(tggate.NotifyCategoryFailed).Enabled() {
		t.Fatal("failed should be off")
	}
	if cfg.Notify.Mode(tggate.NotifyCategoryCommitGate).Enabled() {
		t.Fatal("commit_gate should be off")
	}
}

func TestLoadLegacyWaitingReviewMapsToReviewCategories(t *testing.T) {
	writeTelegramYAML(t, "tg-legacy", `enabled: true
bot_token: "tok"
allow_from: [1]
chat_ids: [-1]
notify:
  waiting_review: false
`)
	cfg, err := tggate.Load("tg-legacy")
	if err != nil {
		t.Fatal(err)
	}
	if cfg.Notify.Mode(tggate.NotifyCategoryReviewRequired).Enabled() {
		t.Fatal("legacy waiting_review:false should disable review_required")
	}
	if cfg.Notify.Mode(tggate.NotifyCategoryReviewFinal).Enabled() {
		t.Fatal("legacy waiting_review:false should disable review_final")
	}
	if cfg.Notify.Mode(tggate.NotifyCategoryCommitGate).Enabled() {
		t.Fatal("legacy waiting_review should not affect commit_gate")
	}
}

func TestLoadExplicitReviewKeysOverrideLegacyWaitingReview(t *testing.T) {
	writeTelegramYAML(t, "tg-override", `enabled: true
bot_token: "tok"
allow_from: [1]
chat_ids: [-1]
notify:
  waiting_review: false
  review_required: sound
`)
	cfg, err := tggate.Load("tg-override")
	if err != nil {
		t.Fatal(err)
	}
	if !cfg.Notify.Mode(tggate.NotifyCategoryReviewRequired).Enabled() {
		t.Fatal("explicit review_required should override legacy waiting_review")
	}
	if cfg.Notify.Mode(tggate.NotifyCategoryReviewFinal).Enabled() {
		t.Fatal("review_final should still follow legacy waiting_review:false")
	}
}
