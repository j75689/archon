package main

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"net"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/j75689/archon/internal/exitcode"
	"github.com/j75689/archon/internal/fingerprint"
	"github.com/j75689/archon/internal/git"
	"github.com/j75689/archon/internal/graph"
	"github.com/j75689/archon/internal/lang/golang"
	mcpsdk "github.com/modelcontextprotocol/go-sdk/mcp"
)

func TestRunHelpListsTimeoutFlag(t *testing.T) {
	var stdout, stderr bytes.Buffer
	code := run([]string{"--help"}, &stdout, &stderr)
	if code != exitcode.OK {
		t.Fatalf("code=%d stderr=%q", code, stderr.String())
	}
	help := stdout.String() + stderr.String()
	if !strings.Contains(help, "--timeout") {
		t.Fatalf("help missing --timeout: %q", help)
	}
}

func TestRunMCPHelpListsHTTPFlags(t *testing.T) {
	var stdout, stderr bytes.Buffer
	code := run([]string{"mcp", "--help"}, &stdout, &stderr)
	if code != exitcode.OK {
		t.Fatalf("code=%d stderr=%q", code, stderr.String())
	}
	help := stdout.String() + stderr.String()
	for _, flag := range []string{"--http", "--http.host", "--http.port", "--http.token", "--root"} {
		if !strings.Contains(help, flag) {
			t.Fatalf("help missing %s: %q", flag, help)
		}
	}
}

