# Archon Documentation

Archon is a tool for tracking and documenting software architecture evolution through structural analysis of source code. It utilizes Git history, structural graph extraction, and LLM-powered changelog generation.

## Component Dependency Graph

```mermaid
graph TD
    cmd_archon[cmd/archon] --> internal_app[internal/app]
    cmd_archon --> internal_config[internal/config]
    cmd_archon --> internal_exitcode[internal/exitcode]
    cmd_archon --> internal_git[internal/git]
    cmd_archon --> internal_llm[internal/llm]
    cmd_archon --> internal_log[internal/log]
    cmd_archon --> internal_mcp[internal/mcp]
    
    internal_app --> internal_config
    internal_app --> internal_exitcode
    internal_app --> internal_fingerprint[internal/fingerprint]
    internal_app --> internal_git
    internal_app --> internal_graph[internal/graph]
    internal_app --> internal_lang[internal/lang]
    internal_app --> internal_lang_golang[internal/lang/golang]
    internal_app --> internal_llm
    internal_app --> internal_log
    internal_app --> internal_prompt[internal/prompt]
    
    internal_mcp --> internal_app
    internal_mcp --> internal_exitcode
    internal_mcp --> internal_fingerprint
    
    internal_fingerprint --> internal_graph
    internal_fingerprint --> internal_lang
    internal_git --> internal_lang
    internal_git --> internal_log
    internal_lang --> internal_graph
    internal_lang_golang --> internal_graph
    internal_lang_golang --> internal_lang
    internal_lang_golang --> internal_log
    internal_llm --> internal_graph
    internal_llm --> internal_log
    internal_prompt --> internal_config
```

## Core Modules

### `internal/app`
The orchestration layer. It manages the Git repository state and coordinates the extraction of graph structures and API signatures. Key features include:
- `Structure(rev)`: Extracts the full dependency graph and API set for a given revision.
- `Diff()`, `Sync()`, and `Changelog()`: Commands for comparing architecture states and generating documentation.

### `internal/mcp`
Provides the Model Context Protocol server interface. This allows external LLM agents to inspect the architecture of the repository directly via Archon's logic.
- `Server`: Wraps the `App` instance to expose structural data as tools.
- `Args`: Defines inputs for drift comparison (`From`, `To` revisions).

### `internal/fingerprint`
Responsible for stability checking. It defines the `Lockfile` format which stores the canonical representation of the repository's structure (`graph` and `APIs`) and its hash.

### `internal/lang`
Abstracts language-specific parsing. The `Extractor` interface allows adding new languages (currently `golang` is implemented) to perform source code analysis.

## Key Types

| Type | Description |
| :--- | :--- |
| `graph.Graph` | A collection of nodes and edges representing project structure. |
| `lang.APISet` | A list of packages and their exported signatures. |
| `config.Values` | Project-specific configuration for LLM integration and generators. |
| `fingerprint.Lockfile` | Persistence layer for architecture state snapshots. |

## Changelog Summary (Recent Changes)

*   **Added**: `internal/mcp` module for LLM-based tool integration.
*   **Added**: Integration between `cmd/archon` and `internal/mcp`.
*   **Updated**: `internal/llm.Client` now includes `Timeout` configuration.
*   **Added**: Enhanced reporting helpers `FormatAPIs` and `FormatGraph` in `internal/app`.
