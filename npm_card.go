package rodlog

import (
	"strings"

	"github.com/charmbracelet/lipgloss"
)

// Constants avoid "magic strings" scattered across parsing logic.
// In Go, untyped string constants are evaluated at compile time with zero runtime overhead.
const (
	npmErrPrefixOld   = "npm ERR!"
	npmErrPrefixNew   = "npm error"
	npmMissingScript  = "Missing script:"
	npmLogLocationMsg = "A complete log of this run can be found in:"
	npmCommandPrefix  = "npm run"
)

// Package-level styles prevent re-allocating lipgloss.Style structs on every render call.
// lipgloss styles are immutable value types, making them safe to share across goroutines.
var (
	npmCardTitleStyle = lipgloss.NewStyle().
				Bold(true).
				Foreground(lipgloss.Color("9")) // ANSI bright red

	npmCardMessageStyle = lipgloss.NewStyle().
				Bold(true)

	npmCardSuggestionStyle = lipgloss.NewStyle().
				Foreground(lipgloss.Color("10")) // ANSI bright green

	npmCardLogPathStyle = lipgloss.NewStyle().
				Foreground(lipgloss.Color("14")) // ANSI bright cyan

	npmCardBoxStyle = lipgloss.NewStyle().
			Border(lipgloss.RoundedBorder()).
			BorderForeground(lipgloss.Color("8")). // ANSI bright black (gray)
			Padding(0, 1)
)

// NPMDiagnostic holds structured fields extracted from raw npm CLI stderr lines.
type NPMDiagnostic struct {
	Message    string
	Suggestion string
	LogPath    string
	Details    []string
}

// ParseNPMDiagnostic acts as a pure function (Functional Core): it takes raw terminal lines
// and transforms them into a structured value without performing I/O or styling.
func ParseNPMDiagnostic(lines []string) NPMDiagnostic {
	// Zero-value initialization: in Go, declaring `var d NPMDiagnostic` initializes
	// all strings to "" and slices to nil, which is safe to append to immediately.
	var d NPMDiagnostic

	for _, rawLine := range lines {
		content := normalizeNPMLine(rawLine)
		if content == "" {
			continue
		}

		// A tagless `switch` in Go is an idiomatic replacement for long `if-else if` chains.
		// Cases are evaluated top-to-bottom, and the first match executes (no `break` needed).
		switch {
		case strings.HasPrefix(content, npmMissingScript):
			d.Message = content

		case strings.HasPrefix(content, npmLogLocationMsg):
			// strings.CutPrefix (introduced in Go 1.20) is safer and cleaner than
			// combining HasPrefix + TrimPrefix.
			if path, ok := strings.CutPrefix(content, npmLogLocationMsg); ok {
				d.LogPath = strings.TrimSpace(path)
			}

		case strings.HasPrefix(content, npmCommandPrefix):
			d.Suggestion = content

		default:
			d.Details = append(d.Details, content)
		}
	}

	return d
}

// RenderNPMCard formats an NPMDiagnostic into a bordered terminal card.
func RenderNPMCard(lines []string, opts Options) string {
	// Guard clause: fail fast on empty input to avoid rendering an empty box.
	if len(lines) == 0 {
		return ""
	}

	width := cardWidth(opts)
	diag := ParseNPMDiagnostic(lines)

	// Pre-allocate slice capacity to reduce memory reallocations during append.
	// 1 (title) + 1 (message) + len(details) + 2 (suggestion + log header/path)
	body := make([]string, 0, 4+len(diag.Details))

	icon := iconOrDefault(opts.DiagnosticIcons.Error, "✖")
	body = append(body, npmCardTitleStyle.Render(icon+" NPM ERROR"))

	if diag.Message != "" {
		body = appendStyledLines(body, diag.Message, width, npmCardMessageStyle)
	}

	for _, detail := range uniqueCardDetails(diag.Message, diag.Details) {
		body = append(body, cardLines(detail, width)...)
	}

	if diag.Suggestion != "" {
		body = appendStyledLines(body, "$ "+diag.Suggestion, width, npmCardSuggestionStyle)
	}

	if diag.LogPath != "" {
		body = append(body, "Debug log:")
		appendStyledLines(body, diag.LogPath, width, npmCardLogPathStyle)
	}

	// Copy the base box style and apply dynamic width constraints per render.
	// Because lipgloss.Style is a value type (struct), calling .Width() returns a modified copy
	// without mutating the global `npmCardBoxStyle`.
	box := npmCardBoxStyle.
		Width(width - 4).
		MaxWidth(width).
		Render(strings.Join(body, "\n"))

	return colorizeRenderedCard(box, NPM, opts) + "\n"
}

// normalizeNPMLine strips trailing newlines and npm's standard log prefixes.
func normalizeNPMLine(line string) string {
	s := strings.TrimSpace(line)
	// Handle both legacy ("npm ERR!") and npm v10+ ("npm error") prefixes.
	s, _ = strings.CutPrefix(s, npmErrPrefixOld)
	s, _ = strings.CutPrefix(s, npmErrPrefixNew)
	return strings.TrimSpace(s)
}

// appendStyledLines wraps text to fit `width` and applies a lipgloss.Style to each resulting line.
// Passing styles by value is idiomatic in lipgloss.
func appendStyledLines(dst []string, text string, width int, style lipgloss.Style) []string {
	for _, line := range cardLines(text, width) {
		dst = append(dst, style.Render(line))
	}
	return dst
}