func TestRunDiffNoRepo(t *testing.T) {
	gitOK(t)

	dir := t.TempDir()
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

func TestRunMCPNoRepo(t *testing.T) {
	gitOK(t)
	dir := t.TempDir()
	t.Chdir(dir)
	var stdout, stderr bytes.Buffer
	code := run([]string{"mcp"}, &stdout, &stderr)
	if code != exitcode.Fail {
		t.Fatalf("code=%d stderr=%q", code, stderr.String())
	}
	if stdout.Len() != 0 {
		t.Fatalf("stdout %q", stdout.String())
	}
	if !strings.Contains(stderr.String(), "not a git repository") {
		t.Fatalf("stderr %q", stderr.String())
	}
}

func TestRunMCPRootOpensRepoFromNonRepoCwd(t *testing.T) {
	repo := initRepo(t)
	t.Chdir(t.TempDir())

	inR, inW, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	outR, outW, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	defer outR.Close()

	oldIn, oldOut := os.Stdin, os.Stdout
	os.Stdin, os.Stdout = inR, outW
	t.Cleanup(func() {
		os.Stdin, os.Stdout = oldIn, oldOut
	})

	var stderr bytes.Buffer
	done := make(chan int, 1)
	go func() {
		done <- run([]string{"mcp", "--root", repo}, os.Stdout, &stderr)
	}()
	if err := inW.Close(); err != nil {
		t.Fatal(err)
	}

	select {
	case code := <-done:
		if strings.Contains(stderr.String(), "not a git repository") {
			t.Fatalf("ignored --root: code=%d stderr=%q", code, stderr.String())
		}
		if code != exitcode.OK && code != exitcode.Fail {
			t.Fatalf("code=%d stderr=%q", code, stderr.String())
		}
	case <-time.After(5 * time.Second):
		t.Fatal("mcp --root hung")
	}
}

func TestRunMCPRootHTTPLoopbackServesAndStops(t *testing.T) {
	repo := initRepo(t)
	t.Chdir(t.TempDir())

	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	port := ln.Addr().(*net.TCPAddr).Port
	if err := ln.Close(); err != nil {
		t.Fatal(err)
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	var stderr bytes.Buffer
	done := make(chan int, 1)
	go func() {
		done <- runContext(ctx, []string{
			"mcp", "--root", repo, "--http", "--http.port", strconv.Itoa(port),
		}, io.Discard, &stderr)
	}()

	endpoint := fmt.Sprintf("http://127.0.0.1:%d", port)
	client := mcpsdk.NewClient(&mcpsdk.Implementation{Name: "test", Version: "v0.0.1"}, nil)
	var session *mcpsdk.ClientSession
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		session, err = client.Connect(context.Background(), &mcpsdk.StreamableClientTransport{
			Endpoint: endpoint,
		}, nil)
		if err == nil {
			break
		}
		time.Sleep(20 * time.Millisecond)
	}
	if err != nil {
		t.Fatalf("connect: %v stderr=%q", err, stderr.String())
	}
	got, err := session.ListTools(context.Background(), nil)
	if err != nil {
		t.Fatal(err)
	}
	if len(got.Tools) != 7 {
		t.Fatalf("len=%d", len(got.Tools))
	}
	if err := session.Close(); err != nil {
		t.Fatal(err)
	}
	cancel()
	select {
	case code := <-done:
		if code != exitcode.OK {
			t.Fatalf("code=%d stderr=%q", code, stderr.String())
		}
	case <-time.After(3 * time.Second):
		t.Fatal("mcp --http did not stop after cancel")
	}
}

func TestRunSyncSucceedsWithoutTagWhileDiffFails(t *testing.T) {
	dir := initRepo(t)
	t.Chdir(dir)

	var syncStdout, syncStderr bytes.Buffer
	if code := run([]string{"sync"}, &syncStdout, &syncStderr); code != exitcode.OK {
		t.Fatalf("sync code=%d stdout=%q stderr=%q", code, syncStdout.String(), syncStderr.String())
	}
	if !strings.Contains(syncStderr.String(), "llm: skip (no client)") {
		t.Fatalf("sync stderr = %q", syncStderr.String())
	}
	if _, err := os.Stat(filepath.Join(dir, "docs", "ARCHITECTURE.md")); !os.IsNotExist(err) {
		t.Fatalf("sync must not write generators without LLM, err=%v", err)
	}

	var diffStdout, diffStderr bytes.Buffer
	if code := run([]string{"diff"}, &diffStdout, &diffStderr); code != exitcode.Fail {
		t.Fatalf("diff code=%d stdout=%q stderr=%q", code, diffStdout.String(), diffStderr.String())
	}
	if !strings.Contains(diffStderr.String(), "pass --from") {
		t.Fatalf("diff stderr = %q", diffStderr.String())
	}
}

func TestRunChangelogReportsWithoutLLM(t *testing.T) {
	dir := initRepo(t)
	t.Chdir(dir)
	runGit(t, dir, "tag", "v1.0.0")
	mustCommitFile(t, dir, "b/b.go", "package b\n", "add b")

	var stdout, stderr bytes.Buffer
	code := run([]string{"changelog"}, &stdout, &stderr)
	if code != exitcode.Gate {
		t.Fatalf("code=%d stdout=%q stderr=%q", code, stdout.String(), stderr.String())
	}
	if !strings.Contains(stdout.String(), "added nodes:") {
		t.Fatalf("stdout=%q", stdout.String())
	}
	if strings.Contains(stderr.String(), "llm") {
		t.Fatalf("stderr=%q", stderr.String())
	}
}

func TestRunCheckVerboseAndQuiet(t *testing.T) {
	dir := initRepo(t)
	t.Chdir(dir)
	writeHEADLockfile(t, dir)

	var stdout, stderr bytes.Buffer
	if code := run([]string{"check"}, &stdout, &stderr); code != exitcode.OK {
		t.Fatalf("quiet check code=%d stderr=%q", code, stderr.String())
	}
	if stdout.Len() != 0 {
		t.Fatalf("stdout=%q", stdout.String())
	}
	if stderr.Len() != 0 {
		t.Fatalf("quiet stderr must be empty: %q", stderr.String())
	}

	stdout.Reset()
	stderr.Reset()
	if code := run([]string{"check", "-v"}, &stdout, &stderr); code != exitcode.OK {
		t.Fatalf("verbose check code=%d stderr=%q", code, stderr.String())
	}
	if stdout.Len() != 0 {
		t.Fatalf("stdout=%q", stdout.String())
	}
	if !strings.Contains(stderr.String(), "snapshot") {
		t.Fatalf("verbose stderr=%q", stderr.String())
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
	g, err := (golang.Extractor{}).Extract(snap)
	if err != nil {
		t.Fatal(err)
	}
	g, err = graph.Compile(g)
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
