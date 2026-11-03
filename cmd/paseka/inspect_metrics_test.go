package main

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/russ-p/paseka/internal/runs"
)

func TestInspectMetricsCLI(t *testing.T) {
	repo := initInspectFixtureRepo(t)

	out := runInspectMetrics(t, repo, "--trace", "trace-cli")
	for _, want := range []string{
		"Trace: trace-cli\n",
		"  observedAt: ",
		"  class: ",
		"  cue: ",
		"  work: ",
		"stage work",
		"  honey: ",
		"  tokens: ",
		"  runs: 1",
		"  active: ",
		"  wall: ",
		"  verification failed: ",
		"  gate rejections: ",
		"  review notes: ",
		"  human feedback: ",
		"  coverage: ",
		"  warnings:",
	} {
		if !strings.Contains(out, want) {
			t.Fatalf("output missing %q:\n%s", want, out)
		}
	}
}

func TestInspectMetricsJSONAndWriteAgree(t *testing.T) {
	repo := initInspectFixtureRepo(t)

	out := runInspectMetrics(t, repo, "--trace", "trace-cli", "--json", "--write")
	var printed runs.TrailMetrics
	if err := json.Unmarshal([]byte(out), &printed); err != nil {
		t.Fatalf("printed JSON: %v\n%s", err, out)
	}
	data, err := os.ReadFile(runs.TrailMetricsPath(repo, "trace-cli"))
	if err != nil {
		t.Fatalf("--write did not persist metrics.json: %v", err)
	}
	var persisted runs.TrailMetrics
	if err := json.Unmarshal(data, &persisted); err != nil {
		t.Fatalf("persisted JSON: %v\n%s", err, data)
	}
	if !persisted.ObservedAt.Equal(printed.ObservedAt) || persisted.TraceID != printed.TraceID ||
		persisted.Runs.Retained != printed.Runs.Retained {
		t.Fatalf("printed and persisted projections disagree:\nprinted   %+v\npersisted %+v", printed, persisted)
	}

	if err := os.Remove(runs.TrailMetricsPath(repo, "trace-cli")); err != nil {
		t.Fatal(err)
	}
	runInspectMetrics(t, repo, "--trace", "trace-cli", "--json")
	if _, err := os.Stat(runs.TrailMetricsPath(repo, "trace-cli")); !os.IsNotExist(err) {
		t.Fatalf("a default read changed the trail: metrics.json exists (err = %v)", err)
	}
	if _, err := os.Stat(filepath.Join(repo, ".paseka", "runs", "trace-cli", "trail-events.ndjson")); !os.IsNotExist(err) {
		t.Fatalf("a default read published a trail event (err = %v)", err)
	}
}

func TestInspectMetricsMissingTrailIsError(t *testing.T) {
	repo := initInspectFixtureRepo(t)

	root := newRoot()
	var out bytes.Buffer
	root.SetOut(&out)
	root.SetErr(&out)
	root.SetArgs([]string{"inspect", "metrics", "-C", repo, "--trace", "trace-absent"})
	err := root.Execute()
	if err == nil || !strings.Contains(err.Error(), "trail not found") {
		t.Fatalf("err = %v, want a missing-trail error\n%s", err, out.String())
	}
}

func runInspectMetrics(t *testing.T, repo string, args ...string) string {
	t.Helper()
	root := newRoot()
	var out bytes.Buffer
	root.SetOut(&out)
	root.SetErr(&out)
	root.SetArgs(append([]string{"inspect", "metrics", "-C", repo}, args...))
	if err := root.Execute(); err != nil {
		t.Fatalf("execute: %v\n%s", err, out.String())
	}
	return out.String()
}
