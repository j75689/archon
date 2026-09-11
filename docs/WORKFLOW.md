# Archon Architecture Documentation

Archon is a Go-based tool designed to manage project documentation by synchronizing code structure, API surfaces, and technical context using LLMs.

## Dependency Graph

```mermaid
graph TD
    cmd_archon[cmd/archon] --> internal_app[internal/app]
    cmd_archon --> internal_config[internal/config]
    cmd_archon --> internal_exitcode[internal/exitcode]
    cmd_archon --> internal_git[internal/git]
    cmd_archon --> internal_llm[internal/llm]
    cmd_archon --> internal_log[internal/log]

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
The orchestration layer. It exposes the primary CLI actions:
* `Changelog()`: Generates changelogs based on code evolution.
* `Check()`: Verifies if documentation is up to date with the code.
* `Diff()`: Reports structural changes in the codebase.
* `Sync()`: Synchronizes documentation blocks with code changes.

### `internal/fingerprint`
Handles the identification of code state using `Lockfile` structures. It provides hashing and diffing capabilities for both graphs and API sets to determine if documentation needs regeneration.

### `internal/lang` & `internal/lang/golang`
Provides the abstraction for code analysis. The `Extractor` interface allows Archon to support multiple languages, with a concrete implementation for Go that extracts dependency graphs and API signatures.

### `internal/llm`
Communicates with LLM providers to generate content based on the structured data provided by the graph and API extractors.

### `internal/doc`
A utility package for surgical manipulation of documentation files (e.g., Markdown), handling the injection and replacement of content within specific "anchor" regions.

## Data Structures

The system relies on three primary data pillars:
1. **Graph**: Represents package relationships and node dependencies.
2. **API Set**: Contains package-level definitions, documentation, and signatures.
3. **Prompt Data**: A composite structure passed to LLM templates containing:
    * `Graph`: The serialized dependency structure.
    * `APIs`: The serialized surface area of the code.
    * `Diff`: The detected structural changes between code versions.
