package app

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"

	"strings"

	"github.com/j75689/archon/internal/config"
	"github.com/j75689/archon/internal/exitcode"
	"github.com/j75689/archon/internal/fingerprint"
	"github.com/j75689/archon/internal/git"
	"github.com/j75689/archon/internal/graph"
	"github.com/j75689/archon/internal/lang"
	"github.com/j75689/archon/internal/lang/golang"
	"github.com/j75689/archon/internal/llm"
	"github.com/j75689/archon/internal/log"
	"github.com/j75689/archon/internal/prompt"
)

type App struct {
	Repo       *git.Repo
	Ext        lang.Extractor
	From       string
	To         string
	Doc        string
	Anchor     string
	Model      string
	BaseURL    string
	APIKey     string
	LLM        *llm.Client
	Log        log.Logger
	Stdout     io.Writer
	Stderr     io.Writer
	Generators []config.Generator
}

func New(repo *git.Repo) *App {
	return &App{
		Repo:       repo,
		Ext:        golang.Extractor{},
		To:         "HEAD",
		Doc:        "docs/ARCHITECTURE.md",
		Anchor:     "data-flow",
		Model:      "gpt-4o-mini",
		BaseURL:    "https://api.openai.com/v1",
		Stdout:     os.Stdout,
		Stderr:     os.Stderr,
		Generators: config.DefaultGenerators,
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
	if a.From != "" {
		fmt.Fprintln(a.Stderr, "warning: --from is ignored by sync")
	}

	to := a.To
	if to == "" {
		to = "HEAD"
	}

	toG, toAPIs, code := a.structureAt(to)
	if code != exitcode.OK {
		return code
	}

	sum, err := fingerprint.Hash(toG, toAPIs)
	if err != nil {
		fmt.Fprintln(a.Stderr, err)
		return exitcode.Fail
	}

	lockPath := filepath.Join(a.Repo.Root, ".archon", "graph.json")
	lf, err := fingerprint.Load(lockPath)
	if err == nil && lf.Hash == sum {
		return exitcode.OK
	}

	if a.LLM == nil {
		a.log().Info("llm: skip (no client)")
		fmt.Fprintln(a.Stderr, "llm: skip (no client)")
		return exitcode.OK
	}

	prevG := fingerprint.GraphFromLock(lf)
	diffText := fingerprint.FormatStructure(graph.DiffGraphs(prevG, toG), fingerprint.DiffAPIs(lf.APIs, toAPIs))
	data := prompt.Data{
		Graph: formatGraph(toG),
		APIs:  formatAPIs(toAPIs),
		Diff:  diffText,
	}

	gens := a.Generators
	if len(gens) == 0 {
		gens = config.DefaultGenerators
	}

	type pending struct {
		rel  string
		dest string
		body string
	}
	var files []pending
	for _, gen := range gens {
		if !generatorRunnable(gen) {
			msg := fmt.Sprintf("generator %s: skip (no prompt)", gen.ID)
			a.log().Info(msg)
			fmt.Fprintln(a.Stderr, msg)
			continue
		}
		tmpl, err := prompt.Load(a.Repo.Root, gen, os.ReadFile)
		if err != nil {
			fmt.Fprintln(a.Stderr, err)
			return exitcode.Fail
		}
		user, err := prompt.Render(tmpl, data)
		if err != nil {
			fmt.Fprintln(a.Stderr, err)
			return exitcode.Fail
		}
		content, err := a.LLM.Complete(context.Background(), user)
		if err != nil {
			a.log().Info("llm: skip (error)")
			fmt.Fprintln(a.Stderr, llm.FormatDeltaError(err))
			return exitcode.OK
		}
		files = append(files, pending{
			rel:  gen.Path,
			dest: filepath.Join(a.Repo.Root, filepath.FromSlash(gen.Path)),
			body: ensureTrailingNL(content),
		})
	}

	var temps []string
	cleanup := func() {
		for _, tmp := range temps {
			_ = os.Remove(tmp)
		}
	}
	for _, f := range files {
		if err := os.MkdirAll(filepath.Dir(f.dest), 0o755); err != nil {
			cleanup()
			fmt.Fprintln(a.Stderr, llm.FormatDeltaError(err))
			return exitcode.OK
		}
		tmp := f.dest + ".tmp"
		if err := os.WriteFile(tmp, []byte(f.body), 0o644); err != nil {
			cleanup()
			_ = os.Remove(tmp)
			fmt.Fprintln(a.Stderr, llm.FormatDeltaError(err))
			return exitcode.OK
		}
		temps = append(temps, tmp)
	}
	for i, f := range files {
		if err := os.Rename(temps[i], f.dest); err != nil {
			cleanup()
			fmt.Fprintln(a.Stderr, llm.FormatDeltaError(err))
			return exitcode.OK
		}
		temps[i] = ""
		a.log().Info("write " + f.rel)
	}

	newLF, err := fingerprint.NewLockfile(toG, toAPIs)
	if err != nil {
		fmt.Fprintln(a.Stderr, llm.FormatDeltaError(err))
		return exitcode.OK
	}
	if err := fingerprint.Write(lockPath, newLF); err != nil {
		fmt.Fprintln(a.Stderr, llm.FormatDeltaError(err))
		return exitcode.OK
	}
	return exitcode.OK
}

func formatGraph(g graph.Graph) string {
	var b strings.Builder
	for _, n := range g.Nodes {
		b.WriteString(n.Key)
		b.WriteByte('\n')
	}
	for _, e := range g.Edges {
		b.WriteString(e.From)
		b.WriteString(" -> ")
		b.WriteString(e.To)
		b.WriteByte('\n')
	}
	return b.String()
}

func formatAPIs(apis lang.APISet) string {
	var b strings.Builder
	for _, api := range apis {
		b.WriteString(api.Key)
		b.WriteByte('\n')
		if api.Doc != "" {
			b.WriteString(" ")
			b.WriteString(api.Doc)
			b.WriteByte('\n')
		}
		for _, sig := range api.Signatures {
			b.WriteString(" - ")
			b.WriteString(sig)
			b.WriteByte('\n')
		}
	}
	return b.String()
}

func ensureTrailingNL(s string) string {
	if strings.HasSuffix(s, "\n") {
		return s
	}
	return s + "\n"
}

func generatorRunnable(gen config.Generator) bool {
	if gen.Prompt != "" {
		return true
	}
	_, ok := prompt.BuiltIn(gen.ID)
	return ok
}
