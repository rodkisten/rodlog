# 🌈 rodlog

> **One stream. Many tools. Beautiful diagnostics.** ✨

A fast Go-powered streaming syntax & semantic highlighter for **npm 📦, TypeScript 🟦, gcloud ☁️, Wrangler ⚡, Docker 🐳, Vitest 🧪 and Node.js**.

## 🚀 Quick start

```bash
go mod tidy
go test -race ./...
go build -o rodlog ./cmd/rodlog

# Built-in themes: default | tokyo-night | dracula | catppuccin | github-dark | nord
npm run build 2>&1 | ./rodlog --color=always --theme=tokyo-night
./rodlog exec --color=always --theme=dracula -- npx tsc --noEmit
# Lossless enriched TypeScript diagnostics for Docker/BuildKit transport:
./rodlog tsc --transport=jsonl -p tsconfig.json
./rodlog themes
./rodlog init-config > ~/.config/rodlog/config.toml
```

## 🎨 Configurable Highlighting Flags

- `--theme`: `default`, `tokyo-night`, `dracula`, `catppuccin`, `github-dark`, `nord`
- `--syntax-style`: Any Chroma style (`monokai`, `tokyonight-night`, `dracula`, `catppuccin-mocha`, `github-dark`, `nord`, etc.)
- `--syntax-profile`: `auto|truecolor|ansi256|ansi16`
- `--syntax-lexer`: Force a specific Chroma lexer (`json`, `yaml`, `toml`, `bash`, `dockerfile`, `typescript`, `go`, `python`, `sql`, `diff`)
- `--highlight-code`, `--highlight-location`, `--highlight-url`, `--highlight-error`, `--highlight-warning`, `--highlight-success`, `--highlight-duration`, `--highlight-cli`
- `--card-width` (`24..160`) and `--card-border` (`rounded|normal|none`)


## 🐳 TypeScript intelligence inside Docker / BuildKit

`rodlog tsc` can expose its Compiler API diagnostics as a lossless JSONL transport:

```bash
rodlog tsc --transport=jsonl -p tsconfig.json
```

Each line is the enriched `Diagnostic` object itself, including related declarations,
signatures, focused parameters/properties/constraints, and documentation when available.
When these lines appear inside BuildKit output, an outer rodlog process recognizes the
embedded `tool: "typescript"` diagnostic and delegates it to the canonical TypeScript
renderer. Docker is responsible only for hierarchy and indentation.

This makes the intended nested pipeline:

```text
Docker → pnpm → rodlog tsc --transport=jsonl → TypeScript Intelligence
                                             ↓
                                  canonical terminal renderer
```
