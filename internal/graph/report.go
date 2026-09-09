package graph

import "strings"

func FormatReport(d Diff) string {
	if d.Empty() {
		return "No first-party dependency changes.\n"
	}
	var b strings.Builder
	writeKeys := func(title string, nodes []Node) {
		if len(nodes) == 0 {
			return
		}
		b.WriteString(title)
		b.WriteByte('\n')
		for _, n := range nodes {
			b.WriteString("- ")
			b.WriteString(n.Key)
			b.WriteByte('\n')
		}
	}
	writeEdges := func(title string, edges []Edge) {
		if len(edges) == 0 {
			return
		}
		b.WriteString(title)
		b.WriteByte('\n')
		for _, e := range edges {
			b.WriteString("- ")
			b.WriteString(e.From)
			b.WriteString(" -> ")
			b.WriteString(e.To)
			b.WriteByte('\n')
		}
	}
	writeKeys("added nodes:", d.AddedNodes)
	writeKeys("removed nodes:", d.RemovedNodes)
	writeEdges("added edges:", d.AddedEdges)
	writeEdges("removed edges:", d.RemovedEdges)
	return b.String()
}
