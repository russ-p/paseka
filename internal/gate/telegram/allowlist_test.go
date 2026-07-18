package telegram_test

import (
	"testing"

	tggate "github.com/paseka/paseka/internal/gate/telegram"
)

func TestAllowedRequiresUserAndChat(t *testing.T) {
	cfg := tggate.Config{
		AllowFrom: []int64{42},
		ChatIDs:   []int64{-100},
	}
	if tggate.Allowed(cfg, 42, -100) != true {
		t.Fatal("expected allowed")
	}
	if tggate.Allowed(cfg, 99, -100) {
		t.Fatal("unexpected allow for user")
	}
	if tggate.Allowed(cfg, 42, -200) {
		t.Fatal("unexpected allow for chat")
	}
	if tggate.Allowed(cfg, 0, -100) {
		t.Fatal("unexpected allow for zero user")
	}
}
