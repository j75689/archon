package llm

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/j75689/archon/internal/graph"
	"github.com/j75689/archon/internal/log"
)

type Client struct {
	BaseURL string
	Model   string
	APIKey  string
	HTTP    *http.Client
	Log     log.Logger
}

type chatRequest struct {
	Model    string        `json:"model"`
	Messages []chatMessage `json:"messages"`
}

type chatMessage struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

type chatResponse struct {
	Choices []struct {
		Message chatMessage `json:"message"`
	} `json:"choices"`
}

func (c *Client) Delta(ctx context.Context, d graph.Diff, subjects []string) (string, error) {
	payload := chatRequest{
		Model: c.Model,
		Messages: []chatMessage{
			{
				Role:    "system",
				Content: "You write architecture release notes for engineers. Respond with 100 to 150 words under the heading Architecture Delta:. Use only the provided first-party dependency graph diff and commit subjects. Explain what packages or dependency edges were added or removed, what that suggests about boundaries or data flow, and any likely impact on coupling or maintainability. Be precise, sober, and concrete. If the change is small, say so without padding. Do not mention mermaid, diagrams, source files, filenames, code snippets, tests, or unavailable context. Do not invent behavior, APIs, or implementation details. Base every statement strictly on the supplied report and subjects.",
			},
			{
				Role:    "user",
				Content: deltaPrompt(d, subjects),
			},
		},
	}

	body, err := json.Marshal(payload)
	if err != nil {
		return "", err
	}

	url := strings.TrimRight(c.BaseURL, "/") + "/chat/completions"
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(body))
	if err != nil {
		return "", err
	}
	req.Header.Set("Content-Type", "application/json")
	if c.APIKey != "" {
		req.Header.Set("Authorization", "Bearer "+c.APIKey)
	}

	log.OrNop(c.Log).Info(fmt.Sprintf("llm: POST %s model=%s", url, c.Model))
	resp, err := c.httpClient().Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()

	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", err
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		detail := CompactErrorText(string(respBody))
		if detail == "" {
			detail = "empty body"
		}
		return "", fmt.Errorf("llm request failed: status %d: %s", resp.StatusCode, detail)
	}
	if len(strings.TrimSpace(string(respBody))) == 0 {
		return "", fmt.Errorf("llm request failed: empty response body")
	}

	var decoded chatResponse
	if err := json.Unmarshal(respBody, &decoded); err != nil {
		return "", err
	}
	if len(decoded.Choices) == 0 {
		return "", fmt.Errorf("llm response missing choices")
	}

	content := strings.TrimSpace(decoded.Choices[0].Message.Content)
	if content == "" {
		return "", fmt.Errorf("llm response missing content")
	}
	log.OrNop(c.Log).Info("llm: done")
	return content, nil
}

// CompactErrorText collapses whitespace so LLM errors fit on one stderr line.
func CompactErrorText(s string) string {
	return strings.Join(strings.Fields(s), " ")
}

// FormatDeltaError renders a Delta failure as one compact stderr line.
func FormatDeltaError(err error) string {
	if err == nil {
		return ""
	}
	return "llm delta: " + CompactErrorText(err.Error())
}

func (c *Client) httpClient() *http.Client {
	if c.HTTP != nil {
		return c.HTTP
	}
	return &http.Client{Timeout: 30 * time.Second}
}

func deltaPrompt(d graph.Diff, subjects []string) string {
	var b strings.Builder
	b.WriteString("Dependency graph diff:\n")
	b.WriteString(graph.FormatReport(d))
	b.WriteString("\nCommit subjects:\n")
	if len(subjects) == 0 {
		b.WriteString("- none\n")
		return b.String()
	}
	for _, subject := range subjects {
		b.WriteString("- ")
		b.WriteString(subject)
		b.WriteByte('\n')
	}
	return b.String()
}
