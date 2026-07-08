package console

import (
	"strings"
	"testing"
)

func TestAppJSUses24HourTimeFormat(t *testing.T) {
	data, err := staticFiles.ReadFile("static/app.js")
	if err != nil {
		t.Fatalf("read app.js: %v", err)
	}
	src := string(data)
	if !strings.Contains(src, "function formatTime(iso)") {
		t.Fatal("formatTime helper missing from app.js")
	}
	if !strings.Contains(src, "hour12: false") {
		t.Fatal("formatTime must use 24-hour clock (hour12: false)")
	}
}
