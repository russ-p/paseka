package opencode

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"regexp"
	"strings"
	"sync"
	"time"

	"github.com/russ-p/paseka/internal/adapters"
)

const (
	adapterName   = "opencode"
	defaultBinary = "opencode"
	// defaultDetectTimeout bounds the one `--version` a dispatch may spawn.
	defaultDetectTimeout = 5 * time.Second
)

const (
	implementationV1 = "opencodev1"
	implementationV2 = "opencodev2"

	installV1 = "npm install -g opencode-ai@1"
	installV2 = "npm install -g opencode-ai@2"
)

// settings are the OpenCode adapter options. They travel as generic RunParams
// options rather than new shared struct fields.
type settings struct {
	version       string
	v1Binary      string
	v2Binary      string
	detectTimeout time.Duration
}

func settingsFrom(params adapters.RunParams) settings {
	s := settings{version: "auto", detectTimeout: defaultDetectTimeout}
	if params.Options == nil {
		return s
	}
	if v := strings.TrimSpace(params.Options["version"]); v != "" {
		s.version = strings.ToLower(v)
	}
	s.v1Binary = strings.TrimSpace(params.Options["v1_binary"])
	s.v2Binary = strings.TrimSpace(params.Options["v2_binary"])
	if v := strings.TrimSpace(params.Options["detect_timeout"]); v != "" {
		if d, err := time.ParseDuration(v); err == nil && d > 0 {
			s.detectTimeout = d
		}
	}
	return s
}

// resolution is which OpenCode major dispatched this launch and the binary that
// must run it. The binary is passed to the implementation so post-run
// accounting talks to the same executable that ran the agent.
type resolution struct {
	implementation string
	binary         string
	major          int
	pinned         bool
}

func resolve(binaryOverride string, params adapters.RunParams) (resolution, error) {
	s := settingsFrom(params)
	base := adapters.ResolveBinary(binaryOverride, params.Binary, defaultBinary)

	switch s.version {
	case "1", "2":
		major := 1
		if s.version == "2" {
			major = 2
		}
		binary := pinnedBinary(base, s, major)
		if _, err := exec.LookPath(binary); err != nil {
			return resolution{}, missingBinaryError(binary, major)
		}
		// A pin is trusted when the binary cannot answer, but a clear answer
		// that disagrees is a one-line configuration fix, not a flag mystery.
		if detected, ok, err := detectCached(binary, s.detectTimeout); err == nil && ok && detected != major {
			return resolution{}, fmt.Errorf(
				"opencode: %q reports OpenCode %d but version is pinned to %d (fix the version pin or the binary path)",
				binary, detected, major)
		}
		return resolution{implementation: implementationFor(major), binary: binary, major: major, pinned: true}, nil
	case "", "auto":
		if _, err := exec.LookPath(base); err != nil {
			return resolution{}, missingBinaryError(base, 0)
		}
		major, _, err := detectCached(base, s.detectTimeout)
		if err != nil {
			return resolution{}, err
		}
		return resolution{implementation: implementationFor(major), binary: base, major: major}, nil
	default:
		return resolution{}, fmt.Errorf("opencode: unsupported version pin %q (use auto, 1, or 2)", s.version)
	}
}

func pinnedBinary(base string, s settings, major int) string {
	if major == 1 && s.v1Binary != "" {
		return s.v1Binary
	}
	if major == 2 && s.v2Binary != "" {
		return s.v2Binary
	}
	return base
}

func implementationFor(major int) string {
	if major == 2 {
		return implementationV2
	}
	return implementationV1
}

func missingBinaryError(binary string, major int) error {
	switch major {
	case 1:
		return fmt.Errorf("opencode: %q not found in PATH (install OpenCode 1: %s)", binary, installV1)
	case 2:
		return fmt.Errorf("opencode: %q not found in PATH (install OpenCode 2: %s)", binary, installV2)
	default:
		return fmt.Errorf("opencode: %q not found in PATH (install OpenCode 1: %s or OpenCode 2: %s)", binary, installV1, installV2)
	}
}

// versionPattern finds the first major.minor token anywhere in a banner, so a
// padded or prefixed version string still parses. An unparsable string is an
// actionable error, never a silent fallback to V1.
var versionPattern = regexp.MustCompile(`(?i)(?:^|[^0-9])v?([0-9]+)\.([0-9]+)`)

type cacheKey struct {
	path  string
	mtime int64
	size  int64
}

var (
	versionCache sync.Map // cacheKey -> int
	// versionCommand is a test seam; production runs the resolved binary.
	versionCommand = func(ctx context.Context, path string) ([]byte, error) {
		return exec.CommandContext(ctx, path, "--version").CombinedOutput()
	}
)

// ResetVersionCache clears the detection cache. Tests use it; production never
// needs to, because a replaced binary changes its mtime or size.
func ResetVersionCache() {
	versionCache.Range(func(key, _ any) bool {
		versionCache.Delete(key)
		return true
	})
}

// detectCached returns the major of an installed binary. The cache is keyed by
// resolved path plus modification time and size, so a burst of runs spawns one
// `--version` and a replaced binary re-detects.
func detectCached(binary string, timeout time.Duration) (int, bool, error) {
	path, err := exec.LookPath(binary)
	if err != nil {
		return 0, false, missingBinaryError(binary, 0)
	}
	info, err := os.Stat(path)
	if err != nil {
		return 0, false, fmt.Errorf("opencode: stat %q: %w", path, err)
	}
	key := cacheKey{path: path, mtime: info.ModTime().UnixNano(), size: info.Size()}
	if major, ok := versionCache.Load(key); ok {
		return major.(int), true, nil
	}

	if timeout <= 0 {
		timeout = defaultDetectTimeout
	}
	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()
	out, runErr := versionCommand(ctx, path)
	raw := strings.TrimSpace(string(out))
	major, ok := parseMajor(raw)
	if !ok {
		if runErr != nil {
			return 0, false, fmt.Errorf(
				"opencode: cannot determine the OpenCode major from %q: %v (output %q); pin it with version: 1 or 2 (params.options.version or ~/.config/paseka/<slug>/adapters/opencode.yaml)",
				path, runErr, raw)
		}
		return 0, false, fmt.Errorf(
			"opencode: cannot determine the OpenCode major from %q: output %q; pin it with version: 1 or 2 (params.options.version or ~/.config/paseka/<slug>/adapters/opencode.yaml)",
			path, raw)
	}
	versionCache.Store(key, major)
	return major, true, nil
}

func parseMajor(raw string) (int, bool) {
	m := versionPattern.FindStringSubmatch(raw)
	if len(m) < 2 {
		return 0, false
	}
	switch m[1] {
	case "1":
		return 1, true
	case "2":
		return 2, true
	default:
		// A future major cannot be assumed to keep V1 or V2 flags.
		return 0, false
	}
}

// implementationName is the implementation a recorded session should be
// continued on. A direct pin reports itself.
func implementationName(adapter string) string {
	switch adapter {
	case implementationV1, implementationV2:
		return adapter
	default:
		return ""
	}
}
