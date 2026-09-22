# Packages

Archon is a system designed to analyze, track, and document the structural evolution of software repositories. It leverages Git for version control, custom language extractors to map code graphs and APIs, and LLMs to process changes, while providing an MCP-compliant interface for external integration.

### github.com/j75689/archon/cmd/archon
This package serves as the primary entry point for the Archon CLI application. It handles execution flow by orchestrating dependencies from the internal packages to process repository data.

### github.com/j75689/archon/internal/app
This package provides the central `App` struct that orchestrates the core logic of the system, including diffing, synchronization, and generating documentation. It exposes methods to extract repository structures and compare API sets across different Git revisions.

### github.com/j75689/archon/internal/config
This package manages the configuration for the Archon system, including loaders for generator definitions and application settings. It defines the structure for `Values` and `Generator` types that control how documentation or analysis prompts are applied.

### github.com/j75689/archon/internal/doc
This package provides utility functions for manipulating documentation files, specifically for managing anchored regions. It includes capabilities to extract, replace, and normalize content within source files based on specified IDs.

### github.com/j75689/archon/internal/exitcode
This package defines the standard exit codes used throughout the application to signal execution results. It provides constants such as `OK`, `Fail`, and `Gate` to ensure consistent reporting.

### github.com/j75689/archon/internal/fingerprint
This package handles the identification and diffing of codebases through hashing and lockfiles. It provides methods to compute structural hashes, compare `APISet` objects, and manage the persistence of snapshots via `Lockfile`.

### github.com/j75689/archon/internal/git
This package wraps Git functionality to provide repository insights such as tag descriptions, commit log subjects, and snapshot extraction. It allows the system to resolve revisions and verify repository states.

### github.com/j75689/archon/internal/graph
This package provides data structures and tools for representing and analyzing code as a directed graph. It includes functionality to compute differences between graphs, format reports, and render Mermaid diagrams.

### github.com/j75689/archon/internal/lang
This package defines the interfaces and types required for language-specific analysis. It specifies the `Extractor` interface, which implementations use to match files and extract API definitions or structural graphs from snapshots.

### github.com/j75689/archon/internal/lang/golang
This package implements the `lang.Extractor` interface for the Go programming language. It provides logic for matching Go files and extracting detailed API and structural data from them.

### github.com/j75689/archon/internal/llm
This package provides a client for interacting with Large Language Models to assist in code analysis. It enables the generation of completion responses and delta summaries based on graph differences and commit subjects.

### github.com/j75689/archon/internal/log
This package defines a logging abstraction for the system. It includes both a functional `Logger` interface and concrete implementations like a `Writer` for outputting logs to io.Writers at different verbosity levels.

### github.com/j75689/archon/internal/mcp
This package implements an MCP (Model Context Protocol) server to expose Archon’s functionality to external clients. It provides HTTP handlers and session management, allowing services to query repository states and synchronization results.

### github.com/j75689/archon/internal/prompt
This package manages the templates and loading logic for LLM prompts used during documentation or analysis. It allows for the integration of built-in templates and custom generator configurations to produce context-aware data.

### Structure diff vs last sync
- github.com/j75689/archon/internal/mcp: func NewHTTPServer(addr string, handler http.Handler) *http.Server
- github.com/j75689/archon/internal/mcp: func ServeHTTP(ctx context.Context, srv *http.Server) error
- github.com/j75689/archon/internal/mcp: var Version
