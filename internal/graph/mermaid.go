package graph

import (
	"fmt"
	"strings"
)

func RenderMermaid(g Graph) string {
	var b strings.Builder
	b.WriteString("```mermaid\nflowchart LR\n")
	idByKey := make(map[string]string, len(g.Nodes))
	for _, n := range g.Nodes {
		idByKey[n.Key] = n.ID
		fmt.Fprintf(&b, "  %s[%q]\n", n.ID, n.Label)
	}
	for _, e := range g.Edges {
		fmt.Fprintf(&b, "  %s --> %s\n", idByKey[e.From], idByKey[e.To])
	}
	b.WriteString("```\n")
	return b.String()
}
