package llm

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/j75689/archon/internal/graph"
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
}
