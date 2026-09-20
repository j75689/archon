package mcp

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
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
		if tool.Name == "diff" && tool.Description != "First-party graph and exported-signature changes from→to" {
			t.Fatalf("diff description %q", tool.Description)
		}
		if tool.Name == "fingerprint" && !strings.Contains(tool.Description, "does not fail") {
			t.Fatalf("fingerprint description %q", tool.Description)
		}
	}
}

func TestNewAdvertisesVersion(t *testing.T) {
	prev := Version
	Version = "vtest"
	t.Cleanup(func() { Version = prev })

	dir := initRepo(t)
	session := connect(t, newTestApp(t, dir))
	info := session.InitializeResult()
	if info == nil || info.ServerInfo == nil {
		t.Fatal("missing server info")
	}
	if info.ServerInfo.Version != "vtest" {
		t.Fatalf("version=%q", info.ServerInfo.Version)
	}
}

func TestIOTransportInitializeStdoutIsJSONLines(t *testing.T) {
	dir := initRepo(t)
	srv := New(newTestApp(t, dir))

	clientR, serverW := io.Pipe()
	serverR, clientW := io.Pipe()
	var captured bytes.Buffer
	out := struct {
		io.Writer
		io.Closer
	}{io.MultiWriter(serverW, &captured), serverW}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	errCh := make(chan error, 1)
	go func() {
		errCh <- srv.MCP.Run(ctx, &mcpsdk.IOTransport{Reader: serverR, Writer: out})
	}()

	client := mcpsdk.NewClient(&mcpsdk.Implementation{Name: "test", Version: "v0.0.1"}, nil)
	session, err := client.Connect(ctx, &mcpsdk.IOTransport{Reader: clientR, Writer: clientW}, nil)
	if err != nil {
		t.Fatal(err)
	}
	got, err := session.ListTools(ctx, nil)
	if err != nil {
		t.Fatal(err)
	}
	if len(got.Tools) != 7 {
		t.Fatalf("len=%d", len(got.Tools))
	}
	if err := session.Close(); err != nil {
		t.Fatal(err)
	}
	_ = clientW.Close()
	cancel()
	<-errCh

	for _, line := range bytes.Split(captured.Bytes(), []byte("\n")) {
		line = bytes.TrimSpace(line)
		if len(line) == 0 {
			continue
		}
		if !json.Valid(line) {
			t.Fatalf("non-JSON stdout line %q", line)
		}
	}
}

func TestResourcesListAndReadHappyPath(t *testing.T) {
	dir := initRepo(t)
	mustCommitFile(t, dir, "a.go", "package m\nfunc Hello() {}\n", "export")

	session := connect(t, newTestApp(t, dir))
	ctx := context.Background()

	got, err := session.ListResources(ctx, nil)
	if err != nil {
		t.Fatal(err)
	}

	want := map[string]bool{
		"archon://fingerprint": true,
		"archon://graph":       true,
		"archon://apis":        true,
	}
	if len(got.Resources) != len(want) {
		t.Fatalf("len=%d", len(got.Resources))
	}
	for _, res := range got.Resources {
		if !want[res.URI] {
			t.Fatalf("unexpected resource %q", res.URI)
		}
	}

	for _, tc := range []struct {
		uri  string
		want string
	}{
		{uri: "archon://fingerprint"},
		{uri: "archon://graph", want: "example.com/m"},
		{uri: "archon://apis", want: "func Hello()"},
	} {
		res, err := session.ReadResource(ctx, &mcpsdk.ReadResourceParams{URI: tc.uri})
		if err != nil {
			t.Fatalf("%s protocol: %v", tc.uri, err)
		}
		if len(res.Contents) != 1 {
			t.Fatalf("%s contents=%d", tc.uri, len(res.Contents))
		}
		if res.Contents[0].Text == "" {
			t.Fatalf("%s returned empty text", tc.uri)
		}
		if tc.want != "" && !strings.Contains(res.Contents[0].Text, tc.want) {
			t.Fatalf("%s text %q", tc.uri, res.Contents[0].Text)
		}
	}
}

