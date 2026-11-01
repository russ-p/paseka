package colony_test

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/russ-p/paseka/internal/colony"
	"gopkg.in/yaml.v3"
)

func writeGateBee(t *testing.T, root, body string) {
	t.Helper()
	beesDir := filepath.Join(root, ".paseka", "bees")
	if err := os.MkdirAll(beesDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(beesDir, "builder.yaml"), []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
}

func TestLoadBeeWithoutChecksHasNoGate(t *testing.T) {
	root := t.TempDir()
	writeGateBee(t, root, `role: builder
adapter: cursor
prompt_template: builder.md
`)
	bee, _, err := colony.LoadBee(root, "builder")
	if err != nil {
		t.Fatal(err)
	}
	if bee.Checks != nil {
		t.Fatalf("un-gated bee has checks: %+v", bee.Checks)
	}
	if bee.CheckGate() != nil {
		t.Fatal("un-gated bee reported a gate")
	}
}

func TestLoadBeeChecksDefaults(t *testing.T) {
	root := t.TempDir()
	writeGateBee(t, root, `role: builder
adapter: cursor
prompt_template: builder.md
checks:
  command: ["sh", "-c", "exit 0"]
`)
	bee, _, err := colony.LoadBee(root, "builder")
	if err != nil {
		t.Fatal(err)
	}
	gate := bee.CheckGate()
	if gate == nil {
		t.Fatal("expected a gate")
	}
	if got := gate.ResolvedRejectionCap(); got != colony.DefaultCheckRejectionCap {
		t.Fatalf("default cap = %d, want %d", got, colony.DefaultCheckRejectionCap)
	}
	timeout, err := gate.TimeoutDuration()
	if err != nil {
		t.Fatal(err)
	}
	if timeout != colony.DefaultCheckTimeout {
		t.Fatalf("default timeout = %s, want %s", timeout, colony.DefaultCheckTimeout)
	}
}

func TestLoadBeeChecksExplicitZeroCapMeansNoCap(t *testing.T) {
	root := t.TempDir()
	writeGateBee(t, root, `role: builder
adapter: cursor
prompt_template: builder.md
checks:
  command: ["sh", "-c", "exit 0"]
  rejection_cap: 0
`)
	bee, _, err := colony.LoadBee(root, "builder")
	if err != nil {
		t.Fatal(err)
	}
	if got := bee.CheckGate().ResolvedRejectionCap(); got != 0 {
		t.Fatalf("cap = %d, want 0 (no cap)", got)
	}
}

func TestLoadBeeRejectsIncompleteChecks(t *testing.T) {
	tests := []struct {
		name string
		body string
		want string
	}{
		{
			name: "without command",
			body: `role: builder
adapter: cursor
prompt_template: builder.md
checks:
  timeout: 30s
`,
			want: "checks.command is required",
		},
		{
			name: "empty block",
			body: `role: builder
adapter: cursor
prompt_template: builder.md
checks: {}
`,
			want: "checks.command is required",
		},
		{
			name: "unparsable timeout",
			body: `role: builder
adapter: cursor
prompt_template: builder.md
checks:
  command: ["sh", "-c", "exit 0"]
  timeout: soon
`,
			want: "checks.timeout must be a duration string",
		},
		{
			name: "zero timeout",
			body: `role: builder
adapter: cursor
prompt_template: builder.md
checks:
  command: ["sh", "-c", "exit 0"]
  timeout: 0s
`,
			want: "checks.timeout must be a positive duration",
		},
		{
			name: "negative timeout",
			body: `role: builder
adapter: cursor
prompt_template: builder.md
checks:
  command: ["sh", "-c", "exit 0"]
  timeout: -5s
`,
			want: "checks.timeout must be a positive duration",
		},
		{
			name: "negative rejection cap",
			body: `role: builder
adapter: cursor
prompt_template: builder.md
checks:
  command: ["sh", "-c", "exit 0"]
  rejection_cap: -1
`,
			want: "checks.rejection_cap must not be negative",
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			root := t.TempDir()
			writeGateBee(t, root, tc.body)
			_, _, err := colony.LoadBee(root, "builder")
			if err == nil || !strings.Contains(err.Error(), tc.want) {
				t.Fatalf("err = %v, want %q", err, tc.want)
			}
		})
	}
}

// The gate survives a config snapshot round trip: a trail bound to its
// configuration must run the same check after a restart.
func TestBeeChecksSurviveYAMLRoundTrip(t *testing.T) {
	root := t.TempDir()
	writeGateBee(t, root, `role: builder
adapter: cursor
prompt_template: builder.md
checks:
  command: ["go", "test", "./..."]
  timeout: 90s
  rejection_cap: 0
`)
	bee, _, err := colony.LoadBee(root, "builder")
	if err != nil {
		t.Fatal(err)
	}
	data, err := yaml.Marshal(bee)
	if err != nil {
		t.Fatal(err)
	}
	var reloaded colony.Bee
	if err := yaml.Unmarshal(data, &reloaded); err != nil {
		t.Fatal(err)
	}
	gate := reloaded.CheckGate()
	if gate == nil {
		t.Fatalf("gate lost in round trip: %s", data)
	}
	timeout, err := gate.TimeoutDuration()
	if err != nil {
		t.Fatal(err)
	}
	if timeout != 90*time.Second {
		t.Fatalf("timeout = %s, want 90s", timeout)
	}
	if got := gate.ResolvedRejectionCap(); got != 0 {
		t.Fatalf("cap = %d, want explicit zero (no cap)", got)
	}
	if strings.Join(gate.Command.Argv(), " ") != "go test ./..." {
		t.Fatalf("command = %v", gate.Command.Argv())
	}
}

func TestLoadBeeChecksCommandMatchesAdapterCommand(t *testing.T) {
	root := t.TempDir()
	writeGateBee(t, root, `role: builder
adapter: cursor
prompt_template: builder.md
checks:
  command: agent -p "--model composer-2.5" $WORKSPACE
`)
	bee, _, err := colony.LoadBee(root, "builder")
	if err != nil {
		t.Fatal(err)
	}

	var adapterCommand colony.Command
	if err := yaml.Unmarshal([]byte(`agent -p "--model composer-2.5" $WORKSPACE`), &adapterCommand); err != nil {
		t.Fatal(err)
	}
	got := bee.CheckGate().Command.Argv()
	want := adapterCommand.Argv()
	if strings.Join(got, "\n") != strings.Join(want, "\n") {
		t.Fatalf("gate command = %v, adapter command = %v", got, want)
	}
}
