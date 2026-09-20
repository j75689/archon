package mcp

import (
	"bytes"
	"context"
	"fmt"
	"path/filepath"
	"strings"
	"sync"

	"github.com/j75689/archon/internal/app"
	"github.com/j75689/archon/internal/exitcode"
	"github.com/j75689/archon/internal/fingerprint"
	mcpsdk "github.com/modelcontextprotocol/go-sdk/mcp"
)

type Server struct {
	MCP *mcpsdk.Server
	App *app.App
	mu  sync.Mutex
}

type Args struct {
	From string `json:"from,omitempty" jsonschema:"git revision for the left side of a drift comparison"`
	To   string `json:"to,omitempty" jsonschema:"git revision to inspect"`
}

type Result struct {
	OK       bool   `json:"ok"`
	ExitCode int    `json:"exit_code"`
	Stale    bool   `json:"stale,omitempty"`
	Changed  bool   `json:"changed,omitempty"`
	Hash     string `json:"hash,omitempty"`
}

// Version is the MCP implementation version advertised to clients.
// The CLI may overwrite this from a build-time version string.
var Version = "dev"

func New(a *app.App) *Server {
	s := &Server{
		App: a,
		MCP: mcpsdk.NewServer(&mcpsdk.Implementation{Name: "archon", Version: Version}, nil),
	}

	mcpsdk.AddTool(s.MCP, &mcpsdk.Tool{
		Name:        "check",
		Description: "Compare structure fingerprint at to vs .archon/graph.json",
	}, s.toolCheck)
	mcpsdk.AddTool(s.MCP, &mcpsdk.Tool{
		Name:        "diff",
		Description: "First-party graph and exported-signature changes from→to",
	}, s.toolDiff)
	mcpsdk.AddTool(s.MCP, &mcpsdk.Tool{
		Name:        "changelog",
		Description: "Print first-party structure changes (same report as diff)",
	}, s.toolChangelog)
	mcpsdk.AddTool(s.MCP, &mcpsdk.Tool{
		Name:        "sync",
		Description: "If fingerprint is stale and LLM is configured, write generator markdown and the lockfile",
	}, s.toolSync)
	mcpsdk.AddTool(s.MCP, &mcpsdk.Tool{
		Name:        "fingerprint",
		Description: "Hash graph+APIs at to and compare .archon/graph.json. Reports stale; does not fail like check.",
	}, s.toolFingerprint)
	mcpsdk.AddTool(s.MCP, &mcpsdk.Tool{
		Name:        "graph",
		Description: "First-party import graph at to",
	}, s.toolGraph)
	mcpsdk.AddTool(s.MCP, &mcpsdk.Tool{
		Name:        "apis",
		Description: "Package docs and exported signatures at to",
	}, s.toolAPIs)

	s.MCP.AddResource(&mcpsdk.Resource{
		URI:      "archon://fingerprint",
		Name:     "fingerprint",
		MIMEType: "text/plain",
	}, s.resource("fingerprint"))
	s.MCP.AddResource(&mcpsdk.Resource{
		URI:      "archon://graph",
		Name:     "graph",
		MIMEType: "text/plain",
	}, s.resource("graph"))
	s.MCP.AddResource(&mcpsdk.Resource{
		URI:      "archon://apis",
		Name:     "apis",
		MIMEType: "text/plain",
	}, s.resource("apis"))

	return s
}

func (s *Server) Connect(ctx context.Context, t mcpsdk.Transport, opts *mcpsdk.ServerSessionOptions) (*mcpsdk.ServerSession, error) {
	return s.MCP.Connect(ctx, t, opts)
}

func (s *Server) RunStdio(ctx context.Context) error {
	return s.MCP.Run(ctx, &mcpsdk.StdioTransport{})
}

func (s *Server) applyArgs(in Args) {
	if in.From != "" {
		s.App.From = in.From
	}
	if in.To != "" {
		s.App.To = in.To
	}
}

func (s *Server) invoke(in Args, fn func() int) (stdout, stderr string, code int) {
	s.mu.Lock()
	defer s.mu.Unlock()

	prevFrom, prevTo := s.App.From, s.App.To
	prevOut, prevErr := s.App.Stdout, s.App.Stderr
	s.applyArgs(in)

	var outBuf, errBuf bytes.Buffer
	s.App.Stdout, s.App.Stderr = &outBuf, &errBuf
	defer func() {
		s.App.From, s.App.To = prevFrom, prevTo
		s.App.Stdout, s.App.Stderr = prevOut, prevErr
	}()

	code = fn()
	return outBuf.String(), errBuf.String(), code
}

