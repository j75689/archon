package app

import (
	"bytes"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/j75689/archon/internal/config"
	"github.com/j75689/archon/internal/exitcode"
	"github.com/j75689/archon/internal/fingerprint"
	"github.com/j75689/archon/internal/git"
	"github.com/j75689/archon/internal/graph"
	"github.com/j75689/archon/internal/lang"
	"github.com/j75689/archon/internal/lang/golang"
	"github.com/j75689/archon/internal/llm"
	"github.com/j75689/archon/internal/log"
)

func gitOK(t *testing.T) {
	t.Helper()
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git not on PATH")
	}
}

func runGit(t *testing.T, dir string, args ...string) {
	t.Helper()
	cmd := exec.Command("git", append([]string{
		"-c", "user.email=t@t.t",
		"-c", "user.name=t",
	}, args...)...)
	cmd.Dir = dir
	cmd.Env = append(
		os.Environ(),
		"GIT_AUTHOR_DATE=2020-01-01T00:00:00Z",
		"GIT_COMMITTER_DATE=2020-01-01T00:00:00Z",
	)
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("git %v: %v\n%s", args, err, out)
	}
}

func tryGit(dir string, args ...string) error {
	cmd := exec.Command("git", args...)
	cmd.Dir = dir
	cmd.Env = append(
		os.Environ(),
		"GIT_AUTHOR_DATE=2020-01-01T00:00:00Z",
		"GIT_COMMITTER_DATE=2020-01-01T00:00:00Z",
	)
	_, err := cmd.CombinedOutput()
	return err
}

func initEmptyRepo(t *testing.T) string {
	t.Helper()
	gitOK(t)

	dir := t.TempDir()
	if err := tryGit(dir, "init", "-b", "main"); err != nil {
		runGit(t, dir, "init")
		runGit(t, dir, "checkout", "-b", "main")
	}
	return dir
}

func initRepo(t *testing.T) string {
	t.Helper()
	dir := initEmptyRepo(t)
	mustCommitFile(t, dir, "go.mod", "module example.com/m\n", "init")
	mustCommitFile(t, dir, "a.go", "package m\n", "a")
	return dir
}

