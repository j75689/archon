package prompt

import (
	"bytes"
	"fmt"
	"path/filepath"
	"text/template"

	"github.com/j75689/archon/internal/config"
)

const groundingRules = `Rules:
- Write GitHub-flavored markdown only. No commentary before or after the document.
- Base every statement strictly on the import graph, exported APIs, and diff below. Do not invent packages, edges, functions, or behavior that are not present in them.
- If the diff section is empty or shows no changes, say so plainly instead of dropping the section.
- Keep each subsection to 1-3 sentences.

`

const dataBlock = `Import graph:
{{.Graph}}

Exported APIs:
{{.APIs}}

Structure diff vs last sync:
{{.Diff}}
`

const mermaidOpen = "```mermaid"
const mermaidClose = "```"

var builtIns = map[string]string{
	"architecture": groundingRules +
		"Produce a document with these sections, in order:\n" +
		"1. \"# Architecture\" and a one-paragraph overview of what the system does.\n" +
		"2. \"## System Graph\": a " + mermaidOpen + " graph TD diagram inside a fenced code block, closed with " + mermaidClose + ". One node per package in the import graph (node id = import path with \"/\" and \".\" replaced by \"_\", label = the real import path); one edge per dependency edge listed below. Do not add nodes or edges that are not in the graph.\n" +
		"3. \"## Key Components\": one \"### <package>\" subsection per package with notable fan-in or fan-out, describing its role using only its listed exported APIs.\n" +
		"4. \"## API Reference\": a markdown table mapping each package to its key exported names.\n" +
		"5. \"## Recent Changes\": summarize the structure diff below, or state there are no structural changes since the last sync.\n\n" +
		dataBlock,

	"workflow": groundingRules +
		"Produce a document with these sections, in order:\n" +
		"1. \"# Workflow\" and a one-paragraph overview of how control and data move through the system.\n" +
		"2. \"## Command / Data Flow\": a " + mermaidOpen + " flowchart LR diagram inside a fenced code block, closed with " + mermaidClose + ", tracing entrypoint packages through the packages they depend on, using only the edges in the import graph below.\n" +
		"3. \"## Core Modules\": describe each entrypoint or orchestrating package and what it coordinates, grounded only in its listed exported APIs.\n" +
		"4. \"## Key Types\": a bullet list of notable exported types and what they represent.\n" +
		"5. \"## Changelog Summary\": summarize the structure diff below, or state there are no structural changes since the last sync.\n\n" +
		dataBlock,

	"packages": groundingRules +
		"Produce a document with these sections, in order:\n" +
		"1. \"# Packages\" and a one-paragraph overview of what the system does.\n" +
		"2. One \"### <full import path>\" subsection per package present in the import graph below, each with 1-3 sentences of prose grounded only in that package's listed exported APIs. Do not describe a package that is not in the import graph, and do not include a diagram.\n\n" +
		dataBlock,
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