func (s *Server) inspect(in Args, kind string) (text, stderr string, out Result, code int) {
	s.mu.Lock()
	defer s.mu.Unlock()

	prevFrom, prevTo := s.App.From, s.App.To
	prevOut, prevErr := s.App.Stdout, s.App.Stderr
	s.applyArgs(in)

	var outBuf, errBuf bytes.Buffer
	s.App.Stdout, s.App.Stderr = &outBuf, &errBuf
	defer func() {
		s.App.From, s.App.To = prevFrom, prevTo
		s.App.Stdout, s.App.Stderr = prevOut, prevErr
	}()

	rev := s.App.To
	if rev == "" {
		rev = "HEAD"
	}

	g, apis, code := s.App.Structure(rev)
	stderr = errBuf.String()
	if code != exitcode.OK {
		return "", stderr, Result{}, code
	}

	switch kind {
	case "graph":
		return app.FormatGraph(g), stderr, Result{}, exitcode.OK
	case "apis":
		return app.FormatAPIs(apis), stderr, Result{}, exitcode.OK
	case "fingerprint":
		sum, err := fingerprint.Hash(g, apis)
		if err != nil {
			return "", err.Error(), Result{}, exitcode.Fail
		}
		lf, err := fingerprint.Load(filepath.Join(s.App.Repo.Root, ".archon", "graph.json"))
		return sum, stderr, Result{
			Hash:  sum,
			Stale: err != nil || lf.Hash != sum,
		}, exitcode.OK
	default:
		return "", fmt.Sprintf("unknown inspect kind %q", kind), Result{}, exitcode.Fail
	}
}

func finish(stdout, stderr string, code int, out Result) (*mcpsdk.CallToolResult, Result, error) {
	out.ExitCode = code
	out.OK = code == exitcode.OK

	text := strings.TrimSpace(strings.Join([]string{stdout, stderr}, "\n"))
	if text == "" && out.Hash != "" {
		text = out.Hash
	}

	res := &mcpsdk.CallToolResult{}
	if text != "" {
		res.Content = []mcpsdk.Content{&mcpsdk.TextContent{Text: text}}
	}
	if code == exitcode.Fail {
		if len(res.Content) == 0 {
			res.Content = []mcpsdk.Content{&mcpsdk.TextContent{Text: "archon failed"}}
		}
		res.IsError = true
	}
	return res, out, nil
}

func (s *Server) toolCheck(_ context.Context, _ *mcpsdk.CallToolRequest, in Args) (*mcpsdk.CallToolResult, Result, error) {
	stdout, stderr, code := s.invoke(in, s.App.Check)
	return finish(stdout, stderr, code, Result{Stale: code == exitcode.Gate})
}

func (s *Server) toolDiff(_ context.Context, _ *mcpsdk.CallToolRequest, in Args) (*mcpsdk.CallToolResult, Result, error) {
	stdout, stderr, code := s.invoke(in, s.App.Diff)
	return finish(stdout, stderr, code, Result{Changed: code == exitcode.Gate})
}

func (s *Server) toolChangelog(_ context.Context, _ *mcpsdk.CallToolRequest, in Args) (*mcpsdk.CallToolResult, Result, error) {
	stdout, stderr, code := s.invoke(in, s.App.Changelog)
	return finish(stdout, stderr, code, Result{Changed: code == exitcode.Gate})
}

func (s *Server) toolSync(_ context.Context, _ *mcpsdk.CallToolRequest, in Args) (*mcpsdk.CallToolResult, Result, error) {
	stdout, stderr, code := s.invoke(in, s.App.Sync)
	return finish(stdout, stderr, code, Result{})
}

func (s *Server) toolFingerprint(_ context.Context, _ *mcpsdk.CallToolRequest, in Args) (*mcpsdk.CallToolResult, Result, error) {
	text, stderr, out, code := s.inspect(in, "fingerprint")
	return finish(text, stderr, code, out)
}

func (s *Server) toolGraph(_ context.Context, _ *mcpsdk.CallToolRequest, in Args) (*mcpsdk.CallToolResult, Result, error) {
	text, stderr, out, code := s.inspect(in, "graph")
	return finish(text, stderr, code, out)
}

func (s *Server) toolAPIs(_ context.Context, _ *mcpsdk.CallToolRequest, in Args) (*mcpsdk.CallToolResult, Result, error) {
	text, stderr, out, code := s.inspect(in, "apis")
	return finish(text, stderr, code, out)
}

func (s *Server) resource(kind string) mcpsdk.ResourceHandler {
	return func(ctx context.Context, req *mcpsdk.ReadResourceRequest) (*mcpsdk.ReadResourceResult, error) {
		text, stderr, out, code := s.inspect(Args{}, kind)
		if code == exitcode.Fail {
			msg := strings.TrimSpace(strings.Join([]string{text, stderr}, "\n"))
			if msg == "" {
				msg = "archon failed"
			}
			return nil, fmt.Errorf("%s", msg)
		}
		if text == "" && out.Hash != "" {
			text = out.Hash
		}
		return &mcpsdk.ReadResourceResult{
			Contents: []*mcpsdk.ResourceContents{{
				URI:      req.Params.URI,
				MIMEType: "text/plain",
				Text:     text,
			}},
		}, nil
	}
}
