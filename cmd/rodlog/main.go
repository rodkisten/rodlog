package main

import (
	"context"
	"flag"
	"fmt"
	"io"
	"os"
	"os/exec"
	"os/signal"
	"strconv"
	"syscall"

	"github.com/alecthomas/chroma/v2/styles"
	"github.com/rodkisten/rodlog"
)

// run is the CLI boundary. Injected streams make parsing testable without a
// real terminal. Exit codes distinguish usage errors from child failures.
func run(args []string, stdin io.Reader, stdout, stderr io.Writer) int {
	if len(args) > 0 {
		switch args[0] {
		case "tools":
			for _, spec := range rodlog.ToolInventory() {
				fmt.Fprintf(stdout, "%s\t%s\t%s\n", spec.Tool, spec.Commands[0], spec.Description)
			}
			return 0
		case "themes":
			fmt.Fprintln(stdout, "Built-in themes:")
			for _, th := range rodlog.BuiltinThemes() {
				fmt.Fprintf(stdout, "  %-14s %s (syntax: %s)\n", th.Name, th.Description, th.Syntax.Style)
			}
			fmt.Fprintf(stdout, "\nChroma syntax styles (%d available): %v\n", len(styles.Names()), styles.Names())
			return 0
		case "init-config":
			fmt.Fprint(stdout, defaultConfigTemplate)
			return 0
		case "version":
			fmt.Fprintln(stdout, "rodlog v0.2.0-dev")
			return 0
		}
	}

	// Exec mode starts a child; otherwise we format an existing stdin stream.
	commandMode := len(args) > 0 && args[0] == "exec"
	if commandMode {
		args = args[1:]
	}
	configPath, configErr := findConfigPath(args)
	if configErr != nil {
		fmt.Fprintln(stderr, configErr)
		return 2
	}
	config, configErr := loadConfig(configPath)
	if configErr != nil {
		fmt.Fprintln(stderr, configErr)
		return 2
	}

	flags := flag.NewFlagSet("rodlog", flag.ContinueOnError)
	flags.String("config", configPath, "TOML configuration file")
	flags.SetOutput(stderr)

	// Core streaming & output flags
	mode := flags.String("mode", env("RODLOG_MODE", fallback(config.Mode, "enhance")), "raw|enhance|transform")
	tool := flags.String("tool", env("RODLOG_TOOL", fallback(config.Tool, "auto")), "auto|npm|typescript|gcloud|wrangler|docker|vitest|node-test|rust|pnpm|go")
	color := flags.String("color", env("RODLOG_COLOR", fallback(config.Color, "auto")), "auto|always|never")
	output := flags.String("output", env("RODLOG_OUTPUT", fallback(config.Output, "terminal")), "terminal|jsonl|github")
	themeName := flags.String("theme", env("RODLOG_THEME", fallback(config.Theme, "default")), "built-in theme: default|tokyo-night|dracula|catppuccin|github-dark|nord")
	cardWidth := flags.Int("card-width", envInt("RODLOG_CARD_WIDTH", config.CardWidth), "max diagnostic card width in columns (24-160)")
	cardBorder := flags.String("card-border", env("RODLOG_CARD_BORDER", ""), "card border style override: rounded|normal|none")

	// Syntax highlighting flags (Chroma)
	syntaxStyle := flags.String("syntax-style", env("RODLOG_SYNTAX_STYLE", config.Syntax.Style), "Chroma syntax theme (e.g. monokai, dracula, github-dark, nord)")
	syntaxProfile := flags.String("syntax-profile", env("RODLOG_SYNTAX_PROFILE", config.Syntax.Profile), "syntax color depth: auto|truecolor|ansi256|ansi16")
	syntaxLexer := flags.String("syntax-lexer", env("RODLOG_SYNTAX_LEXER", config.Syntax.Lexer), "force Chroma lexer (e.g. json, yaml, bash, dockerfile, typescript, go)")

	// Per-token semantic highlight flags
	hlCode := flags.String("highlight-code", env("RODLOG_HIGHLIGHT_CODE", config.Highlight.Code), "error code token color (#RRGGBB or SGR)")
	hlLocation := flags.String("highlight-location", env("RODLOG_HIGHLIGHT_LOCATION", config.Highlight.Location), "file:line:col token color (#RRGGBB or SGR)")
	hlURL := flags.String("highlight-url", env("RODLOG_HIGHLIGHT_URL", config.Highlight.URL), "URL token color (#RRGGBB or SGR)")
	hlError := flags.String("highlight-error", env("RODLOG_HIGHLIGHT_ERROR", config.Highlight.Error), "error keyword token color")
	hlWarning := flags.String("highlight-warning", env("RODLOG_HIGHLIGHT_WARNING", config.Highlight.Warning), "warning keyword token color")
	hlSuccess := flags.String("highlight-success", env("RODLOG_HIGHLIGHT_SUCCESS", config.Highlight.Success), "success keyword token color")
	hlDuration := flags.String("highlight-duration", env("RODLOG_HIGHLIGHT_DURATION", config.Highlight.Duration), "duration/size token color")
	hlCLI := flags.String("highlight-cli", env("RODLOG_HIGHLIGHT_CLI", config.Highlight.CLI), "CLI binary token color")

	// Docker BuildKit flags
	dockerSeparator := flags.String("docker-separator", env("RODLOG_DOCKER_SEPARATOR", config.Docker.Separator), "divider shown beneath Docker BuildKit step cards")
	dockerBadgeBG := flags.String("docker-badge-bg", env("RODLOG_DOCKER_BADGE_BG", config.Docker.Colors.BadgeBackground), "Docker badge background (#RRGGBB or ANSI SGR)")
	dockerBadgeText := flags.String("docker-badge-text", env("RODLOG_DOCKER_BADGE_TEXT", config.Docker.Colors.BadgeText), "Docker badge text color")
	dockerDone := flags.String("docker-done", env("RODLOG_DOCKER_DONE", config.Docker.Colors.Done), "Docker DONE status color")
	dockerCached := flags.String("docker-cached", env("RODLOG_DOCKER_CACHED", config.Docker.Colors.Cached), "Docker CACHED status color")
	dockerError := flags.String("docker-error", env("RODLOG_DOCKER_ERROR", config.Docker.Colors.Error), "Docker ERROR status color")
	dockerDivider := flags.String("docker-divider-color", env("RODLOG_DOCKER_DIVIDER_COLOR", config.Docker.Colors.Separator), "Docker divider color")

	// Global semantic palette flags
	semanticError := flags.String("semantic-error", env("RODLOG_SEMANTIC_ERROR", config.Semantic.Error), "semantic error color")
	semanticWarning := flags.String("semantic-warning", env("RODLOG_SEMANTIC_WARNING", config.Semantic.Warning), "semantic warning color")
	semanticSuccess := flags.String("semantic-success", env("RODLOG_SEMANTIC_SUCCESS", config.Semantic.Success), "semantic success color")
	semanticInfo := flags.String("semantic-info", env("RODLOG_SEMANTIC_INFO", config.Semantic.Info), "semantic info color")
	semanticMuted := flags.String("semantic-muted", env("RODLOG_SEMANTIC_MUTED", config.Semantic.Muted), "semantic muted color")
	semanticCode := flags.String("semantic-code", env("RODLOG_SEMANTIC_CODE", config.Semantic.Code), "semantic diagnostic code color")
	semanticLocation := flags.String("semantic-location", env("RODLOG_SEMANTIC_LOCATION", config.Semantic.Location), "semantic file location color")

	// Icon & preset flags
	iconError := flags.String("icon-error", env("RODLOG_ICON_ERROR", config.Icons.Error), "error diagnostic icon")
	iconWarning := flags.String("icon-warning", env("RODLOG_ICON_WARNING", config.Icons.Warning), "warning diagnostic icon")
	iconInfo := flags.String("icon-info", env("RODLOG_ICON_INFO", config.Icons.Info), "info diagnostic icon")
	iconSuggestion := flags.String("icon-suggestion", env("RODLOG_ICON_SUGGESTION", config.Icons.Suggestion), "suggestion icon")
	iconExplanation := flags.String("icon-explanation", env("RODLOG_ICON_EXPLANATION", config.Icons.Explanation), "explanation icon")
	presetPath := flags.String("preset", env("RODLOG_PRESET", "default"), "semantic preset: default|off|path to TOML file")
	jsonOutput := flags.Bool("json", false, "request JSON for allowlisted commands and show a summary table in transform mode")

	if err := flags.Parse(args); err != nil {
		return 2
	}
	if *mode != "raw" && *mode != "enhance" && *mode != "transform" {
		fmt.Fprintln(stderr, "invalid mode:", *mode)
		return 2
	}
	validTools := map[string]bool{
		"auto": true, "npm": true, "typescript": true, "gcloud": true,
		"wrangler": true, "docker": true, "vitest": true, "node-test": true, "rust": true, "pnpm": true, "go": true,
	}
	if !validTools[*tool] {
		fmt.Fprintln(stderr, "invalid tool:", *tool)
		return 2
	}
	if *color != "auto" && *color != "always" && *color != "never" {
		fmt.Fprintln(stderr, "invalid color:", *color)
		return 2
	}
	if *output != "terminal" && *output != "jsonl" && *output != "github" {
		fmt.Fprintln(stderr, "invalid output:", *output)
		return 2
	}

	themeBundle, themeErr := rodlog.ResolveTheme(*themeName)
	if themeErr != nil {
		fmt.Fprintln(stderr, themeErr)
		return 2
	}

	enableColor := *color == "always" || (*color == "auto" && os.Getenv("NO_COLOR") == "" && os.Getenv("TERM") != "dumb" && isTerminal(stdout))

	dockerColors, colorErr := rodlog.ParseDockerColors(rodlog.DockerColors{
		BadgeBackground: fallback(*dockerBadgeBG, themeBundle.Docker.BadgeBackground),
		BadgeText:       fallback(*dockerBadgeText, themeBundle.Docker.BadgeText),
		Done:            fallback(*dockerDone, themeBundle.Docker.Done),
		Cached:          fallback(*dockerCached, themeBundle.Docker.Cached),
		Error:           fallback(*dockerError, themeBundle.Docker.Error),
		Separator:       fallback(*dockerDivider, themeBundle.Docker.Separator),
	})
	if colorErr != nil {
		fmt.Fprintln(stderr, colorErr)
		return 2
	}

	semanticColors, semanticErr := rodlog.ParseSemanticColors(rodlog.SemanticColors{
		Error:    fallback(*semanticError, fallback(*hlError, themeBundle.Semantic.Error)),
		Warning:  fallback(*semanticWarning, fallback(*hlWarning, themeBundle.Semantic.Warning)),
		Success:  fallback(*semanticSuccess, fallback(*hlSuccess, themeBundle.Semantic.Success)),
		Info:     fallback(*semanticInfo, themeBundle.Semantic.Info),
		Muted:    fallback(*semanticMuted, fallback(*hlDuration, fallback(*hlCLI, themeBundle.Semantic.Muted))),
		Code:     fallback(*semanticCode, fallback(*hlCode, themeBundle.Semantic.Code)),
		Location: fallback(*semanticLocation, fallback(*hlLocation, fallback(*hlURL, themeBundle.Semantic.Location))),
	})
	if semanticErr != nil {
		fmt.Fprintln(stderr, semanticErr)
		return 2
	}

	if _, hlErr := rodlog.ParseHighlightColors(rodlog.HighlightColors{
		Code: *hlCode, Location: *hlLocation, URL: *hlURL,
		Error: *hlError, Warning: *hlWarning, Success: *hlSuccess,
		Duration: *hlDuration, CLI: *hlCLI,
	}); hlErr != nil {
		fmt.Fprintln(stderr, hlErr)
		return 2
	}

	icons, iconErr := rodlog.ParseDiagnosticIcons(rodlog.DiagnosticIcons{
		Error: *iconError, Warning: *iconWarning, Info: *iconInfo,
		Suggestion: *iconSuggestion, Explanation: *iconExplanation,
	})
	if iconErr != nil {
		fmt.Fprintln(stderr, iconErr)
		return 2
	}

	syntaxTheme, syntaxErr := rodlog.ParseSyntaxTheme(rodlog.SyntaxTheme{
		Style:   fallback(*syntaxStyle, themeBundle.Syntax.Style),
		Profile: fallback(*syntaxProfile, themeBundle.Syntax.Profile),
		Lexer:   *syntaxLexer,
	})
	if syntaxErr != nil {
		fmt.Fprintln(stderr, syntaxErr)
		return 2
	}

	var preset *rodlog.SemanticPreset
	if *presetPath != "off" {
		if *presetPath == "default" {
			loaded := rodlog.DefaultSemanticPreset()
			preset = &loaded
		} else {
			data, err := os.ReadFile(*presetPath)
			if err != nil {
				fmt.Fprintln(stderr, "preset:", err)
				return 2
			}
			loaded, err := rodlog.ParseSemanticPreset(data)
			if err != nil {
				fmt.Fprintln(stderr, "preset:", err)
				return 2
			}
			preset = &loaded
		}
		if *cardWidth >= 24 && *cardWidth <= 160 {
			preset.Diagnostics.Card.MaxWidth = *cardWidth
		}
		if *cardBorder != "" {
			preset.Diagnostics.Card.Border = *cardBorder
		}
	}

	engine := rodlog.New(rodlog.Options{
		Mode:            rodlog.Mode(*mode),
		Tool:            rodlog.Tool(*tool),
		Color:           enableColor,
		DockerSeparator: *dockerSeparator,
		DockerColors:    dockerColors,
		SemanticColors:  semanticColors,
		DiagnosticIcons: icons,
		SyntaxTheme:     syntaxTheme,
		Preset:          preset,
	})

	switch *output {
	case "jsonl":
		engine.Renderer = rodlog.JSONLRenderer{}
	case "github":
		engine.Renderer = rodlog.AnnotationRenderer{}
	}

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	if !commandMode {
		if err := engine.Process(ctx, stdin, stdout); err != nil {
			fmt.Fprintln(stderr, err)
			return 1
		}
		return 0
	}

	argv := flags.Args()
	if len(argv) == 0 {
		fmt.Fprintln(stderr, "usage: rodlog exec [flags] -- COMMAND [ARGS...]")
		return 2
	}
	if *tool == "auto" {
		if detected := rodlog.DetectCommand(argv); detected != rodlog.Generic {
			engine.Options.Tool = detected
		}
	}
	if *jsonOutput {
		argv, _ = rodlog.JSONCommand(argv)
	}

	cmd := exec.CommandContext(ctx, argv[0], argv[1:]...)
	pipeReader, pipeWriter := io.Pipe()
	cmd.Stdout = pipeWriter
	cmd.Stderr = pipeWriter

	if err := cmd.Start(); err != nil {
		fmt.Fprintln(stderr, err)
		return 127
	}

	waitCh := make(chan error, 1)
	go func() {
		err := cmd.Wait()
		pipeWriter.Close()
		waitCh <- err
	}()

	processErr := engine.Process(ctx, pipeReader, stdout)
	if processErr != nil {
		fmt.Fprintln(stderr, processErr)
		if cmd.Process != nil {
			_ = cmd.Process.Kill()
		}
	}

	waitErr := <-waitCh
	if processErr != nil {
		return 1
	}
	if waitErr == nil {
		return 0
	}
	if exit, ok := waitErr.(*exec.ExitError); ok {
		return exit.ExitCode()
	}
	fmt.Fprintln(stderr, waitErr)
	return 1
}

