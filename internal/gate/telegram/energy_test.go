package telegram_test

import (
	"testing"

	tggate "github.com/paseka/paseka/internal/gate/telegram"
)

func TestParseEnergyCommandArgs(t *testing.T) {
	action, traceID, amount, err := tggate.ParseEnergyCommandArgs("")
	if err != nil || action != "show" || traceID != "" || amount != 0 {
		t.Fatalf("empty: action=%q trace=%q amount=%d err=%v", action, traceID, amount, err)
	}

	action, traceID, amount, err = tggate.ParseEnergyCommandArgs("trace-abc")
	if err != nil || action != "show" || traceID != "trace-abc" || amount != 0 {
		t.Fatalf("show: action=%q trace=%q amount=%d err=%v", action, traceID, amount, err)
	}

	action, traceID, amount, err = tggate.ParseEnergyCommandArgs("add trace-abc 12")
	if err != nil || action != "add" || traceID != "trace-abc" || amount != 12 {
		t.Fatalf("add: action=%q trace=%q amount=%d err=%v", action, traceID, amount, err)
	}

	_, _, _, err = tggate.ParseEnergyCommandArgs("add trace-abc")
	if err == nil {
		t.Fatal("expected usage error for incomplete add")
	}
}

func TestFormatEnergyShow(t *testing.T) {
	got := tggate.FormatEnergyShow("trace-1", 3, 12)
	want := "Trace: trace-1\nhoney: 3/12"
	if got != want {
		t.Fatalf("got %q want %q", got, want)
	}
}