func mustCommitFile(t *testing.T, dir, name, body, msg string) {
	t.Helper()
	p := filepath.Join(dir, name)
	if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(p, []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
	runGit(t, dir, "add", name)
	runGit(t, dir, "commit", "-m", msg)
}

func newApp(t *testing.T, dir string) (*App, *bytes.Buffer, *bytes.Buffer) {
	t.Helper()
	r, err := git.Open(dir)
	if err != nil {
		t.Fatal(err)
	}
	out, errb := new(bytes.Buffer), new(bytes.Buffer)
	a := New(r)
	a.Stdout, a.Stderr = out, errb
	return a, out, errb
}

func TestDiffNoTag(t *testing.T) {
	dir := initRepo(t)

	a, _, errb := newApp(t, dir)
	if code := a.Diff(); code != exitcode.Fail {
		t.Fatalf("code %d stderr %s", code, errb)
	}
	if !strings.Contains(errb.String(), "pass --from") {
		t.Fatalf("stderr %s", errb)
	}
}

func TestDiffAddedIsolatedPackage(t *testing.T) {
	dir := initRepo(t)
	runGit(t, dir, "tag", "v1.0.0")
	mustCommitFile(t, dir, "b/b.go", "package b\n", "b")

	a, out, _ := newApp(t, dir)
	if code := a.Diff(); code != exitcode.Gate {
		t.Fatalf("code %d out %s", code, out)
	}
	if !strings.Contains(out.String(), "example.com/m/b") {
		t.Fatalf("out %s", out)
	}
}

func TestDiffPrivateBodyOnly(t *testing.T) {
	dir := initRepo(t)
	mustCommitFile(t, dir, "a.go", "package m\nfunc unexported() { println(1) }\n", "body1")
	runGit(t, dir, "tag", "v1.0.0")
	mustCommitFile(t, dir, "a.go", "package m\nfunc unexported() { println(2) }\n", "body2")

	a, out, _ := newApp(t, dir)
	if code := a.Diff(); code != exitcode.OK {
		t.Fatalf("code %d out %s", code, out)
	}
	if out.String() != "No first-party structure changes.\n" {
		t.Fatalf("out %q", out.String())
	}
}

func TestDiffExportedSignatureChange(t *testing.T) {
	dir := initRepo(t)
	mustCommitFile(t, dir, "a.go", "package m\nfunc Hello() {}\n", "export")
	runGit(t, dir, "tag", "v1.0.0")
	mustCommitFile(t, dir, "a.go", "package m\nfunc Hello(name string) {}\n", "change signature")

	a, out, _ := newApp(t, dir)
	if code := a.Diff(); code != exitcode.Gate {
		t.Fatalf("code %d out %s", code, out)
	}
	for _, want := range []string{
		"added signatures:\n- example.com/m: func Hello(name string)",
		"removed signatures:\n- example.com/m: func Hello()",
	} {
		if !strings.Contains(out.String(), want) {
			t.Fatalf("missing %q in %q", want, out.String())
		}
	}
}

func TestCheckLockfileMatchAndStale(t *testing.T) {
	dir := initRepo(t)

	a, _, errb := newApp(t, dir)
	if code := a.Check(); code != exitcode.Gate {
		t.Fatalf("missing lockfile code %d %s", code, errb)
	}
	if !strings.Contains(errb.String(), "archon sync") {
		t.Fatalf("stderr %s", errb)
	}

	writeHEADLockfile(t, dir)

	a, _, errb = newApp(t, dir)
	a.From = "HEAD"
	if code := a.Check(); code != exitcode.OK {
		t.Fatalf("match code %d %s", code, errb)
	}
	if !strings.Contains(errb.String(), "--from is ignored") {
		t.Fatalf("stderr %s", errb)
	}

	mustCommitFile(t, dir, "a.go", "package m\nfunc Hello() {}\n", "export")
	a, _, errb = newApp(t, dir)
	if code := a.Check(); code != exitcode.Gate {
		t.Fatalf("stale after export code %d %s", code, errb)
	}
}

func TestCheckPrivateBodyDoesNotStale(t *testing.T) {
	dir := initRepo(t)
	mustCommitFile(t, dir, "a.go", "package m\nfunc unexported() { println(1) }\n", "b1")
	writeHEADLockfile(t, dir)
	mustCommitFile(t, dir, "a.go", "package m\nfunc unexported() { println(2) }\n", "b2")

	a, _, errb := newApp(t, dir)
	if code := a.Check(); code != exitcode.OK {
		t.Fatalf("code %d %s", code, errb)
	}
}

func TestCheckVerboseMatchAndStale(t *testing.T) {
	dir := initRepo(t)
	writeHEADLockfile(t, dir)

	a, out, errb := newApp(t, dir)
	a.Log = log.Writer{W: errb, Level: 1}
	if code := a.Check(); code != exitcode.OK {
		t.Fatalf("match code %d stderr %s", code, errb)
	}
	if out.Len() != 0 {
		t.Fatalf("stdout must stay empty: %q", out)
	}
	got := errb.String()
	for _, want := range []string{
		"snapshot HEAD:",
		"extract go:",
		"compile:",
		"compare .archon/graph.json: match",
	} {
		if !strings.Contains(got, want) {
			t.Fatalf("missing %q in %q", want, got)
		}
	}
	if strings.Contains(got, "snapshot: a.go") {
		t.Fatalf("level 1 must not include per-file: %q", got)
	}

	mustCommitFile(t, dir, "a.go", "package m\nfunc Hello() {}\n", "export")
	a, _, errb = newApp(t, dir)
	a.Log = log.Writer{W: errb, Level: 1}
	if code := a.Check(); code != exitcode.Gate {
		t.Fatalf("stale code %d", code)
	}
	if !strings.Contains(errb.String(), "compare .archon/graph.json: stale") {
		t.Fatalf("missing stale compare: %q", errb)
	}
	if !strings.Contains(errb.String(), "architecture fingerprint is stale or missing; run archon sync") {
		t.Fatalf("missing gate line: %q", errb)
	}
}

func TestCheckVerboseLevel2IncludesSnapshotPath(t *testing.T) {
	dir := initRepo(t)
	writeHEADLockfile(t, dir)

	a, _, errb := newApp(t, dir)
	a.Log = log.Writer{W: errb, Level: 2}
	if code := a.Check(); code != exitcode.OK {
		t.Fatalf("check %d %s", code, errb)
	}
	if !strings.Contains(errb.String(), "snapshot: a.go") {
		t.Fatalf("level 2 missing path: %q", errb)
	}
}

func TestCheckMissingLockfileLogsStaleCompare(t *testing.T) {
	dir := initRepo(t)
	a, _, errb := newApp(t, dir)
	a.Log = log.Writer{W: errb, Level: 1}
	if code := a.Check(); code != exitcode.Gate {
		t.Fatalf("code %d", code)
	}
	if !strings.Contains(errb.String(), "compare .archon/graph.json: stale") {
		t.Fatalf("stderr %q", errb)
	}
}

func TestCheckPassesWithLockfileWithoutTag(t *testing.T) {
	dir := initRepo(t)
	writeHEADLockfile(t, dir)
	a, _, errb := newApp(t, dir)
	if code := a.Check(); code != exitcode.OK {
		t.Fatalf("check code %d stderr %s", code, errb)
	}
}

func writeHEADLockfile(t *testing.T, dir string) {
	t.Helper()
	r, err := git.Open(dir)
	if err != nil {
		t.Fatal(err)
	}
	snap, err := r.Snapshot("HEAD", golang.WantFile)
	if err != nil {
		t.Fatal(err)
	}
	g, err := graph.Compile(mustExtract(t, snap))
	if err != nil {
		t.Fatal(err)
	}
	apis, err := (golang.Extractor{}).ExtractAPIs(snap)
	if err != nil {
		t.Fatal(err)
	}
	lf, err := fingerprint.NewLockfile(g, apis)
	if err != nil {
		t.Fatal(err)
	}
	if err := fingerprint.Write(filepath.Join(dir, ".archon", "graph.json"), lf); err != nil {
		t.Fatal(err)
	}
}

func TestSyncNoOpWhenLockfileCurrent(t *testing.T) {
	dir := initRepo(t)
	writeHEADLockfile(t, dir)
	var n int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		atomic.AddInt32(&n, 1)
	}))
	defer srv.Close()
	a, _, _ := newApp(t, dir)
	a.LLM = &llm.Client{BaseURL: srv.URL, Model: "t", HTTP: srv.Client()}
	if code := a.Sync(); code != exitcode.OK {
		t.Fatal(code)
	}
	if atomic.LoadInt32(&n) != 0 {
		t.Fatalf("requests %d", n)
	}
}