func env(key, defaultVal string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return defaultVal
}

func envInt(key string, defaultVal int) int {
	if v := os.Getenv(key); v != "" {
		if n, err := strconv.Atoi(v); err == nil {
			return n
		}
	}
	return defaultVal
}

// isTerminal conservatively detects a character device for auto-color.
func isTerminal(w io.Writer) bool {
	f, ok := w.(*os.File)
	if !ok {
		return false
	}
	s, e := f.Stat()
	return e == nil && (s.Mode()&os.ModeCharDevice) != 0
}

const defaultConfigTemplate = `# rodlog configuration (~/.config/rodlog/config.toml)
mode = "enhance"       # raw | enhance | transform
tool = "auto"          # auto | npm | typescript | gcloud | wrangler | docker | vitest | node-test
color = "auto"         # auto | always | never
output = "terminal"    # terminal | jsonl | github
theme = "tokyo-night"  # default | tokyo-night | dracula | catppuccin | github-dark | nord
card_width = 72        # 24..160 columns

[syntax]
style = "tokyonight-night"
profile = "truecolor"  # auto | truecolor | ansi256 | ansi16
lexer = ""             # optional forced lexer (e.g. "json", "bash", "dockerfile")

[highlight]
code = "#BB9AF7"
location = "#7DCFFF"
url = "#7AA2F7"
error = "#F7768E"
warning = "#E0AF68"
success = "#9ECE6A"
duration = "#565F89"
cli = "#565F89"

[docker]
separator = "────────────────────────────────────"

[docker.colors]
badge_background = "#1A1B26"
badge_text = "#C0CAF5"
done = "#9ECE6A"
cached = "#565F89"
error = "#F7768E"
separator = "#7AA2F7"
`

func main() { os.Exit(run(os.Args[1:], os.Stdin, os.Stdout, os.Stderr)) }
