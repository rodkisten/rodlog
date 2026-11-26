// Package rodlog implements streaming output enhancement with conservative tool detection.
package rodlog

import (
	"bufio"
	"context"
	"fmt"
	"io"
	"regexp"
	"slices"
	"strings"

	"github.com/charmbracelet/x/ansi"
)

// Mode controls how much transformation rodlog applies to the output stream.
type Mode string

const (
	Raw       Mode = "raw"
	Enhance   Mode = "enhance"
	Transform Mode = "transform"
)

// Tool identifies the CLI or build system that produced a log stream.
type Tool string

const (
	Auto       Tool = "auto"
	NPM        Tool = "npm"
	TypeScript Tool = "typescript"
	GCloud     Tool = "gcloud"
	Wrangler   Tool = "wrangler"
	Docker     Tool = "docker"
	Vitest     Tool = "vitest"
	NodeTest   Tool = "node-test"
	Generic    Tool = "generic"
)

// Level represents the semantic severity of a single log event.
type Level string

const (
	Info    Level = "info"
	Warning Level = "warning"
	Error   Level = "error"
	Success Level = "success"
)

// Event represents a single classified line (and optional parsed Diagnostic) in the stream.
type Event struct {
	Diagnostic *Diagnostic
	Sequence   uint64
	Tool       Tool
	Level      Level
	Text       string
}

// Options configures streaming behavior, color themes, icons, and diagnostic presets.
type Options struct {
	Mode    Mode
	Tool    Tool
	Color   bool
	MaxLine int
	// DockerSeparator replaces the default 36-character BuildKit divider.
	DockerSeparator string
	DockerColors    DockerColors
	SemanticColors  SemanticColors
	DiagnosticIcons DiagnosticIcons
	SyntaxTheme     SyntaxTheme
	Preset          *SemanticPreset
}

// Renderer allows callers to plug in a custom event presentation strategy.
type Renderer interface {
	Render(Event) ([]byte, error)
}

// Engine coordinates incremental reading, classification, and rendering.
type Engine struct {
	Options  Options
	Renderer Renderer
}

const (
	defaultReadBufferBytes = 32 * 1024
	defaultMaxLineBytes    = 1024 * 1024
	maxCardLines           = 80
	maxCardBytes           = 64 * 1024
)

// ansiPattern matches standard CSI escape sequences (used for fast pre-checks).
var ansiPattern = regexp.MustCompile("\x1b\\[[0-9;]*[A-Za-z]")

// toolDetectionOrder defines deterministic priority when multiple tool signatures could match.
// More specific formats (TypeScript, Vitest, NodeTest) are checked before general runners (NPM).
var toolDetectionOrder = []Tool{
	TypeScript,
	Vitest,
	NodeTest,
	NPM,
	GCloud,
	Wrangler,
	Docker,
}

// toolPatterns holds lowercase substring signatures for conservative tool detection.
var toolPatterns = map[Tool][]string{
	NPM:        {"npm err!", "npm warn", "npm error", "npm run", "npm notice"},
	TypeScript: {"error ts", "warning ts", "tsc ", ".tsx(", ".ts(", ".mts(", ".cts(", "found 1 error in", " errors in "},
	GCloud:     {"gcloud ", "error: (gcloud", "warning: (gcloud", "updated property", "deploying container"},
	Wrangler:   {"wrangler ", "cloudflare", "workers.dev", "wrangler.toml", "wrangler.jsonc", "✘ [error]", "▲ [warning]"},
	Docker:     {"docker ", "step 1/", "=> [", "dockerfile:", "exporting layers", "writing image", "error: failed to solve:"},
	Vitest:     {"vitest ", "vitest run", "❯ ", "test files ", "tests  ", "fail  "},
	NodeTest:   {"node --test", "✖ tests ", "ℹ tests ", "✖ failing tests:", "not ok "},
}