func TestSyncWritesGeneratorsAndLockfile(t *testing.T) {
	dir := initRepo(t)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		body, _ := io.ReadAll(r.Body)
		if strings.Contains(string(body), "func hidden") || strings.Contains(string(body), "println") {
			t.Error("leaked body")
		}
		_, _ = w.Write([]byte(`{"choices":[{"message":{"content":"# doc\n"}}]}`))
	}))
	defer srv.Close()
	a, _, errb := newApp(t, dir)
	a.Log = log.Writer{W: errb, Level: 1}
	a.LLM = &llm.Client{BaseURL: srv.URL, Model: "t", HTTP: srv.Client()}
	if code := a.Sync(); code != exitcode.OK {
		t.Fatalf("%d %s", code, errb)
	}
	for _, p := range []string{"docs/ARCHITECTURE.md", "docs/WORKFLOW.md", "docs/PACKAGES.md"} {
		b, err := os.ReadFile(filepath.Join(dir, p))
		if err != nil {
			t.Fatal(err)
		}
		if string(b) != "# doc\n" {
			t.Fatalf("%s %q", p, b)
		}
		if !strings.Contains(errb.String(), "write "+p) {
			t.Fatalf("missing write %s: %q", p, errb)
		}
	}
	if _, err := os.Stat(filepath.Join(dir, ".archon", "graph.json")); err != nil {
		t.Fatal(err)
	}
	a2, _, _ := newApp(t, dir)
	if code := a2.Check(); code != exitcode.OK {
		t.Fatalf("check %d", code)
	}
}

func TestSyncLLMErrorLeavesFilesUntouched(t *testing.T) {
	dir := initRepo(t)
	docPath := filepath.Join(dir, "docs", "ARCHITECTURE.md")
	if err := os.MkdirAll(filepath.Dir(docPath), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(docPath, []byte("keep\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "boom", http.StatusInternalServerError)
	}))
	defer srv.Close()
	a, _, errb := newApp(t, dir)
	a.LLM = &llm.Client{BaseURL: srv.URL, Model: "t", HTTP: srv.Client()}
	if code := a.Sync(); code != exitcode.OK {
		t.Fatal(code)
	}
	b, _ := os.ReadFile(docPath)
	if string(b) != "keep\n" {
		t.Fatalf("overwrote %q", b)
	}
	if _, err := os.Stat(filepath.Join(dir, ".archon", "graph.json")); !os.IsNotExist(err) {
		t.Fatalf("lockfile written: %v", err)
	}
	if !strings.Contains(errb.String(), "llm") {
		t.Fatalf("stderr %q", errb)
	}
}

