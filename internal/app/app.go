package app

import (
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	"github.com/j75689/archon/internal/doc"
	"github.com/j75689/archon/internal/exitcode"
	"github.com/j75689/archon/internal/git"
	"github.com/j75689/archon/internal/graph"
	"github.com/j75689/archon/internal/lang"
	"github.com/j75689/archon/internal/lang/golang"
)

type App struct {
	Repo   *git.Repo
	Ext    lang.Extractor
	From   string
	To     string
	Doc    string
	Anchor string
	Stdout io.Writer
	Stderr io.Writer
}

func New(repo *git.Repo) *App {
	return &App{
		Repo:   repo,
		Ext:    golang.Extractor{},
		To:     "HEAD",
		Doc:    "docs/ARCHITECTURE.md",
		Anchor: "data-flow",
		Stdout: os.Stdout,
		Stderr: os.Stderr,
	}
}

func (a *App) graphAt(rev string) (graph.Graph, int) {
	snap, err := a.Repo.Snapshot(rev, golang.WantFile)
	if err != nil {
		fmt.Fprintln(a.Stderr, err)
		return graph.Graph{}, exitcode.Fail
	}
	if !a.Ext.Match(snap) {
		fmt.Fprintln(a.Stderr, "no supported language (need go.mod at repository root)")
		return graph.Graph{}, exitcode.Fail
	}
	g, err := a.Ext.Extract(snap)
	if err != nil {
		fmt.Fprintln(a.Stderr, err)
		return graph.Graph{}, exitcode.Fail
	}
	g, err = graph.Compile(g)
	if err != nil {
		fmt.Fprintln(a.Stderr, err)
		return graph.Graph{}, exitcode.Fail
	}
	return g, exitcode.OK
}

func (a *App) Check() int {
	if a.From != "" {
		fmt.Fprintln(a.Stderr, "warning: --from is ignored by check")
	}

	to := a.To
	if to == "" {
		to = "HEAD"
	}

	g, code := a.graphAt(to)
	if code != exitcode.OK {
		return code
	}

	src, err := os.ReadFile(filepath.Join(a.Repo.Root, filepath.FromSlash(a.Doc)))
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			fmt.Fprintln(a.Stderr, "architecture doc is stale or missing; run archon sync")
			return exitcode.Gate
		}
		fmt.Fprintln(a.Stderr, err)
		return exitcode.Fail
	}

	region, ok, err := doc.ExtractRegion(src, a.Anchor)
	if err != nil {
		fmt.Fprintln(a.Stderr, err)
		return exitcode.Fail
	}

	want := strings.TrimSpace(doc.NormalizeNL(graph.RenderMermaid(g)))
	got := strings.TrimSpace(doc.NormalizeNL(region))
	if !ok || got != want {
		fmt.Fprintln(a.Stderr, "architecture doc is stale or missing; run archon sync")
		return exitcode.Gate
	}

	return exitcode.OK
}

func (a *App) Diff() int {
	to := a.To
	if to == "" {
		to = "HEAD"
	}

	fr, err := a.Repo.ResolveFrom(to, a.From)
	if err != nil {
		if errors.Is(err, git.ErrNoTag) {
			fmt.Fprintln(a.Stderr, err)
			return exitcode.Fail
		}
		fmt.Fprintln(a.Stderr, err)
		return exitcode.Fail
	}

	if fr.EmptyDiff {
		fmt.Fprint(a.Stdout, graph.FormatReport(graph.Diff{}))
		return exitcode.OK
	}

	fromG, code := a.graphAt(fr.From)
	if code != exitcode.OK {
		return code
	}
	toG, code := a.graphAt(to)
	if code != exitcode.OK {
		return code
	}

	d := graph.DiffGraphs(fromG, toG)
	fmt.Fprint(a.Stdout, graph.FormatReport(d))
	if d.Empty() {
		return exitcode.OK
	}
	return exitcode.Gate
}

func (a *App) Sync() int {
	to := a.To
	if to == "" {
		to = "HEAD"
	}

	toG, code := a.graphAt(to)
	if code != exitcode.OK {
		return code
	}

	payload := graph.RenderMermaid(toG)
	docPath := filepath.Join(a.Repo.Root, filepath.FromSlash(a.Doc))

	body, err := buildSyncedDoc(docPath, a.Anchor, payload)
	if err != nil {
		fmt.Fprintln(a.Stderr, err)
		return exitcode.Fail
	}
	if err := os.MkdirAll(filepath.Dir(docPath), 0o755); err != nil {
		fmt.Fprintln(a.Stderr, err)
		return exitcode.Fail
	}
	if err := os.WriteFile(docPath, body, 0o644); err != nil {
		fmt.Fprintln(a.Stderr, err)
		return exitcode.Fail
	}

	fr, err := a.Repo.ResolveFrom(to, a.From)
	if err != nil {
		if a.From == "" && errors.Is(err, git.ErrNoTag) {
			fmt.Fprintln(a.Stderr, "warning:", err)
			return exitcode.OK
		}
		fmt.Fprintln(a.Stderr, err)
		return exitcode.Fail
	}

	if fr.EmptyDiff {
		fmt.Fprint(a.Stdout, graph.FormatReport(graph.Diff{}))
		return exitcode.OK
	}

	fromG, code := a.graphAt(fr.From)
	if code != exitcode.OK {
		return code
	}

	fmt.Fprint(a.Stdout, graph.FormatReport(graph.DiffGraphs(fromG, toG)))
	return exitcode.OK
}

func buildSyncedDoc(path, anchor, payload string) ([]byte, error) {
	src, err := os.ReadFile(path)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return doc.NewDocument(payload, anchor), nil
		}
		return nil, err
	}

	_, ok, err := doc.ExtractRegion(src, anchor)
	if err != nil {
		return nil, err
	}
	if !ok {
		return doc.AppendAnchor(src, anchor, payload), nil
	}
	return doc.ReplaceRegion(src, anchor, payload)
}
