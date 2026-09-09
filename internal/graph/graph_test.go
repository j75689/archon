package graph

import (
	"strings"
	"testing"
)

func TestCompileAssignsIDsAndSorts(t *testing.T) {
	g, err := Compile(Graph{
		Nodes: []Node{
			{Key: "m/b", Label: "b"},
			{Key: "m", Label: "."},
			{Key: "m/a", Label: "a"},
		},
		Edges: []Edge{{From: "m/b", To: "m/a"}, {From: "m", To: "m/b"}},
	})
	if err != nil {
		t.Fatal(err)
	}
	if g.Nodes[0].Key != "m" || g.Nodes[0].ID != "n_root" {
		t.Fatalf("root: %+v", g.Nodes[0])
	}
	if g.Nodes[1].ID != "n_a" || g.Nodes[2].ID != "n_b" {
		t.Fatalf("ids: %+v", g.Nodes)
	}
	if g.Edges[0] != (Edge{From: "m", To: "m/b"}) {
		t.Fatalf("edges sorted: %+v", g.Edges)
	}
}

func TestCompileIDCollision(t *testing.T) {
	_, err := Compile(Graph{Nodes: []Node{
		{Key: "m/foo/bar", Label: "foo/bar"},
		{Key: "m/foo_bar", Label: "foo_bar"},
	}})
	if err == nil {
		t.Fatal("expected collision error")
	}
}

func TestRenderMermaidStable(t *testing.T) {
	g, err := Compile(Graph{
		Nodes: []Node{{Key: "m", Label: "."}, {Key: "m/internal/git", Label: "internal/git"}},
		Edges: []Edge{{From: "m", To: "m/internal/git"}},
	})
	if err != nil {
		t.Fatal(err)
	}
	want := "```mermaid\nflowchart LR\n  n_root[\".\"]\n  n_internal_git[\"internal/git\"]\n  n_root --> n_internal_git\n```\n"
	got := RenderMermaid(g)
	if got != want {
		t.Fatalf("got %q want %q", got, want)
	}
	if RenderMermaid(g) != got {
		t.Fatal("not byte-stable")
	}
}

func TestDiffGraphsAndReport(t *testing.T) {
	from, _ := Compile(Graph{Nodes: []Node{{Key: "m/a", Label: "a"}}})
	to, _ := Compile(Graph{
		Nodes: []Node{{Key: "m/a", Label: "a"}, {Key: "m/b", Label: "b"}},
		Edges: []Edge{{From: "m/a", To: "m/b"}},
	})
	d := DiffGraphs(from, to)
	if d.Empty() {
		t.Fatal("expected non-empty")
	}
	if len(d.AddedNodes) != 1 || d.AddedNodes[0].Key != "m/b" {
		t.Fatalf("added nodes: %+v", d.AddedNodes)
	}
	if len(d.AddedEdges) != 1 {
		t.Fatalf("added edges: %+v", d.AddedEdges)
	}
	rep := FormatReport(d)
	if !strings.Contains(rep, "m/b") || !strings.Contains(rep, "m/a -> m/b") {
		t.Fatalf("report: %s", rep)
	}
	if !DiffGraphs(from, from).Empty() {
		t.Fatal("identical should be empty")
	}
	if FormatReport(Diff{}) != "No first-party dependency changes.\n" {
		t.Fatalf("empty report: %q", FormatReport(Diff{}))
	}
}
