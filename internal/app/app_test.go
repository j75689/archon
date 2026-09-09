package app

import (
	"bytes"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/j75689/archon/internal/exitcode"
	"github.com/j75689/archon/internal/git"
	"github.com/j75689/archon/internal/graph"
	"github.com/j75689/archon/internal/lang"
	"github.com/j75689/archon/internal/lang/golang"
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

func mustExtract(t *testing.T, snap lang.Snapshot) graph.Graph {
	t.Helper()
	var e golang.Extractor
	g, err := e.Extract(snap)
	if err != nil {
		t.Fatal(err)
	}
	return g
}
