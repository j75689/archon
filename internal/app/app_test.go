package app

import (
	"bytes"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/j75689/archon/internal/doc"
	"github.com/j75689/archon/internal/exitcode"
	"github.com/j75689/archon/internal/git"
	"github.com/j75689/archon/internal/graph"
	"github.com/j75689/archon/internal/lang"
	"github.com/j75689/archon/internal/lang/golang"
	"github.com/j75689/archon/internal/llm"
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
	if out.String() != "No first-party dependency changes.\n" {
		t.Fatalf("out %q", out.String())
	}
}

func TestCheckDocPathNotFile(t *testing.T) {
	dir := initRepo(t)

	docPath := filepath.Join(dir, "docs", "ARCHITECTURE.md")
	if err := os.MkdirAll(docPath, 0o755); err != nil {
		t.Fatal(err)
	}

	a, _, errb := newApp(t, dir)
	if code := a.Check(); code != exitcode.Fail {
		t.Fatalf("doc is directory: code %d stderr %s", code, errb)
	}
	if strings.Contains(errb.String(), "archon sync") {
		t.Fatalf("expected read error not gate message, got %s", errb)
	}
}

func TestCheckMissingAndMatch(t *testing.T) {
	dir := initRepo(t)

	a, _, errb := newApp(t, dir)
	if code := a.Check(); code != exitcode.Gate {
		t.Fatalf("missing doc code %d %s", code, errb)
	}

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
	payload := graph.RenderMermaid(g)
	body := "# Architecture\n\n<!-- ARCHON:START:data-flow -->\n" + payload + "<!-- ARCHON:END:data-flow -->\n"
	if err := os.MkdirAll(filepath.Join(dir, "docs"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "docs", "ARCHITECTURE.md"), []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}

	a, _, errb = newApp(t, dir)
	if code := a.Check(); code != exitcode.OK {
		t.Fatalf("match code %d %s", code, errb)
	}

	stale := strings.Replace(body, payload, "```mermaid\nflowchart LR\n```\n", 1)
	if err := os.WriteFile(filepath.Join(dir, "docs", "ARCHITECTURE.md"), []byte(stale), 0o644); err != nil {
		t.Fatal(err)
	}

	a, _, errb = newApp(t, dir)
	a.From = "HEAD"
	if code := a.Check(); code != exitcode.Gate {
		t.Fatalf("stale code %d", code)
	}
	if !strings.Contains(errb.String(), "archon sync") {
		t.Fatalf("stderr %s", errb)
	}
	if !strings.Contains(errb.String(), "--from is ignored") {
		t.Fatalf("stderr %s", errb)
	}
}

func TestSyncCreatesDocAndCheckPassesWithoutTag(t *testing.T) {
	dir := initRepo(t)

	a, out, errb := newApp(t, dir)
	if code := a.Sync(); code != exitcode.OK {
		t.Fatalf("sync code %d stdout %s stderr %s", code, out, errb)
	}
	if !strings.Contains(errb.String(), "pass --from") {
		t.Fatalf("stderr %q", errb.String())
	}

	body, err := os.ReadFile(filepath.Join(dir, "docs", "ARCHITECTURE.md"))
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(body), "\r") {
		t.Fatalf("doc must use LF, got %q", string(body))
	}

	a, _, errb = newApp(t, dir)
	if code := a.Check(); code != exitcode.OK {
		t.Fatalf("check code %d stderr %s", code, errb)
	}
}

func TestSyncPreservesRationaleAndReplacesAnchoredRegion(t *testing.T) {
	dir := initRepo(t)

	docPath := filepath.Join(dir, "docs", "ARCHITECTURE.md")
	if err := os.MkdirAll(filepath.Dir(docPath), 0o755); err != nil {
		t.Fatal(err)
	}
	const rationale = "# Architecture\n\nThis rationale must survive.\n\n"
	const stale = "<!-- ARCHON:START:data-flow -->\n```mermaid\nflowchart LR\n  stale[\"stale\"]\n```\n<!-- ARCHON:END:data-flow -->\n"
	if err := os.WriteFile(docPath, []byte(rationale+stale), 0o644); err != nil {
		t.Fatal(err)
	}

	a, out, errb := newApp(t, dir)
	if code := a.Sync(); code != exitcode.OK {
		t.Fatalf("sync code %d stdout %s stderr %s", code, out, errb)
	}

	body, err := os.ReadFile(docPath)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(body), "This rationale must survive.") {
		t.Fatalf("rationale lost: %s", body)
	}
	region, ok, err := doc.ExtractRegion(body, "data-flow")
	if err != nil || !ok {
		t.Fatalf("extract: ok=%v err=%v", ok, err)
	}

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
	want := strings.TrimSuffix(graph.RenderMermaid(g), "\n")
	if region != want {
		t.Fatalf("region %q want %q", region, want)
	}
}

func TestChangelogPrintsReportWithoutLLM(t *testing.T) {
	dir := initRepo(t)
	runGit(t, dir, "tag", "v1.0.0")
	mustCommitFile(t, dir, "b/b.go", "package b\n", "add b")

	a, out, errb := newApp(t, dir)
	if code := a.Changelog(); code != exitcode.OK {
		t.Fatalf("code %d stdout %s stderr %s", code, out, errb)
	}
	if !strings.Contains(out.String(), "added nodes:") {
		t.Fatalf("stdout %q", out.String())
	}
	if strings.Contains(errb.String(), "llm") {
		t.Fatalf("stderr %q", errb.String())
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
	if out.String() != "No first-party dependency changes.\n" {
		t.Fatalf("stdout %q", out.String())
	}
	if got := atomic.LoadInt32(&requests); got != 0 {
		t.Fatalf("requests = %d", got)
	}
}

func TestSyncFallsBackToReportWhenLLMFails(t *testing.T) {
	dir := initRepo(t)
	runGit(t, dir, "tag", "v1.0.0")
	mustCommitFile(t, dir, "b/b.go", "package b\n", "add b")

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "boom", http.StatusInternalServerError)
	}))
	defer srv.Close()

	a, out, errb := newApp(t, dir)
	a.LLM = &llm.Client{BaseURL: srv.URL, Model: "gpt-test", HTTP: srv.Client()}
	if code := a.Sync(); code != exitcode.OK {
		t.Fatalf("code %d stdout %s stderr %s", code, out, errb)
	}
	if !strings.Contains(out.String(), "added nodes:") {
		t.Fatalf("stdout %q", out.String())
	}
	if !strings.Contains(errb.String(), "llm") {
		t.Fatalf("stderr %q", errb.String())
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
