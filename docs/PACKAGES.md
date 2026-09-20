# Archon Documentation

Archon is a tool for architectural analysis, documentation, and drift detection. It provides insights into codebase structure and API signatures, enabling automated verification and LLM-powered insights.

## Packages

### `github.com/j75689/archon/cmd/archon`
The entry point for the Archon CLI application. It coordinates configuration, logging, and command-line execution for the core logic provided by `internal/app`.

### `github.com/j75689/archon/internal/app`
The core application logic. It orchestrates the extraction of codebase structure and API information, performs diffing, and manages synchronization tasks.

### `github.com/j75689/archon/internal/config`
Handles the loading and parsing of project-specific configuration, including generator definitions and LLM settings.

### `github.com/j75689/archon/internal/doc`
Provides utilities for embedding and updating documentation anchors within files, allowing for stable regions that can be updated without overwriting surrounding text.

### `github.com/j75689/archon/internal/exitcode`
Defines standardized exit codes for the application, facilitating integration with CI/CD pipelines.

### `github.com/j75689/archon/internal/fingerprint`
Responsible for computing hashes, creating lockfiles, and detecting drifts between codebase versions. It compares graph structures and API sets to identify changes.

### `github.com/j75689/archon/internal/git`
Wraps Git operations. It provides functionality for navigating repository history, snapshotting code at specific revisions, and describing tags.

### `github.com/j75689/archon/internal/graph`
Defines the graph data model and provides operations for computing differences between graphs, compiling graphs, and rendering them into visualization formats like Mermaid.

### `github.com/j75689/archon/internal/lang`
Defines the interfaces for source code analysis. It provides the abstractions used to extract structural and API-level data from different programming languages.

### `github.com/j75689/archon/internal/lang/golang`
Implements the `lang.Extractor` interface for Go projects. It inspects Go source code to build structural graphs and identify exported API signatures.

### `github.com/j75689/archon/internal/llm`
Handles interactions with LLM providers. It generates natural language summaries and insights based on the structural diffs provided by the graph analysis.

### `github.com/j75689/archon/internal/log`
Provides a simple, consistent logging interface used across all internal packages, with support for verbose output.

### `github.com/j75689/archon/internal/mcp`
Implements the Model Context Protocol (MCP) server. This allows AI assistants and external tools to interact with Archon’s analysis capabilities directly via standardized interfaces.

### `github.com/j75689/archon/internal/prompt`
Manages the templates used for LLM interaction. It loads and renders prompts using project data to drive automated reporting.
