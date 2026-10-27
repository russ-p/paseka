package opencode

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/russ-p/paseka/internal/adapters"
)

func writeFakeBinary(t *testing.T, dir, body string) string {
	t.Helper()
	path := filepath.Join(dir, "opencode")
	if err := os.WriteFile(path, []byte(body), 0o755); err != nil {
		t.Fatal(err)
	}
	return path
}

func stubVersion(t *testing.T, out string, err error, calls *int) {
	t.Helper()
	prev := versionCommand
	versionCommand = func(context.Context, string) ([]byte, error) {
		if calls != nil {
			*calls = *calls + 1
		}
		return []byte(out), err
	}
	t.Cleanup(func() { versionCommand = prev })
}

func TestParseMajor(t *testing.T) {
	cases := []struct {
		raw  string
		want int
		ok   bool
	}{
		{"1.18.35", 1, true},
		{"2.0.27", 2, true},
		{"v2.0.27", 2, true},
		{"OpenCode v1.2.3 (build abc)", 1, true},
		{"opencode 2.1.0\n", 2, true},
		{"3.0.0", 0, false},
		{"", 0, false},
		{"not a version", 0, false},
		{"2", 0, false},
	}
	for _, tc := range cases {
		got, ok := parseMajor(tc.raw)
		if got != tc.want || ok != tc.ok {
			t.Fatalf("parseMajor(%q) = %d,%v want %d,%v", tc.raw, got, ok, tc.want, tc.ok)
		}
	}
}

func TestResolveAutoDetectsAndCaches(t *testing.T) {
	dir := t.TempDir()
	binary := writeFakeBinary(t, dir, "fake")
	calls := 0
	stubVersion(t, "2.0.27\n", nil, &calls)

	first, err := resolve(binary, adapters.RunParams{})
	if err != nil {
		t.Fatal(err)
	}
	if first.implementation != implementationV2 || first.major != 2 || first.binary != binary || first.pinned {
		t.Fatalf("first resolution = %+v", first)
	}
	if calls != 1 {
		t.Fatalf("version calls = %d, want 1", calls)
	}

	if _, err := resolve(binary, adapters.RunParams{}); err != nil {
		t.Fatal(err)
	}
	if calls != 1 {
		t.Fatalf("cached resolution ran version again: %d calls", calls)
	}

	// A replaced binary changes its size, so the cache key changes and the
	// version probe runs again.
	if err := os.WriteFile(binary, []byte("a different fake body"), 0o755); err != nil {
		t.Fatal(err)
	}
	stubVersion(t, "1.18.35\n", nil, &calls)
	got, err := resolve(binary, adapters.RunParams{})
	if err != nil {
		t.Fatal(err)
	}
	if got.implementation != implementationV1 {
		t.Fatalf("replaced binary implementation = %q", got.implementation)
	}
	if calls != 2 {
		t.Fatalf("version calls after replacement = %d, want 2", calls)
	}
}

func TestResolvePinSkipsDispatchDetection(t *testing.T) {
	dir := t.TempDir()
	binary := writeFakeBinary(t, dir, "fake")
	stubVersion(t, "", errors.New("does not answer"), nil)

	got, err := resolve(binary, adapters.RunParams{Options: map[string]string{"version": "1"}})
	if err != nil {
		t.Fatal(err)
	}
	if got.implementation != implementationV1 || !got.pinned || got.binary != binary {
		t.Fatalf("pinned resolution = %+v", got)
	}
}

func TestResolvePinMismatchFails(t *testing.T) {
	dir := t.TempDir()
	binary := writeFakeBinary(t, dir, "fake")
	stubVersion(t, "2.1.0", nil, nil)

	_, err := resolve(binary, adapters.RunParams{Options: map[string]string{"version": "1"}})
	if err == nil || !strings.Contains(err.Error(), "pinned") {
		t.Fatalf("pin mismatch error = %v", err)
	}
}

func TestResolveUnsupportedPin(t *testing.T) {
	_, err := resolve("opencode", adapters.RunParams{Options: map[string]string{"version": "3"}})
	if err == nil || !strings.Contains(err.Error(), "unsupported version pin") {
		t.Fatalf("unsupported pin error = %v", err)
	}
}

func TestResolveUnparsableVersionFailsClosed(t *testing.T) {
	dir := t.TempDir()
	binary := writeFakeBinary(t, dir, "fake")
	stubVersion(t, "definitely not opencode", nil, nil)

	_, err := resolve(binary, adapters.RunParams{})
	if err == nil {
		t.Fatal("expected an error")
	}
	for _, want := range []string{"cannot determine", "definitely not opencode", "pin it with version"} {
		if !strings.Contains(err.Error(), want) {
			t.Fatalf("error %q missing %q", err, want)
		}
	}
}

func TestResolvePinnedBinaryOverridesBase(t *testing.T) {
	dir := t.TempDir()
	v1 := writeFakeBinary(t, dir, "v1")
	stubVersion(t, "1.18.35", nil, nil)

	got, err := resolve("missing-base", adapters.RunParams{Options: map[string]string{
		"version":   "1",
		"v1_binary": v1,
	}})
	if err != nil {
		t.Fatal(err)
	}
	if got.binary != v1 || got.implementation != implementationV1 {
		t.Fatalf("resolution = %+v", got)
	}
}

func TestResolveMissingBinaryNamesInstallRoute(t *testing.T) {
	missing := filepath.Join(t.TempDir(), "no-opencode")
	for _, tc := range []struct {
		name    string
		options map[string]string
		want    string
	}{
		{"auto", nil, "opencode-ai@1"},
		{"v1", map[string]string{"version": "1"}, "opencode-ai@1"},
		{"v2", map[string]string{"version": "2"}, "opencode-ai@2"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			_, err := resolve(missing, adapters.RunParams{Options: tc.options})
			if err == nil || !strings.Contains(err.Error(), tc.want) {
				t.Fatalf("error = %v, want install route %q", err, tc.want)
			}
		})
	}
}

func TestResolveImplementationReportsDetectedMajor(t *testing.T) {
	dir := t.TempDir()
	binary := writeFakeBinary(t, dir, "fake")
	stubVersion(t, "1.18.35", nil, nil)

	got, err := New().ResolveImplementation(adapters.RunParams{Binary: binary})
	if err != nil {
		t.Fatal(err)
	}
	if got != implementationV1 {
		t.Fatalf("implementation = %q", got)
	}
}

func TestResolveDetectTimeoutIsBounded(t *testing.T) {
	dir := t.TempDir()
	binary := writeFakeBinary(t, dir, "fake")
	prev := versionCommand
	versionCommand = func(ctx context.Context, _ string) ([]byte, error) {
		deadline, ok := ctx.Deadline()
		if !ok {
			return nil, errors.New("no deadline")
		}
		if time.Until(deadline) > time.Second {
			return nil, errors.New("deadline too far away")
		}
		return []byte("2.0.0"), nil
	}
	t.Cleanup(func() { versionCommand = prev })

	if _, err := resolve(binary, adapters.RunParams{Options: map[string]string{
		"detect_timeout": "250ms",
	}}); err != nil {
		t.Fatal(err)
	}
}

func TestProxySessionUsageNeedsImplementationOrBinary(t *testing.T) {
	if got := resolveHandleImplementation(adapters.SessionHandle{}); got != "" {
		t.Fatalf("implementation = %q", got)
	}
	if got := resolveHandleImplementation(adapters.SessionHandle{Implementation: implementationV2}); got != implementationV2 {
		t.Fatalf("implementation = %q", got)
	}
}
