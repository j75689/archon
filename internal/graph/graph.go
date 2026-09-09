package graph

import (
	"fmt"
	"sort"
)

type Node struct {
	Key   string
	Label string
	ID    string
}

type Edge struct {
	From string
	To   string
}

type Graph struct {
	Nodes []Node
	Edges []Edge
}

type Diff struct {
	AddedNodes   []Node
	RemovedNodes []Node
	AddedEdges   []Edge
	RemovedEdges []Edge
}

func (d Diff) Empty() bool {
	return len(d.AddedNodes) == 0 && len(d.RemovedNodes) == 0 &&
		len(d.AddedEdges) == 0 && len(d.RemovedEdges) == 0
}

func nodeID(label string) string {
	if label == "." {
		return "n_root"
	}
	id := make([]byte, 0, 2+len(label))
	id = append(id, 'n', '_')
	for i := 0; i < len(label); i++ {
		c := label[i]
		if (c >= 'A' && c <= 'Z') || (c >= 'a' && c <= 'z') || (c >= '0' && c <= '9') {
			id = append(id, c)
		} else {
			id = append(id, '_')
		}
	}
	return string(id)
}

func Compile(g Graph) (Graph, error) {
	nodes := append([]Node(nil), g.Nodes...)
	sort.Slice(nodes, func(i, j int) bool { return nodes[i].Key < nodes[j].Key })
	ids := map[string]string{}
	nodeKeys := make(map[string]struct{}, len(nodes))
	for i := range nodes {
		id := nodeID(nodes[i].Label)
		if other, ok := ids[id]; ok && other != nodes[i].Key {
			return Graph{}, fmt.Errorf("mermaid id collision %q between %q and %q", id, other, nodes[i].Key)
		}
		ids[id] = nodes[i].Key
		nodeKeys[nodes[i].Key] = struct{}{}
		nodes[i].ID = id
	}
	edges := make([]Edge, 0, len(g.Edges))
	for _, edge := range g.Edges {
		if _, ok := nodeKeys[edge.From]; !ok {
			continue
		}
		if _, ok := nodeKeys[edge.To]; !ok {
			continue
		}
		edges = append(edges, edge)
	}
	sort.Slice(edges, func(i, j int) bool {
		if edges[i].From != edges[j].From {
			return edges[i].From < edges[j].From
		}
		return edges[i].To < edges[j].To
	})
	return Graph{Nodes: nodes, Edges: edges}, nil
}

func DiffGraphs(from, to Graph) Diff {
	fromN := map[string]Node{}
	toN := map[string]Node{}
	for _, n := range from.Nodes {
		fromN[n.Key] = n
	}
	for _, n := range to.Nodes {
		toN[n.Key] = n
	}
	var d Diff
	for k, n := range toN {
		if _, ok := fromN[k]; !ok {
			d.AddedNodes = append(d.AddedNodes, n)
		}
	}
	for k, n := range fromN {
		if _, ok := toN[k]; !ok {
			d.RemovedNodes = append(d.RemovedNodes, n)
		}
	}
	type ek struct{ f, t string }
	fromE := map[ek]Edge{}
	toE := map[ek]Edge{}
	for _, e := range from.Edges {
		fromE[ek{e.From, e.To}] = e
	}
	for _, e := range to.Edges {
		toE[ek{e.From, e.To}] = e
	}
	for k, e := range toE {
		if _, ok := fromE[k]; !ok {
			d.AddedEdges = append(d.AddedEdges, e)
		}
	}
	for k, e := range fromE {
		if _, ok := toE[k]; !ok {
			d.RemovedEdges = append(d.RemovedEdges, e)
		}
	}
	sort.Slice(d.AddedNodes, func(i, j int) bool { return d.AddedNodes[i].Key < d.AddedNodes[j].Key })
	sort.Slice(d.RemovedNodes, func(i, j int) bool { return d.RemovedNodes[i].Key < d.RemovedNodes[j].Key })
	sort.Slice(d.AddedEdges, func(i, j int) bool {
		if d.AddedEdges[i].From != d.AddedEdges[j].From {
			return d.AddedEdges[i].From < d.AddedEdges[j].From
		}
		return d.AddedEdges[i].To < d.AddedEdges[j].To
	})
	sort.Slice(d.RemovedEdges, func(i, j int) bool {
		if d.RemovedEdges[i].From != d.RemovedEdges[j].From {
			return d.RemovedEdges[i].From < d.RemovedEdges[j].From
		}
		return d.RemovedEdges[i].To < d.RemovedEdges[j].To
	})
	return d
}
