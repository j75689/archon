package golang

import (
	"bytes"
	"fmt"
	"go/parser"
	"go/token"
	"path"
	"regexp"
	"strings"

	"github.com/j75689/archon/internal/graph"
	"github.com/j75689/archon/internal/lang"
)

var moduleLine = regexp.MustCompile(`(?m)^module\s+(\S+)`)

type Extractor struct{}

func (Extractor) Name() string { return "go" }

func (Extractor) Match(s lang.Snapshot) bool {
	_, ok := parseModule(s.Files["go.mod"])
	return ok
}

func WantFile(p string) bool {
	p = path.Clean(strings.ReplaceAll(p, "\\", "/"))
	if p == "go.mod" {
		return true
	}
	if !strings.HasSuffix(p, ".go") || strings.HasSuffix(p, "_test.go") {
		return false
	}
	for _, part := range strings.Split(p, "/") {
		if part == "vendor" || part == "testdata" {
			return false
		}
	}
	return true
}

func (Extractor) Extract(s lang.Snapshot) (graph.Graph, error) {
	mod, ok := parseModule(s.Files["go.mod"])
	if !ok {
		return graph.Graph{}, fmt.Errorf("%s: missing or invalid go.mod", s.Rev)
	}

	type pkg struct {
		key   string
		label string
		imps  map[string]struct{}
	}

	pkgs := map[string]*pkg{}
	fset := token.NewFileSet()

	for p, src := range s.Files {
		if p == "go.mod" || !WantFile(p) {
			continue
		}
		if skipGenerated(src) || skipIgnore(src) {
			continue
		}

		af, err := parser.ParseFile(fset, p, src, parser.ParseComments)
		if err != nil {
			return graph.Graph{}, fmt.Errorf("%s: %w", p, err)
		}

		dir := path.Dir(p)
		if dir == "." {
			dir = ""
		}

		key := mod
		label := "."
		if dir != "" {
			key = mod + "/" + dir
			label = dir
		}

		pk := pkgs[key]
		if pk == nil {
			pk = &pkg{key: key, label: label, imps: map[string]struct{}{}}
			pkgs[key] = pk
		}

		for _, is := range af.Imports {
			imp := strings.Trim(is.Path.Value, `"`)
			if imp == "C" {
				continue
			}
			if imp == mod || strings.HasPrefix(imp, mod+"/") {
				pk.imps[imp] = struct{}{}
			}
		}
	}

	var g graph.Graph
	for _, pk := range pkgs {
		g.Nodes = append(g.Nodes, graph.Node{Key: pk.key, Label: pk.label})
		for imp := range pk.imps {
			g.Edges = append(g.Edges, graph.Edge{From: pk.key, To: imp})
		}
	}
	return g, nil
}

func parseModule(b []byte) (string, bool) {
	m := moduleLine.FindSubmatch(b)
	if m == nil {
		return "", false
	}
	return string(m[1]), true
}

func skipGenerated(src []byte) bool {
	for _, line := range bytes.Split(src, []byte("\n")) {
		t := bytes.TrimSpace(line)
		if len(t) == 0 {
			continue
		}
		if bytes.HasPrefix(t, []byte("//")) {
			c := string(bytes.TrimSpace(t[2:]))
			return strings.Contains(c, "Code generated") && strings.Contains(c, "DO NOT EDIT")
		}
		return false
	}
	return false
}

func skipIgnore(src []byte) bool {
	s := string(src)
	return strings.Contains(s, "//go:build ignore") || strings.Contains(s, "// +build ignore")
}
