package colony_test

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/paseka/paseka/internal/colony"
)

func TestBuildTopologyGolden(t *testing.T) {
	root := filepath.Join("testdata", "topology-fixture")
	topo, err := colony.BuildTopology(root)
	if err != nil {
		t.Fatal(err)
	}

	bees, err := colony.LoadAllBees(root)
	if err != nil {
		t.Fatal(err)
	}
	if len(topo.Bees) != len(bees) {
		t.Fatalf("bees = %d, want %d from LoadAllBees", len(topo.Bees), len(bees))
	}
	for role := range bees {
		if !topologyHasBee(topo, role) {
			t.Fatalf("topology missing bee %q", role)
		}
	}

	assertNoIntentNodes(t, topo)
	assertNoBeeToBeeEdges(t, topo)
	assertScoutIntakeSubscribe(t, topo)
	assertInviteEdges(t, topo)

	goldenPath := filepath.Join(root, "topology.golden.json")
	got, err := json.MarshalIndent(struct {
		Bees   []colony.TopologyBee   `json:"bees"`
		Events []colony.TopologyEvent `json:"events"`
		Edges  []colony.TopologyEdge  `json:"edges"`
	}{
		Bees:   topo.Bees,
		Events: topo.Events,
		Edges:  topo.Edges,
	}, "", "  ")
	if err != nil {
		t.Fatal(err)
	}
	got = append(got, '\n')

	if os.Getenv("UPDATE_TOPOLOGY_GOLDEN") != "" {
		if err := os.WriteFile(goldenPath, got, 0o644); err != nil {
			t.Fatal(err)
		}
		t.Logf("updated %s", goldenPath)
		return
	}

	want, err := os.ReadFile(goldenPath)
	if err != nil {
		t.Fatalf("read golden: %v (run with UPDATE_TOPOLOGY_GOLDEN=1 to create)", err)
	}
	if string(got) != string(want) {
		t.Fatalf("topology mismatch:\n%s", diffJSONLines(string(want), string(got)))
	}
}

func TestTopologyMermaidGolden(t *testing.T) {
	root := filepath.Join("testdata", "topology-fixture")
	topo, err := colony.BuildTopology(root)
	if err != nil {
		t.Fatal(err)
	}

	got := colony.NormalizedTopologyMermaid(topo)
	if got == "" {
		t.Fatal("empty mermaid output")
	}

	again := colony.NormalizedTopologyMermaid(topo)
	if got != again {
		t.Fatal("mermaid render is not deterministic")
	}

	goldenPath := filepath.Join(root, "topology.golden.mermaid")
	if os.Getenv("UPDATE_TOPOLOGY_GOLDEN") != "" {
		if err := os.WriteFile(goldenPath, []byte(got+"\n"), 0o644); err != nil {
			t.Fatal(err)
		}
		t.Logf("updated %s", goldenPath)
		return
	}

	want, err := os.ReadFile(goldenPath)
	if err != nil {
		t.Fatalf("read golden: %v (run with UPDATE_TOPOLOGY_GOLDEN=1 to create)", err)
	}
	if got != strings.TrimSpace(string(want)) {
		t.Fatalf("mermaid mismatch:\n%s", diffJSONLines(string(want), got+"\n"))
	}
}

func TestTopologyMermaidNodeIDsSanitized(t *testing.T) {
	root := filepath.Join("testdata", "topology-fixture")
	topo, err := colony.BuildTopology(root)
	if err != nil {
		t.Fatal(err)
	}
	mermaid := colony.TopologyMermaid(topo)
	for _, ev := range topo.Events {
		if strings.Contains(ev.ID, "/") && strings.Contains(mermaid, ev.ID+"[") {
			// display labels may contain slashes; node ids must not use raw event ids
			rawID := ev.ID
			if strings.Contains(mermaid, rawID+" -->") || strings.Contains(mermaid, "--> "+rawID) {
				t.Fatalf("unsanitized event id %q used as mermaid node id", rawID)
			}
		}
	}
}

func topologyHasBee(topo colony.Topology, role string) bool {
	for _, bee := range topo.Bees {
		if bee.Role == role {
			return true
		}
	}
	return false
}

