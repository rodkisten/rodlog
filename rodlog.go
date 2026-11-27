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

type Mode string

const (
	Raw       Mode = "raw"
	Enhance   Mode = "enhance"
	Transform Mode = "transform"
)

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

type Level string

const (
	Info    Level = "info"
	Warning Level = "warning"
	Error   Level = "error"
	Success Level = "success"
)

type Event struct {
	Diagnostic *Diagnostic
	Sequence   uint64
	Tool       Tool
	Level      Level
	Text       string
}

type Options struct {
	Mode            Mode
	Tool            Tool
	Color           bool
	MaxLine         int
	DockerSeparator string
	DockerColors    DockerColors
	SemanticColors  SemanticColors
	DiagnosticIcons DiagnosticIcons
	SyntaxTheme     SyntaxTheme
	Preset          *SemanticPreset
}

type Renderer interface {
	Render(Event) ([]byte, error)
}

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

var ansi = regexp.MustCompile("\x1b\[[0-9;]*[A-Za-z]")

var toolDetectionOrder = []Tool{
	TypeScript,
	Vitest,
	NodeTest,
	NPM,
	GCloud,
	Wrangler,
	Docker,
}

var toolPatterns = map[Tool][]string{
	NPM:        {"npm err!", "npm warn", "npm error", "npm run", "npm notice"},
	TypeScript: {"error ts", "warning ts", "tsc ", ".tsx(", ".ts(", ".mts(", ".cts(", "found 1 error in", " errors in "},
	GCloud:     {"gcloud ", "error: (gcloud", "warning: (gcloud", "updated property", "deploying container"},
	Wrangler:   {"wrangler ", "cloudflare", "workers.dev", "wrangler.toml", "wrangler.jsonc", "✘ [error]", "▲ [warning]"},
	Docker:     {"docker ", "step 1/", "=> [", "dockerfile:", "exporting layers", "writing image", "error: failed to solve:"},
	Vitest:     {"vitest ", "vitest run", "❯ ", "test files ", "tests  ", "fail  "},
	NodeTest:   {"node --test", "✖ tests ", "ℹ tests ", "✖ failing tests:", "not ok "},
}

func Detect(s string) Tool {
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

func Classify(s string) Level {
	lower := strings.ToLower(ansi.Strip(s))
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

type streamProcessor struct {
	opts     Options
	renderer Renderer
	dst      io.Writer

	seq         uint64
	currentTool Tool

	counts    map[Level]int
	seen      map[string]bool
	grouped   map[string]map[Level]int
	lastGroup string

	buildSteps *dockerBuildSteps

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

	if sp.tryBufferNPMLine(text) {
		return nil
	}
	if err := sp.flushNPM(); err != nil {
		return fmt.Errorf("write npm card: %w", err)
	}

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

	if sp.shouldFormatDockerStep(text) {
		if formatted, ok := sp.buildSteps.format(text); ok {
			if _, writeErr := io.WriteString(sp.dst, formatted); writeErr != nil {
				return fmt.Errorf("write Docker step: %w", writeErr)
			}
			return nil
		}
	}

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

func Render(ev Event, opts Options) []byte {
	if opts.Mode == Raw || !opts.Color {
		return []byte(ev.Text)
	}

	text := ev.Text
	hasExistingANSI := ansi.MatchString(text)

	if ev.Diagnostic != nil && opts.Mode == Transform && !hasExistingANSI {
		return renderDiagnosticWithTheme(ev.Diagnostic, opts.SemanticColors, opts.DiagnosticIcons)
	}
	if hasExistingANSI {
		return []byte(text)
	}

	if opts.Mode == Transform {
		if table, ok := JSONTable(strings.TrimSpace(text), opts.Color); ok {
			return []byte(table)
		}
	}

	if ev.Tool == Docker {
		if colored, ok := highlightDocker(text); ok {
			return []byte(colored)
		}
	}

	trimmedRight := strings.TrimRight(text, "\r\n")
	lineEnding := text[len(trimmedRight):]

	lang := opts.SyntaxTheme.Lexer
	if lang == "" {
		lang = syntaxLanguage(text)
	}
	if lang != "" {
		if colored, err := SyntaxHighlightWithTheme(trimmedRight, lang, opts.SyntaxTheme); err == nil {
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