// Detect inspects a log line and identifies the emitting tool conservatively.
// Returns Generic when no strong signature is present.
func Detect(s string) Tool {
	// BuildKit progress IDs are emitted without the word "docker".
	if m := buildKitHeader.FindStringSubmatch(s); m != nil {
		if buildKitInstruction.MatchString(m[3]) || strings.HasPrefix(m[2], "internal") {
			return Docker
		}
	}
	if buildKitStatus.MatchString(s) ||
		buildKitLegacyHeader.MatchString(s) ||
		buildKitSolveError.MatchString(s) ||
		dockerComposeLine.MatchString(s) ||
		dockerComposeLifecycle.MatchString(s) {
		return Docker
	}

	lower := strings.ToLower(ansi.Strip(s))
	for _, tool := range toolDetectionOrder {
		for _, token := range toolPatterns[tool] {
			if strings.Contains(lower, token) {
				return tool
			}
		}
	}
	return Generic
}

// Classify assigns a semantic Level to a single line after stripping ANSI codes.
func Classify(s string) Level {
	lower := strings.ToLower(ansi.Strip(s))

	// Avoid misclassifying zero-error summaries like "0 errors" or "no errors found".
	hasZeroErrors := strings.Contains(lower, "0 errors") || strings.Contains(lower, "no errors")

	switch {
	case (!hasZeroErrors && strings.Contains(lower, "error")),
		strings.Contains(lower, "npm err!"),
		strings.Contains(lower, "failed"),
		strings.Contains(lower, "fatal"),
		strings.Contains(lower, "panic:"),
		strings.Contains(lower, "✘"),
		strings.Contains(lower, "✖"):
		return Error

	case strings.Contains(lower, "warn"),
		strings.Contains(lower, "deprecated"),
		strings.Contains(lower, "▲"):
		return Warning

	case strings.Contains(lower, "success"),
		strings.Contains(lower, "passed"),
		strings.Contains(lower, "healthy"),
		strings.Contains(lower, "done"),
		strings.Contains(lower, "✔"),
		strings.Contains(lower, "✓"):
		return Success

	default:
		return Info
	}
}

// New constructs an Engine with safe defaults for zero-value Options fields.
func New(opts Options) *Engine {
	if opts.Mode == "" {
		opts.Mode = Enhance
	}
	if opts.Tool == "" {
		opts.Tool = Auto
	}
	if opts.MaxLine <= 0 {
		opts.MaxLine = defaultMaxLineBytes
	}
	return &Engine{Options: opts}
}

// Process consumes command output incrementally, preserving its original order.
// Raw mode copies bytes exactly. In other modes, each line is classified and
// optionally parsed into a structured diagnostic before being rendered.
// A renderer failure falls back to the original bytes, never hiding errors.
// Colored Transform mode deduplicates diagnostics and summarizes them at EOF.
func (e *Engine) Process(ctx context.Context, src io.Reader, dst io.Writer) error {
	if e.Options.Mode == Raw {
		_, err := io.Copy(dst, src)
		return err
	}

	sp := newStreamProcessor(e.Options, e.Renderer, dst)
	reader := bufio.NewReaderSize(src, defaultReadBufferBytes)

	for {
		if err := ctx.Err(); err != nil {
			_ = sp.flushAll()
			return err
		}

		line, readErr := reader.ReadSlice('\n')
		if readErr == bufio.ErrBufferFull {
			// Long lines are emitted in bounded fragments, never silently truncated.
			readErr = nil
		}

		if len(line) > 0 {
			if err := sp.processLine(line); err != nil {
				return err
			}
		}

		if readErr == io.EOF {
			if err := sp.flushAll(); err != nil {
				return err
			}
			return sp.writeSummary()
		}
		if readErr != nil {
			return readErr
		}
	}
}

// streamProcessor encapsulates mutable per-stream state during Engine.Process,
// keeping the main read loop concise and readable.
type streamProcessor struct {
	opts     Options
	renderer Renderer
	dst      io.Writer

	seq         uint64
	currentTool Tool

	// Deduplication and EOF summary counters for Transform mode.
	counts    map[Level]int
	seen      map[string]bool
	grouped   map[string]map[Level]int
	lastGroup string

	// Stateful Docker BuildKit / Compose formatter.
	buildSteps *dockerBuildSteps

	// Multi-line block accumulators for NPM and cloud/CLI diagnostic cards.
	npmLines []string
	npmBytes int

	diagLines []string
	diagBytes int
	diagTool  Tool
}

