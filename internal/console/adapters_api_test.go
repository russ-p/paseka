package console_test

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"runtime"
	"sort"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"
	"unicode/utf8"

	"github.com/russ-p/paseka/internal/colony"
	"github.com/russ-p/paseka/internal/console"
	"github.com/russ-p/paseka/internal/sessions"
)

func newAdapterServer(t *testing.T, ctxColony colony.Context, probe console.AdapterProbeFunc) *console.Server {
	t.Helper()
	return newAdapterServerWithTimeout(t, ctxColony, probe, 0)
}

func newAdapterServerWithTimeout(t *testing.T, ctxColony colony.Context, probe console.AdapterProbeFunc, timeout time.Duration) *console.Server {
	t.Helper()
	return console.NewServer(console.Options{
		Addr:                "127.0.0.1:0",
		Colony:              ctxColony,
		Sessions:            sessions.NewManager(),
		ProbeAdapters:       probe,
		AdapterProbeTimeout: timeout,
	})
}

func getAdapterCLIs(t *testing.T, srv *console.Server, query string) (console.AdapterCLIsView, string) {
	t.Helper()
	rec := httptest.NewRecorder()
	srv.Handler().ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/api/system/adapters"+query, nil))
	if rec.Code != http.StatusOK {
		t.Fatalf("GET /api/system/adapters%s status = %d body=%s", query, rec.Code, rec.Body.String())
	}
	var view console.AdapterCLIsView
	if err := json.NewDecoder(rec.Body).Decode(&view); err != nil {
		t.Fatal(err)
	}
	return view, rec.Body.String()
}

func adapterRow(t *testing.T, view console.AdapterCLIsView, name string) console.AdapterCLIView {
	t.Helper()
	for _, row := range view.Adapters {
		if row.Name == name {
			return row
		}
	}
	t.Fatalf("adapter %q not in %+v", name, view.Adapters)
	return console.AdapterCLIView{}
}

// probeRecorder collects what the probes were asked to look for, so a target
// assertion is about the resolved binary rather than a name the handler could
// have invented. The four probes run concurrently, so it locks.
type probeRecorder struct {
	mu      sync.Mutex
	targets []console.AdapterProbeTarget
}

func (r *probeRecorder) seen() []console.AdapterProbeTarget {
	r.mu.Lock()
	defer r.mu.Unlock()
	return append([]console.AdapterProbeTarget(nil), r.targets...)
}

func (r *probeRecorder) seenNames() []string {
	names := make([]string, 0, len(r.targets))
	for _, target := range r.seen() {
		names = append(names, target.Name)
	}
	sort.Strings(names)
	return names
}

// foundProbe answers found for every adapter.
func foundProbe(recorder *probeRecorder) console.AdapterProbeFunc {
	return func(_ context.Context, target console.AdapterProbeTarget) console.AdapterProbeResult {
		recorder.mu.Lock()
		recorder.targets = append(recorder.targets, target)
		recorder.mu.Unlock()
		return console.AdapterProbeResult{
			Found:   true,
			Path:    "/usr/local/bin/" + target.Binary,
			Version: "1.2.3",
		}
	}
}

func TestAdapterCLIsAPIProbesTheCatalogTheColonyResolves(t *testing.T) {
	repo := initConsoleRepo(t)
	ctxColony := setupConsoleHome(t, repo)

	recorder := &probeRecorder{}
	view, _ := getAdapterCLIs(t, newAdapterServer(t, ctxColony, foundProbe(recorder)), "")

	// `script` is absent on purpose: its binary is whatever a bee's `command:`
	// says, so there is no catalog entry to probe and no row to render.
	if len(view.Adapters) != 4 {
		t.Fatalf("adapters = %+v, want the four CLI adapters", view.Adapters)
	}
	names := make([]string, 0, len(view.Adapters))
	for _, row := range view.Adapters {
		names = append(names, row.Name)
	}
	if strings.Join(names, ",") != "cursor,pi,claude,opencode" {
		t.Fatalf("adapter names = %v", names)
	}
	if view.ProbedAt == "" {
		t.Fatal("probedAt missing: a cached answer must say when it was measured")
	}
	// The binary comes from the colony context, and the cursor.yaml the test
	// home wrote resolves to `agent` — the same name the adapter launches.
	cursor := adapterRow(t, view, "cursor")
	if cursor.Binary != "agent" {
		t.Fatalf("cursor binary = %q, want the context's `agent`", cursor.Binary)
	}
	// One call per adapter, in any order: the four run concurrently, so the
	// assertion cannot be about a sequence.
	if got := strings.Join(recorder.seenNames(), ","); got != "claude,cursor,opencode,pi" {
		t.Fatalf("probed %q, want one call per catalogued adapter", got)
	}
}

