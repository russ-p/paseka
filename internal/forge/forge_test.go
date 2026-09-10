package forge

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestInvokeUpsertGetAndNotFound(t *testing.T) {
	stub := writeForgeStub(t)
	ctx := context.Background()
	opts := InvokeOpts{Command: []string{stub}, ColonyRoot: t.TempDir(), TraceID: "trace-1"}

	caps, err := Invoke(ctx, opts, Request{Op: OpCapabilities})
	if err != nil {
		t.Fatal(err)
	}
	if caps.ProtocolVersion != ProtocolVersion {
		t.Fatalf("caps = %+v", caps)
	}

	got, err := Invoke(ctx, opts, Request{Op: OpGet, Head: "feature/x"})
	if err != nil {
		t.Fatal(err)
	}
	if got.Found {
		t.Fatalf("get before upsert found = %+v", got)
	}

	up, err := Invoke(ctx, opts, Request{Op: OpUpsert, Head: "feature/x", Base: "main", Title: "T", Body: "B"})
	if err != nil {
		t.Fatal(err)
	}
	if !up.Found || up.URL == "" || up.State != StateOpen {
		t.Fatalf("upsert = %+v", up)
	}

	again, err := Invoke(ctx, opts, Request{Op: OpUpsert, Head: "feature/x", Title: "T2"})
	if err != nil {
		t.Fatal(err)
	}
	if again.Number != up.Number {
		t.Fatalf("second upsert number = %d want %d", again.Number, up.Number)
	}
}

func TestInvokeGarbageStdoutFails(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "bad.sh")
	if err := os.WriteFile(path, []byte("#!/bin/sh\necho not-json\n"), 0o755); err != nil {
		t.Fatal(err)
	}
	_, err := Invoke(context.Background(), InvokeOpts{Command: []string{path}}, Request{Op: OpGet, Head: "feature/x"})
	if err == nil {
		t.Fatal("expected garbage stdout error")
	}
}

func TestInvokeUpsertFoundFalseFails(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "false.sh")
	script := `#!/bin/sh
echo '{"protocolVersion":1,"found":false}'
`
	if err := os.WriteFile(path, []byte(script), 0o755); err != nil {
		t.Fatal(err)
	}
	_, err := Invoke(context.Background(), InvokeOpts{Command: []string{path}}, Request{Op: OpUpsert, Head: "feature/x"})
	if err == nil {
		t.Fatal("expected found:false upsert error")
	}
}

func TestCheckCommandMissing(t *testing.T) {
	err := CheckCommand([]string{filepath.Join(t.TempDir(), "nope")}, t.TempDir())
	if err == nil || !strings.Contains(err.Error(), "not executable") {
		t.Fatalf("err = %v", err)
	}
}

func writeForgeStub(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	path := filepath.Join(dir, "forge-stub.sh")
	state := filepath.Join(dir, "state.json")
	logf := filepath.Join(dir, "calls.log")
	script := `#!/bin/sh
op="$1"
input=$(cat)
echo "$op" >> "` + logf + `"
state="` + state + `"
case "$op" in
capabilities)
  echo '{"protocolVersion":1,"ops":["upsert","get"]}'
  ;;
get)
  if [ -f "$state" ]; then cat "$state"; else echo '{"protocolVersion":1,"found":false}'; fi
  ;;
upsert)
  if [ -f "$state" ]; then cat "$state"; else
    echo '{"protocolVersion":1,"found":true,"number":42,"url":"https://example.test/pr/42","head":"feature/x","base":"main","state":"open","draft":false}' > "$state"
    cat "$state"
  fi
  ;;
*)
  echo "unknown op" >&2
  exit 1
  ;;
esac
`
	if err := os.WriteFile(path, []byte(script), 0o755); err != nil {
		t.Fatal(err)
	}
	return path
}