func newStreamProcessor(opts Options, renderer Renderer, dst io.Writer) *streamProcessor {
	steps := newDockerBuildSteps()
	if colors, err := ParseDockerColors(opts.DockerColors); err == nil {
		steps.colors = colors
	}
	if opts.DockerSeparator != "" {
		steps.separator = opts.DockerSeparator
	}

	return &streamProcessor{
		opts:        opts,
		renderer:    renderer,
		dst:         dst,
		currentTool: Generic,
		counts:      map[Level]int{Error: 0, Warning: 0},
		seen:        make(map[string]bool),
		grouped:     make(map[string]map[Level]int),
		buildSteps:  steps,
	}
}

func (sp *streamProcessor) isDefaultColoredTransform() bool {
	return sp.opts.Color && sp.opts.Mode == Transform && sp.renderer == nil
}

func (sp *streamProcessor) flushAll() error {
	if err := sp.flushNPM(); err != nil {
		return err
	}
	return sp.flushDiagnostic()
}

func (sp *streamProcessor) flushNPM() error {
	if len(sp.npmLines) == 0 {
		return nil
	}
	lines := sp.npmLines
	sp.npmLines = nil
	sp.npmBytes = 0

	if sp.isDefaultColoredTransform() {
		_, err := io.WriteString(sp.dst, RenderNPMCard(lines, sp.opts))
		return err
	}
	_, err := io.WriteString(sp.dst, strings.Join(lines, ""))
	return err
}

func (sp *streamProcessor) flushDiagnostic() error {
	if len(sp.diagLines) == 0 {
		return nil
	}
	lines := sp.diagLines
	sp.diagLines = nil
	sp.diagBytes = 0

	if sp.opts.Preset != nil && sp.opts.Preset.Diagnostics.Card.Enabled {
		card := RenderDiagnosticCard(ParseDiagnosticCard(sp.diagTool, lines), sp.opts)
		_, err := io.WriteString(sp.dst, card)
		return err
	}
	_, err := io.WriteString(sp.dst, strings.Join(lines, ""))
	return err
}

func (sp *streamProcessor) processLine(rawLine []byte) error {
	sp.seq++
	text := string(rawLine)

	// 1. Accumulate contiguous NPM error lines into a bordered card in Transform mode.
	if sp.tryBufferNPMLine(text) {
		return nil
	}
	if err := sp.flushNPM(); err != nil {
		return fmt.Errorf("write npm card: %w", err)
	}

	// 2. Accumulate multi-line GCloud / Wrangler diagnostic blocks into cards when enabled.
	buffered, err := sp.tryBufferDiagnosticCardLine(text)
	if err != nil {
		return err
	}
	if buffered {
		return nil
	}
	if err := sp.flushDiagnostic(); err != nil {
		return fmt.Errorf("write diagnostic card: %w", err)
	}

	// 3. Update active tool context (sticky across consecutive lines unless overridden).
	if sp.opts.Tool != Auto {
		sp.currentTool = sp.opts.Tool
	} else if detected := Detect(text); detected != Generic {
		sp.currentTool = detected
	}

	event := Event{
		Sequence: sp.seq,
		Tool:     sp.currentTool,
		Level:    Classify(text),
		Text:     text,
	}

	// 4. Stateful Docker BuildKit & Compose formatting.
	if sp.shouldFormatDockerStep(text) {
		if formatted, ok := sp.buildSteps.format(text); ok {
			if _, writeErr := io.WriteString(sp.dst, formatted); writeErr != nil {
				return fmt.Errorf("write Docker step: %w", writeErr)
			}
			return nil
		}
	}

	// 5. Structured diagnostic parsing, deduplication, and file-grouped headers.
	event.Diagnostic = ParseDiagnostic(sp.currentTool, text)
	if event.Diagnostic != nil {
		event.Level = event.Diagnostic.Level
		if sp.isDefaultColoredTransform() {
			skip, groupErr := sp.trackDiagnosticGroup(event.Diagnostic)
			if groupErr != nil {
				return groupErr
			}
			if skip {
				return nil
			}
		}
	}

	// 6. Render single event with automatic fallback to original bytes on renderer error.
	var out []byte
	var renderErr error
	if sp.renderer != nil {
		out, renderErr = sp.renderer.Render(event)
	} else {
		out = Render(event, sp.opts)
	}
	if renderErr != nil {
		out = rawLine
	}

	if _, writeErr := sp.dst.Write(out); writeErr != nil {
		return fmt.Errorf("write output: %w", writeErr)
	}
	return nil
}

