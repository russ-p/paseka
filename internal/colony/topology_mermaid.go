package colony

import (
	"fmt"
	"strings"
)

// TopologyMermaid renders a canonical flowchart LR diagram from structured topology.
func TopologyMermaid(topo Topology) string {
	var b strings.Builder
	b.WriteString("flowchart LR\n")

	b.WriteString("  subgraph events [Events]\n")
	for _, ev := range topo.Events {
		fmt.Fprintf(&b, "    %s[\"%s\"]\n", topologyEventNodeID(ev.ID), mermaidEscapeLabel(ev.ID))
	}
	b.WriteString("  end\n")

	b.WriteString("  subgraph bees [Bees]\n")
	for _, bee := range topo.Bees {
		fmt.Fprintf(&b, "    %s[\"%s\"]\n", topologyBeeNodeID(bee.Role), mermaidEscapeLabel(beeMermaidLabel(bee)))
	}
	b.WriteString("  end\n")

	for _, edge := range topo.Edges {
		b.WriteString(topologyEdgeMermaid(edge))
		b.WriteByte('\n')
	}

	return strings.TrimRight(b.String(), "\n")
}

func beeMermaidLabel(bee TopologyBee) string {
	if len(bee.Intents) == 0 {
		return bee.Role
	}
	return bee.Role + "\nintents: " + strings.Join(bee.Intents, ", ")
}

func topologyEdgeMermaid(edge TopologyEdge) string {
	label := topologyEdgeLabel(edge)
	switch edge.Kind {
	case "subscribe":
		from := topologyEventNodeID(edge.From)
		to := topologyBeeNodeID(edge.To)
		return fmt.Sprintf("  %s -->|%s| %s", from, label, to)
	case "publish":
		from := topologyBeeNodeID(edge.From)
		to := topologyEventNodeID(edge.To)
		return fmt.Sprintf("  %s -->|%s| %s", from, label, to)
	case "invite":
		from := topologyEventNodeID(edge.From)
		if edge.To != "" {
			to := topologyBeeNodeID(edge.To)
			return fmt.Sprintf("  %s -->|%s| %s", from, label, to)
		}
		return fmt.Sprintf("  %s -->|%s| bees", from, label)
	default:
		return ""
	}
}

func topologyEdgeLabel(edge TopologyEdge) string {
	switch edge.Kind {
	case "subscribe":
		parts := []string{"subscribe", string(edge.Dispatch)}
		if edge.Implicit {
			parts = append(parts, "implicit")
		}
		return strings.Join(parts, " ")
	case "publish":
		return "publish"
	case "invite":
		parts := []string{"invite"}
		if edge.BeeFrom != "" {
			parts = append(parts, "from="+edge.BeeFrom)
		}
		if edge.Intent != "" {
			parts = append(parts, "intent="+edge.Intent)
		}
		return strings.Join(parts, " ")
	default:
		return edge.Kind
	}
}

func topologyEventNodeID(eventID string) string {
	return "E_" + sanitizeMermaidID(eventID)
}

func topologyBeeNodeID(role string) string {
	return "B_" + sanitizeMermaidID(role)
}

func sanitizeMermaidID(id string) string {
	id = strings.TrimSpace(id)
	if id == "" {
		return "empty"
	}
	var b strings.Builder
	for _, r := range id {
		switch {
		case r >= 'a' && r <= 'z', r >= 'A' && r <= 'Z', r >= '0' && r <= '9':
			b.WriteRune(r)
		case r == '*':
			b.WriteString("star")
		default:
			b.WriteByte('_')
		}
	}
	out := strings.Trim(b.String(), "_")
	if out == "" {
		return "node"
	}
	if out[0] >= '0' && out[0] <= '9' {
		return "n_" + out
	}
	return out
}

func mermaidEscapeLabel(label string) string {
	return strings.NewReplacer(`"`, `\"`, "\n", `\n`).Replace(label)
}

// NormalizedTopologyMermaid trims trailing whitespace for stable comparisons.
func NormalizedTopologyMermaid(topo Topology) string {
	return strings.TrimSpace(TopologyMermaid(topo))
}
