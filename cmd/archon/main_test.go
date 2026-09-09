package main

import (
	"bytes"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/j75689/archon/internal/exitcode"
)

func TestRunDiffNoRepo(t *testing.T) {
	dir, err := os.MkdirTemp("/private/tmp", "archon-norepo-*")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.RemoveAll(dir) })
	t.Chdir(dir)

	var stdout, stderr bytes.Buffer
	code := run([]string{"diff"}, &stdout, &stderr)
	if code != exitcode.Fail {
		t.Fatalf("code = %d, stderr = %q", code, stderr.String())
	}
	if stdout.Len() != 0 {
		t.Fatalf("stdout = %q", stdout.String())
	}
	if !strings.Contains(stderr.String(), "not a git repository") {
		t.Fatalf("stderr = %q", stderr.String())
	}
}

func TestRunSyncSucceedsWithoutTagWhileDiffFails(t *testing.T) {
	dir := initRepo(t)
	t.Chdir(dir)

	var syncStdout, syncStderr bytes.Buffer
	if code := run([]string{"sync"}, &syncStdout, &syncStderr); code != exitcode.OK {
		t.Fatalf("sync code=%d stdout=%q stderr=%q", code, syncStdout.String(), syncStderr.String())
	}
	if !strings.Contains(syncStderr.String(), "pass --from") {
		t.Fatalf("sync stderr = %q", syncStderr.String())
	}
	if _, err := os.Stat(filepath.Join(dir, "docs", "ARCHITECTURE.md")); err != nil {
		t.Fatalf("sync did not create doc: %v", err)
	}

	var diffStdout, diffStderr bytes.Buffer
	if code := run([]string{"diff"}, &diffStdout, &diffStderr); code != exitcode.Fail {
		t.Fatalf("diff code=%d stdout=%q stderr=%q", code, diffStdout.String(), diffStderr.String())
	}
	if !strings.Contains(diffStderr.String(), "pass --from") {
		t.Fatalf("diff stderr = %q", diffStderr.String())
	}
}

func TestRunSyncUsesRepoConfigDocPath(t *testing.T) {
	dir := initRepo(t)
	t.Chdir(dir)

	if err := os.WriteFile(filepath.Join(dir, "archon.yaml"), []byte("doc: docs/CUSTOM.md\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	var stdout, stderr bytes.Buffer
	if code := run([]string{"sync"}, &stdout, &stderr); code != exitcode.OK {
		t.Fatalf("sync code=%d stdout=%q stderr=%q", code, stdout.String(), stderr.String())
	}
	if _, err := os.Stat(filepath.Join(dir, "docs", "CUSTOM.md")); err != nil {
		t.Fatalf("custom doc not created: %v", err)
	}
	if _, err := os.Stat(filepath.Join(dir, "docs", "ARCHITECTURE.md")); !os.IsNotExist(err) {
		t.Fatalf("default doc path should be unused, err=%v", err)
	}
}

func TestRunChangelogContinuesOnLLMError(t *testing.T) {
	dir := initRepo(t)
	t.Chdir(dir)
	runGit(t, dir, "tag", "v1.0.0")
	mustCommitFile(t, dir, "b/b.go", "package b\n", "add b")

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "boom", http.StatusInternalServerError)
	}))
	defer srv.Close()

	t.Setenv("ARCHON_BASE_URL", srv.URL)

	var stdout, stderr bytes.Buffer
	code := run([]string{"changelog"}, &stdout, &stderr)
	if code != exitcode.OK {
		t.Fatalf("code=%d stdout=%q stderr=%q", code, stdout.String(), stderr.String())
	}
	if !strings.Contains(stdout.String(), "added nodes:") {
		t.Fatalf("stdout=%q", stdout.String())
	}
	if !strings.Contains(stderr.String(), "llm") {
		t.Fatalf("stderr=%q", stderr.String())
	}
}

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
