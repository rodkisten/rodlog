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
