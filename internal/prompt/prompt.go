package prompt

import (
	"bytes"
	"fmt"
	"path/filepath"
	"text/template"

	"github.com/j75689/archon/internal/config"
)

const promptBody = `Write markdown only. Use this import graph:
{{.Graph}}

Exported APIs:
{{.APIs}}

Structure diff vs last sync:
{{.Diff}}
`

var builtIns = map[string]string{
	"architecture": "Include a system mermaid diagram.\n\n" + promptBody,
	"workflow":     "Include a command/data mermaid diagram.\n\n" + promptBody,
	"packages":     "Include per-package prose.\n\n" + promptBody,
}

type Data struct {
	Graph string
	APIs  string
	Diff  string
}

func Render(tmpl string, data Data) (string, error) {
	parsed, err := template.New("prompt").Parse(tmpl)
	if err != nil {
		return "", err
	}

	var rendered bytes.Buffer
	if err := parsed.Execute(&rendered, data); err != nil {
		return "", err
	}
	return rendered.String(), nil
}

func BuiltIn(id string) (string, bool) {
	tmpl, ok := builtIns[id]
	return tmpl, ok
}

func Load(repoRoot string, gen config.Generator, readFile func(string) ([]byte, error)) (string, error) {
	if gen.Prompt != "" {
		path := filepath.Join(repoRoot, gen.Prompt)
		body, err := readFile(path)
		if err != nil {
			return "", fmt.Errorf("read prompt %q: %w", path, err)
		}
		return string(body), nil
	}

	tmpl, ok := BuiltIn(gen.ID)
	if !ok {
		return "", fmt.Errorf("no built-in prompt for generator %q", gen.ID)
	}
	return tmpl, nil
}