func TestSyncSkipNoClientDoesNotWriteLockfile(t *testing.T) {
	dir := initRepo(t)
	a, _, errb := newApp(t, dir)
	if code := a.Sync(); code != exitcode.OK {
		t.Fatal(code)
	}
	if !strings.Contains(errb.String(), "llm: skip (no client)") {
		t.Fatalf("stderr %q", errb)
	}
	if _, err := os.Stat(filepath.Join(dir, ".archon", "graph.json")); !os.IsNotExist(err) {
		t.Fatal("lockfile")
	}
}

func TestSyncUserPromptAppearsInRequest(t *testing.T) {
	dir := initRepo(t)
	if err := os.WriteFile(filepath.Join(dir, "custom.md"), []byte("UNIQUE_PROMPT_TOKEN\n{{.Graph}}"), 0o644); err != nil {
		t.Fatal(err)
	}
	var got string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		b, _ := io.ReadAll(r.Body)
		got = string(b)
		_, _ = w.Write([]byte(`{"choices":[{"message":{"content":"ok"}}]}`))
	}))
	defer srv.Close()
	a, _, _ := newApp(t, dir)
	a.Generators = []config.Generator{{ID: "architecture", Path: "docs/ARCHITECTURE.md", Prompt: "custom.md"}}
	a.LLM = &llm.Client{BaseURL: srv.URL, Model: "t", HTTP: srv.Client()}
	if code := a.Sync(); code != exitcode.OK {
		t.Fatal(code)
	}
	if !strings.Contains(got, "UNIQUE_PROMPT_TOKEN") {
		t.Fatalf("request %s", got)
	}
}

func TestSyncSkipsCustomGeneratorWithoutPrompt(t *testing.T) {
	dir := initRepo(t)
	if err := os.MkdirAll(filepath.Join(dir, "prompts"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "prompts", "adr.md"), []byte("ADR_PROMPT\n{{.Graph}}"), 0o644); err != nil {
		t.Fatal(err)
	}

	var n int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		atomic.AddInt32(&n, 1)
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"choices":[{"message":{"content":"# doc\n"}}]}`))
	}))
	defer srv.Close()

	a, _, errb := newApp(t, dir)
	a.Generators = []config.Generator{
		{ID: "architecture", Path: "docs/ARCHITECTURE.md"},
		{ID: "adr", Path: "docs/ADR.md", Prompt: "prompts/adr.md"},
		{ID: "notes", Path: "docs/NOTES.md"},
	}
	a.LLM = &llm.Client{BaseURL: srv.URL, Model: "t", HTTP: srv.Client()}
	if code := a.Sync(); code != exitcode.OK {
		t.Fatalf("code %d stderr %s", code, errb)
	}
	if got := atomic.LoadInt32(&n); got != 2 {
		t.Fatalf("LLM requests %d, want 2", got)
	}
	for _, p := range []string{"docs/ARCHITECTURE.md", "docs/ADR.md"} {
		b, err := os.ReadFile(filepath.Join(dir, p))
		if err != nil {
			t.Fatal(err)
		}
		if string(b) != "# doc\n" {
			t.Fatalf("%s %q", p, b)
		}
	}
	if _, err := os.Stat(filepath.Join(dir, "docs", "NOTES.md")); !os.IsNotExist(err) {
		t.Fatalf("NOTES.md written: %v", err)
	}
	if !strings.Contains(errb.String(), "generator notes: skip (no prompt)") {
		t.Fatalf("stderr %q", errb)
	}
	if strings.Contains(errb.String(), "generator architecture: skip") || strings.Contains(errb.String(), "generator adr: skip") {
		t.Fatalf("skipped a runnable generator: %q", errb)
	}
	if _, err := os.Stat(filepath.Join(dir, ".archon", "graph.json")); err != nil {
		t.Fatal(err)
	}
}

func TestSyncCustomPromptMissingFileFails(t *testing.T) {
	dir := initRepo(t)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		t.Error("LLM must not be called")
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"choices":[{"message":{"content":"# doc\n"}}]}`))
	}))
	defer srv.Close()

	a, _, errb := newApp(t, dir)
	a.Generators = []config.Generator{
		{ID: "adr", Path: "docs/ADR.md", Prompt: "prompts/missing.md"},
	}
	a.LLM = &llm.Client{BaseURL: srv.URL, Model: "t", HTTP: srv.Client()}
	if code := a.Sync(); code != exitcode.Fail {
		t.Fatalf("code %d stderr %s", code, errb)
	}
	if !strings.Contains(errb.String(), "read prompt") {
		t.Fatalf("stderr %q", errb)
	}
	if _, err := os.Stat(filepath.Join(dir, ".archon", "graph.json")); !os.IsNotExist(err) {
		t.Fatalf("lockfile written: %v", err)
	}
}

