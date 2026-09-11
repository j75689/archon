package golang

import (
	"bytes"
	"fmt"
	"go/ast"
	"go/parser"
	"go/printer"
	"go/token"
	"path"
	"regexp"
	"sort"
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

func (e Extractor) ExtractAPIs(s lang.Snapshot) (lang.APISet, error) {
	mod, ok := parseModule(s.Files["go.mod"])
	if !ok {
		return nil, fmt.Errorf("%s: missing or invalid go.mod", s.Rev)
	}

	type pkg struct {
		api  lang.PackageAPI
		docs map[string]struct{}
	}

	pkgs := map[string]*pkg{}
	fset := token.NewFileSet()
	lg := log.OrNop(e.Log)

	files := make([]string, 0, len(s.Files))
	for p := range s.Files {
		files = append(files, p)
	}
	sort.Strings(files)

	for _, p := range files {
		src := s.Files[p]
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
			return nil, fmt.Errorf("%s: %w", p, err)
		}

		dir := path.Dir(p)
		if dir == "." {
			dir = ""
		}
		key := mod
		if dir != "" {
			key = mod + "/" + dir
		}

		pk := pkgs[key]
		if pk == nil {
			pk = &pkg{
				api:  lang.PackageAPI{Key: key},
				docs: map[string]struct{}{},
			}
			pkgs[key] = pk
		}

		if af.Doc != nil {
			doc := strings.TrimSpace(af.Doc.Text())
			if doc != "" {
				if _, exists := pk.docs[doc]; !exists {
					if pk.api.Doc != "" {
						pk.api.Doc += "\n"
					}
					pk.api.Doc += doc
					pk.docs[doc] = struct{}{}
				}
			}
		}

		for _, decl := range af.Decls {
			switch decl := decl.(type) {
			case *ast.FuncDecl:
				if !ast.IsExported(decl.Name.Name) {
					continue
				}
				sig := *decl
				sig.Doc = nil
				sig.Body = nil
				pk.api.Signatures = append(pk.api.Signatures, printNode(fset, &sig))
			case *ast.GenDecl:
				pk.api.Signatures = append(pk.api.Signatures, exportedGenDecls(fset, decl)...)
			}
		}
	}

	apis := make(lang.APISet, 0, len(pkgs))
	for _, pk := range pkgs {
		sort.Strings(pk.api.Signatures)
		apis = append(apis, pk.api)
	}
	sort.Slice(apis, func(i, j int) bool {
		return apis[i].Key < apis[j].Key
	})
	return apis, nil
}

func exportedGenDecls(fset *token.FileSet, decl *ast.GenDecl) []string {
	var signatures []string
	for _, spec := range decl.Specs {
		var exported ast.Spec
		switch spec := spec.(type) {
		case *ast.TypeSpec:
			if !ast.IsExported(spec.Name.Name) {
				continue
			}
			copy := *spec
			copy.Doc = nil
			copy.Comment = nil
			filterStructFields(copy.Type)
			exported = &copy
		case *ast.ValueSpec:
			copy := *spec
			copy.Doc = nil
			copy.Comment = nil
			copy.Names = nil
			copy.Values = nil
			for _, name := range spec.Names {
				if !ast.IsExported(name.Name) {
					continue
				}
				copy.Names = append(copy.Names, name)
			}
			if len(copy.Names) == 0 {
				continue
			}
			exported = &copy
		}
		if exported == nil {
			continue
		}
		copy := *decl
		copy.Doc = nil
		copy.Lparen = token.NoPos
		copy.Rparen = token.NoPos
		copy.Specs = []ast.Spec{exported}
		signatures = append(signatures, printNode(fset, &copy))
	}
	return signatures
}

func filterStructFields(node ast.Node) {
	ast.Inspect(node, func(node ast.Node) bool {
		structType, ok := node.(*ast.StructType)
		if !ok {
			return true
		}
		fields := make([]*ast.Field, 0, len(structType.Fields.List))
		for _, field := range structType.Fields.List {
			copy := *field
			copy.Doc = nil
			copy.Comment = nil
			if len(field.Names) == 0 {
				if exportedEmbeddedField(field.Type) {
					fields = append(fields, &copy)
				}
				continue
			}
			copy.Names = nil
			for _, name := range field.Names {
				if ast.IsExported(name.Name) {
					copy.Names = append(copy.Names, name)
				}
			}
			if len(copy.Names) != 0 {
				fields = append(fields, &copy)
			}
		}
		structType.Fields.List = fields
		return true
	})
}

func exportedEmbeddedField(expr ast.Expr) bool {
	switch expr := expr.(type) {
	case *ast.Ident:
		return ast.IsExported(expr.Name)
	case *ast.SelectorExpr:
		return ast.IsExported(expr.Sel.Name)
	case *ast.StarExpr:
		return exportedEmbeddedField(expr.X)
	case *ast.IndexExpr:
		return exportedEmbeddedField(expr.X)
	case *ast.IndexListExpr:
		return exportedEmbeddedField(expr.X)
	case *ast.ParenExpr:
		return exportedEmbeddedField(expr.X)
	default:
		return false
	}
}

func printNode(fset *token.FileSet, node any) string {
	var buf bytes.Buffer
	_ = printer.Fprint(&buf, fset, node)
	return buf.String()
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