func assertNoIntentNodes(t *testing.T, topo colony.Topology) {
	t.Helper()
	for _, ev := range topo.Events {
		if strings.Contains(ev.ID, "intent") {
			t.Fatalf("unexpected intent event node %q", ev.ID)
		}
	}
	for _, edge := range topo.Edges {
		if edge.Kind == "intent" {
			t.Fatalf("unexpected intent edge: %+v", edge)
		}
	}
}

func assertNoBeeToBeeEdges(t *testing.T, topo colony.Topology) {
	t.Helper()
	beeRoles := map[string]bool{}
	for _, bee := range topo.Bees {
		beeRoles[bee.Role] = true
	}
	for _, edge := range topo.Edges {
		if beeRoles[edge.From] && beeRoles[edge.To] {
			t.Fatalf("bee-to-bee edge: %+v", edge)
		}
	}
}

func assertScoutIntakeSubscribe(t *testing.T, topo colony.Topology) {
	t.Helper()
	for _, edge := range topo.Edges {
		if edge.Kind == "subscribe" && edge.To == "scout" && edge.From == "SIGNAL/feature.requested" && edge.Dispatch == colony.DispatchDirect {
			return
		}
	}
	t.Fatal("missing subscribe edge SIGNAL/feature.requested -> scout (direct)")
}

func assertImplicitSubscribe(t *testing.T, topo colony.Topology, role string) {
	t.Helper()
	for _, edge := range topo.Edges {
		if edge.Kind == "subscribe" && edge.To == role && edge.Implicit {
			if edge.From != "SIGNAL/task.ready" || edge.Dispatch != colony.DispatchTask {
				t.Fatalf("implicit subscribe = %+v, want SIGNAL/task.ready task", edge)
			}
			return
		}
	}
	t.Fatalf("missing implicit subscribe edge for %q", role)
}

func assertInviteEdges(t *testing.T, topo colony.Topology) {
	t.Helper()
	var grill, session, wildcard bool
	for _, edge := range topo.Edges {
		if edge.Kind != "invite" {
			continue
		}
		switch edge.From {
		case "SIGNAL/feature.classified":
			grill = edge.To == "drone" && edge.Intent == "grilling" &&
				edge.Match != nil && edge.Match["decision"] == "grill"
		case "SIGNAL/review.needed":
			session = edge.BeeFrom == "bee" && edge.Intent == "grilling" && edge.To == "" &&
				edge.Match != nil && edge.Match["decision"] == "session"
		case "INSIGHT/*":
			wildcard = edge.To == "scout" && edge.Intent == "survey"
		}
	}
	if !grill {
		t.Fatal("missing invite edge for SIGNAL/feature.classified")
	}
	if !session {
		t.Fatal("missing invite edge for SIGNAL/review.needed with beeFrom")
	}
	if !wildcard {
		t.Fatal("missing invite edge for INSIGHT/*")
	}
}

func diffJSONLines(want, got string) string {
	wantLines := strings.Split(want, "\n")
	gotLines := strings.Split(got, "\n")
	max := len(wantLines)
	if len(gotLines) > max {
		max = len(gotLines)
	}
	var b strings.Builder
	for i := 0; i < max; i++ {
		wl, gl := "", ""
		if i < len(wantLines) {
			wl = wantLines[i]
		}
		if i < len(gotLines) {
			gl = gotLines[i]
		}
		if wl != gl {
			b.WriteString("--- want\n+++ got\n")
			b.WriteString(wl)
			b.WriteString("\n")
			b.WriteString(gl)
			b.WriteString("\n")
		}
	}
	return b.String()
}

func TestBuildTopologyMermaidMatchesRenderer(t *testing.T) {
	root := filepath.Join("testdata", "topology-fixture")
	topo, err := colony.BuildTopology(root)
	if err != nil {
		t.Fatal(err)
	}
	if topo.Mermaid != colony.NormalizedTopologyMermaid(topo) {
		t.Fatal("BuildTopology.Mermaid must match NormalizedTopologyMermaid(topo)")
	}
	structured := colony.Topology{Bees: topo.Bees, Events: topo.Events, Edges: topo.Edges}
	if colony.NormalizedTopologyMermaid(structured) != topo.Mermaid {
		t.Fatal("Mermaid must be derivable from structured topology alone")
	}
}
