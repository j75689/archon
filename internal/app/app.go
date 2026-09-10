package app

import (
	"context"
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
	"github.com/j75689/archon/internal/llm"
	"github.com/j75689/archon/internal/log"
)

type App struct {
	Repo    *git.Repo
	Ext     lang.Extractor
	From    string
	To      string
	Doc     string
	Anchor  string
	Model   string
	BaseURL string
	APIKey  string
	LLM     *llm.Client
	Log     log.Logger
	Stdout  io.Writer
	Stderr  io.Writer
}

func New(repo *git.Repo) *App {
	return &App{
		Repo:    repo,
		Ext:     golang.Extractor{},
		To:      "HEAD",
		Doc:     "docs/ARCHITECTURE.md",
		Anchor:  "data-flow",
		Model:   "gpt-4o-mini",
		BaseURL: "https://api.openai.com/v1",
		Stdout:  os.Stdout,
		Stderr:  os.Stderr,
	}
}

func (a *App) attachLog() {
	l := log.OrNop(a.Log)
	if a.Repo != nil {
		a.Repo.Log = l
	}
	switch ext := a.Ext.(type) {
	case golang.Extractor:
		ext.Log = l
		a.Ext = ext
	case *golang.Extractor:
		ext.Log = l
	}
	if a.LLM != nil {
		a.LLM.Log = l
	}
}

func (a *App) log() log.Logger {
	return log.OrNop(a.Log)
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
	a.log().Info(fmt.Sprintf("extract go: %d packages, %d edges", len(g.Nodes), len(g.Edges)))
	g, err = graph.Compile(g)
	if err != nil {
		fmt.Fprintln(a.Stderr, err)
		return graph.Graph{}, exitcode.Fail
	}
	a.log().Info(fmt.Sprintf("compile: %d nodes, %d edges", len(g.Nodes), len(g.Edges)))
	return g, exitcode.OK
}

func (a *App) Check() int {
	a.attachLog()
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
			a.log().Info(fmt.Sprintf("compare %s anchor=%s: stale", a.Doc, a.Anchor))
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
		a.log().Info(fmt.Sprintf("compare %s anchor=%s: stale", a.Doc, a.Anchor))
		fmt.Fprintln(a.Stderr, "architecture doc is stale or missing; run archon sync")
		return exitcode.Gate
	}

	a.log().Info(fmt.Sprintf("compare %s anchor=%s: match", a.Doc, a.Anchor))
	return exitcode.OK
}

func (a *App) Diff() int {
	a.attachLog()
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

func (a *App) Changelog() int {
	a.attachLog()
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
	if d.Empty() || a.LLM == nil {
		return exitcode.OK
	}

	subjects, err := a.Repo.LogSubjects(fr.From, to, 50, 8192)
	if err != nil {
		fmt.Fprintln(a.Stderr, "warning:", err)
		return exitcode.OK
	}
	delta, err := a.LLM.Delta(context.Background(), d, subjects)
	if err != nil {
		fmt.Fprintln(a.Stderr, llm.FormatDeltaError(err))
		return exitcode.OK
	}

	if !strings.HasSuffix(graph.FormatReport(d), "\n\n") {
		fmt.Fprintln(a.Stdout)
	}
	fmt.Fprintln(a.Stdout, delta)
	return exitcode.OK
}

func (a *App) Sync() int {
	a.attachLog()
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
		fmt.Fprintln(a.Stderr, "warning:", err)
		return exitcode.OK
	}

	if fr.EmptyDiff {
		fmt.Fprint(a.Stdout, graph.FormatReport(graph.Diff{}))
		return exitcode.OK
	}

	fromG, code := a.graphAt(fr.From)
	if code != exitcode.OK {
		return exitcode.OK
	}

	d := graph.DiffGraphs(fromG, toG)
	if d.Empty() || a.LLM == nil {
		fmt.Fprint(a.Stdout, graph.FormatReport(d))
		return exitcode.OK
	}

	subjects, err := a.Repo.LogSubjects(fr.From, to, 50, 8192)
	if err != nil {
		fmt.Fprintln(a.Stderr, "warning:", err)
		fmt.Fprint(a.Stdout, graph.FormatReport(d))
		return exitcode.OK
	}
	delta, err := a.LLM.Delta(context.Background(), d, subjects)
	if err != nil {
		fmt.Fprintln(a.Stderr, llm.FormatDeltaError(err))
		fmt.Fprint(a.Stdout, graph.FormatReport(d))
		return exitcode.OK
	}

	fmt.Fprintln(a.Stdout, delta)
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
