# Archon

A Go CLI that treats the first-party import graph and exported API summaries as **inputs** to configurable LLM generators. CI gates on a deterministic **structure fingerprint**, not on LLM markdown.

This repository dogfoods the gate: [`.archon/graph.json`](.archon/graph.json) is the committed fingerprint. `archon sync` writes `docs/ARCHITECTURE.md`, `docs/WORKFLOW.md`, and `docs/PACKAGES.md` when that fingerprint is stale and an LLM is configured.

## Requirements

- Go 1.26+
- `git` on `PATH` (no fetch; missing objects fail)

## Install

```bash
go install github.com/j75689/archon/cmd/archon@latest
```

From a clone:

```bash
make install
# or
make build   # writes bin/archon
```

## Commands

Shared flags: `--from` (left revision), `--to` (default `HEAD`), `--doc` (unused by the fingerprint gate), `-v` / `-vv` (progress on stderr; repeat for per-file).

| Command | What it does | Exit 1 when |
|---|---|---|
| `archon check` | Compare the structure fingerprint at `--to` to `.archon/graph.json`. Ignores `--from`. No LLM. | Lockfile missing or hash stale |
| `archon diff` | Print first-party graph and exported-signature changes `--from` → `--to`. Default `--from` is the latest reachable tag. | Graph or APIs changed |
| `archon sync` | If the fingerprint is stale and an LLM is configured, write generator markdown atomically and update the lockfile. | Never (stale without LLM, or LLM failure, still exits 0) |
| `archon changelog` | Print the same structure report as `diff`. No LLM. Never writes files. | Structure changed |

Exit **2** is an execution error (not a git repo, missing `git`, unreadable revision, parse failure, I/O).

`--from` is required for `diff` / `changelog` if there is no reachable tag (`pass --from or create a tag`). `check` and `sync` ignore `--from`.

## What counts as structure

The fingerprint hashes the canonical JSON of:

- **Graph:** first-party packages in the same Go module (from root `go.mod`)
- **APIs:** package comments plus exported signatures (no function bodies)

**Out of the graph and APIs:** stdlib, third-party, `*_test.go`, `vendor/`, `testdata/`, `//go:build ignore`, generated files (`Code generated` … `DO NOT EDIT`). Isolated packages (no first-party imports) still appear as nodes. Imports of packages that have no included `.go` files do not create edges.

Private / body-only edits do not change the fingerprint, do not call the LLM, and do not fail `check`. Adding an exported signature without new imports **does** stale the fingerprint.

## Configuration

First non-empty value wins: **flags → environment → `./archon.yaml` → `~/.config/archon/config.yaml`**. Archon does **not** load `.env` files.

```yaml
# archon.yaml
model: gpt-4o-mini
base_url: https://api.openai.com/v1
generators:
  - id: architecture
    path: docs/ARCHITECTURE.md
    prompt: prompts/architecture.md
  - id: workflow
    path: docs/WORKFLOW.md
    prompt: prompts/workflow.md
  - id: packages
    path: docs/PACKAGES.md
    prompt: prompts/packages.md
```

If `generators` is omitted or empty, those three default paths are used with built-in prompts. `prompt:` is a path relative to the repo root; a missing file is an error. Built-in prompts apply only when `prompt` is empty on `architecture`, `workflow`, or `packages`. Any other `id` with no `prompt` is skipped (`generator <id>: skip (no prompt)`) and does not fail `sync`.

| Env | Meaning |
|---|---|
| `ARCHON_MODEL` | Chat model (default `gpt-4o-mini`) |
| `ARCHON_BASE_URL` | OpenAI-compatible base URL |
| `OPENAI_API_KEY` | API key (do not put keys in YAML) |

The LLM is used only when a key is set **or** `base_url` / `ARCHON_BASE_URL` is set explicitly (for Ollama, e.g. `http://localhost:11434/v1`). Requests receive the graph, package docs, and exported signatures — never function bodies or `.env*` files. Failures print one stderr line, leave generator files and the lockfile untouched, and exit 0.

`sync` with no LLM configured prints `llm: skip (no client)`, does not write the lockfile, and exits 0 (`check` still fails in CI until someone syncs with an LLM or seeds `.archon/graph.json`).

## Development

```bash
make help
make all    # fmt-check, vet, test, build, archon check
```

## License

[MIT](LICENSE)
