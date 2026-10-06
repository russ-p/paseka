package console

import (
	"context"
	"net/http"
	"os"
	"os/exec"
	"strings"
	"sync"
	"time"
	"unicode/utf8"

	"github.com/russ-p/paseka/internal/colony"
)

const (
	// adapterProbeTimeout bounds one `--version`. A CLI that hangs on start-up
	// must not hold the block open forever, and the four probes run
	// concurrently, so this is a ceiling on the slowest adapter rather than on
	// the read.
	adapterProbeTimeout = 5 * time.Second
	// adapterVersionRunes caps what a CLI printed reaches the JSON. A version
	// banner is one line; a binary that prints its help instead must not be
	// able to ship a page of text into a table cell.
	adapterVersionRunes = 120
	// adapterOutputRunes caps what a probe reads out of a CLI at all. The version
	// cap is what reaches a cell; this is what stops a `binary:` pointing at
	// something that prints without end from growing the process's memory while
	// it answers.
	adapterOutputRunes = 4096
)

// AdapterCLIView is one row of the Agent CLIs block: a CLI adapter the colony
// can launch, and what this box can actually run.
//
// A missing binary is a state, not an error — `Found` is false, the row still
// carries its name, and `Error` stays empty. `Error` is for the narrower case
// of a binary that was found and then refused to report a version.
type AdapterCLIView struct {
	Name string `json:"name"`
	// Binary is the effective binary the colony resolved for this adapter, so a
	// `binary:` in ~/.config/paseka/<slug>/adapters/<name>.yaml is what the row
	// is about rather than the default.
	Binary  string `json:"binary"`
	Found   bool   `json:"found"`
	Path    string `json:"path,omitempty"`
	Version string `json:"version,omitempty"`
	Error   string `json:"error,omitempty"`
}

// AdapterCLIsView is GET /api/system/adapters.
type AdapterCLIsView struct {
	Adapters []AdapterCLIView `json:"adapters"`
	// ProbedAt is when the snapshot was measured. The block is cached until an
	// operator asks for a re-probe, so without it a served-from-cache answer is
	// indistinguishable from a live one.
	ProbedAt string `json:"probedAt"`
}

// AdapterProbeTarget is what a probe is handed: the catalog name and the
// effective binary resolved for it.
type AdapterProbeTarget struct {
	Name   string
	Binary string
}

// AdapterProbeResult is one probe's verdict. A binary that was not detected
// answers zero values and no error — presence is a fact, not a failure.
type AdapterProbeResult struct {
	Found   bool
	Path    string
	Version string
	Err     error
}

// AdapterProbeFunc detects one adapter's binary. Nil uses the exec probe; a
// caller can inject one so the cache and refresh paths are testable without
// running a real CLI.
type AdapterProbeFunc func(ctx context.Context, target AdapterProbeTarget) AdapterProbeResult

// adapterCLINames is the catalog this block probes: the four adapters whose
// binary the colony resolves. `script` is deliberately absent — its binary is
// whatever each bee's `command:` says, so there is no single entry to look for.
var adapterCLINames = []string{"cursor", "pi", "claude", "opencode"}

// adapterProber runs the four probes concurrently and caches what they said.
//
// The cache is process-side and has no TTL: what it holds is the answer to "is
// this CLI installed", which changes when a human installs something, and a
// block that nobody is looking at should not be re-asking. `Refresh` drops it.
type adapterProber struct {
	ctx     colony.Context
	probe   AdapterProbeFunc
	timeout time.Duration

	mu    sync.Mutex
	view  AdapterCLIsView
	valid bool

	// runMu holds one probe run at a time. Two requests arriving together must
	// not exec the same four binaries twice, and the second one finds the cache
	// the first one filled.
	runMu sync.Mutex
}

func newAdapterProber(ctx colony.Context, probe AdapterProbeFunc, timeout time.Duration) *adapterProber {
	if probe == nil {
		probe = newExecAdapterProbe(ctx)
	}
	if timeout <= 0 {
		timeout = adapterProbeTimeout
	}
	return &adapterProber{ctx: ctx, probe: probe, timeout: timeout}
}

// snapshot answers from the cache, or probes and caches. force drops the cache
// first, which is what the block's Refresh control sends.
func (p *adapterProber) snapshot(ctx context.Context, force bool) AdapterCLIsView {
	p.mu.Lock()
	if !force && p.valid {
		view := p.view
		p.mu.Unlock()
		return view
	}
	p.mu.Unlock()

	p.runMu.Lock()
	// Another run may have filled the cache while this request waited for the
	// lock; a plain read takes that answer rather than exec'ing twice.
	p.mu.Lock()
	if !force && p.valid {
		view := p.view
		p.mu.Unlock()
		p.runMu.Unlock()
		return view
	}
	p.mu.Unlock()
	view := p.run(ctx)
	p.runMu.Unlock()

	// A cancelled request measured nothing, so it must not leave its rows in
	// the cache as though they had been answered.
	if ctx.Err() == nil {
		p.mu.Lock()
		p.view, p.valid = view, true
		p.mu.Unlock()
	}
	return view
}

