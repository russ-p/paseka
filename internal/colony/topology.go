package colony

import (
	"fmt"
	"sort"
	"strings"

	"github.com/paseka/paseka/internal/protocol"
)

// Topology is a static, config-derived EDA graph of bees and bus events.
type Topology struct {
	Bees    []TopologyBee   `json:"bees"`
	Events  []TopologyEvent `json:"events"`
	Edges   []TopologyEdge  `json:"edges"`
	Mermaid string          `json:"mermaid,omitempty"`
}

// TopologyBee is a bee node with prompt vocabulary annotations.
type TopologyBee struct {
	Role          string   `json:"role"`
	Adapter       string   `json:"adapter"`
	Intents       []string `json:"intents,omitempty"`
	DefaultIntent string   `json:"defaultIntent,omitempty"`
}

// TopologyEvent is a bus event kind referenced by topology edges.
type TopologyEvent struct {
	Type string `json:"type"`
	Kind string `json:"kind,omitempty"`
	ID   string `json:"id"`
}

// TopologyEdge connects bees and events (subscribe, publish, or invite).
type TopologyEdge struct {
	Kind     string            `json:"kind"`
	From     string            `json:"from"`
	To       string            `json:"to,omitempty"`
	Dispatch DispatchMode      `json:"dispatch,omitempty"`
	Implicit bool              `json:"implicit,omitempty"`
	Intent   string            `json:"intent,omitempty"`
	Match    map[string]string `json:"match,omitempty"`
	BeeFrom  string            `json:"beeFrom,omitempty"`
}

// BuildTopology projects colony EDA wiring from filesystem config only.
func BuildTopology(colonyRoot string) (Topology, error) {
	bees, err := LoadAllBees(colonyRoot)
	if err != nil {
		return Topology{}, err
	}
	manifest, err := LoadColony(colonyRoot)
	if err != nil {
		return Topology{}, err
	}

	roles := sortedBeeRoles(bees)
	eventByID := map[string]TopologyEvent{}
	var edges []TopologyEdge

	topo := Topology{Bees: make([]TopologyBee, 0, len(roles))}
	for _, role := range roles {
		bee := bees[role]
		adapter, err := bee.ResolveAdapter()
		if err != nil {
			return Topology{}, err
		}
		intents, defaultIntent, err := DiscoverIntents(colonyRoot, bee)
		if err != nil {
			return Topology{}, err
		}
		topo.Bees = append(topo.Bees, TopologyBee{
			Role:          role,
			Adapter:       adapter,
			Intents:       intents,
			DefaultIntent: defaultIntent,
		})

		if len(bee.Subscribes) == 0 {
			id := topologyEventID(protocol.EventSignal, string(protocol.TaskEventReady))
			addTopologyEvent(eventByID, id, protocol.EventSignal, string(protocol.TaskEventReady))
			edges = append(edges, TopologyEdge{
				Kind:     "subscribe",
				From:     id,
				To:       role,
				Dispatch: DispatchTask,
				Implicit: true,
			})
		} else {
			for _, sub := range bee.Subscribes {
				id, err := topologyEventIDFromRule(sub.EventRule)
				if err != nil {
					return Topology{}, fmt.Errorf("colony: bee %q subscribe: %w", role, err)
				}
				addTopologyEventFromID(eventByID, id, sub.EventRule)
				edges = append(edges, TopologyEdge{
					Kind:     "subscribe",
					From:     id,
					To:       role,
					Dispatch: sub.ResolvedDispatch(),
				})
			}
		}

		for _, pub := range bee.Publishes {
			id, err := topologyEventIDFromRule(pub.EventRule)
			if err != nil {
				return Topology{}, fmt.Errorf("colony: bee %q publish: %w", role, err)
			}
			addTopologyEventFromID(eventByID, id, pub.EventRule)
			edges = append(edges, TopologyEdge{
				Kind: "publish",
				From: role,
				To:   id,
			})
		}
	}

	for _, rule := range manifest.AutoInvites {
		id, err := topologyEventIDFromRule(rule.When)
		if err != nil {
			return Topology{}, fmt.Errorf("colony: auto_invite when: %w", err)
		}
		addTopologyEventFromID(eventByID, id, rule.When)

		edge := TopologyEdge{
			Kind: "invite",
			From: id,
		}
		if d := strings.TrimSpace(rule.Invite.Bee.Default); d != "" {
			edge.To = d
		} else if f := strings.TrimSpace(rule.Invite.Bee.From); f != "" {
			edge.BeeFrom = f
		}
		if d := strings.TrimSpace(rule.Invite.Intent.Default); d != "" {
			edge.Intent = d
		}
		if len(rule.Match) > 0 {
			edge.Match = copyStringMap(rule.Match)
		}
		edges = append(edges, edge)
	}

	topo.Events = sortedTopologyEvents(eventByID)
	topo.Edges = sortTopologyEdges(edges)
	topo.Mermaid = TopologyMermaid(topo)
	return topo, nil
}

