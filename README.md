# 🌈 rodlog

> **One stream. Many tools. Beautiful diagnostics.** ✨

A fast Go-powered streaming syntax & semantic highlighter for **npm 📦, TypeScript 🟦, gcloud ☁️, Wrangler ⚡, Docker 🐳, Vitest 🧪 and Node.js**.

## 🚀 Quick start

```bash
go mod download
go test -race ./...
go build -o rodlog ./cmd/rodlog

# Built-in themes: default | tokyo-night | dracula | catppuccin | github-dark | nord
npm run build 2>&1 | ./rodlog --color=always --theme=tokyo-night
./rodlog exec --color=always --theme=dracula -- npx tsc --noEmit
# Lossless enriched TypeScript diagnostics for Docker/BuildKit transport:
./rodlog tsc --transport=jsonl -p tsconfig.json

# Generate the huge BuildKit fixture from real Compiler API diagnostics:
scripts/build-docker-typescript-fixture.sh \
  ./rodlog \
  testdata/typescript-intelligence/tsconfig.json \
  /tmp/rodlog-docker-typescript.log
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


### Generated Docker torture fixture

The Docker + TypeScript torture fixture is generated from the real TypeScript
intelligence adapter instead of being hand-maintained:

```bash
scripts/build-docker-typescript-fixture.sh \
  ./rodlog \
  testdata/typescript-intelligence/tsconfig.json \
  /tmp/rodlog-docker-typescript.log

./rodlog \
  --color=always \
  --theme=tokyo-night \
  --syntax-profile=truecolor \
  --mode=transform \
  --tool=auto \
  --detail=normal \
  --symbols=unicode \
  < /tmp/rodlog-docker-typescript.log
```

This fixture transports every enriched diagnostic from the Compiler API through
a BuildKit-shaped stream. It is intentionally large and acts as a visual and
semantic regression test for nested TypeScript diagnostics.


## 💡 TypeScript Quick Fix previews

The TypeScript Intelligence adapter also asks the TypeScript Language Service for
code-fix actions. Fixes are **preview-only**: rodlog never edits source files in this
mode.

A diagnostic may include:

```text
✦ Quick fix: Change spelling to 'toUpperCase'
  ⌖ src/errors-fixes.ts:3:24
  - toUppercase
  + toUpperCase
```

The same `fixes` data is preserved by `--transport=jsonl`, so a TypeScript
diagnostic crossing Docker/BuildKit keeps its Language Service suggestions and edit
preview. Applying fixes is intentionally a separate future command and is not enabled
implicitly.
