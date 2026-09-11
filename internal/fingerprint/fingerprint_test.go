package fingerprint

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/j75689/archon/internal/graph"
	"github.com/j75689/archon/internal/lang"
)

func sample() (graph.Graph, lang.APISet) {
	g := graph.Graph{
		Nodes: []graph.Node{{Key: "m/b", Label: "b", ID: "n_b"}, {Key: "m", Label: ".", ID: "n_root"}},
		Edges: []graph.Edge{{From: "m/b", To: "m"}, {From: "m", To: "m/b"}},
	}
	apis := lang.APISet{
		{Key: "m/b", Signatures: []string{"func Z()", "func A()"}},
		{Key: "m", Doc: "root", Signatures: []string{"func Hello()"}},
	}
	return g, apis
}

func TestHashStableAndIgnoresMermaidID(t *testing.T) {
	g, apis := sample()
	h1, err := Hash(g, apis)
	if err != nil {
		t.Fatal(err)
	}
	g.Nodes[0].ID = "changed"
	h2, err := Hash(g, apis)
	if err != nil {
		t.Fatal(err)
	}
	if h1 != h2 || len(h1) != 64 {
		t.Fatalf("h1=%s h2=%s", h1, h2)
	}
}

func TestHashChangesOnNewSignature(t *testing.T) {
	g, apis := sample()
	h1, _ := Hash(g, apis)
	apis[1].Signatures = append(apis[1].Signatures, "func Bye()")
	h2, _ := Hash(g, apis)
	if h1 == h2 {
		t.Fatal("expected hash change")
	}
}

func TestLockfileRoundTrip(t *testing.T) {
	dir := t.TempDir()
	p := filepath.Join(dir, ".archon", "graph.json")
	g, apis := sample()
	lf, err := NewLockfile(g, apis)
	if err != nil {
		t.Fatal(err)
	}
	if err := Write(p, lf); err != nil {
		t.Fatal(err)
	}
	got, err := Load(p)
	if err != nil {
		t.Fatal(err)
	}
	if got.Hash != lf.Hash {
		t.Fatalf("hash %s vs %s", got.Hash, lf.Hash)
	}
	if _, err := Load(filepath.Join(dir, "missing.json")); !os.IsNotExist(err) {
		t.Fatalf("missing: %v", err)
	}
}
