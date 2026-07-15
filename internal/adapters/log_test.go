package adapters

import (
	"strings"
	"testing"

	"github.com/paseka/paseka/internal/logging"
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

func TestAppendFailureOutputPrefersStderrAndResult(t *testing.T) {
	fields := appendFailureOutput(nil, AgentDoneOutput{
		Stdout:  `{"type":"error"}`,
		Stderr:  "authentication failed",
		Summary: "could not reach API",
	})
	got := fieldMap(fields)
	if got["stderr"] != "authentication failed" {
		t.Fatalf("stderr = %q", got["stderr"])
	}
	if got["result"] != "could not reach API" {
		t.Fatalf("result = %q", got["result"])
	}
	if _, ok := got["stdout"]; ok {
		t.Fatalf("stdout should be omitted when stderr/result present: %v", got)
	}
}

func TestAppendFailureOutputFallsBackToStdout(t *testing.T) {
	fields := appendFailureOutput(nil, AgentDoneOutput{Stdout: "missing binary"})
	got := fieldMap(fields)
	if got["stdout"] != "missing binary" {
		t.Fatalf("stdout = %q", got["stdout"])
	}
}

func TestTruncateForLog(t *testing.T) {
	long := strings.Repeat("x", logOutputMaxLen+10)
	got := truncateForLog(long)
	if len(got) <= logOutputMaxLen {
		t.Fatalf("expected truncation suffix, got len=%d", len(got))
	}
	if !strings.Contains(got, "bytes total") {
		t.Fatalf("expected total byte count in %q", got)
	}
}

func fieldMap(fields []logging.Field) map[string]string {
	out := make(map[string]string, len(fields))
	for _, f := range fields {
		out[f.Key] = f.Value
	}
	return out
}
