package llm

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/j75689/archon/internal/graph"
	"github.com/j75689/archon/internal/log"
)

func TestDeltaSendsReportAndSubjectsOnly(t *testing.T) {
	var gotPath string
	var gotAuth string
	var gotReq chatRequest

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotPath = r.URL.Path
		gotAuth = r.Header.Get("Authorization")

		body, err := io.ReadAll(r.Body)
		if err != nil {
			t.Fatalf("read body: %v", err)
		}
		if err := json.Unmarshal(body, &gotReq); err != nil {
			t.Fatalf("unmarshal request: %v", err)
		}

		w.Header().Set("Content-Type", "application/json")
		_, _ = io.WriteString(w, `{"choices":[{"message":{"content":"Architecture delta summary."}}]}`)
	}))
	defer srv.Close()

	c := &Client{
		BaseURL: srv.URL,
		Model:   "gpt-test",
		APIKey:  "secret",
		HTTP:    srv.Client(),
	}

	diff := graph.Diff{
		AddedEdges: []graph.Edge{{From: "example.com/m", To: "example.com/m/api"}},
	}
	subjects := []string{"add api edge", "refine sync output"}

	got, err := c.Delta(context.Background(), diff, subjects)
	if err != nil {
		t.Fatalf("Delta error: %v", err)
	}
	if got != "Architecture delta summary." {
		t.Fatalf("Delta = %q", got)
	}
	if gotPath != "/chat/completions" {
		t.Fatalf("path = %q", gotPath)
	}
	if gotAuth != "Bearer secret" {
		t.Fatalf("authorization = %q", gotAuth)
	}
	if gotReq.Model != "gpt-test" {
		t.Fatalf("model = %q", gotReq.Model)
	}
	if len(gotReq.Messages) != 2 {
		t.Fatalf("messages = %#v", gotReq.Messages)
	}
	body := gotReq.Messages[1].Content
	if !strings.Contains(body, graph.FormatReport(diff)) {
		t.Fatalf("request missing diff report: %s", body)
	}
	if !strings.Contains(body, "add api edge") || !strings.Contains(body, "refine sync output") {
		t.Fatalf("request missing subjects: %s", body)
	}
	if strings.Contains(body, ".go") {
		t.Fatalf("request leaked source path: %s", body)
	}
	if strings.Contains(body, "package ") {
		t.Fatalf("request leaked source code: %s", body)
	}
	if strings.Contains(body, "func unexported") {
		t.Fatalf("request leaked fixture body: %s", body)
	}
}

func TestDeltaLogsPostAndDoneWithoutSecrets(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = io.WriteString(w, `{"choices":[{"message":{"content":"Architecture delta summary."}}]}`)
	}))
	defer srv.Close()

	var buf bytes.Buffer
	c := &Client{
		BaseURL: srv.URL,
		Model:   "gpt-test",
		APIKey:  "sk-test-secret",
		HTTP:    srv.Client(),
		Log:     log.Writer{W: &buf, Level: 1},
	}
	if _, err := c.Delta(context.Background(), graph.Diff{
		AddedEdges: []graph.Edge{{From: "example.com/m", To: "example.com/m/api"}},
	}, []string{"add api edge"}); err != nil {
		t.Fatal(err)
	}
	got := buf.String()
	wantPost := "llm: POST " + srv.URL + "/chat/completions model=gpt-test\n"
	if !strings.Contains(got, wantPost) {
		t.Fatalf("missing POST: %q", got)
	}
	if !strings.Contains(got, "llm: done\n") {
		t.Fatalf("missing done: %q", got)
	}
	if strings.Index(got, wantPost) > strings.Index(got, "llm: done\n") {
		t.Fatalf("POST must precede done: %q", got)
	}
	if strings.Contains(got, "sk-test-secret") || strings.Contains(got, "Authorization") {
		t.Fatalf("leaked secret: %q", got)
	}
}

func TestDeltaHTTPErrorLogsPostNotDone(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "boom", http.StatusInternalServerError)
	}))
	defer srv.Close()

	var buf bytes.Buffer
	c := &Client{
		BaseURL: srv.URL,
		Model:   "gpt-test",
		HTTP:    srv.Client(),
		Log:     log.Writer{W: &buf, Level: 1},
	}
	_, err := c.Delta(context.Background(), graph.Diff{
		AddedNodes: []graph.Node{{Key: "example.com/m/b"}},
	}, nil)
	if err == nil {
		t.Fatal("expected error")
	}
	got := buf.String()
	if !strings.Contains(got, "llm: POST "+srv.URL+"/chat/completions model=gpt-test\n") {
		t.Fatalf("missing POST: %q", got)
	}
	if strings.Contains(got, "llm: done") {
		t.Fatalf("must not log done on error: %q", got)
	}
}