func (sp *streamProcessor) tryBufferNPMLine(text string) bool {
	if !sp.isDefaultColoredTransform() {
		return false
	}
	isNPM := sp.opts.Tool == NPM || (sp.opts.Tool == Auto && (sp.currentTool == NPM || Detect(text) == NPM))
	if !isNPM {
		return false
	}

	trimmed := strings.TrimSpace(text)
	if !strings.HasPrefix(trimmed, "npm ERR!") && !strings.HasPrefix(trimmed, "npm error") {
		return false
	}
	if len(sp.npmLines) >= maxCardLines || sp.npmBytes+len(text) > maxCardBytes {
		return false
	}

	sp.npmLines = append(sp.npmLines, text)
	sp.npmBytes += len(text)
	return true
}

func (sp *streamProcessor) tryBufferDiagnosticCardLine(text string) (bool, error) {
	if !sp.isDefaultColoredTransform() || sp.opts.Preset == nil || !sp.opts.Preset.Diagnostics.Card.Enabled {
		return false, nil
	}

	candidate := sp.opts.Tool
	if candidate == Auto {
		candidate = sp.currentTool
		if detected := Detect(text); detected != Generic {
			candidate = detected
		}
	}
	if candidate != GCloud && candidate != Wrangler {
		return false, nil
	}

	clean := strings.TrimSpace(text)
	start := (candidate == GCloud && strings.HasPrefix(clean, "ERROR:")) ||
		(candidate == Wrangler && (strings.Contains(clean, "[ERROR]") || strings.HasPrefix(clean, "ERROR:")))

	if start {
		if err := sp.flushDiagnostic(); err != nil {
			return false, err
		}
		sp.diagTool = candidate
		sp.diagLines = []string{text}
		sp.diagBytes = len(text)
		return true, nil
	}

	// Note: check both real tab '\t' and leading space for indented continuation lines.
	isIndented := strings.HasPrefix(text, " ") || strings.HasPrefix(text, "\t")
	isWranglerHint := candidate == Wrangler && (strings.Contains(strings.ToLower(clean), "authenticated") || strings.Contains(clean, "wrangler login"))
	continuation := len(sp.diagLines) > 0 && sp.diagTool == candidate && (isIndented || isWranglerHint)

	if continuation && len(sp.diagLines) < maxCardLines && sp.diagBytes+len(text) <= maxCardBytes {
		sp.diagLines = append(sp.diagLines, text)
		sp.diagBytes += len(text)
		return true, nil
	}

	return false, nil
}

func (sp *streamProcessor) shouldFormatDockerStep(text string) bool {
	if sp.renderer != nil || !sp.opts.Color || sp.opts.Mode == Raw {
		return false
	}
	if sp.currentTool == Docker || buildKitFinalError.MatchString(text) || buildKitSolveError.MatchString(text) {
		return true
	}
	if m := buildKitChild.FindStringSubmatch(text); m != nil && sp.buildSteps.numbers[m[1]] > 0 {
		return true
	}
	return false
}

func (sp *streamProcessor) trackDiagnosticGroup(diag *Diagnostic) (skip bool, err error) {
	key := fmt.Sprintf("%s:%d:%d:%s:%s", diag.File, diag.Line, diag.Column, diag.Code, diag.Message)
	if sp.seen[key] {
		return true, nil
	}
	sp.seen[key] = true
	sp.counts[diag.Level]++

	group := diag.File
	if group == "" {
		group = string(diag.Tool)
	}
	if sp.grouped[group] == nil {
		sp.grouped[group] = make(map[Level]int)
	}
	sp.grouped[group][diag.Level]++

	if group != sp.lastGroup {
		if _, writeErr := fmt.Fprintf(sp.dst, "\n\x1b[1;36m▸ %s\x1b[0m\n", group); writeErr != nil {
			return false, writeErr
		}
		sp.lastGroup = group
	}
	return false, nil
}

