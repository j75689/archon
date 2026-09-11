package app

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"

	"github.com/j75689/archon/internal/doc"
	"github.com/j75689/archon/internal/exitcode"
	"github.com/j75689/archon/internal/fingerprint"
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

func (a *App) structureAt(rev string) (graph.Graph, lang.APISet, int) {
	snap, err := a.Repo.Snapshot(rev, golang.WantFile)
	if err != nil {
		fmt.Fprintln(a.Stderr, err)
		return graph.Graph{}, nil, exitcode.Fail
	}
	if !a.Ext.Match(snap) {
		fmt.Fprintln(a.Stderr, "no supported language (need go.mod at repository root)")
		return graph.Graph{}, nil, exitcode.Fail
	}
	g, err := a.Ext.Extract(snap)
	if err != nil {
		fmt.Fprintln(a.Stderr, err)
		return graph.Graph{}, nil, exitcode.Fail
	}
	a.log().Info(fmt.Sprintf("extract go: %d packages, %d edges", len(g.Nodes), len(g.Edges)))
	g, err = graph.Compile(g)
	if err != nil {
		fmt.Fprintln(a.Stderr, err)
		return graph.Graph{}, nil, exitcode.Fail
	}
	a.log().Info(fmt.Sprintf("compile: %d nodes, %d edges", len(g.Nodes), len(g.Edges)))
	apis, err := a.Ext.ExtractAPIs(snap)
	if err != nil {
		fmt.Fprintln(a.Stderr, err)
		return graph.Graph{}, nil, exitcode.Fail
	}
	return g, apis, exitcode.OK
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

	g, apis, code := a.structureAt(to)
	if code != exitcode.OK {
		return code
	}

	sum, err := fingerprint.Hash(g, apis)
	if err != nil {
		fmt.Fprintln(a.Stderr, err)
		return exitcode.Fail
	}
	lf, err := fingerprint.Load(filepath.Join(a.Repo.Root, ".archon", "graph.json"))
	if err != nil {
		a.log().Info("compare .archon/graph.json: stale")
		fmt.Fprintln(a.Stderr, "architecture fingerprint is stale or missing; run archon sync")
		return exitcode.Gate
	}
	if lf.Hash != sum {
		a.log().Info("compare .archon/graph.json: stale")
		fmt.Fprintln(a.Stderr, "architecture fingerprint is stale or missing; run archon sync")
		return exitcode.Gate
	}
	a.log().Info("compare .archon/graph.json: match")
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
		a.log().Info("resolve from: empty (same commit)")
		fmt.Fprint(a.Stdout, fingerprint.FormatStructure(graph.Diff{}, fingerprint.APIDiff{}))
		return exitcode.OK
	}
	a.log().Info("resolve from: " + fr.From)

	fromG, fromAPIs, code := a.structureAt(fr.From)
	if code != exitcode.OK {
		return code
	}
	toG, toAPIs, code := a.structureAt(to)
	if code != exitcode.OK {
		return code
	}

	d := graph.DiffGraphs(fromG, toG)
	ad := fingerprint.DiffAPIs(fromAPIs, toAPIs)
	fmt.Fprint(a.Stdout, fingerprint.FormatStructure(d, ad))
	if d.Empty() && ad.Empty() {
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
		a.log().Info("resolve from: empty (same commit)")
		fmt.Fprint(a.Stdout, fingerprint.FormatStructure(graph.Diff{}, fingerprint.APIDiff{}))
		return exitcode.OK
	}
	a.log().Info("resolve from: " + fr.From)

	fromG, fromAPIs, code := a.structureAt(fr.From)
	if code != exitcode.OK {
		return code
	}
	toG, toAPIs, code := a.structureAt(to)
	if code != exitcode.OK {
		return code
	}

	d := graph.DiffGraphs(fromG, toG)
	ad := fingerprint.DiffAPIs(fromAPIs, toAPIs)
	fmt.Fprint(a.Stdout, fingerprint.FormatStructure(d, ad))
	if d.Empty() && ad.Empty() {
		return exitcode.OK
	}
	return exitcode.Gate
}

func (a *App) Sync() int {
	a.attachLog()
	to := a.To
	if to == "" {
		to = "HEAD"
	}

	toG, _, code := a.structureAt(to)
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
	a.log().Info("write " + a.Doc)

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
		a.log().Info("resolve from: empty (same commit)")
		fmt.Fprint(a.Stdout, graph.FormatReport(graph.Diff{}))
		return exitcode.OK
	}
	a.log().Info("resolve from: " + fr.From)

	fromG, _, code := a.structureAt(fr.From)
	if code != exitcode.OK {
		return exitcode.OK
	}

	d := graph.DiffGraphs(fromG, toG)
	if d.Empty() {
		fmt.Fprint(a.Stdout, graph.FormatReport(d))
		return exitcode.OK
	}
	if a.LLM == nil {
		a.log().Info("llm: skip (no client)")
		fmt.Fprint(a.Stdout, graph.FormatReport(d))
		return exitcode.OK
	}

	subjects, err := a.Repo.LogSubjects(fr.From, to, 50, 8192)
	if err != nil {
		a.log().Info("llm: skip (subjects)")
		fmt.Fprintln(a.Stderr, "warning:", err)
		fmt.Fprint(a.Stdout, graph.FormatReport(d))
		return exitcode.OK
	}
	delta, err := a.LLM.Delta(context.Background(), d, subjects)
	if err != nil {
		a.log().Info("llm: skip (error)")
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
