package adapters

import (
	"strings"
	"testing"
)

func TestRedactArgs(t *testing.T) {
	args := []string{"-p", "--workspace", "/tmp/ws", "--api-key", "secret", "this is a very long prompt that should not appear in logs because it exceeds the threshold for redaction in adapter launch debug output"}
	got := RedactArgs(args)
	joined := strings.Join(got, " ")
	if strings.Contains(joined, "secret") {
		t.Fatalf("api key leaked: %q", joined)
	}
	if strings.Contains(joined, "very long prompt") {
		t.Fatalf("prompt leaked: %q", joined)
	}
	if !strings.Contains(joined, "<redacted>") || !strings.Contains(joined, "<prompt>") {
		t.Fatalf("expected redactions in %q", joined)
	}
}

func TestRedactArgsInlineAPIKey(t *testing.T) {
	got := RedactArgs([]string{"--api-key=secret"})
	if strings.Contains(strings.Join(got, " "), "secret") {
		t.Fatalf("inline api key leaked")
	}
}