func TestChangelogPrintsReportWithoutLLM(t *testing.T) {
	dir := initRepo(t)
	runGit(t, dir, "tag", "v1.0.0")
	mustCommitFile(t, dir, "b/b.go", "package b\n", "add b")

	var requests int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		atomic.AddInt32(&requests, 1)
		http.Error(w, "should not happen", http.StatusInternalServerError)
	}))
	defer srv.Close()

	a, out, errb := newApp(t, dir)
	a.LLM = &llm.Client{BaseURL: srv.URL, Model: "gpt-test", HTTP: srv.Client()}
	if code := a.Changelog(); code != exitcode.Gate {
		t.Fatalf("code %d stdout %s stderr %s", code, out, errb)
	}
	if !strings.Contains(out.String(), "added nodes:") {
		t.Fatalf("stdout %q", out.String())
	}
	if strings.Contains(errb.String(), "llm") {
		t.Fatalf("stderr %q", errb.String())
	}
	if got := atomic.LoadInt32(&requests); got != 0 {
		t.Fatalf("requests = %d", got)
	}
}

func TestChangelogSkipsLLMForPrivateOnlyChange(t *testing.T) {
	dir := initRepo(t)
	mustCommitFile(t, dir, "a.go", "package m\nfunc unexported() { println(1) }\n", "body1")
	runGit(t, dir, "tag", "v1.0.0")
	mustCommitFile(t, dir, "a.go", "package m\nfunc unexported() { println(2) }\n", "body2")

	var requests int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		atomic.AddInt32(&requests, 1)
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"choices":[{"message":{"content":"should not happen"}}]}`))
	}))
	defer srv.Close()

	a, out, errb := newApp(t, dir)
	a.LLM = &llm.Client{BaseURL: srv.URL, Model: "gpt-test", HTTP: srv.Client()}
	if code := a.Changelog(); code != exitcode.OK {
		t.Fatalf("code %d stdout %s stderr %s", code, out, errb)
	}
	if out.String() != "No first-party structure changes.\n" {
		t.Fatalf("stdout %q", out.String())
	}
	if got := atomic.LoadInt32(&requests); got != 0 {
		t.Fatalf("requests = %d", got)
	}
}

func TestDiffVerboseResolveFromAndEmpty(t *testing.T) {
	dir := initRepo(t)
	runGit(t, dir, "tag", "v1.0.0")
	a, _, errb := newApp(t, dir)
	a.Log = log.Writer{W: errb, Level: 1}
	if code := a.Diff(); code != exitcode.OK {
		t.Fatalf("empty diff code %d %s", code, errb)
	}
	if !strings.Contains(errb.String(), "resolve from: empty (same commit)") {
		t.Fatalf("stderr %q", errb)
	}

	mustCommitFile(t, dir, "b/b.go", "package b\n", "b")
	a, out, errb := newApp(t, dir)
	a.Log = log.Writer{W: errb, Level: 1}
	if code := a.Diff(); code != exitcode.Gate {
		t.Fatalf("code %d out %s stderr %s", code, out, errb)
	}
	if !strings.Contains(errb.String(), "resolve from: v1.0.0") {
		t.Fatalf("stderr %q", errb)
	}
}

func TestSyncVerboseSkipNoClient(t *testing.T) {
	dir := initRepo(t)
	a, _, errb := newApp(t, dir)
	a.Log = log.Writer{W: errb, Level: 1}
	if code := a.Sync(); code != exitcode.OK {
		t.Fatalf("code %d %s", code, errb)
	}
	if !strings.Contains(errb.String(), "llm: skip (no client)") {
		t.Fatalf("missing skip: %q", errb)
	}
}

func mustExtract(t *testing.T, snap lang.Snapshot) graph.Graph {
	t.Helper()
	var e golang.Extractor
	g, err := e.Extract(snap)
	if err != nil {
		t.Fatal(err)
	}
	return g
}
