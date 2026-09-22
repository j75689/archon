# Architecture

Archon is a modular tool designed for analyzing, documenting, and detecting drift in codebase structures and APIs. It leverages Git for version control snapshots, language-specific extractors to map graph dependencies and API signatures, and LLM-driven generation to provide insights or changelogs based on those structural differences.

## System Graph

```mermaid
graph TD
    cmd_archon[github.com/j75689/archon/cmd/archon]
    internal_app[github.com/j75689/archon/internal/app]
    internal_config[github.com/j75689/archon/internal/config]
    internal_doc[github.com/j75689/archon/internal/doc]
    internal_exitcode[github.com/j75689/archon/internal/exitcode]
    internal_fingerprint[github.com/j75689/archon/internal/fingerprint]
    internal_git[github.com/j75689/archon/internal/git]
    internal_graph[github.com/j75689/archon/internal/graph]
    internal_lang[github.com/j75689/archon/internal/lang]
    internal_lang_golang[github.com/j75689/archon/internal/lang/golang]
    internal_llm[github.com/j75689/archon/internal/llm]
    internal_log[github.com/j75689/archon/internal/log]
    internal_mcp[github.com/j75689/archon/internal/mcp]
    internal_prompt[github.com/j75689/archon/internal/prompt]
    cmd_archon --> internal_app
    cmd_archon --> internal_config
    cmd_archon --> internal_exitcode
    cmd_archon --> internal_git
    cmd_archon --> internal_llm
    cmd_archon --> internal_log
    cmd_archon --> internal_mcp
    internal_app --> internal_config
    internal_app --> internal_exitcode
    internal_app --> internal_fingerprint
    internal_app --> internal_git
    internal_app --> internal_graph
    internal_app --> internal_lang
    internal_app --> internal_lang_golang
    internal_app --> internal_llm
    internal_app --> internal_log
    internal_app --> internal_prompt
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
    internal_mcp --> internal_app
    internal_mcp --> internal_exitcode
    internal_mcp --> internal_fingerprint
    internal_prompt --> internal_config
```

## Key Components

### internal/app
This package serves as the central orchestrator, providing methods to compare code revisions, extract structural graphs and APIs, and manage synchronization tasks. It interfaces with Git for repository access and utilizes specialized extractors and LLM clients to process codebase changes.

### internal/fingerprint
This package is responsible for generating, comparing, and managing project fingerprints. It facilitates the creation of lockfiles that capture the state of a graph and its associated APIs, allowing for diffing and hash verification.

### internal/lang/golang
This package implements language-specific logic for Go projects. It provides the `Extractor` implementation to scan snapshots and resolve dependency graphs and exported API signatures.

### internal/mcp
This package provides the Model Context Protocol (MCP) server implementation for Archon. It enables external agents to interact with the application’s core logic through standardized transports, including HTTP handlers and stdio execution.

## API Reference

| Package | Key Exported Names |
| :--- | :--- |
| `cmd/archon` | `errExit` |
| `internal/app` | `App`, `New`, `Sync`, `Structure`, `Diff`, `Changelog` |
| `internal/config` | `Load`, `Values`, `Generator` |
| `internal/doc` | `ExtractRegion`, `ReplaceRegion`, `NewDocument`, `AppendAnchor` |
| `internal/exitcode` | `OK`, `Fail`, `Gate` |
| `internal/fingerprint` | `Lockfile`, `DiffAPIs`, `Hash`, `NewLockfile` |
| `internal/git` | `Repo`, `Open`, `Snapshot`, `LogSubjects`, `DescribeTag` |
| `internal/graph` | `Graph`, `Diff`, `RenderMermaid`, `DiffGraphs` |
| `internal/lang` | `Extractor`, `Snapshot`, `APISet` |
| `internal/lang/golang` | `Extractor`, `WantFile` |
| `internal/llm` | `Client`, `Complete`, `Delta` |
| `internal/log` | `Logger`, `Writer`, `Nop` |
| `internal/mcp` | `Server`, `New`, `RunStdio`, `HTTPHandler`, `Version` |
| `internal/prompt` | `Render`, `Load`, `BuiltIn` |

## Recent Changes

The `internal/mcp` package has been updated with new exported functions `NewHTTPServer` and `ServeHTTP` for HTTP-based server management, along with a new `Version` variable.