func (sp *streamProcessor) writeSummary() error {
	if !sp.isDefaultColoredTransform() || sp.counts[Error]+sp.counts[Warning] == 0 {
		return nil
	}

	groups := make([]string, 0, len(sp.grouped))
	for name := range sp.grouped {
		groups = append(groups, name)
	}
	slices.Sort(groups)

	for _, name := range groups {
		_, err := fmt.Fprintf(sp.dst, "  \x1b[2m%s\x1b[0m  \x1b[31m%d errors\x1b[0m · \x1b[33m%d warnings\x1b[0m\n",
			name, sp.grouped[name][Error], sp.grouped[name][Warning])
		if err != nil {
			return err
		}
	}

	_, err := fmt.Fprintf(sp.dst, "\n\x1b[1mDiagnostics: \x1b[31m%d errors\x1b[0m, \x1b[33m%d warnings\x1b[0m\n",
		sp.counts[Error], sp.counts[Warning])
	return err
}

// Render applies presentation to a single event. Existing ANSI sequences win
// over our palette; unrecognized output is preserved rather than rewritten.
// Transform may replace recognized diagnostics with structured error cards.
func Render(ev Event, opts Options) []byte {
	if opts.Mode == Raw || !opts.Color {
		return []byte(ev.Text)
	}

	text := ev.Text
	hasExistingANSI := ansiPattern.MatchString(text)

	if ev.Diagnostic != nil && opts.Mode == Transform && !hasExistingANSI {
		return renderDiagnosticWithTheme(ev.Diagnostic, opts.SemanticColors, opts.DiagnosticIcons)
	}
	if hasExistingANSI {
		return []byte(text)
	}

	// Structured payloads use Chroma; diagnostic lines retain semantic token highlighting.
	if opts.Mode == Transform {
		if table, ok := JSONTable(strings.TrimSpace(text), opts.Color); ok {
			return []byte(table)
		}
	}

	// Docker output gets contextual styling before generic severity coloring.
	// Do not rewrite an already-colored stream or fabricate a diagnostic.
	if ev.Tool == Docker {
		if colored, ok := highlightDocker(text); ok {
			return []byte(colored)
		}
	}

	trimmedRight := strings.TrimRight(text, "\r\n")
	lineEnding := text[len(trimmedRight):]

	if language := syntaxLanguage(text); language != "" {
		if colored, err := SyntaxHighlightWithTheme(trimmedRight, language, opts.SyntaxTheme); err == nil {
			return []byte(colored + lineEnding)
		}
	}

	if opts.Preset != nil && isPresetSupportedTool(ev.Tool) {
		if IsStepLine(text, ev.Tool, *opts.Preset) {
			return []byte("\x1b[2;3m" + trimmedRight + "\x1b[0m" + lineEnding)
		}
		if colored := HighlightWithPreset(text, ev.Tool, *opts.Preset, opts.SemanticColors); colored != text {
			return []byte(colored)
		}
	}

	// Tool-aware spans take precedence over whole-line severity styling.
	if highlighted := highlightLineWithColors(text, opts.SemanticColors); highlighted != text {
		return []byte(highlighted)
	}

	color := resolveLevelColor(ev.Level, opts.SemanticColors)
	if color == "" {
		return []byte(text)
	}

	return []byte("\x1b[" + color + "m" + trimmedRight + "\x1b[0m" + lineEnding)
}

func isPresetSupportedTool(tool Tool) bool {
	switch tool {
	case GCloud, Wrangler, Docker, TypeScript:
		return true
	default:
		return false
	}
}

func resolveLevelColor(level Level, colors SemanticColors) string {
	switch level {
	case Error:
		return semanticColor(colors.Error, "31")
	case Warning:
		return semanticColor(colors.Warning, "33")
	case Success:
		return semanticColor(colors.Success, "32")
	default:
		return ""
	}
}