func TestDeltaHTTPError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "boom", http.StatusInternalServerError)
	}))
	defer srv.Close()

	c := &Client{
		BaseURL: srv.URL,
		Model:   "gpt-test",
		HTTP:    srv.Client(),
	}

	_, err := c.Delta(context.Background(), graph.Diff{
		AddedNodes: []graph.Node{{Key: "example.com/m/api"}},
	}, []string{"add api"})
	if err == nil {
		t.Fatal("expected error")
	}
	if !strings.Contains(err.Error(), "500") {
		t.Fatalf("error = %v", err)
	}
	if strings.Contains(err.Error(), "\n") {
		t.Fatalf("error must be one line: %q", err.Error())
	}
}

func TestDeltaHTTPErrorCompactMultiLineBody(t *testing.T) {
	body := "{\n  \"error\": {\n    \"message\": \"internal\nfailure\"\n  }\n}\n"
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
		_, _ = io.WriteString(w, body)
	}))
	defer srv.Close()

	c := &Client{
		BaseURL: srv.URL,
		Model:   "gpt-test",
		HTTP:    srv.Client(),
	}

	_, err := c.Delta(context.Background(), graph.Diff{
		AddedNodes: []graph.Node{{Key: "example.com/m/api"}},
	}, []string{"add api"})
	if err == nil {
		t.Fatal("expected error")
	}
	if strings.Contains(err.Error(), "\n") {
		t.Fatalf("error must be one line: %q", err.Error())
	}
	want := CompactErrorText(body)
	if !strings.Contains(err.Error(), want) {
		t.Fatalf("error = %q, want compact body %q", err.Error(), want)
	}
}

func TestDeltaEmptyResponseBody(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	c := &Client{
		BaseURL: srv.URL,
		Model:   "gpt-test",
		HTTP:    srv.Client(),
	}

	_, err := c.Delta(context.Background(), graph.Diff{
		AddedNodes: []graph.Node{{Key: "example.com/m/api"}},
	}, []string{"add api"})
	if err == nil {
		t.Fatal("expected error")
	}
	if !strings.Contains(err.Error(), "empty response body") {
		t.Fatalf("error = %v", err)
	}
}

func TestDeltaEmptyAssistantContent(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = io.WriteString(w, `{"choices":[{"message":{"content":""}}]}`)
	}))
	defer srv.Close()

	c := &Client{
		BaseURL: srv.URL,
		Model:   "gpt-test",
		HTTP:    srv.Client(),
	}

	_, err := c.Delta(context.Background(), graph.Diff{
		AddedNodes: []graph.Node{{Key: "example.com/m/api"}},
	}, []string{"add api"})
	if err == nil {
		t.Fatal("expected error")
	}
	if !strings.Contains(err.Error(), "missing content") {
		t.Fatalf("error = %v", err)
	}
}

func TestCompleteSendsUserStringAndReturnsAssistantContent(t *testing.T) {
	var got chatRequest
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, err := io.ReadAll(r.Body)
		if err != nil {
			t.Fatal(err)
		}
		if err := json.Unmarshal(body, &got); err != nil {
			t.Fatal(err)
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = io.WriteString(w, `{"choices":[{"message":{"content":"# Generated\n"}}]}`)
	}))
	defer srv.Close()

	c := &Client{BaseURL: srv.URL, Model: "gpt-test", HTTP: srv.Client()}
	content, err := c.Complete(context.Background(), "rendered user prompt")
	if err != nil {
		t.Fatal(err)
	}
	if content != "# Generated" {
		t.Fatalf("content = %q", content)
	}
	if len(got.Messages) != 2 {
		t.Fatalf("messages = %#v", got.Messages)
	}
	if got.Messages[0] != (chatMessage{
		Role:    "system",
		Content: "You write markdown documentation. Reply with the full document only.",
	}) {
		t.Fatalf("system message = %#v", got.Messages[0])
	}
	if got.Messages[1] != (chatMessage{Role: "user", Content: "rendered user prompt"}) {
		t.Fatalf("user message = %#v", got.Messages[1])
	}
}

func TestCompleteHTTPErrorLogsPostNotDone(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "boom", http.StatusInternalServerError)
	}))
	defer srv.Close()

	var buf bytes.Buffer
	c := &Client{
		BaseURL: srv.URL,
		Model:   "gpt-test",
		HTTP:    srv.Client(),
		Log:     log.Writer{W: &buf, Level: 1},
	}
	if _, err := c.Complete(context.Background(), "prompt"); err == nil {
		t.Fatal("expected error")
	}
	if !strings.Contains(buf.String(), "llm: POST ") {
		t.Fatalf("missing POST: %q", buf.String())
	}
	if strings.Contains(buf.String(), "llm: done") {
		t.Fatalf("must not log done on error: %q", buf.String())
	}
}
