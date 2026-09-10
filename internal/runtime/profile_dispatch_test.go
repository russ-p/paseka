package runtime_test

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/russ-p/paseka/internal/colony"
	"github.com/russ-p/paseka/internal/protocol"
	"github.com/russ-p/paseka/internal/runs"
	"github.com/russ-p/paseka/internal/runtime"
)

func TestBeeRunProfileRemapWritesMeta(t *testing.T) {
	repo := initMixedAdapterRepo(t)
	fakePi := filepath.Join(t.TempDir(), "fake-pi")
	if err := os.WriteFile(fakePi, []byte("#!/bin/sh\nprintf '%s\\n' '{\"summary\":\"profile ok\"}'\n"), 0o755); err != nil {
		t.Fatal(err)
	}
	setupPiDispatchHome(t, repo, fakePi)
	if err := os.MkdirAll(filepath.Join(repo, ".paseka", "profiles"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(repo, ".paseka", "profiles", "pi.yaml"), []byte("adapter: pi\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	colony.SetProcessProfile(colony.ProfileSelection{Name: "pi", FlagSet: true})
	t.Cleanup(func() { colony.SetProcessProfile(colony.ProfileSelection{}) })

	d := runtime.NewDispatcher()
	res, err := d.BeeRun(context.Background(), runtime.BeeRunRequest{
		StartDir: repo,
		Bee:      "scout",
		TraceID:  "trace-profile-pi",
		Task:     "try pi",
		NoBus:    true,
	})
	if err != nil {
		t.Fatal(err)
	}
	if res.Result.Status != string(protocol.StatusCompleted) {
		t.Fatalf("status = %q", res.Result.Status)
	}

	runDir := runs.Dir{ColonyRoot: repo, TraceID: "trace-profile-pi", AgentID: res.AgentID}
	data, err := os.ReadFile(runDir.MetaPath())
	if err != nil {
		t.Fatal(err)
	}
	var meta runs.Meta
	if err := json.Unmarshal(data, &meta); err != nil {
		t.Fatal(err)
	}
	if meta.Adapter != "pi" {
		t.Fatalf("meta adapter = %q", meta.Adapter)
	}
	if meta.Profile != "pi" {
		t.Fatalf("meta profile = %q", meta.Profile)
	}
	req, err := runDir.ReadRequest()
	if err != nil {
		t.Fatal(err)
	}
	if req.Adapter != "pi" {
		t.Fatalf("request adapter = %q", req.Adapter)
	}
}