func sortedBeeRoles(bees map[string]Bee) []string {
	roles := make([]string, 0, len(bees))
	for role := range bees {
		roles = append(roles, role)
	}
	sort.Strings(roles)
	return roles
}

func topologyEventIDFromRule(rule EventRule) (string, error) {
	typ, err := rule.EventType()
	if err != nil {
		return "", err
	}
	return topologyEventID(typ, rule.Kind), nil
}

func topologyEventID(typ protocol.EventType, kind string) string {
	kind = strings.TrimSpace(kind)
	if kind == "" {
		return string(typ) + "/*"
	}
	return string(typ) + "/" + kind
}

func addTopologyEventFromID(events map[string]TopologyEvent, id string, rule EventRule) {
	typ, err := rule.EventType()
	if err != nil {
		return
	}
	addTopologyEvent(events, id, typ, strings.TrimSpace(rule.Kind))
}

func addTopologyEvent(events map[string]TopologyEvent, id string, typ protocol.EventType, kind string) {
	if _, ok := events[id]; ok {
		return
	}
	events[id] = TopologyEvent{
		Type: string(typ),
		Kind: kind,
		ID:   id,
	}
}

func sortedTopologyEvents(events map[string]TopologyEvent) []TopologyEvent {
	out := make([]TopologyEvent, 0, len(events))
	for _, ev := range events {
		out = append(out, ev)
	}
	sort.Slice(out, func(i, j int) bool {
		return out[i].ID < out[j].ID
	})
	return out
}

func sortTopologyEdges(edges []TopologyEdge) []TopologyEdge {
	sort.Slice(edges, func(i, j int) bool {
		a, b := edges[i], edges[j]
		if a.Kind != b.Kind {
			return a.Kind < b.Kind
		}
		if a.From != b.From {
			return a.From < b.From
		}
		if a.To != b.To {
			return a.To < b.To
		}
		if a.BeeFrom != b.BeeFrom {
			return a.BeeFrom < b.BeeFrom
		}
		if a.Dispatch != b.Dispatch {
			return a.Dispatch < b.Dispatch
		}
		if a.Implicit != b.Implicit {
			return a.Implicit
		}
		if a.Intent != b.Intent {
			return a.Intent < b.Intent
		}
		return matchLess(a.Match, b.Match)
	})
	return edges
}

func matchLess(a, b map[string]string) bool {
	akeys := sortedMapKeys(a)
	bkeys := sortedMapKeys(b)
	if len(akeys) != len(bkeys) {
		return len(akeys) < len(bkeys)
	}
	for i, k := range akeys {
		if k != bkeys[i] {
			return k < bkeys[i]
		}
		if a[k] != b[bkeys[i]] {
			return a[k] < b[bkeys[i]]
		}
	}
	return false
}

func sortedMapKeys(m map[string]string) []string {
	if len(m) == 0 {
		return nil
	}
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	return keys
}

func copyStringMap(in map[string]string) map[string]string {
	if len(in) == 0 {
		return nil
	}
	out := make(map[string]string, len(in))
	for k, v := range in {
		out[k] = v
	}
	return out
}
