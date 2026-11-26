package rodlog

import (
	"os"
	"strconv"
	"strings"

	"github.com/charmbracelet/lipgloss"
	"github.com/mattn/go-runewidth"
)

const (
	defaultCardWidth   = 36
	minCardWidth       = 24
	maxCardWidth       = 160
	cardHorizontalTrim = 4 // 2 border glyphs + 2 horizontal padding spaces
)

// cardWidth caps physical output, rather than relying on the CI viewer to wrap it.
// Precedence: default (36) -> RODLOG_CARD_WIDTH -> COLUMNS (if narrower) -> Preset MaxWidth (if narrower).
func cardWidth(opts Options) int {
	// GitHub Actions mobile shows fewer columns than the CI runner reports.
	// Keep the default safe for a narrow phone; allow explicit opt-in overrides.
	width := defaultCardWidth

	if n, ok := parseBoundedEnvInt("RODLOG_CARD_WIDTH", minCardWidth, maxCardWidth); ok {
		width = n
	}

	// Only narrow the card if COLUMNS is smaller than the configured width;
	// a wide desktop terminal (e.g. 220 cols) shouldn't stretch mobile-safe cards
	// unless RODLOG_CARD_WIDTH was explicitly set.
	if n, ok := parseBoundedEnvInt("COLUMNS", minCardWidth, width-1); ok {
		width = n
	}

	if opts.Preset != nil {
		presetMax := opts.Preset.Diagnostics.Card.MaxWidth
		if presetMax >= minCardWidth && presetMax < width {
			width = presetMax
		}
	}

	return width
}

// wrapCardText wraps before adding ANSI styling, preserving semantic highlighting
// and never breaking an escape sequence or a Unicode display cell.
func wrapCardText(input string, width int) []string {
	if width < 1 {
		return []string{input}
	}

	paragraphs := strings.Split(input, "\n")
	// Pre-allocate at least one line per paragraph to avoid early slice growth.
	result := make([]string, 0, len(paragraphs))

	for _, paragraph := range paragraphs {
		if strings.TrimSpace(paragraph) == "" {
			result = append(result, "")
			continue
		}

		var currentLine strings.Builder
		currentWidth := 0

		for _, word := range strings.Fields(paragraph) {
			wordWidth := lipgloss.Width(word)

			// If adding a space + word overflows the current line, flush the line first.
			if currentWidth > 0 && currentWidth+1+wordWidth > width {
				result = append(result, currentLine.String())
				currentLine.Reset()
				currentWidth = 0
			}

			// Hard-wrap single tokens longer than `width` (e.g. file paths, URLs, SHA hashes).
			if wordWidth > width {
				if currentWidth > 0 {
					result = append(result, currentLine.String())
					currentLine.Reset()
					currentWidth = 0
				}

				chunks := splitLongWord(word, width)
				// Append all full chunks except the last one, which becomes the start
				// of the active line so short trailing words can share it if space allows.
				for _, chunk := range chunks[:len(chunks)-1] {
					result = append(result, chunk)
				}
				lastChunk := chunks[len(chunks)-1]
				currentLine.WriteString(lastChunk)
				currentWidth = lipgloss.Width(lastChunk)
				continue
			}

			if currentWidth > 0 {
				currentLine.WriteByte(' ')
				currentWidth++
			}
			currentLine.WriteString(word)
			currentWidth += wordWidth
		}

		if currentWidth > 0 {
			result = append(result, currentLine.String())
		}
	}

	return result
}

// uniqueCardDetails deduplicates secondary diagnostic lines against both the primary
// message and previously seen details (case-insensitively), preserving original order.
func uniqueCardDetails(message string, details []string) []string {
	// In Go, `map[string]struct{}` is the idiomatic zero-allocation Set representation
	// because `struct{}` occupies 0 bytes of memory.
	seen := make(map[string]struct{}, len(details)+1)

	if s := cleanCardText(message); s != "" {
		seen[strings.ToLower(s)] = struct{}{}
	}

	result := make([]string, 0, len(details))
	for _, detail := range details {
		cleaned := cleanCardText(detail)
		if cleaned == "" {
			continue
		}

		key := strings.ToLower(cleaned)
		if _, exists := seen[key]; exists {
			continue
		}

		seen[key] = struct{}{}
		result = append(result, cleaned)
	}

	return result
}

// cardLines wraps every field, including API names and error codes, before
// Lip Gloss adds padding and borders. This avoids viewer-side reflow.
func cardLines(value string, width int) []string {
	return wrapCardText(value, width-cardHorizontalTrim)
}

// parseBoundedEnvInt reads an integer environment variable and returns it only if
// it parses cleanly and falls within the inclusive [minVal, maxVal] range.
func parseBoundedEnvInt(name string, minVal, maxVal int) (int, bool) {
	raw := strings.TrimSpace(os.Getenv(name))
	if raw == "" {
		return 0, false
	}
	n, err := strconv.Atoi(raw)
	if err != nil || n < minVal || n > maxVal {
		return 0, false
	}
	return n, true
}

// splitLongWord breaks an oversized token into chunks that each fit within `width`
// terminal display cells in O(n) time, respecting wide East Asian runes and emojis.
func splitLongWord(word string, width int) []string {
	var chunks []string
	var buf strings.Builder
	bufWidth := 0

	for _, r := range word {
		rw := runewidth.RuneWidth(r)
		// Ensure progress even if width == 1 and a wide rune (width 2) appears.
		if bufWidth > 0 && bufWidth+rw > width {
			chunks = append(chunks, buf.String())
			buf.Reset()
			bufWidth = 0
		}
		buf.WriteRune(r)
		bufWidth += rw
	}

	if buf.Len() > 0 {
		chunks = append(chunks, buf.String())
	}
	return chunks
}
