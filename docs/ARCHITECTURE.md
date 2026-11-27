# 🧠 rodlog architecture

A guided tour of the command-to-diagnostic pipeline.

```mermaid
flowchart TD
 A[Command or stdin] --> B[CLI flags and mode]
 B --> C{Exec?}
 C -->|Yes| D[Detect command and start child]
 C -->|No| E[Read stdin]
 D --> F[Shared stdout/stderr pipe]
 F --> G[Streaming Engine.Process]
 E --> G
 G --> H[Detect tool and parse diagnostics]
 H --> I[Event retains original Text]
 I --> J{Output renderer}
 J --> K[Terminal: raw/enhance/transform]
 J --> L[JSONL]
 J --> M[GitHub annotations]
 K --> N[Preserve ANSI or highlight semantic tokens]
```

## Follow a log line

1. `cmd/rodlog/main.go` selects stdin or a child process, parses flags, resolves themes, and preserves the child's exit status.
2. `rodlog.go` consumes the stream incrementally and retains original text in every `Event`.
3. `semantic.go` optionally extracts structured diagnostics; unknown lines stay ordinary output.
4. `highlight.go` resolves overlapping severity, code and location spans by priority.
5. `syntax.go` uses Chroma conservatively for recognized source snippets, not arbitrary log lines.
6. `diagnostic_render.go` formats cards, with source excerpts supplied by `diagnostic_frame.go`.
7. `integrations.go` supplies machine-readable JSONL and GitHub Actions annotations.
