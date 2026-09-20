package mcp

import (
	"bytes"
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"testing"

	"github.com/j75689/archon/internal/app"
	"github.com/j75689/archon/internal/exitcode"
	"github.com/j75689/archon/internal/fingerprint"
	"github.com/j75689/archon/internal/git"
	mcpsdk "github.com/modelcontextprotocol/go-sdk/mcp"
)

func newTestApp(t *testing.T, dir string) *app.App {
	t.Helper()
	r, err := git.Open(dir)
	if err != nil {
		t.Fatal(err)
	}
	a := app.New(r)
	a.Stdout, a.Stderr = new(bytes.Buffer), new(bytes.Buffer)
	return a
}

func connect(t *testing.T, a *app.App) *mcpsdk.ClientSession {
	t.Helper()
	srv := New(a)
	ctx := context.Background()
	t1, t2 := mcpsdk.NewInMemoryTransports()
	serverSession, err := srv.Connect(ctx, t1, nil)
	if err != nil {
		t.Fatal(err)
	}
	client := mcpsdk.NewClient(&mcpsdk.Implementation{Name: "test", Version: "v0.0.1"}, nil)
	session, err := client.Connect(ctx, t2, nil)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		_ = session.Close()
		_ = serverSession.Close()
	})
	return session
}

func TestToolsListHasSevenNames(t *testing.T) {
	dir := initRepo(t)
	session := connect(t, newTestApp(t, dir))
	got, err := session.ListTools(context.Background(), nil)
	if err != nil {
		t.Fatal(err)
	}
	want := map[string]bool{
		"check": true, "diff": true, "changelog": true, "sync": true,
		"fingerprint": true, "graph": true, "apis": true,
	}
	if len(got.Tools) != 7 {
		t.Fatalf("len=%d", len(got.Tools))
	}
	for _, tool := range got.Tools {
		if !want[tool.Name] {
			t.Fatalf("unexpected %q", tool.Name)
		}
	}
}

func TestFingerprintGraphAPIsWithoutLLM(t *testing.T) {
	dir := initRepo(t)
	session := connect(t, newTestApp(t, dir))
	ctx := context.Background()
	for _, name := range []string{"fingerprint", "graph", "apis"} {
		res, err := session.CallTool(ctx, &mcpsdk.CallToolParams{Name: name})
		if err != nil {
			t.Fatalf("%s protocol: %v", name, err)
		}
		if res.IsError {
			t.Fatalf("%s isError: %+v", name, res)
		}
	}
}

func TestCheckStaleWithoutLockfile(t *testing.T) {
	dir := initRepo(t)
	session := connect(t, newTestApp(t, dir))
	res, err := session.CallTool(context.Background(), &mcpsdk.CallToolParams{Name: "check"})
	if err != nil {
		t.Fatal(err)
	}
	if res.IsError {
		t.Fatalf("isError %+v", res)
	}
	out, _ := res.StructuredContent.(map[string]any)
	if out["ok"] != false {
		t.Fatalf("ok=%v", out["ok"])
	}
	if out["stale"] != true {
		t.Fatalf("stale=%v", out["stale"])
	}
}

func TestCheckMatchAfterSeedingLockfile(t *testing.T) {
	dir := initRepo(t)
	a := newTestApp(t, dir)
	g, apis, code := a.Structure("HEAD")
	if code != exitcode.OK {
		t.Fatal(code)
	}
	lf, err := fingerprint.NewLockfile(g, apis)
	if err != nil {
		t.Fatal(err)
	}
	if err := fingerprint.Write(filepath.Join(dir, ".archon", "graph.json"), lf); err != nil {
		t.Fatal(err)
	}
	session := connect(t, a)
	res, err := session.CallTool(context.Background(), &mcpsdk.CallToolParams{Name: "check"})
	if err != nil {
		t.Fatal(err)
	}
	if res.IsError {
		t.Fatal(res)
	}
	out, _ := res.StructuredContent.(map[string]any)
	if out["ok"] != true {
		t.Fatalf("ok=%v", out["ok"])
	}
}

func TestNoGoModIsToolError(t *testing.T) {
	dir := initEmptyRepo(t)
	mustCommitFile(t, dir, "README.md", "x\n", "init")
	session := connect(t, newTestApp(t, dir))
	res, err := session.CallTool(context.Background(), &mcpsdk.CallToolParams{Name: "graph"})
	if err != nil {
		t.Fatal(err)
	}
	if !res.IsError {
		t.Fatalf("want isError, got %+v", res)
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
