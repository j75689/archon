package git

import (
	"bytes"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/j75689/archon/internal/lang/golang"
	"github.com/j75689/archon/internal/log"
)

func gitOK(t *testing.T) {
	t.Helper()
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git not on PATH")
	}
}

func runGit(t *testing.T, dir string, args ...string) string {
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
	return string(out)
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

	if err := os.WriteFile(filepath.Join(dir, "go.mod"), []byte("module example.com/m\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "a.go"), []byte("package m\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	runGit(t, dir, "add", ".")
	runGit(t, dir, "commit", "-m", "init")
	return dir
}

func TestOpenAndSnapshot(t *testing.T) {
	dir := initRepo(t)
	if err := os.WriteFile(filepath.Join(dir, "readme.txt"), []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
	runGit(t, dir, "add", ".")
	runGit(t, dir, "commit", "-m", "add readme")

	r, err := Open(dir)
	if err != nil {
		t.Fatal(err)
	}

	s, err := r.Snapshot("HEAD", golang.WantFile)
	if err != nil {
		t.Fatal(err)
	}

	if s.Rev == "" {
		t.Fatal("missing resolved revision")
	}
	if _, ok := s.Files["go.mod"]; !ok {
		t.Fatal("missing go.mod")
	}
	if _, ok := s.Files["a.go"]; !ok {
		t.Fatal("missing a.go")
	}
	if _, ok := s.Files["readme.txt"]; ok {
		t.Fatal("should not fetch non-go files")
	}
}

func TestSnapshotLogsRevCountAndSortedPaths(t *testing.T) {
	dir := initRepo(t)
	r, err := Open(dir)
	if err != nil {
		t.Fatal(err)
	}
	var buf bytes.Buffer
	r.Log = log.Writer{W: &buf, Level: 2}
	if _, err := r.Snapshot("HEAD", golang.WantFile); err != nil {
		t.Fatal(err)
	}
	got := buf.String()
	if !strings.Contains(got, "snapshot HEAD: 2 files") {
		t.Fatalf("missing info: %q", got)
	}
	infoIdx := strings.Index(got, "snapshot HEAD: 2 files\n")
	aIdx := strings.Index(got, "snapshot: a.go\n")
	modIdx := strings.Index(got, "snapshot: go.mod\n")
	if infoIdx < 0 || aIdx < 0 || modIdx < 0 {
		t.Fatalf("missing lines: %q", got)
	}
	if !(infoIdx < aIdx && aIdx < modIdx) {
		t.Fatalf("want info then sorted paths, got %q", got)
	}
}

func TestSnapshotSilentWithoutLogger(t *testing.T) {
	dir := initRepo(t)
	r, err := Open(dir)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := r.Snapshot("HEAD", golang.WantFile); err != nil {
		t.Fatal(err)
	}
}

func TestOpenNotRepo(t *testing.T) {
	gitOK(t)

	_, err := Open(t.TempDir())
	if !errors.Is(err, ErrNotRepo) {
		t.Fatalf("expected ErrNotRepo, got %v", err)
	}
}

func TestVerifyMissingRev(t *testing.T) {
	dir := initRepo(t)

	r, err := Open(dir)
	if err != nil {
		t.Fatal(err)
	}

	if _, err := r.Verify("v9.9.9"); err == nil {
		t.Fatal("expected missing rev error")
	}
}

func TestDescribeTag(t *testing.T) {
	dir := initRepo(t)
	runGit(t, dir, "tag", "v0.1.0")

	r, err := Open(dir)
	if err != nil {
		t.Fatal(err)
	}

	tag, err := r.DescribeTag("HEAD")
	if err != nil {
		t.Fatal(err)
	}
	if tag != "v0.1.0" {
		t.Fatalf("got %q want %q", tag, "v0.1.0")
	}
}

func TestDescribeTagMissing(t *testing.T) {
	dir := initRepo(t)

	r, err := Open(dir)
	if err != nil {
		t.Fatal(err)
	}

	_, err = r.DescribeTag("HEAD")
	if !errors.Is(err, ErrNoTag) {
		t.Fatalf("expected ErrNoTag, got %v", err)
	}
}

func TestLogSubjects(t *testing.T) {
	dir := initRepo(t)

	if err := os.WriteFile(filepath.Join(dir, "b.go"), []byte("package m\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	runGit(t, dir, "add", ".")
	runGit(t, dir, "commit", "-m", "add b")

	if err := os.WriteFile(filepath.Join(dir, "c.go"), []byte("package m\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	runGit(t, dir, "add", ".")
	runGit(t, dir, "commit", "-m", "add c")

	r, err := Open(dir)
	if err != nil {
		t.Fatal(err)
	}

	subs, err := r.LogSubjects("HEAD~2", "HEAD", 10, 1024)
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(subs, []string{"add c", "add b"}) {
		t.Fatalf("subjects: %#v", subs)
	}
}

func TestResolveFromExplicit(t *testing.T) {
	dir := initEmptyRepo(t)
	mustCommitFile(t, dir, "go.mod", "module example.com/m\n", "c1")
	r, _ := Open(dir)
	h, _ := r.Verify("HEAD")
	got, err := r.ResolveFrom("HEAD", h)
	if err != nil || got.From != h || got.EmptyDiff {
		t.Fatalf("%+v %v", got, err)
	}
}

func TestResolveFromNoTag(t *testing.T) {
	dir := initEmptyRepo(t)
	mustCommitFile(t, dir, "go.mod", "module example.com/m\n", "c1")
	r, _ := Open(dir)
	_, err := r.ResolveFrom("HEAD", "")
	if !errors.Is(err, ErrNoTag) {
		t.Fatalf("got %v", err)
	}
}

func TestResolveFromLatestTagAndFirstTag(t *testing.T) {
	dir := initEmptyRepo(t)
	mustCommitFile(t, dir, "go.mod", "module example.com/m\n", "c1")
	runGit(t, dir, "tag", "v0.1.0")
	r, _ := Open(dir)
	got, err := r.ResolveFrom("v0.1.0", "")
	if err != nil || !got.EmptyDiff {
		t.Fatalf("first tag: %+v %v", got, err)
	}
	mustCommitFile(t, dir, "a.go", "package m\n", "c2")
	runGit(t, dir, "tag", "v0.2.0")
	got, err = r.ResolveFrom("v0.2.0", "")
	if err != nil || got.EmptyDiff || got.From != "v0.1.0" {
		t.Fatalf("second tag: %+v %v", got, err)
	}
	mustCommitFile(t, dir, "b.go", "package m\n", "c3")
	got, err = r.ResolveFrom("HEAD", "")
	if err != nil || got.From != "v0.2.0" {
		t.Fatalf("head after tags: %+v %v", got, err)
	}
}

func mustCommitFile(t *testing.T, dir, name, body, msg string) {
	t.Helper()
	if err := os.WriteFile(filepath.Join(dir, name), []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
	runGit(t, dir, "add", name)
	runGit(t, dir, "commit", "-m", msg)
}

func TestLogSubjectsLimits(t *testing.T) {
	dir := initRepo(t)

	if err := os.WriteFile(filepath.Join(dir, "b.go"), []byte("package m\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	runGit(t, dir, "add", ".")
	runGit(t, dir, "commit", "-m", "abcdef")

	if err := os.WriteFile(filepath.Join(dir, "c.go"), []byte("package m\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	runGit(t, dir, "add", ".")
	runGit(t, dir, "commit", "-m", "ghijkl")

	r, err := Open(dir)
	if err != nil {
		t.Fatal(err)
	}

	subs, err := r.LogSubjects("HEAD~2", "HEAD", 1, 1024)
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(subs, []string{"ghijkl"}) {
		t.Fatalf("subjects with maxN: %#v", subs)
	}

	subs, err = r.LogSubjects("HEAD~2", "HEAD", 10, 7)
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(subs, []string{"ghijkl"}) {
		t.Fatalf("subjects with maxBytes: %#v", subs)
	}
}