// run probes every adapter concurrently. One adapter failing, timing out, or
// not being installed is that row's fact; it never fails the read, which is the
// partial-snapshot tolerance SystemView.Error has for a box that would not
// answer half its questions.
func (p *adapterProber) run(ctx context.Context) AdapterCLIsView {
	targets := p.targets()
	view := AdapterCLIsView{
		Adapters: make([]AdapterCLIView, len(targets)),
		ProbedAt: time.Now().UTC().Format(time.RFC3339),
	}
	var wg sync.WaitGroup
	for i, target := range targets {
		wg.Add(1)
		go func() {
			defer wg.Done()
			view.Adapters[i] = p.probeOne(ctx, target)
		}()
	}
	wg.Wait()
	return view
}

// targets reads the effective binary per adapter from the colony context, which
// is what the adapters themselves launch — so the row is about the binary that
// would really run, not a hardcoded catalog name.
func (p *adapterProber) targets() []AdapterProbeTarget {
	targets := make([]AdapterProbeTarget, 0, len(adapterCLINames))
	for _, name := range adapterCLINames {
		targets = append(targets, AdapterProbeTarget{
			Name:   name,
			Binary: strings.TrimSpace(colony.AdapterExtra(p.ctx, name).Binary),
		})
	}
	return targets
}

func (p *adapterProber) probeOne(ctx context.Context, target AdapterProbeTarget) AdapterCLIView {
	row := AdapterCLIView{Name: target.Name, Binary: target.Binary}
	probeCtx, cancel := context.WithTimeout(ctx, p.timeout)
	defer cancel()
	result := p.probe(probeCtx, target)
	row.Found = result.Found
	row.Path = result.Path
	row.Version = result.Version
	if result.Err != nil {
		row.Error = result.Err.Error()
	}
	return row
}

// newExecAdapterProbe returns the real probe: `exec.LookPath` for presence —
// the test every adapter already applies before it runs — then a bounded
// `--version` for the string.
func newExecAdapterProbe(ctx colony.Context) AdapterProbeFunc {
	credentials := adapterCredentialEnv(ctx)
	return func(probeCtx context.Context, target AdapterProbeTarget) AdapterProbeResult {
		path, err := exec.LookPath(target.Binary)
		if err != nil {
			// Not installed is a verdict, not a fault: the row says so.
			return AdapterProbeResult{}
		}
		cmd := exec.CommandContext(probeCtx, path, "--version")
		// A `--version` has no reason to see a credential, and a CLI that
		// printed one into its banner would put it in this response.
		cmd.Env = withoutEnv(os.Environ(), credentials)
		// Combined: some CLIs report their version on stderr. Capped: whatever it
		// prints, the response is one line.
		var out cappedBuffer
		cmd.Stdout, cmd.Stderr = &out, &out
		runErr := cmd.Run()
		version := firstLine(out.String(), adapterVersionRunes)
		if runErr != nil {
			if version == "" {
				return AdapterProbeResult{Found: true, Path: path, Err: runErr}
			}
			// A banner that arrived alongside a non-zero exit is still the
			// version; the exit code is the CLI being fussy.
			return AdapterProbeResult{Found: true, Path: path, Version: version}
		}
		return AdapterProbeResult{Found: true, Path: path, Version: version}
	}
}

// adapterCredentialEnv is the set of environment variables holding an adapter's
// API key, taken from the same machine-local config the adapters read.
func adapterCredentialEnv(ctx colony.Context) map[string]bool {
	names := map[string]bool{}
	for _, name := range []string{
		ctx.Cursor.APIKeyEnv,
		ctx.Pi.APIKeyEnv,
		ctx.Claude.APIKeyEnv,
	} {
		if trimmed := strings.TrimSpace(name); trimmed != "" {
			names[trimmed] = true
		}
	}
	return names
}

// withoutEnv drops the named variables and keeps everything else. An allowlist
// would break a CLI installed under a version manager, which needs its own
// PATH-adjacent variables to answer at all.
func withoutEnv(env []string, drop map[string]bool) []string {
	if len(drop) == 0 {
		return env
	}
	kept := make([]string, 0, len(env))
	for _, entry := range env {
		name, _, _ := strings.Cut(entry, "=")
		if drop[name] {
			continue
		}
		kept = append(kept, entry)
	}
	return kept
}

// cappedBuffer keeps the first adapterOutputRunes a CLI prints and counts what
// came after, so one probe cannot be made to allocate without bound.
type cappedBuffer struct {
	kept []rune
	read int
}

func (b *cappedBuffer) Write(p []byte) (int, error) {
	b.read += len(p)
	for _, r := range string(p) {
		if len(b.kept) >= adapterOutputRunes {
			break
		}
		b.kept = append(b.kept, r)
	}
	return len(p), nil
}

func (b *cappedBuffer) String() string { return string(b.kept) }

// firstLine is the first non-empty line, trimmed and capped at a rune boundary
// so a truncated version is still valid text.
func firstLine(out string, limit int) string {
	for _, line := range strings.Split(out, "\n") {
		line = strings.TrimSpace(strings.TrimSuffix(line, "\r"))
		if line == "" {
			continue
		}
		if utf8.RuneCountInString(line) <= limit {
			return line
		}
		return string([]rune(line)[:limit]) + "…"
	}
	return ""
}

func (a *api) handleSystemAdapters(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	force := r.URL.Query().Get("refresh") == "1"
	writeJSON(w, a.probe.snapshot(r.Context(), force))
}
