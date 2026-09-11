# Archon Documentation

Archon is a tool for documenting and managing software project architecture, specifically focusing on dependency graphs and API evolution.

## Packages

### `cmd/archon`
The entry point for the Archon CLI application. It handles process initialization and error reporting.

### `internal/app`
Orchestrates the core logic, including changelog generation, dependency checking, and synchronization of documentation with code state.

### `internal/config`
Manages application configuration, including generator definitions for automated documentation tasks.

### `internal/doc`
Provides utilities for manipulating documentation files, specifically handling anchors and regions to maintain machine-readable metadata within text documents.

### `internal/exitcode`
Defines standard exit codes for the CLI to distinguish between success, documentation gating, and general failures.

### `internal/fingerprint`
Responsible for calculating the state of a repository by hashing dependency graphs and API sets. It manages lockfiles to detect drift between code and documentation.

### `internal/git`
Provides wrappers around Git operations, enabling snapshotting of repository states, diffing changes between revisions, and resolving repository metadata.

### `internal/graph`
Defines the structure of dependency graphs. It provides logic for diffing graphs and rendering them into visual formats like Mermaid.

### `internal/lang`
Defines the interfaces required to support multi-language analysis, specifically providing methods for code extraction and API identification.

### `internal/lang/golang`
Implements the `lang.Extractor` interface for Go, allowing Archon to analyze Go modules, extract structural graphs, and identify public API signatures.

### `internal/llm`
Handles interactions with Large Language Models to generate intelligent summaries or documentation based on the detected structural and API changes.

### `internal/log`
Provides a consistent logging interface across the application, supporting varying levels of verbosity.

### `internal/prompt`
Manages templates used for interacting with LLMs. It includes a rendering engine to inject graph, API, and diff data into configured prompts.
