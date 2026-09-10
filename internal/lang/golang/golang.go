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
	"github.com/j75689/archon/internal/log"
)

var moduleLine = regexp.MustCompile(`(?m)^module\s+(\S+)`)

type Extractor struct {
	Log log.Logger
}

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

func (e Extractor) Extract(s lang.Snapshot) (graph.Graph, error) {
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
	lg := log.OrNop(e.Log)

	for p, src := range s.Files {
		if p == "go.mod" || !WantFile(p) {
			continue
		}
		if skipGenerated(src) {
			lg.Debug("skip: " + p + " (generated)")
			continue
		}
		if skipIgnore(src) {
			lg.Debug("skip: " + p + " (ignore)")
			continue
		}

		lg.Debug("parse: " + p)
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
	for _, line := range headerCommentLines(src) {
		if strings.Contains(line, "Code generated") && strings.Contains(line, "DO NOT EDIT") {
			return true
		}
	}
	return false
}

func skipIgnore(src []byte) bool {
	for _, line := range headerCommentLines(src) {
		switch {
		case strings.HasPrefix(line, "go:build"):
			if strings.TrimSpace(strings.TrimPrefix(line, "go:build")) == "ignore" {
				return true
			}
		case strings.HasPrefix(line, "+build"):
			if strings.TrimSpace(strings.TrimPrefix(line, "+build")) == "ignore" {
				return true
			}
		}
	}
	return false
}

func headerCommentLines(src []byte) []string {
	var lines []string
	inBlockComment := false

	for _, raw := range bytes.Split(src, []byte("\n")) {
		line := string(bytes.TrimSpace(raw))
		if inBlockComment {
			if idx := strings.Index(line, "*/"); idx >= 0 {
				lines = append(lines, strings.TrimSpace(line[:idx]))
				inBlockComment = false
				if strings.TrimSpace(line[idx+2:]) != "" {
					return lines
				}
				continue
			}
			lines = append(lines, strings.TrimSpace(line))
			continue
		}

		if line == "" {
			continue
		}
		if strings.HasPrefix(line, "package ") {
			return lines
		}
		if strings.HasPrefix(line, "//") {
			lines = append(lines, strings.TrimSpace(strings.TrimPrefix(line, "//")))
			continue
		}
		if strings.HasPrefix(line, "/*") {
			line = strings.TrimSpace(strings.TrimPrefix(line, "/*"))
			if idx := strings.Index(line, "*/"); idx >= 0 {
				lines = append(lines, strings.TrimSpace(line[:idx]))
				if strings.TrimSpace(line[idx+2:]) != "" {
					return lines
				}
				continue
			}
			lines = append(lines, strings.TrimSpace(line))
			inBlockComment = true
			continue
		}
		return lines
	}
	return lines
}