func TestSyncWithoutLLMDoesNotWrite(t *testing.T) {
	dir := initRepo(t)
	session := connect(t, newTestApp(t, dir))
	res, err := session.CallTool(context.Background(), &mcpsdk.CallToolParams{Name: "sync"})
	if err != nil {
		t.Fatal(err)
	}
	if res.IsError {
		t.Fatalf("isError %+v", res)
	}
	out := structuredResult(t, res)
	if out["ok"] != true {
		t.Fatalf("ok=%v", out["ok"])
	}
	if structuredInt(t, out, "exit_code") != exitcode.OK {
		t.Fatalf("exit_code=%v", out["exit_code"])
	}
	if _, err := os.Stat(filepath.Join(dir, ".archon", "graph.json")); !os.IsNotExist(err) {
		t.Fatalf("lockfile written: %v", err)
	}
	text := ""
	for _, c := range res.Content {
		if tc, ok := c.(*mcpsdk.TextContent); ok {
			text += tc.Text
		}
	}
	if !strings.Contains(text, "llm: skip (no client)") {
		t.Fatalf("text %q", text)
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

func TestNoGoModIsResourceError(t *testing.T) {
	dir := initEmptyRepo(t)
	mustCommitFile(t, dir, "README.md", "x\n", "init")
	session := connect(t, newTestApp(t, dir))
	_, err := session.ReadResource(context.Background(), &mcpsdk.ReadResourceParams{URI: "archon://graph"})
	if err == nil {
		t.Fatal("want read resource error")
	}
	if !strings.Contains(err.Error(), "no supported language") {
		t.Fatalf("err %q", err)
	}
}

func TestDiffAndChangelogReturnStructuredChangeResult(t *testing.T) {
	dir := initRepo(t)
	runGit(t, dir, "tag", "v1.0.0")
	mustCommitFile(t, dir, "b/b.go", "package b\n", "b")

	session := connect(t, newTestApp(t, dir))
	ctx := context.Background()
	for _, name := range []string{"diff", "changelog"} {
		res, err := session.CallTool(ctx, &mcpsdk.CallToolParams{Name: name})
		if err != nil {
			t.Fatalf("%s protocol: %v", name, err)
		}
		if res.IsError {
			t.Fatalf("%s isError: %+v", name, res)
		}
		out := structuredResult(t, res)
		if out["ok"] != false {
			t.Fatalf("%s ok=%v", name, out["ok"])
		}
		if out["changed"] != true {
			t.Fatalf("%s changed=%v", name, out["changed"])
		}
		if structuredInt(t, out, "exit_code") != exitcode.Gate {
			t.Fatalf("%s exit_code=%v", name, out["exit_code"])
		}
		if !strings.Contains(textContent(res.Content), "added nodes:") {
			t.Fatalf("%s content=%q", name, textContent(res.Content))
		}
	}
}

func TestGraphToolToArgUsesOlderCommitAndRestoresAppState(t *testing.T) {
	dir := initRepo(t)
	old := gitRev(t, dir, "HEAD")
	mustCommitFile(t, dir, "b/b.go", "package b\n", "b")

	a := newTestApp(t, dir)
	a.To = "HEAD"
	session := connect(t, a)

	res, err := session.CallTool(context.Background(), &mcpsdk.CallToolParams{
		Name:      "graph",
		Arguments: map[string]any{"to": old},
	})
	if err != nil {
		t.Fatal(err)
	}
	if res.IsError {
		t.Fatalf("isError %+v", res)
	}
	text := textContent(res.Content)
	if !strings.Contains(text, "example.com/m") {
		t.Fatalf("content %q", text)
	}
	if strings.Contains(text, "example.com/m/b") {
		t.Fatalf("content should not include newer package: %q", text)
	}
	if a.To != "HEAD" {
		t.Fatalf("app.To=%q", a.To)
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

func gitRev(t *testing.T, dir, rev string) string {
	t.Helper()
	cmd := exec.Command("git", "rev-parse", rev)
	cmd.Dir = dir
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("git rev-parse %s: %v\n%s", rev, err, out)
	}
	return strings.TrimSpace(string(out))
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

func textContent(content []mcpsdk.Content) string {
	var text string
	for _, c := range content {
		if tc, ok := c.(*mcpsdk.TextContent); ok {
			text += tc.Text
		}
	}
	return text
}

func structuredResult(t *testing.T, res *mcpsdk.CallToolResult) map[string]any {
	t.Helper()
	out, ok := res.StructuredContent.(map[string]any)
	if !ok {
		t.Fatalf("structuredContent=%T %#v", res.StructuredContent, res.StructuredContent)
	}
	return out
}

func structuredInt(t *testing.T, out map[string]any, key string) int {
	t.Helper()
	switch v := out[key].(type) {
	case float64:
		return int(v)
	case int:
		return v
	case string:
		n, err := strconv.Atoi(v)
		if err != nil {
			t.Fatalf("%s=%q", key, v)
		}
		return n
	default:
		t.Fatalf("%s=%T %#v", key, v, v)
	}
	return 0
}