func TestAdapterCLIsAPIProbesTheConfiguredBinaryRatherThanTheDefault(t *testing.T) {
	repo := initConsoleRepo(t)
	ctxColony := setupConsoleHome(t, repo)
	if err := os.WriteFile(configHomeFile(t, ctxColony.Slug, filepath.Join("adapters", "pi.yaml")),
		[]byte("binary: /opt/bin/pi-alt\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	// The context was resolved before pi.yaml existed, so re-resolve: the point
	// is that the probe follows the file, not a hardcoded name.
	resolved, err := colony.ResolveContext(repo)
	if err != nil {
		t.Fatal(err)
	}

	recorder := &probeRecorder{}
	view, _ := getAdapterCLIs(t, newAdapterServer(t, resolved, foundProbe(recorder)), "")

	if got := adapterRow(t, view, "pi").Binary; got != "/opt/bin/pi-alt" {
		t.Fatalf("pi binary = %q, want the machine-local override", got)
	}
	var asked string
	for _, target := range recorder.seen() {
		if target.Name == "pi" {
			asked = target.Binary
		}
	}
	if asked != "/opt/bin/pi-alt" {
		t.Fatalf("probe was handed %q, want the configured binary", asked)
	}
}

func TestAdapterCLIsAPICachesTheProbeUntilRefresh(t *testing.T) {
	repo := initConsoleRepo(t)
	ctxColony := setupConsoleHome(t, repo)

	var calls atomic.Int32
	probe := func(_ context.Context, target console.AdapterProbeTarget) console.AdapterProbeResult {
		run := calls.Add(1)
		return console.AdapterProbeResult{Found: true, Path: "/usr/local/bin/" + target.Binary, Version: fmt.Sprintf("run-%d", run)}
	}
	srv := newAdapterServer(t, ctxColony, probe)

	first, _ := getAdapterCLIs(t, srv, "")
	second, _ := getAdapterCLIs(t, srv, "")
	// Re-opening the accordion is a second read of one answer: four binaries are
	// exec'd once, and the second read is the cache.
	if calls.Load() != 4 {
		t.Fatalf("probe calls after two reads = %d, want 4", calls.Load())
	}
	if adapterRow(t, second, "claude").Version != adapterRow(t, first, "claude").Version {
		t.Fatalf("cached read returned %q, want the cached %q",
			adapterRow(t, second, "claude").Version, adapterRow(t, first, "claude").Version)
	}

	refreshed, _ := getAdapterCLIs(t, srv, "?refresh=1")
	if calls.Load() != 8 {
		t.Fatalf("probe calls after refresh = %d, want 8", calls.Load())
	}
	if got := adapterRow(t, refreshed, "claude").Version; got == adapterRow(t, first, "claude").Version {
		t.Fatalf("refresh served %q, want the new run's answer", got)
	}
	// The refresh replaced the cache rather than sitting beside it.
	afterRefresh, _ := getAdapterCLIs(t, srv, "")
	if calls.Load() != 8 {
		t.Fatalf("probe calls after a read following refresh = %d, want 8", calls.Load())
	}
	if adapterRow(t, afterRefresh, "claude").Version != adapterRow(t, refreshed, "claude").Version {
		t.Fatal("the read after a refresh was not served from the refreshed cache")
	}
}

func TestAdapterCLIsAPIReportsAMissingBinaryAsAStateNotAnError(t *testing.T) {
	repo := initConsoleRepo(t)
	ctxColony := setupConsoleHome(t, repo)

	probe := func(_ context.Context, target console.AdapterProbeTarget) console.AdapterProbeResult {
		if target.Name == "opencode" {
			return console.AdapterProbeResult{}
		}
		return console.AdapterProbeResult{Found: true, Path: "/usr/local/bin/" + target.Binary, Version: "1.2.3"}
	}
	view, _ := getAdapterCLIs(t, newAdapterServer(t, ctxColony, probe), "")

	// One adapter missing is a row with a verdict, and the other three are still
	// answered: the whole response does not fail over one uninstalled CLI.
	opencode := adapterRow(t, view, "opencode")
	if opencode.Found {
		t.Fatalf("opencode found = true, want false")
	}
	if opencode.Path != "" || opencode.Version != "" || opencode.Error != "" {
		t.Fatalf("opencode row = %+v, want no path, no version, and no error", opencode)
	}
	if !adapterRow(t, view, "claude").Found {
		t.Fatalf("claude row = %+v, want the other adapters still answered", adapterRow(t, view, "claude"))
	}
}

func TestAdapterCLIsAPIKeepsTheRowsASlowAdapterCouldNotAnswer(t *testing.T) {
	repo := initConsoleRepo(t)
	ctxColony := setupConsoleHome(t, repo)

	// A binary that never answers: the row says so and the read still returns.
	probe := func(probeCtx context.Context, target console.AdapterProbeTarget) console.AdapterProbeResult {
		if target.Name == "pi" {
			<-probeCtx.Done()
			return console.AdapterProbeResult{Found: true, Path: "/usr/local/bin/pi", Err: probeCtx.Err()}
		}
		return console.AdapterProbeResult{Found: true, Path: "/usr/local/bin/" + target.Binary, Version: "1.2.3"}
	}
	srv := newAdapterServerWithTimeout(t, ctxColony, probe, 50*time.Millisecond)

	started := time.Now()
	view, _ := getAdapterCLIs(t, srv, "")
	// Concurrent probes mean the ceiling is the slowest adapter, not their sum.
	if elapsed := time.Since(started); elapsed > 3*time.Second {
		t.Fatalf("read took %s, want the per-adapter bound to hold", elapsed)
	}

	pi := adapterRow(t, view, "pi")
	if pi.Error == "" {
		t.Fatalf("pi row = %+v, want the timeout reported on the row", pi)
	}
	if pi.Version != "" {
		t.Fatalf("pi version = %q, want none for a probe that timed out", pi.Version)
	}
	if !adapterRow(t, view, "cursor").Found {
		t.Fatalf("cursor row = %+v, want a timed-out adapter to leave the others alone", adapterRow(t, view, "cursor"))
	}
}

func TestAdapterCLIProbeReportsTheBinarysOwnVersionAndNoCredential(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("the fake CLI is a shell script")
	}
	repo := initConsoleRepo(t)
	ctxColony := setupConsoleHome(t, repo)
	slug := ctxColony.Slug

	// A stand-in for the real CLI that reports a version on one line and echoes
	// the credential variables back, so a probe that leaked them into the
	// environment would put their values in this response.
	binary := filepath.Join(t.TempDir(), "fake-agent")
	script := "#!/bin/sh\n" +
		"echo \"cursor-agent 1.2.3 (homegrown)\"\n" +
		"echo \"second line is not the version\"\n" +
		"echo \"leak: ${CURSOR_API_KEY:-} ${ANTHROPIC_API_KEY:-}\"\n"
	if err := os.WriteFile(binary, []byte(script), 0o755); err != nil {
		t.Fatal(err)
	}
	for _, name := range []string{"cursor", "claude"} {
		if err := os.WriteFile(configHomeFile(t, slug, filepath.Join("adapters", name+".yaml")),
			[]byte("binary: "+binary+"\n"), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	// An absolute path that cannot exist, so the not-found verdict comes from the
	// real exec path rather than from whatever this machine happens to have
	// installed.
	if err := os.WriteFile(configHomeFile(t, slug, filepath.Join("adapters", "opencode.yaml")),
		[]byte("binary: /nonexistent/paseka-opencode\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	t.Setenv("CURSOR_API_KEY", "cursor-secret-value")
	t.Setenv("ANTHROPIC_API_KEY", "anthropic-secret-value")

	resolved, err := colony.ResolveContext(repo)
	if err != nil {
		t.Fatal(err)
	}
	// No injected probe: this runs the real exec path end to end.
	view, body := getAdapterCLIs(t, newAdapterServer(t, resolved, nil), "")

	// Grepping the raw body rather than decoding a struct is the point: a
	// decoded test would pass whatever field a later change added.
	for _, secret := range []string{"cursor-secret-value", "anthropic-secret-value", "leak:"} {
		if strings.Contains(body, secret) {
			t.Fatalf("response carried the credential %q: %s", secret, body)
		}
	}

	cursor := adapterRow(t, view, "cursor")
	if !cursor.Found {
		t.Fatalf("cursor row = %+v, want the fake CLI detected", cursor)
	}
	if cursor.Path != binary {
		t.Fatalf("cursor path = %q, want the resolved binary %q", cursor.Path, binary)
	}
	if cursor.Version != "cursor-agent 1.2.3 (homegrown)" {
		t.Fatalf("cursor version = %q, want the first line only", cursor.Version)
	}
	// A binary the box does not have is still a row with a name and a verdict.
	opencode := adapterRow(t, view, "opencode")
	if opencode.Found || opencode.Path != "" || opencode.Error != "" {
		t.Fatalf("opencode row = %+v, want a plain not-found verdict", opencode)
	}
}

func TestAdapterCLIProbeCapsAVersionLineAtARuneBoundary(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("the fake CLI is a shell script")
	}
	repo := initConsoleRepo(t)
	ctxColony := setupConsoleHome(t, repo)

	// A binary that answers `--version` with a paragraph: the table cell must
	// hold a version string, not whatever the CLI felt like printing.
	binary := filepath.Join(t.TempDir(), "chatty-agent")
	script := "#!/bin/sh\nprintf '%s\\n' \"" + strings.Repeat("v", 400) + "\"\n"
	if err := os.WriteFile(binary, []byte(script), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(configHomeFile(t, ctxColony.Slug, filepath.Join("adapters", "cursor.yaml")),
		[]byte("binary: "+binary+"\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	resolved, err := colony.ResolveContext(repo)
	if err != nil {
		t.Fatal(err)
	}

	view, _ := getAdapterCLIs(t, newAdapterServer(t, resolved, nil), "")
	version := adapterRow(t, view, "cursor").Version
	if got := utf8.RuneCountInString(version); got != 121 {
		t.Fatalf("version is %d runes (%q), want the cap plus its ellipsis", got, version)
	}
	if !strings.HasSuffix(version, "…") {
		t.Fatalf("version = %q, want the truncation marked", version)
	}
}

func TestAdapterCLIProbeKeepsAWordyBinaryFromGrowingTheResponse(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("the fake CLI is a shell script")
	}
	repo := initConsoleRepo(t)
	ctxColony := setupConsoleHome(t, repo)

	// A binary that never stops talking. The version cap already keeps it out of
	// the cell; this is about the read itself, because `binary:` is operator
	// config and nothing stops it pointing at something enormous.
	binary := filepath.Join(t.TempDir(), "loud-agent")
	script := "#!/bin/sh\nhead -c 400000 /dev/zero | tr '\\0' 'w'\n"
	if err := os.WriteFile(binary, []byte(script), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(configHomeFile(t, ctxColony.Slug, filepath.Join("adapters", "cursor.yaml")),
		[]byte("binary: "+binary+"\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	resolved, err := colony.ResolveContext(repo)
	if err != nil {
		t.Fatal(err)
	}

	view, body := getAdapterCLIs(t, newAdapterServer(t, resolved, nil), "")

	if !adapterRow(t, view, "cursor").Found {
		t.Fatalf("cursor row = %+v, want the loud binary detected", adapterRow(t, view, "cursor"))
	}
	if len(body) > 4096 {
		t.Fatalf("response is %d bytes, want it bounded by the read cap: %.200q", len(body), body)
	}
}

func TestAdapterCLIsAPIRejectsAWrite(t *testing.T) {
	repo := initConsoleRepo(t)
	ctxColony := setupConsoleHome(t, repo)

	recorder := &probeRecorder{}
	rec := httptest.NewRecorder()
	newAdapterServer(t, ctxColony, foundProbe(recorder)).Handler().ServeHTTP(
		rec, httptest.NewRequest(http.MethodPost, "/api/system/adapters", nil))
	if rec.Code != http.StatusMethodNotAllowed {
		t.Fatalf("POST status = %d, want 405", rec.Code)
	}
	// The probe is a read of the box, so a refused write must not have run it.
	if seen := recorder.seen(); len(seen) != 0 {
		t.Fatalf("probe calls = %d, want none", len(seen))
	}
}
