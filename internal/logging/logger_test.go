package logging

import (
	"bytes"
	"os"
	"strings"
	"testing"
)

func TestParseLevel(t *testing.T) {
	tests := []struct {
		in   string
		want Level
		err  bool
	}{
		{"info", LevelInfo, false},
		{"DEBUG", LevelDebug, false},
		{"warn", LevelWarn, false},
		{"error", LevelError, false},
		{"nope", LevelInfo, true},
	}
	for _, tc := range tests {
		got, err := ParseLevel(tc.in)
		if tc.err && err == nil {
			t.Fatalf("ParseLevel(%q) expected error", tc.in)
		}
		if !tc.err && err != nil {
			t.Fatalf("ParseLevel(%q): %v", tc.in, err)
		}
		if !tc.err && got != tc.want {
			t.Fatalf("ParseLevel(%q) = %v, want %v", tc.in, got, tc.want)
		}
	}
}

func TestLevelGating(t *testing.T) {
	var buf bytes.Buffer
	l := New(Options{Level: LevelInfo, Writer: &buf, NoColor: true})

	l.Debug("hidden")
	l.Info("visible")
	if strings.Contains(buf.String(), "hidden") {
		t.Fatalf("debug should be gated at info level")
	}
	if !strings.Contains(buf.String(), "visible") {
		t.Fatalf("info should be visible: %q", buf.String())
	}
}

func TestComponentAndFields(t *testing.T) {
	var buf bytes.Buffer
	l := New(Options{Level: LevelInfo, Writer: &buf, NoColor: true}).WithComponent("runtime")
	l.Info("listening", F("colony", "demo"), F("subject", "paseka.events.>"))

	out := buf.String()
	for _, want := range []string{"INFO", "runtime", "listening", "colony=demo", "subject=paseka.events.>"} {
		if !strings.Contains(out, want) {
			t.Fatalf("output %q missing %q", out, want)
		}
	}
}

func TestNoColorDisablesANSI(t *testing.T) {
	var buf bytes.Buffer
	l := New(Options{Level: LevelInfo, Writer: &buf, NoColor: true})
	l.Error("boom")
	if strings.Contains(buf.String(), "\033[") {
		t.Fatalf("expected plain output, got ANSI: %q", buf.String())
	}
}

func TestNO_COLOR(t *testing.T) {
	t.Setenv("NO_COLOR", "1")
	var buf bytes.Buffer
	force := true
	l := New(Options{Level: LevelInfo, Writer: &buf, Color: &force})
	l.Warn("check")
	if strings.Contains(buf.String(), "\033[") {
		t.Fatalf("NO_COLOR should disable ANSI even when Color=true")
	}
}

func TestDefaultLogger(t *testing.T) {
	var buf bytes.Buffer
	SetDefault(New(Options{Level: LevelDebug, Writer: &buf, NoColor: true}))
	defer SetDefault(New(Options{Level: LevelInfo, Writer: os.Stderr}))

	Component("bus").Debug("event", F("trace", "t1"))
	if !strings.Contains(buf.String(), "bus") || !strings.Contains(buf.String(), "trace=t1") {
		t.Fatalf("unexpected default logger output: %q", buf.String())
	}
}
