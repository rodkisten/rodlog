package rodlog

import (
	"strings"
	"unicode/utf8"

	"github.com/charmbracelet/x/ansi"
)

// ANSI SGR escape sequences used for deterministic post-layout coloring.
// Keeping them as constants makes the intent clear and prevents typo-induced escape bugs.
const (
	ansiReset       = "\x1b[0m"
	ansiDimGray     = "\x1b[2;90m"
	ansiBoldRed     = "\x1b[1;31m"
	ansiBoldYellow  = "\x1b[1;33m"
	ansiYellow      = "\x1b[33m"
	ansiGreen       = "\x1b[32m"
	ansiCyan        = "\x1b[36m"
	ansiBlue        = "\x1b[34m"
	ansiMagenta     = "\x1b[35m"
	ansiBrightWhite = "\x1b[37m"
)

// colorizeRenderedCard runs AFTER Lip Gloss has finished layout. Lip Gloss
// detects a non-TTY in GitHub Actions and may strip styles while rendering.
// Apply semantic ANSI codes only to the final text cells, preserving the
// Unicode border geometry and ensuring --color=always actually means always.
func colorizeRenderedCard(rendered string, tool Tool, opts Options) string {
	if !opts.Color || rendered == "" {
		return rendered
	}

	lines := strings.Split(rendered, "\n")
	for i, line := range lines {
		if line == "" {
			continue
		}

		// Decode only the first and last UTF-8 runes in O(1) time without allocating
		// a full []rune slice for the entire line.
		firstRune, firstWidth := utf8.DecodeRuneInString(line)
		lastRune, lastWidth := utf8.DecodeLastRuneInString(line)

		// Guard against malformed UTF-8 or lines too narrow to contain left + content + right.
		if firstRune == utf8.RuneError || lastRune == utf8.RuneError || len(line) < firstWidth+lastWidth+1 {
			continue
		}

		// 1. Horizontal border frames (top, bottom, or internal section dividers).
		// Borders are single-cell glyphs; never apply text semantic rules to them.
		if isHorizontalBorderGlyph(firstRune) {
			lines[i] = ansiDimGray + ansi.Strip(line) + ansiReset
			continue
		}

		// 2. Content rows enclosed by vertical box-drawing borders.
		if !isVerticalBorderGlyph(firstRune) || !isVerticalBorderGlyph(lastRune) {
			continue
		}

		leftBorder := line[:firstWidth]
		rightBorder := line[len(line)-lastWidth:]
		rawInner := line[firstWidth : len(line)-lastWidth]

		// Strip any partial ANSI codes Lip Gloss might have emitted in TTY mode
		// before evaluating semantic prefixes; otherwise invisible escape bytes
		// (like "\x1b[1m") would break strings.HasPrefix checks.
		strippedInner := ansi.Strip(rawInner)
		plain := strings.TrimSpace(strippedInner)

		styledInner := rawInner
		if plain != "" {
			styledInner = colorizeCardContent(rawInner, strippedInner, plain, tool, opts)
		}

		lines[i] = ansiDimGray + leftBorder + ansiReset +
			styledInner +
			ansiDimGray + rightBorder + ansiReset
	}

	return strings.Join(lines, "\n")
}

// colorizeCardContent applies semantic foreground styling to the interior cells
// of a card row while preserving leading/trailing padding spaces.
func colorizeCardContent(rawInner, strippedInner, plain string, tool Tool, opts Options) string {
	switch {
	// Critical failures & error headers
	case hasAnyPrefix(plain, "✖", "✗", "ERROR", "FATAL", "PANIC") ||
		strings.Contains(plain, "ERROR") ||
		hasAnyPrefix(plain, "Expected:", "Received:"):
		return ansiBoldRed + strippedInner + ansiReset

	// Warnings & deprecations
	case hasAnyPrefix(plain, "⚠", "WARN", "WARNING", "DEPRECATED"):
		return ansiBoldYellow + strippedInner + ansiReset

	// Metadata keys: error codes, status codes, file locations
	case hasAnyPrefix(plain, "Code:", "Status:", "Exit:", "File:", "Location:", "At:", "Rule:"):
		return ansiYellow + strippedInner + ansiReset

	// Endpoints, APIs, commands, and URLs
	case hasAnyPrefix(plain, "API:", "Endpoint:", "Method:", "URL:", "Docs:", "See:"):
		return ansiCyan + strippedInner + ansiReset

	// Diagnostic log file paths
	case hasAnyPrefix(plain, "Debug log:", "Log:", "Trace:"):
		return ansiBlue + strippedInner + ansiReset

	// Actionable suggestions, hints, and shell commands
	case hasAnyPrefix(plain, "Possible fixes:", "Fix:", "Hint:", "Tip:", "Suggestion:", "›", "→", "$ "):
		return ansiGreen + strippedInner + ansiReset

	// Tool / stack badges or contextual labels
	case hasAnyPrefix(plain, "Cause:", "Reason:", "Context:"):
		return ansiMagenta + strippedInner + ansiReset

	default:
		highlighted := rawInner
		if opts.Preset != nil {
			highlighted = HighlightWithPreset(strippedInner, tool, *opts.Preset, opts.SemanticColors)
		}
		// Keep the principal message visually distinct even when no token matches.
		if !strings.Contains(highlighted, "\x1b[") {
			return ansiBrightWhite + highlighted + ansiReset
		}
		return highlighted
	}
}

// isHorizontalBorderGlyph reports whether r starts a top, bottom, or divider border line
// across Rounded, Normal, Thick, Double, and ASCII Lip Gloss border presets.
func isHorizontalBorderGlyph(r rune) bool {
	switch r {
	case '╭', '╰', // RoundedBorder
		'┌', '└', '├', // NormalBorder
		'┏', '┗', '┣', // ThickBorder
		'╔', '╚', '╠', '╞', // DoubleBorder
		'+': // ASCII border
		return true
	default:
		return false
	}
}

// isVerticalBorderGlyph reports whether r is a valid left/right box border cell.
func isVerticalBorderGlyph(r rune) bool {
	switch r {
	case '│', '┃', '║', '|':
		return true
	default:
		return false
	}
}

// hasAnyPrefix is a small variadic helper that keeps multi-prefix switch cases readable.
// In Go, `...string` packs arguments into a slice without requiring callers to write `[]string{...}`.
func hasAnyPrefix(s string, prefixes ...string) bool {
	for _, p := range prefixes {
		if strings.HasPrefix(s, p) {
			return true
		}
	}
	return false
}
